package wango

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
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
	server.RegisterRPCHandler(uri, testRPCHandler)
	if len(server.rpcHandlers) != 1 {
		t.Fatal("No handlers registered")
	}
	res, err := connectAndRPC(path, uri, nil)
	if err != nil {
		t.Fatal("RPC failed")
	}
	if res.(string) != "test-"+uri {
		t.Fatal("invalid RPC befaviour")
	}

	uri = "net.wango.rgx"
	server.RegisterRPCHandler(regexp.MustCompile(`^net\.wango\..*`), testRPCHandler)
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

func testRPCHandler(connID string, uri string, args ...interface{}) (interface{}, error) {
	return "test-" + uri, nil
}

func testRPCHandlerWithErrorReturn(connID string, uri string, args ...interface{}) (interface{}, error) {
	return nil, errors.New("RPC error")
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
