package wango

import (
	"golang.org/x/net/websocket"
)

type connector interface{}

type connKeeper interface {
	add(conn *websocket.Conn, extra interface{}) string
	del(connID string)
	get(connID string) (*Conn, error)
}
