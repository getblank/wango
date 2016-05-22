package wango

import (
	"strconv"

	"github.com/pkg/errors"
	"golang.org/x/net/websocket"
)

func (c *Conn) receiveWelcome() error {
	var data []byte
	err := websocket.Message.Receive(c.connection, &data)
	if err != nil {
		return errors.Wrap(err, "Can't receive welcome message")
	}
	msgType, msg, err := parseMessage(data)
	if err != nil {
		return errors.Wrap(err, "Parsing welcome message")
	}
	if msgType != msgWelcome {
		if typeString, ok := msgIntTypes[msgType]; ok {
			return errors.New("First message received must be welcome. Received: " + typeString)
		}
		return errors.New("First message received must be welcome. Received unknown type " + strconv.Itoa(msgType))
	}
	if len(msg) < 3 {
		return errors.New("Invalid welcome message")
	}
	// sessionID, ok := msg[1].(string)
	// if !ok {
	// 	return errors.New("Invalid welcome message. SessionID is not a string")
	// }
	// c.sessionID = sessionID

	// protocolVersion, ok := msg[1].(float64)
	// if !ok {
	// 	return errors.New("Invalid welcome message. ProtocolVersion is not a number")
	// }
	// c.protocolVersion = int(protocolVersion)

	// serverIdent, ok := msg[1].(string)
	// if !ok {
	// 	return errors.New("Invalid welcome message. ServerIdent is not a string")
	// }
	// c.serverIdent = serverIdent

	// if c.sessionOpenCallback != nil {
	// 	c.sessionOpenCallback(c.sessionID)
	// }

	return nil
}
