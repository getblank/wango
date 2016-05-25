package wango

import (
	"encoding/json"

	"github.com/pkg/errors"
)

func parseMessage(_msg []byte) (int, []interface{}, error) {
	var msg []interface{}
	err := json.Unmarshal(_msg, &msg)
	if err != nil {
		return 0, nil, errors.Wrap(err, "when unmarshaling wamp message")
	}
	if len(msg) < 1 {
		return 0, nil, errors.New("invalid wamp message")
	}
	if numCallType, ok := msg[0].(float64); ok {
		return int(numCallType), msg, nil
	}
	if strCallType, ok := msg[0].(string); ok {
		callType, ok := msgTxtTypes[strCallType]
		if !ok {
			return 0, nil, errors.Errorf("unknown call type: %s", strCallType)
		}
		return callType, msg, nil
	}
	return 0, nil, errors.New("unknown call type")
}

func parseWampMessage(typ int, msg []interface{}) (*wampMsg, error) {
	if typ == msgCall && len(msg) < 3 {
		return nil, errors.New("invalid wamp message")
	} else if len(msg) < 2 {
		return nil, errors.New("invalid wamp message")
	}
	message := new(wampMsg)
	switch typ {
	case msgCall:
		if msg[1] == nil {
			return nil, errors.New("invalid wamp message. callID is nil")
		}
		message.ID = msg[1]
		uri, ok := msg[2].(string)
		if !ok {
			return nil, errors.New("invalid wamp message. uri is not a string")
		}
		message.URI = uri
		if len(msg) > 3 {
			message.Args = msg[3:]
		}
	case msgCallResult, msgCallError:
		if msg[1] == nil {
			return nil, errors.New("invalid wamp message. callID is nil")
		}
		message.ID = msg[1]
		if len(msg) > 2 {
			message.Args = msg[2:]
		}
	case msgSubscribe, msgUnsubscribe, msgPublish, msgSubscribed, msgSubscribeError, msgUnsubscribed, msgUnsubscribeError, msgEvent:
		uri, ok := msg[1].(string)
		if !ok {
			return nil, errors.New("invalid wamp message. uri is not a string")
		}
		message.URI = uri
		if len(msg) > 2 {
			message.Args = msg[2:]
		}
	}
	return message, nil
}
