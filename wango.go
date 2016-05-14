package wango

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/pkg/errors"
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

	ErrHandlerAlreadyRegistered = errors.New("Handler already registered")
	ErrRPCNotRegistered         = errors.New("RPC not registered")
	ErrSubURINotRegistered      = errors.New("Sub URI not registered")
	ErrForbidden                = errors.New("403 forbidden")
	ErrNotSubscribes            = errors.New("Not subscribed")
)

func newUUIDv4() string {
	u := [16]byte{}
	rand.Read(u[:16])

	u[8] = (u[8] | 0x80) & 0xBf
	u[6] = (u[6] | 0x40) & 0x4f

	return fmt.Sprintf("%x-%x-%x-%x-%x", u[:4], u[4:6], u[6:8], u[8:10], u[10:])
}

func init() {

}
