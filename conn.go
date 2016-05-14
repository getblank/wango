package wango

import (
	"sync"

	"golang.org/x/net/websocket"
)

type conn struct {
	id                string
	connection        *websocket.Conn
	extra             interface{}
	sendChan          chan interface{}
	subRequests       subRequestsListeners
	unsubRequests     subRequestsListeners
	callResults       map[string]chan *callResult
	callResultsLocker sync.Mutex
    eventHandlers map[string]EventHandler
	eventHandlersLocker sync.RWMutex
}

// EventHandler is an interface for handlers to published events. The uri
// is the URI of the event and event is the event centents.
type EventHandler func(uri string, event interface{})

func (c *conn) send(msg interface{}) {
	c.sendChan <- msg
}

func (c *conn) sender() {
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
