package wango

import (
	"sync"

	"golang.org/x/net/websocket"
)

// Conn represents a websocket connection
type Conn struct {
	id                  string
	connection          *websocket.Conn
	extra               interface{}
	extraLocker         sync.RWMutex
	sendChan            chan interface{}
	breakChan           chan struct{}
	subRequests         subRequestsListeners
	unsubRequests       subRequestsListeners
	callResults         map[string]chan *callResult
	callResultsLocker   sync.Mutex
	eventHandlers       map[string]EventHandler
	eventHandlersLocker sync.RWMutex
	connected           bool
}

// EventHandler is an interface for handlers to published events. The uri
// is the URI of the event and event is the event centents.
type EventHandler func(uri string, event interface{})

func (c *Conn) Close() {
	c.breakChan <- struct{}{}
}

func (c *Conn) Connected() bool {
	c.extraLocker.RLock()
	defer c.extraLocker.RUnlock()
	connected := c.connected
	return connected
}

func (c *Conn) GetExtra() interface{} {
	c.extraLocker.RLock()
	extra := c.extra
	c.extraLocker.RUnlock()
	return extra
}

// ID returns connection ID
func (c *Conn) ID() string {
	return c.id
}

// RemoteAddr returns remote address
func (c *Conn) RemoteAddr() string {
	return c.connection.Request().RemoteAddr
}

func (c *Conn) SetExtra(extra interface{}) {
	c.extraLocker.Lock()
	c.extra = extra
	c.extraLocker.Unlock()
}

func (c *Conn) send(msg interface{}) {
	c.sendChan <- msg
}

func (c *Conn) sender() {
	for msg := range c.sendChan {
		err := websocket.Message.Send(c.connection, msg)
		if err != nil {
			println("Error when send message", err)
		}
	}
}

type subRequestsListener struct {
	id string
	ch chan error
}

type callResult struct {
	result interface{}
	err    error
}

func (s subRequestsListeners) addRequest(id, uri string, ch chan error) {
	s.locker.Lock()
	defer s.locker.Unlock()
	if s.listeners[uri] == nil {
		s.listeners[uri] = []subRequestsListener{}
	}
	s.listeners[uri] = append(s.listeners[uri], subRequestsListener{id, ch})
}

func (s subRequestsListeners) getRequests(uri string) []subRequestsListener {
	s.locker.Lock()
	defer s.locker.Unlock()
	listeners := s.listeners[uri]
	delete(s.listeners, uri)
	return listeners
}
