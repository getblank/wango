package wango

import "encoding/json"

func createMessage(args ...interface{}) ([]byte, error) {
	return json.Marshal(args)
}

func createHeartbeatEvent(counter int) ([]byte, error) {
	return createMessage(msgIntTypes[msgHeartbeat], counter)
}

func createWelcomeMessage(id string) ([]byte, error) {
	return createMessage(msgWelcome, id, identity)
}

func createError(err interface{}) string {
	var text string
	switch err.(type) {
	case error:
		text = err.(error).Error()
	case string:
		text = err.(string)
	}
	return "error#" + text
}
