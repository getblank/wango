package wango

import (
	"fmt"
	"io"
	"math/rand"
	"regexp"
	"sync"
	"time"

	"github.com/ivahaev/go-logger"

	"github.com/pkg/errors"
	"golang.org/x/net/websocket"
)

const (
	msgWelcome = iota
	msgPrefix
	msgCall
	msgCallResult
	msgCallError
	msgSubscribe
	msgUnsubscribe
	msgPublish
	msgEvent
	msgSubscribed
	msgSubscribeError
	msgHeartbeat = 20
)

const (
	hbLimit            = 5
	hbTimeout          = time.Second * 5
	sendChanBufferSize = 50
	identity           = "wango"
)

var (
	msgIntTypes = map[int]string{
		0:  "WELCOME",
		1:  "PREFIX",
		2:  "CALL",
		3:  "CALLRESULT",
		4:  "CALLERROR",
		5:  "SUBSCRIBE",
		6:  "UNSUBSCRIBE",
		7:  "PUBLISH",
		8:  "EVENT",
		9:  "SUBSCRIBED",
		10: "SUBSCRIBEERROR",
		20: "HB",
	}
	msgTxtTypes = map[string]int{
		"WELCOME":        0,
		"PREFIX":         1,
		"CALL":           2,
		"CALLRESULT":     3,
		"CALLERROR":      4,
		"SUBSCRIBE":      5,
		"UNSUBSCRIBE":    6,
		"PUBLISH":        7,
		"EVENT":          8,
		"SUBSCRIBED":     9,
		"SUBSCRIBEERROR": 10,
		"HB":             20,
	}

	ErrHandlerAlreadyRegistered = errors.New("Handler already registered")
	ErrRPCNotRegistered         = errors.New("RPC not registered")
)

type callMsg struct {
	CallID string
	URI    string
	Args   []interface{}
}

type WS struct {
	connections       map[string]*conn
	connectionsLocker sync.RWMutex
	rpcHandlers       map[string]RPCHandler
	rpcRgxHandlers    map[*regexp.Regexp]RPCHandler
	// subHandlers       map[regexp]subHandler
	// subscribers       subscr
	openCB  func()
	closeCB func()
}

func New() *WS {
	server := new(WS)
	server.connections = map[string]*conn{}
	server.rpcHandlers = map[string]RPCHandler{}
	server.rpcRgxHandlers = map[*regexp.Regexp]RPCHandler{}
	return server
}

func (server *WS) RegisterRPCHandler(_uri interface{}, fn RPCHandler) error {
	switch _uri.(type) {
	case string:
		uri := _uri.(string)
		if _, ok := server.rpcHandlers[uri]; ok {
			return ErrHandlerAlreadyRegistered
		}
		server.rpcHandlers[uri] = fn
	case *regexp.Regexp:
		rgx := _uri.(*regexp.Regexp)
		if _, ok := server.rpcRgxHandlers[rgx]; ok {
			return ErrHandlerAlreadyRegistered
		}
		server.rpcRgxHandlers[rgx] = fn
	}

	return nil
}

func (server *WS) WampHandler(ws *websocket.Conn, extra interface{}) {
	c := server.addConnection(ws, extra)
	defer server.deleteConnection(c.id)

	go c.sender()

	server.receive(c)
}

func (server *WS) receive(c *conn) {
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
			logger.Error(err)
			continue
		}
		switch msgType {
		case msgCall:
			server.handleRPCCall(c, msg)
		case msgSubscribe:
		case msgUnsubscribe:
		case msgHeartbeat:
		}
	}
}

func (server *WS) handleRPCCall(c *conn, msg []interface{}) {
	rpcMessage, err := parseCallMessage(msg)
	if err != nil {
		logger.Error("Can't parse rpc message", err.Error())
		return
	}
	uri := rpcMessage.URI
	handler, ok := server.rpcHandlers[uri]
	if ok {
		res, err := handler(c.id, uri, rpcMessage.Args...)
		if err != nil {
			response, _ := createMessage(msgCallError, rpcMessage.CallID, err)
			// TODO: error handling
			c.send(response)
			return
		}
		response, _ := createMessage(msgCallResult, rpcMessage.CallID, res)
		c.send(response)
		return
	}
	for rgx, handler := range server.rpcRgxHandlers {
		if rgx.MatchString(uri) {
			res, err := handler(c.id, uri, rpcMessage.Args...)
			if err != nil {
				response, _ := createMessage(msgCallError, rpcMessage.CallID, err)
				c.send(response)
				return
			}
			response, _ := createMessage(msgCallResult, rpcMessage.CallID, res)
			c.send(response)
			return
		}
	}
	response, _ := createMessage(msgCallError, rpcMessage.CallID, ErrRPCNotRegistered)
	c.send(response)
}

func (c *conn) send(msg interface{}) {
	c.sendChan <- msg
}

func (c *conn) sender() {
	for msg := range c.sendChan {
		websocket.Message.Send(c.connection, msg)
	}
}

func (server *WS) addConnection(ws *websocket.Conn, extra interface{}) *conn {
	cn := new(conn)
	cn.connection = ws
	cn.id = newUUIDv4()
	cn.extra = extra
	cn.sendChan = make(chan interface{}, sendChanBufferSize)
	server.connectionsLocker.Lock()
	defer server.connectionsLocker.Unlock()
	server.connections[cn.id] = cn

	return cn
}

func (server *WS) getConnection(id string) (*conn, error) {
	server.connectionsLocker.RLock()
	defer server.connectionsLocker.RUnlock()
	cn, ok := server.connections[id]
	if !ok {
		return nil, errors.New("NOT FOUND")
	}

	return cn, nil
}

func (server *WS) deleteConnection(id string) {
	server.connectionsLocker.Lock()
	defer server.connectionsLocker.Unlock()
	delete(server.connections, id)
}

type conn struct {
	id         string
	connection *websocket.Conn
	extra      interface{}
	sendChan   chan interface{}
}

type RPCHandler func(connID string, uri string, args ...interface{}) (interface{}, error)

func newUUIDv4() string {
	u := [16]byte{}
	rand.Read(u[:16])

	u[8] = (u[8] | 0x80) & 0xBf
	u[6] = (u[6] | 0x40) & 0x4f

	return fmt.Sprintf("%x-%x-%x-%x-%x", u[:4], u[4:6], u[6:8], u[8:10], u[10:])
}

func init() {

}
