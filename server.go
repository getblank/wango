package wango

import (
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"golang.org/x/net/websocket"
)

// Server represents a WAMP server that handles RPC and pub/sub.
type Server struct {
	connections       map[string]*conn
	connectionsLocker sync.RWMutex
	rpcHandlers       map[string]RPCHandler
	rpcRgxHandlers    map[*regexp.Regexp]RPCHandler
	subHandlers       map[string]subHandler
	subscribers       map[string]subscribersMap
	subscribersLocker sync.RWMutex
	openCB            func()
	closeCB           func()
}

// RPCHandler describes func for handling RPC requests
type RPCHandler func(connID string, uri string, args ...interface{}) (interface{}, error)

// SubHandler describes func for handling RPC requests
type SubHandler func(connID string, uri string, args ...interface{}) bool

// PubHandler describes func for handling publish event before sending to subscribers
type PubHandler func(uri string, event interface{}, extra interface{}) (bool, interface{})

type subHandler struct {
	subHandler SubHandler
	pubHandler PubHandler
}
type subscribersMap map[string]bool

// New creates new WS struct and returns pointer to it
func NewServer() *Server {
	server := new(Server)
	server.connections = map[string]*conn{}
	server.rpcHandlers = map[string]RPCHandler{}
	server.rpcRgxHandlers = map[*regexp.Regexp]RPCHandler{}
	server.subHandlers = map[string]subHandler{}
	server.subscribers = map[string]subscribersMap{}
	return server
}

// RegisterRPCHandler registers RPC handler function for provided URI
func (s *Server) RegisterRPCHandler(_uri interface{}, fn RPCHandler) error {
	switch _uri.(type) {
	case string:
		uri := _uri.(string)
		if _, ok := s.rpcHandlers[uri]; ok {
			return errors.Wrap(ErrHandlerAlreadyRegistered, "when registering string rpcHandler")
		}
		s.rpcHandlers[uri] = fn
	case *regexp.Regexp:
		rgx := _uri.(*regexp.Regexp)
		for k := range s.rpcRgxHandlers {
			if k.String() == rgx.String() {
				return errors.Wrap(ErrHandlerAlreadyRegistered, "when registering rgx rpcHandler")
			}
		}
		s.rpcRgxHandlers[rgx] = fn
	}

	return nil
}

// RegisterSubHandler registers subscription handler function for provided URI
func (s *Server) RegisterSubHandler(uri string, fnSub SubHandler, fnPub PubHandler) error {
	if _, ok := s.subHandlers[uri]; ok {
		return errors.Wrap(ErrHandlerAlreadyRegistered, "when registering subHandler")
	}

	s.subHandlers[uri] = subHandler{
		subHandler: fnSub,
		pubHandler: fnPub,
	}
	return nil
}

// Publish used for publish event
func (s *Server) Publish(uri string, event interface{}) {
	var pubHandler PubHandler
	handler, ok := s.subHandlers[uri]
	if ok {
		pubHandler = handler.pubHandler
	}
	s.subscribersLocker.RLock()
	subscribers, ok := s.subscribers[uri]
	if !ok {
		s.subscribersLocker.RUnlock()
		return
	}
	if len(subscribers) == 0 {
		s.subscribersLocker.RUnlock()
		return
	}
	// need to copy ids to prevent long locking
	subscriberIds := make([]string, len(subscribers))
	i := 0
	for id := range subscribers {
		subscriberIds[i] = id
		i++
	}
	s.subscribersLocker.RUnlock()
	for _, id := range subscriberIds {
		c, err := s.getConnection(id)
		if err != nil {
			println("Connection not found", err)
			continue
		}

		var response []byte
		if pubHandler != nil {
			allow, modifiedEvent := pubHandler(uri, event, c.extra)
			if !allow {
				// not allowed to send
				continue
			}
			response, _ = createMessage(msgEvent, uri, modifiedEvent)
		} else {
			response, _ = createMessage(msgEvent, uri, event)
		}
		c.send(response)
	}
}

// WampHandler handles every *websocket.Conn connection
// If extra data provided, it will kept in connection and will pass to rpc/pub/sub handlers
func (s *Server) WampHandler(ws *websocket.Conn, extra interface{}) {
	c := s.addConnection(ws, extra)
	defer s.deleteConnection(c.id)

	go c.sender()

	s.receive(c)
}

func (s *Server) receive(c *conn) {
	defer c.connection.Close()
	var data string
	for {
		err := websocket.Message.Receive(c.connection, &data)
		if err != nil {
			if err != io.EOF {
				// Error receiving message
			}
			break
		}
		msgType, msg, err := parseMessage(data)
		if err != nil {
			// error parsing!!!
			println("Error:", err.Error())
		}
		switch msgType {
		case msgPrefix:
		// not implemented
		case msgCall:
			s.handleRPCCall(c, msg)
		case msgCallResult:
		case msgCallError:
		case msgSubscribe:
			s.handleSubscribe(c, msg)
		case msgUnsubscribe:
			s.handleUnSubscribe(c, msg)
		case msgPublish:
			s.handlePublish(c, msg)
		case msgEvent:
		case msgSubscribed:
		// not implemented
		case msgHeartbeat:
			s.handleHeartbeat(c, msg, data)
		}
	}
}

func (s *Server) handleRPCCall(c *conn, msg []interface{}) {
	rpcMessage, err := parseWampMessage(msgCall, msg)
	if err != nil {
		println("Can't parse rpc message", err.Error())
		return
	}

	uri := rpcMessage.URI
	handler, ok := s.rpcHandlers[uri]
	if !ok {
		var rgx *regexp.Regexp
		for rgx, handler = range s.rpcRgxHandlers {
			if rgx.MatchString(uri) {
				ok = true
				break
			}
		}
	}

	if ok {
		res, err := handler(c.id, uri, rpcMessage.Args...)
		if err != nil {
			response, _ := createMessage(msgCallError, rpcMessage.ID, createError(err))
			// TODO: error handling
			c.send(response)
			return
		}
		response, _ := createMessage(msgCallResult, rpcMessage.ID, res)
		c.send(response)
		return
	}

	response, _ := createMessage(msgCallError, rpcMessage.ID, createError(ErrRPCNotRegistered))
	c.send(response)
}

func (s *Server) handleSubscribe(c *conn, msg []interface{}) {
	subMessage, err := parseWampMessage(msgSubscribe, msg)
	if err != nil {
		println("Can't parse rpc message", err.Error())
		return
	}

	_uri := subMessage.URI
	s.subscribersLocker.Lock()
	defer s.subscribersLocker.Unlock()
	for uri, handler := range s.subHandlers {
		if strings.HasPrefix(_uri, uri) {
			if handler.subHandler(c.id, _uri, subMessage.Args...) {
				if _, ok := s.subscribers[_uri]; !ok {
					s.subscribers[_uri] = subscribersMap{}
				}
				s.subscribers[_uri][c.id] = subscriberExists
				response, _ := createMessage(msgSubscribed, _uri)
				go c.send(response)
				return
			}
			response, _ := createMessage(msgSubscribeError, _uri, createError(ErrForbidden))
			go c.send(response)
			return
		}
	}
	response, _ := createMessage(msgSubscribeError, _uri, createError(ErrSubURINotRegistered))
	go c.send(response)
}

func (s *Server) handleUnSubscribe(c *conn, msg []interface{}) {
	unsubMessage, err := parseWampMessage(msgUnsubscribe, msg)
	if err != nil {
		println("Can't parse rpc message", err.Error())
		return
	}

	_uri := unsubMessage.URI
	s.subscribersLocker.Lock()
	defer s.subscribersLocker.Unlock()
	for uri, subscribers := range s.subscribers {
		if uri == _uri {
			if _, ok := subscribers[c.id]; ok {
				delete(subscribers, c.id)
				response, _ := createMessage(msgUnsubscribed, _uri)
				go c.send(response)
				return
			}
			response, _ := createMessage(msgUnSubscribeError, _uri, createError(ErrNotSubscribes))
			go c.send(response)
			return
		}
	}
	response, _ := createMessage(msgUnSubscribeError, _uri, createError(ErrSubURINotRegistered))
	go c.send(response)
}

func (s *Server) handleHeartbeat(c *conn, msg []interface{}, data string) {
	c.send(data)
}

func (s *Server) handlePublish(c *conn, msg []interface{}) {
	pubMessage, err := parseWampMessage(msgUnsubscribe, msg)
	if err != nil {
		println("Can't parse publish message", err.Error())
		return
	}
	var event interface{}
	if len(pubMessage.Args) > 0 {
		event = pubMessage.Args[0]
	}
	s.Publish(pubMessage.URI, event)
}

func (s *Server) addConnection(ws *websocket.Conn, extra interface{}) *conn {
	cn := new(conn)
	cn.connection = ws
	cn.id = newUUIDv4()
	cn.extra = extra
	cn.sendChan = make(chan interface{}, sendChanBufferSize)
	s.connectionsLocker.Lock()
	defer s.connectionsLocker.Unlock()
	s.connections[cn.id] = cn

	return cn
}

func (s *Server) getConnection(id string) (*conn, error) {
	s.connectionsLocker.RLock()
	defer s.connectionsLocker.RUnlock()
	cn, ok := s.connections[id]
	if !ok {
		return nil, errors.New("NOT FOUND")
	}

	return cn, nil
}

func (s *Server) deleteConnection(id string) {
	s.connectionsLocker.Lock()
	defer s.connectionsLocker.Unlock()
	s.subscribersLocker.Lock()
	defer s.subscribersLocker.Unlock()
	delete(s.connections, id)
	for _, subscribers := range s.subscribers {
		delete(subscribers, id)
	}
}
