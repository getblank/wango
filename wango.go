package wango

import (
	"errors"
	"fmt"
	"time"

	"github.com/getblank/uuid"
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
	msgUnsubscribed
	msgUnsubscribeError
	msgHeartbeat = 20
)

const (
	hbLimit            = 5
	hbTimeout          = time.Second * 5
	sendChanBufferSize = 50
	identity           = "wango"
	subscriberExists   = true
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
		11: "UNSUBSCRIBED",
		12: "UNSUBSCRIBEERROR",
		20: "HB",
	}
	msgTxtTypes = map[string]int{
		"WELCOME":          0,
		"PREFIX":           1,
		"CALL":             2,
		"CALLRESULT":       3,
		"CALLERROR":        4,
		"SUBSCRIBE":        5,
		"UNSUBSCRIBE":      6,
		"PUBLISH":          7,
		"EVENT":            8,
		"SUBSCRIBED":       9,
		"SUBSCRIBEERROR":   10,
		"UNSUBSCRIBED":     11,
		"UNSUBSCRIBEERROR": 12,
		"HB":               20,
	}

	errHandlerAlreadyRegistered = errors.New("Handler already registered")
	errRPCNotRegistered         = errors.New("RPC not registered")
	errSubURINotRegistered      = errors.New("Sub URI not registered")
	errForbidden                = errors.New("403 forbidden")
	errNotSubscribes            = errors.New("Not subscribed")
	errConnectionClosed         = errors.New("Connection closed")

	debugMode bool
)

func newUUIDv4() string {
	return uuid.NewV4()
}

func logger(in ...interface{}) {
	if debugMode {
		fmt.Println(in...)
	}
}
