package wango

import "golang.org/x/net/websocket"

type conn struct {
	id         string
	connection *websocket.Conn
	extra      interface{}
	sendChan   chan interface{}
}

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
