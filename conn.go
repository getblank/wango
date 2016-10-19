package wango

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

// Conn represents a websocket connection
type Conn struct {
	id                  string
	connection          *websocket.Conn
	extra               interface{}
	extraLocker         *sync.RWMutex
	sendChan            chan []byte
	breakChan           chan struct{}
	subRequests         subRequestsListeners
	unsubRequests       subRequestsListeners
	callResults         map[interface{}]chan *callResult
	callResultsLocker   *sync.Mutex
	eventHandlers       map[string]EventHandler
	eventHandlersLocker *sync.RWMutex
	connected           bool
	clientConnection    bool
	aliveTimer          *time.Timer
	aliveTimeout        time.Duration
	aliveMutex          *sync.Mutex
	stringMode          bool
}

// EventHandler is an interface for handlers to published events. The uri
// is the URI of the event and event is the event centents.
type EventHandler func(uri string, event interface{})

// Close closes websocket connection
func (c *Conn) Close() {
	c.breakChan <- struct{}{}
}

// Connected returns true if websocket connection established and not closed
func (c *Conn) Connected() bool {
	c.extraLocker.RLock()
	defer c.extraLocker.RUnlock()
	connected := c.connected
	return connected
}

// GetExtra returns extra data stored in connection
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

// Request returns related *http.Request
func (c *Conn) Request() *http.Request {
	return c.connection.Request()
}

// SendEvent sends event for provided uri directly to connection
func (c *Conn) SendEvent(uri string, event interface{}) error {
	msg, _ := createMessage(msgIntTypes[msgEvent], uri, event)
	if !c.Connected() {
		return errConnectionClosed
	}
	c.send(msg)
	return nil
}

// SetExtra stores extra data in connection
func (c *Conn) SetExtra(extra interface{}) {
	c.extraLocker.Lock()
	c.extra = extra
	c.extraLocker.Unlock()
}

// StringMode sets a string mode to use TextFrame encoding for sending messages
func (c *Conn) StringMode() {
	c.stringMode = true
}

func (c *Conn) heartbeating() {
	var hbSequence int
	ticker := time.NewTicker(heartBeatFrequency)
	for c.Connected() {
		msg, _ := createMessage(msgIntTypes[msgHeartbeat], hbSequence)
		hbSequence++
		logger("HB message: ", string(msg))
		c.send(msg)
		<-ticker.C
	}
	ticker.Stop()
}

func (c *Conn) resetTimeoutTimer() {
	c.aliveMutex.Lock()
	if c.aliveTimer.Stop() {
		c.aliveTimer.Reset(c.aliveTimeout)
	}
	c.aliveMutex.Unlock()
}

func (c *Conn) send(msg []byte) {
	c.sendChan <- msg
}

func (c *Conn) sender() {
	var err error
	for msg := range c.sendChan {
		logger("Sending message ", string(msg))
		if c.stringMode {
			err = websocket.Message.Send(c.connection, string(msg))
		} else {
			err = websocket.Message.Send(c.connection, msg)
		}
		if err != nil {
			logger("Error when send message", err.Error())
			c.breakChan <- struct{}{}
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

func (s subRequestsListeners) closeRequests() {
	s.locker.Lock()
	defer s.locker.Unlock()
	for _, listeners := range s.listeners {
		for _, listener := range listeners {
			listener.ch <- errConnectionClosed
		}
	}
}
