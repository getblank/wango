package wango

import (
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/net/websocket"
)

var (
	defaultConnectionTimeout = time.Second * 15
	heartBeatFrequency       = time.Second * 3
)

// Wango represents a WAMP server that handles RPC and pub/sub.
type Wango struct {
	connections       map[string]*Conn
	connectionsLocker *sync.RWMutex
	rpcHandlers       map[string]RPCHandler
	rpcRgxHandlers    map[*regexp.Regexp]RPCHandler
	subHandlers       map[string]subHandler
	subscribers       map[string]subscribersMap
	subscribersLocker *sync.RWMutex
	openCB            func(*Conn)
	closeCB           func(*Conn)
	aliveTimeout      time.Duration
	stringMode        bool
}

// RPCHandler describes func for handling RPC requests
type RPCHandler func(c *Conn, uri string, args ...interface{}) (interface{}, error)

// SubHandler describes func for handling RPC requests
type SubHandler func(c *Conn, uri string, args ...interface{}) (interface{}, error)

// PubHandler describes func for handling publish event before sending to subscribers
type PubHandler func(uri string, event interface{}, extra interface{}) (bool, interface{})

type subHandler struct {
	subHandler   SubHandler
	unsubHandler SubHandler
	pubHandler   PubHandler
}

type subscribersMap map[string]bool

type subRequestsListeners struct {
	locker    *sync.Mutex
	listeners map[string][]subRequestsListener
}

// Connect connects to server with provided URI and origin
func Connect(url, origin string, timeout ...time.Duration) (*Wango, error) {
	if !strings.HasPrefix(url, "ws://") && !strings.HasPrefix(url, "wss://") {
		url = "ws://" + url
	}
	ws, err := websocket.Dial(url, "wamp", origin)
	if err != nil {
		return nil, errors.Wrap(err, "Can't connect to "+url)
	}
	w := New()
	c := w.addConnection(ws, nil)
	c.clientConnection = true
	if timeout != nil {
		w.aliveTimeout = timeout[0]
	} else {
		w.aliveTimeout = defaultConnectionTimeout
	}

	err = c.receiveWelcome()
	if err != nil {
		return nil, err
	}
	go c.sender()
	go w.receive(c)
	go c.heartbeating()

	return w, nil
}

// DebugMode sets debug flag after that will print debug messages to stdout
func DebugMode() {
	debugMode = true
}

// New creates new Wango and returns pointer to it
func New(timeout ...time.Duration) *Wango {
	w := new(Wango)
	w.connections = map[string]*Conn{}
	w.connectionsLocker = new(sync.RWMutex)
	w.rpcHandlers = map[string]RPCHandler{}
	w.rpcRgxHandlers = map[*regexp.Regexp]RPCHandler{}
	w.subHandlers = map[string]subHandler{}
	w.subscribers = map[string]subscribersMap{}
	w.subscribersLocker = new(sync.RWMutex)
	if timeout != nil {
		w.aliveTimeout = timeout[0]
	} else {
		w.aliveTimeout = defaultConnectionTimeout
	}

	return w
}

// Call used for call RPC
func (w *Wango) Call(uri string, data ...interface{}) (interface{}, error) {
	if len(w.connections) == 0 {
		return nil, errors.New("Not connected")
	}
	w.connectionsLocker.RLock()
	var c *Conn
	for _, _c := range w.connections {
		c = _c
		break
	}
	w.connectionsLocker.RUnlock()

	id := newUUIDv4()
	ch := make(chan *callResult)
	c.callResultsLocker.Lock()
	c.callResults[id] = ch
	c.callResultsLocker.Unlock()
	args := make([]interface{}, 3+len(data))
	args[0] = msgIntTypes[msgCall]
	args[1] = id
	args[2] = uri
	for i, arg := range data {
		args[3+i] = arg
	}
	msg, _ := createMessage(args...)
	c.send(msg)

	res := <-ch
	return res.result, res.err
}

// Disconnect used to disconnect all clients in server mode, or to disconnect from server in client mode
func (w *Wango) Disconnect() {
	w.connectionsLocker.RLock()
	for _, c := range w.connections {
		go func(ch chan struct{}) {
			ch <- struct{}{}
		}(c.breakChan)
	}
	w.connectionsLocker.RUnlock()
}

// GetConnection returns connection for connID provided.
func (w *Wango) GetConnection(id string) (*Conn, error) {
	return w.getConnection(id)
}

// SetSessionOpenCallback sets callback that will called when new connection will established.
// Callback passes connection struct as only argument.
func (w *Wango) SetSessionOpenCallback(cb func(*Conn)) {
	w.openCB = cb
}

// SetSessionCloseCallback sets callback that will called when connection will closed
// Callback passes connection struct as only argument.
func (w *Wango) SetSessionCloseCallback(cb func(*Conn)) {
	w.closeCB = cb
}

// StringMode sets a string mode to use TextFrame encoding for sending messages
func (w *Wango) StringMode() {
	w.stringMode = true
}

// Publish used for publish event
func (w *Wango) Publish(uri string, event interface{}) {
	var pubHandler PubHandler
	handler, ok := w.subHandlers[uri]
	if ok {
		pubHandler = handler.pubHandler
	}
	w.subscribersLocker.RLock()
	subscribers, ok := w.subscribers[uri]
	if !ok {
		w.subscribersLocker.RUnlock()
		return
	}
	if len(subscribers) == 0 {
		w.subscribersLocker.RUnlock()
		return
	}
	// need to copy ids to prevent long locking
	subscriberIds := make([]string, len(subscribers))
	i := 0
	for id := range subscribers {
		subscriberIds[i] = id
		i++
	}
	w.subscribersLocker.RUnlock()
	for _, id := range subscriberIds {
		c, err := w.getConnection(id)
		if err != nil {
			logger("Connection not found", err)
			continue
		}

		var response []byte
		if pubHandler != nil {
			allow, modifiedEvent := pubHandler(uri, event, c.extra)
			if !allow {
				// not allowed to send
				continue
			}
			response, _ = createMessage(msgIntTypes[msgEvent], uri, modifiedEvent)
		} else {
			response, _ = createMessage(msgIntTypes[msgEvent], uri, event)
		}
		go c.send(response)
	}
}

// RegisterRPCHandler registers RPC handler function for provided URI
func (w *Wango) RegisterRPCHandler(_uri interface{}, fn func(c *Conn, uri string, args ...interface{}) (interface{}, error)) error {
	switch _uri.(type) {
	case string:
		uri := _uri.(string)
		if _, ok := w.rpcHandlers[uri]; ok {
			return errors.Wrap(errHandlerAlreadyRegistered, "when registering string rpcHandler")
		}
		w.rpcHandlers[uri] = fn
	case *regexp.Regexp:
		rgx := _uri.(*regexp.Regexp)
		for k := range w.rpcRgxHandlers {
			if k.String() == rgx.String() {
				return errors.Wrap(errHandlerAlreadyRegistered, "when registering rgx rpcHandler")
			}
		}
		w.rpcRgxHandlers[rgx] = fn
	}

	return nil
}

// RegisterSubHandler registers subscription handler function for provided URI.
// fnSub, fnUnsub and fnPub can be nil.
// fnSub will called when subscribe event arrived.
// fnUnsub will called when unsubscribe event arrived.
// fnPub will called when called Publish method. It can control sending event to connections. If first returned argument is true,
// then to connection will send data from second argument.
func (w *Wango) RegisterSubHandler(uri string, fnSub func(c *Conn, uri string, args ...interface{}) (interface{}, error), fnUnsub func(c *Conn, uri string, args ...interface{}) (interface{}, error), fnPub func(uri string, event interface{}, extra interface{}) (bool, interface{})) error {
	if _, ok := w.subHandlers[uri]; ok {
		return errors.Wrap(errHandlerAlreadyRegistered, "when registering subHandler")
	}

	w.subHandlers[uri] = subHandler{
		subHandler:   fnSub,
		unsubHandler: fnUnsub,
		pubHandler:   fnPub,
	}
	return nil
}

// SendEvent sends event for provided uri directly to receivers in slice connIDs
func (w *Wango) SendEvent(uri string, event interface{}, connIDs []string) {
	msg, _ := createMessage(msgIntTypes[msgEvent], uri, event)
	for _, id := range connIDs {
		c, err := w.getConnection(id)
		if err != nil {
			continue
		}
		c.send(msg)
	}
}

// Subscribe sends subscribe request for uri provided.
func (w *Wango) Subscribe(uri string, fn func(uri string, event interface{}), id ...string) error {
	if uri == "" {
		return errors.New("Empty uri")
	}
	if len(w.connections) == 0 {
		return errors.New("No active connections")
	}
	w.connectionsLocker.RLock()
	var c *Conn
	if id != nil {
		_c, ok := w.connections[id[0]]
		if !ok {
			w.connectionsLocker.RUnlock()
			return errors.New("Connection not found for id: " + id[0])
		}
		c = _c
	} else {
		for _, _c := range w.connections {
			c = _c
			break
		}
	}
	w.connectionsLocker.RUnlock()

	resChan := make(chan error)
	c.subRequests.addRequest(c.id, uri, resChan)

	c.eventHandlersLocker.Lock()
	c.eventHandlers[uri] = fn
	c.eventHandlersLocker.Unlock()

	msg, _ := createMessage(msgIntTypes[msgSubscribe], uri)
	c.send(msg)

	err := <-resChan

	return err
}

// Unsubscribe sends unsubscribe request for uri provided
func (w *Wango) Unsubscribe(uri string, id ...string) error {
	if uri == "" {
		return errors.New("Empty uri")
	}
	if len(w.connections) == 0 {
		return errors.New("No active connections")
	}

	w.connectionsLocker.RLock()
	var c *Conn
	if id != nil {
		_c, ok := w.connections[id[0]]
		if !ok {
			w.connectionsLocker.RUnlock()
			return errors.New("Connection not found for id: " + id[0])
		}
		c = _c
	} else {
		for _, _c := range w.connections {
			c = _c
			break
		}
	}
	w.connectionsLocker.RUnlock()

	resChan := make(chan error)
	c.unsubRequests.addRequest(c.id, uri, resChan)

	c.eventHandlersLocker.Lock()
	delete(c.eventHandlers, uri)
	c.eventHandlersLocker.Unlock()

	msg, _ := createMessage(msgIntTypes[msgUnsubscribe], uri)
	c.send(msg)

	err := <-resChan

	return err
}

// WampHandler handles every *websocket.Conn connection
// If extra data provided, it will kept in connection and will pass to rpc/pub/sub handlers
func (w *Wango) WampHandler(ws *websocket.Conn, extra interface{}) {
	c := w.addConnection(ws, extra)
	if w.openCB != nil {
		w.openCB(c)
	}
	c.stringMode = w.stringMode

	go c.sender()

	response, _ := createWelcomeMessage(c.id)
	c.send(response)
	w.receive(c)
}

func (w *Wango) receive(c *Conn) {
	defer func() {
		c.connection.Close()
		w.deleteConnection(c)
	}()
	dataChan := make(chan []byte)
	go func() {
		var data []byte
		for {
			err := websocket.Message.Receive(c.connection, &data)
			if err != nil {
				logger("Message receiving error: ", err)
				if err != io.EOF {
					// Error receiving message
				}
				c.breakChan <- struct{}{}
				break
			}
			dataChan <- data
		}
	}()
MessageLoop:
	for {
		select {
		case <-c.breakChan:
			logger("breakChan signal received, will close connection.")
			break MessageLoop
		case data := <-dataChan:
			msgType, msg, err := parseMessage(data)
			if err != nil {
				// error parsing!!!
				logger("Error:", err.Error())
			}
			c.resetTimeoutTimer()

			switch msgType {
			case msgPrefix:
			// not implemented
			case msgCall:
				w.handleRPCCall(c, msg)

			case msgCallResult:
				w.handleCallResult(c, msg)

			case msgCallError:
				w.handleCallError(c, msg)

			case msgSubscribe:
				w.handleSubscribe(c, msg)

			case msgUnsubscribe:
				w.handleUnSubscribe(c, msg)

			case msgPublish:
				w.handlePublish(c, msg)

			case msgEvent:
				w.handleEvent(c, msg)

			case msgSubscribed:
				w.handleSubscribed(c, msg)

			case msgSubscribeError:
				w.handleSubscribeError(c, msg)

			case msgUnsubscribed:
				w.handleUnsubscribed(c, msg)

			case msgUnsubscribeError:
				w.handleUnsubscribeError(c, msg)

			case msgHeartbeat:
				w.handleHeartbeat(c, msg, data)
			}
		}
	}
}

func (w *Wango) handleCallResult(c *Conn, msg []interface{}) {
	callResultMessage, err := parseWampMessage(msgCallResult, msg)
	if err != nil {
		logger(err)
		return
	}
	c.callResultsLocker.Lock()
	resChan, ok := c.callResults[callResultMessage.ID]
	c.callResultsLocker.Unlock()
	if !ok {
		logger("Achtung! No res chan!")
		return
	}
	res := new(callResult)
	if len(callResultMessage.Args) > 0 {
		res.result = callResultMessage.Args[0]
	}
	resChan <- res
}

func (w *Wango) handleCallError(c *Conn, msg []interface{}) {
	callResultMessage, err := parseWampMessage(msgCallResult, msg)
	if err != nil {
		logger(err)
		return
	}
	c.callResultsLocker.Lock()
	resChan, ok := c.callResults[callResultMessage.ID]
	c.callResultsLocker.Unlock()
	if !ok {
		logger("Achtung! No res chan!")
	}
	res := new(callResult)
	res.err = errors.New("RPC error#")
	if len(callResultMessage.Args) > 0 {
		if _err := callResultMessage.Args[0].(string); ok {
			err = errors.New("RPC error#" + _err)
		}
	}
	resChan <- &callResult{nil, err}
}

func (w *Wango) handleEvent(c *Conn, msg []interface{}) {
	eventMessage, err := parseWampMessage(msgEvent, msg)
	if err != nil {
		logger(err)
		return
	}
	c.eventHandlersLocker.RLock()
	handler, ok := c.eventHandlers[eventMessage.URI]
	c.eventHandlersLocker.RUnlock()
	if !ok {
		return
	}
	var event interface{}
	if len(eventMessage.Args) > 0 {
		event = eventMessage.Args[0]
	}
	go handler(eventMessage.URI, event)
}

func (w *Wango) handleSubscribed(c *Conn, msg []interface{}) {
	subMessage, err := parseWampMessage(msgSubscribed, msg)
	if err != nil {
		logger(err)
		return
	}
	listeners := c.subRequests.getRequests(subMessage.URI)
	for _, l := range listeners {
		l.ch <- nil
	}
	if len(subMessage.Args) != 0 {
		c.eventHandlersLocker.RLock()
		handler, ok := c.eventHandlers[subMessage.URI]
		c.eventHandlersLocker.RUnlock()
		if !ok {
			return
		}
		handler(subMessage.URI, subMessage.Args[0])
	}
}

func (w *Wango) handleSubscribeError(c *Conn, msg []interface{}) {
	subMessage, err := parseWampMessage(msgSubscribeError, msg)
	if err != nil {
		logger(err)
		return
	}
	listeners := c.subRequests.getRequests(subMessage.URI)
	err = errors.New("Sub error#")
	if len(subMessage.Args) != 0 {
		if _err, ok := subMessage.Args[0].(string); ok {
			err = errors.New("Sub error#" + _err)
		}
	}
	for _, l := range listeners {
		l.ch <- err
	}
}

func (w *Wango) handleUnsubscribed(c *Conn, msg []interface{}) {
	subMessage, err := parseWampMessage(msgUnsubscribed, msg)
	if err != nil {
		logger(err)
		return
	}
	listeners := c.unsubRequests.getRequests(subMessage.URI)
	for _, l := range listeners {
		l.ch <- nil
	}
}

func (w *Wango) handleUnsubscribeError(c *Conn, msg []interface{}) {
	subMessage, err := parseWampMessage(msgUnsubscribeError, msg)
	if err != nil {
		logger(err)
		return
	}
	listeners := c.unsubRequests.getRequests(subMessage.URI)
	err = errors.New("Unsub error#")
	if len(subMessage.Args) != 0 {
		if _err, ok := subMessage.Args[0].(string); ok {
			err = errors.New("Unsub error#" + _err)
		}
	}
	for _, l := range listeners {
		l.ch <- err
	}
}

func (w *Wango) handleRPCCall(c *Conn, msg []interface{}) {
	rpcMessage, err := parseWampMessage(msgCall, msg)
	if err != nil {
		logger("Can't parse rpc message", err.Error())
		return
	}

	uri := rpcMessage.URI
	handler, ok := w.rpcHandlers[uri]
	if !ok {
		var rgx *regexp.Regexp
		for rgx, handler = range w.rpcRgxHandlers {
			if rgx.MatchString(uri) {
				ok = true
				break
			}
		}
	}

	if ok {
		// gouroutine to prevent block message reading
		go func() {
			res, err := handler(c, uri, rpcMessage.Args...)
			if err != nil {
				response, _ := createMessage(msgIntTypes[msgCallError], rpcMessage.ID, createError(err))
				// TODO: error handling
				c.send(response)
				return
			}
			response, _ := createMessage(msgIntTypes[msgCallResult], rpcMessage.ID, res)
			c.send(response)
		}()
		return
	}

	response, _ := createMessage(msgIntTypes[msgCallError], rpcMessage.ID, createError(errRPCNotRegistered))
	c.send(response)
}

func (w *Wango) handleSubscribe(c *Conn, msg []interface{}) {
	subMessage, err := parseWampMessage(msgSubscribe, msg)
	if err != nil {
		logger("Can't parse rpc message", err.Error())
		return
	}

	_uri := subMessage.URI
	w.subscribersLocker.Lock()
	defer w.subscribersLocker.Unlock()
	for uri, handler := range w.subHandlers {
		if strings.HasPrefix(_uri, uri) {
			var data interface{}
			if handler.subHandler != nil {
				data, err = handler.subHandler(c, _uri, subMessage.Args...)
				if err != nil {
					response, _ := createMessage(msgIntTypes[msgSubscribeError], _uri, createError(err))
					go c.send(response)
					return
				}
			}
			if _, ok := w.subscribers[_uri]; !ok {
				w.subscribers[_uri] = subscribersMap{}
			}
			w.subscribers[_uri][c.id] = subscriberExists
			response, _ := createMessage(msgIntTypes[msgSubscribed], _uri, data)
			go c.send(response)
			return
		}
	}
	response, _ := createMessage(msgIntTypes[msgSubscribeError], _uri, createError(errSubURINotRegistered))
	go c.send(response)
}

func (w *Wango) handleUnSubscribe(c *Conn, msg []interface{}) {
	unsubMessage, err := parseWampMessage(msgUnsubscribe, msg)
	if err != nil {
		logger("Can't parse rpc message", err.Error())
		return
	}

	_uri := unsubMessage.URI
	w.subscribersLocker.Lock()
	defer w.subscribersLocker.Unlock()
	for uri, subscribers := range w.subscribers {
		if uri == _uri {
			if _, ok := subscribers[c.id]; ok {
				delete(subscribers, c.id)
				response, _ := createMessage(msgIntTypes[msgUnsubscribed], _uri)
				go c.send(response)
				for handlersURI, handler := range w.subHandlers {
					if strings.HasPrefix(_uri, handlersURI) && handler.unsubHandler != nil {
						handler.unsubHandler(c, _uri, nil)
					}
				}
				return
			}
			response, _ := createMessage(msgIntTypes[msgUnsubscribeError], _uri, createError(errNotSubscribes))
			go c.send(response)
			return
		}
	}
	response, _ := createMessage(msgIntTypes[msgUnsubscribeError], _uri, createError(errSubURINotRegistered))
	go c.send(response)
}

func (w *Wango) handleHeartbeat(c *Conn, msg []interface{}, data []byte) {
	if !c.clientConnection {
		c.send(data)
	}
}

func (w *Wango) handlePublish(c *Conn, msg []interface{}) {
	pubMessage, err := parseWampMessage(msgPublish, msg)
	if err != nil {
		logger("Can't parse publish message", err.Error())
		return
	}
	var event interface{}
	if len(pubMessage.Args) > 0 {
		event = pubMessage.Args[0]
	}
	go w.Publish(pubMessage.URI, event)
}

func (w *Wango) addConnection(ws *websocket.Conn, extra interface{}) *Conn {
	cn := new(Conn)
	cn.connection = ws
	cn.id = newUUIDv4()
	cn.extra = extra
	cn.sendChan = make(chan []byte, sendChanBufferSize)
	cn.eventHandlers = map[string]EventHandler{}
	cn.callResults = map[interface{}]chan *callResult{}
	cn.subRequests = subRequestsListeners{listeners: map[string][]subRequestsListener{}, locker: new(sync.Mutex)}
	cn.unsubRequests = subRequestsListeners{listeners: map[string][]subRequestsListener{}, locker: new(sync.Mutex)}
	cn.breakChan = make(chan struct{})
	cn.connected = true
	cn.aliveMutex = new(sync.Mutex)
	cn.aliveTimeout = w.aliveTimeout
	cn.aliveTimer = time.AfterFunc(cn.aliveTimeout, func() {
		logger("Connection timeout", cn.ID())
		cn.Close()
	})
	cn.extraLocker = new(sync.RWMutex)
	cn.callResultsLocker = new(sync.Mutex)
	cn.eventHandlersLocker = new(sync.RWMutex)
	w.connectionsLocker.Lock()
	w.connections[cn.id] = cn
	w.connectionsLocker.Unlock()

	return cn
}

func (w *Wango) getConnection(id string) (*Conn, error) {
	w.connectionsLocker.RLock()
	cn, ok := w.connections[id]
	w.connectionsLocker.RUnlock()
	if !ok {
		return nil, errors.New("NOT FOUND")
	}

	return cn, nil
}

func (w *Wango) deleteConnection(c *Conn) {
	c.aliveTimer.Stop()
	c.extraLocker.Lock()
	c.connected = false
	c.extraLocker.Unlock()

	w.connectionsLocker.Lock()
	delete(w.connections, c.id)
	w.connectionsLocker.Unlock()

	go c.subRequests.closeRequests()

	w.subscribersLocker.Lock()
	for _, subscribers := range w.subscribers {
		delete(subscribers, c.id)
	}
	w.subscribersLocker.Unlock()

	if w.closeCB != nil {
		w.closeCB(c)
	}
}
