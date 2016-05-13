package wango

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

var origin = "http://localhost/"
var url = "ws://localhost:1234"

func TestAcceptingConcurrentConnections(t *testing.T) {
	path := "/wamp-opening"
	server := createWampServer(path)
	numberConnections := 10
	for i := 0; i < numberConnections; i++ {
		go connectForOneSecond(path)
	}
	time.Sleep(time.Millisecond * 10)

	totalConnections := len(server.connections)
	if totalConnections != numberConnections {
		t.Fatal("Invalid connections number when connecting", totalConnections)
	}
}

func TestClosingConcurrentConnections(t *testing.T) {
	path := "/wamp-closing"
	server := createWampServer(path)
	numberConnections := 10
	for i := 0; i < numberConnections; i++ {
		go connectForOneSecond(path)
	}

	time.Sleep(time.Second * 2)

	totalConnections := len(server.connections)
	if totalConnections != 0 {
		t.Fatal("Invalid connections number when closing", totalConnections)
	}
}

func TestRPCHandling(t *testing.T) {
	path := "/wamp-rpc"
	server := createWampServer(path)

	var uri = "net.wango.test"
	err := server.RegisterRPCHandler(uri, testRPCHandler)
	if err != nil {
		t.Fatal("Can't register rpcHandler")
	}
	if len(server.rpcHandlers) != 1 {
		t.Fatal("No handlers registered")
	}
	err = server.RegisterRPCHandler(uri, testRPCHandler)
	if err == nil {
		t.Fatal("Must not register register rpcHandler")
	}
	res, err := connectAndRPC(path, uri, nil)
	if err != nil {
		t.Fatal("RPC failed")
	}
	if res.(string) != "test-"+uri {
		t.Fatal("invalid RPC befaviour")
	}

	uri = "net.wango.rgx"
	err = server.RegisterRPCHandler(regexp.MustCompile(`^net\.wango\..*`), testRPCHandler)
	if err != nil {
		t.Fatal("Can't register rgx rpcHandler")
	}
	err = server.RegisterRPCHandler(regexp.MustCompile(`^net\.wango\..*`), testRPCHandler)
	if err == nil {
		t.Fatal("Must not register register rgx rpcHandler")
	}
	res, err = connectAndRPC(path, uri, nil)
	if err != nil {
		t.Fatal("RPC failed")
	}
	if res.(string) != "test-"+uri {
		t.Fatal("invalid RPC befaviour")
	}

	uri = "wango.rgx"
	server.RegisterRPCHandler(regexp.MustCompile(`^wango\..*`), testRPCHandlerWithErrorReturn)
	res, err = connectAndRPC(path, uri, nil)
	if err == nil {
		t.Fatal("RPC failed. No error returns")
	}
}

func TestSubHandling(t *testing.T) {
	path := "/wamp-sub"
	server := createWampServer(path)

	uri := "wango.sub-test"
	err := server.RegisterSubHandler(uri, testSubHandler, nil)
	if err != nil {
		t.Fatal("Can't register handler", err)
	}
	if len(server.subHandlers) != 1 {
		t.Fatal("subHandler not registered")
	}

	err = server.RegisterSubHandler(uri, testSubHandler, nil)
	if err == nil {
		t.Fatal("Must not register handler")
	}
	if len(server.subHandlers) != 1 {
		t.Fatal("subHandler not registered")
	}

	connectAndSub(t, path, uri+".test", nil)

	if len(server.subscribers) == 0 {
		t.Fatal("subHandler not registered")
	}

	connectAndSub(t, path, uri+".error", true)

	uri = uri + ".wait"
	eventToSend := "test-event"
	appendix := "appendix"
	err = server.RegisterSubHandler(uri, testSubHandler, func(_uri string, event interface{}, extra interface{}) (bool, interface{}) {
		if _uri != uri {
			t.Fatal("Uri mismatched")
		}
		return true, event.(string) + appendix
	})
	go func() {
		time.Sleep(time.Millisecond * 200)
		server.Publish(uri, eventToSend)
	}()
	res, err := connectAndWaitForEvent(t, path, uri, nil)
	if err != nil {
		t.Fatal("Can't wait for event")
	}
	if res == nil {
		t.Fatal("Invalid event received - nil")
	}

	eventReceived, ok := res.(string)
	if !ok {
		t.Fatal("Invalid event received. Not string", eventReceived)
	}
	if eventReceived != eventToSend+appendix {
		t.Fatal("Invalid event received", eventReceived, "!=", eventToSend+appendix)
	}
}

func testRPCHandler(connID string, uri string, args ...interface{}) (interface{}, error) {
	return "test-" + uri, nil
}

func testRPCHandlerWithErrorReturn(connID string, uri string, args ...interface{}) (interface{}, error) {
	return nil, errors.New("RPC error")
}

func testSubHandler(connID string, uri string, args ...interface{}) bool {
	if strings.Contains(uri, "error") {
		return false
	}
	return true
}

func connectForOneSecond(path string) {
	ws, err := websocket.Dial(url+path, "", origin)
	if err != nil {
		log.Fatal(err)
	}
	time.Sleep(time.Second * 1)
	ws.Close()
}

func connectAndRPC(path, uri string, args ...interface{}) (interface{}, error) {
	ws, err := websocket.Dial(url+path, "", origin)
	defer ws.Close()
	if err != nil {
		log.Fatal(err)
	}
	msgId := newUUIDv4()
	message, err := createMessage(msgCall, msgId, uri)
	websocket.Message.Send(ws, message)
	for {
		var msg string
		err := websocket.Message.Receive(ws, &msg)
		if err != nil {
			return nil, err
		}
		var message []interface{}
		err = json.Unmarshal([]byte(msg), &message)
		if err != nil {
			panic("Can't unmarshal message")
		}
		if message[0].(float64) == msgCallResult && message[1].(string) == msgId {
			return message[2], nil
		}
		if message[0].(float64) == msgCallError && message[1].(string) == msgId {
			return nil, errors.New(message[2].(string))
		}
	}
}

func connectAndSub(t *testing.T, path, uri string, args ...interface{}) {
	ws, err := websocket.Dial(url+path, "", origin)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	message, err := createMessage(msgSubscribe, uri)
	websocket.Message.Send(ws, message)
	ch := make(chan string)

	go func() {
		var msg string
		err := websocket.Message.Receive(ws, &msg)
		if err != nil {
			t.Fatal("Message receive failed", err)
		}
		ch <- msg
	}()

	timer := time.NewTimer(time.Second)
	for {
		select {
		case msg := <-ch:
			var message []interface{}
			err = json.Unmarshal([]byte(msg), &message)
			if err != nil {
				t.Fatal("Can't unmarshal message")
			}
			if message[1].(string) == uri {
				if message[0].(float64) == msgSubscribed {
					return
				}
				if message[0].(float64) == msgSubscribeError {
					if args != nil && args[0].(bool) {
						return
					}
					t.Fatal(message[2])
				}
			}
		case <-timer.C:
			t.Fatal("Time is gone")
		}
	}
}

func connectAndWaitForEvent(t *testing.T, path, uri string, args ...interface{}) (interface{}, error) {
	ws, err := websocket.Dial(url+path, "", origin)
	defer ws.Close()
	if err != nil {
		log.Fatal(err)
	}
	message, err := createMessage(msgSubscribe, uri)
	websocket.Message.Send(ws, message)
	ch := make(chan string)
	errChan := make(chan error)

	go func() {
		for {
			var msg string
			err := websocket.Message.Receive(ws, &msg)
			if err != nil {
				errChan <- err
				return
			}
			ch <- msg
		}
	}()

	timer := time.NewTimer(time.Second)
	for {
		select {
		case err := <-errChan:
			return nil, err
		case msg := <-ch:
			var message []interface{}
			err = json.Unmarshal([]byte(msg), &message)
			if err != nil {
				panic("Can't unmarshal message")
			}
			if message[1].(string) == uri {
				if message[0].(float64) == msgEvent {
					return message[2], nil
				}
				if message[0].(float64) == msgSubscribeError {
					t.Fatal(message[2])
				}
			}
		case <-timer.C:
			return nil, errors.New("Time is gone")
		}
	}

	return nil, nil
}

func connectAndHeartbeat(t *testing.T, path, uri string, args ...interface{}) {
	ws, err := websocket.Dial(url+path, "", origin)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	currentHB := 0
	ch := make(chan string)

	go func() {
		var msg string
		err := websocket.Message.Receive(ws, &msg)
		if err != nil {
			t.Fatal("Message receive failed", err)
		}
		ch <- msg
	}()

	go func() {
		err := websocket.Message.Send(ws, `["HB",`+strconv.Itoa(currentHB)+`]`)
		if err != nil {
			t.Fatal("Message send failed", err)
		}
		currentHB++
	}()

	timer := time.NewTimer(time.Second * 2)
	for {
		select {
		case msg := <-ch:
			var message []interface{}
			err = json.Unmarshal([]byte(msg), &message)
			if err != nil {
				t.Fatal("Can't unmarshal message")
			}
			if message[0].(float64) == msgHeartbeat {
				if int(message[1].(float64)) != currentHB {
					t.Fatal("Heartbeat is not matched", message[1], "!=", currentHB)
				}
			}
		case <-timer.C:
			println("Current heartbeat", currentHB)
			if currentHB != 6 {
				t.Fatal("Heartbeat is not working")
			}
			return
		}
	}
}

func createWampServer(path string) *WS {
	wampServer := New()
	http.Handle(path, websocket.Handler(func(ws *websocket.Conn) {
		wampServer.WampHandler(ws, nil)
	}))
	return wampServer
}

func init() {
	go func() {
		err := http.ListenAndServe(":1234", nil)
		if err != nil {
			panic("ListenAndServe: " + err.Error())
		}
	}()
}
