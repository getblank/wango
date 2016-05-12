package wango

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"

	"golang.org/x/net/websocket"
)

type WS struct {
	connections       map[string]*conn
	connectionsLocker sync.RWMutex
	// rpcHandlers       map[regexp]rpcHandler
	// subHandlers       map[regexp]subHandler
	// subscribers       subscr
	openCB  func()
	closeCB func()
}

func New() *WS {
	ws := new(WS)
	ws.connections = map[string]*conn{}
	return ws
}

func (ws *WS) WampHandler(connection *websocket.Conn, extra interface{}) {
	ws.addConnection(connection, extra)
	var data interface{}
	websocket.Message.Receive(connection, &data)

	websocket.Message.Send(connection, "pong")
}

type conn struct {
	id         string
	connection *websocket.Conn
	extra      interface{}
}

func (c *WS) addConnection(connection *websocket.Conn, extra interface{}) string {
	cn := new(conn)
	cn.connection = connection
	cn.id = newUUIDv4()
	cn.extra = extra
	c.connectionsLocker.Lock()
	defer c.connectionsLocker.Unlock()
	c.connections[cn.id] = cn
	return cn.id
}

func (c *WS) deleteConnection(id string) {
	c.connectionsLocker.Lock()
	defer c.connectionsLocker.Unlock()
	delete(c.connections, id)
}

func (c *WS) getConnection(id string) (*conn, error) {
	c.connectionsLocker.RLock()
	defer c.connectionsLocker.RUnlock()
	cn, ok := c.connections[id]
	if !ok {
		return nil, errors.New("NOT FOUND")
	}
	return cn, nil
}

func newUUIDv4() string {
	u := [16]byte{}
	_, err := rand.Read(u[:16])
	if err != nil {
		panic(err)
	}

	u[8] = (u[8] | 0x80) & 0xBf
	u[6] = (u[6] | 0x40) & 0x4f

	return fmt.Sprintf("%x-%x-%x-%x-%x", u[:4], u[4:6], u[6:8], u[8:10], u[10:])
}

func init() {

}
