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
var url = "ws://localhost:1243"

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
		t.Fatalf("Invalid connections number %d when connecting, expected: %d", totalConnections, numberConnections)
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
	_, err = connectAndRPC(path, uri, nil)
	if err == nil {
		t.Fatal("RPC failed. No error returns")
	}
}

func TestSubHandling(t *testing.T) {
	path := "/wamp-sub"
	server := createWampServer(path)

	uri := "wango.sub-test"
	err := server.RegisterSubHandler(uri, testSubHandler, nil, nil)
	if err != nil {
		t.Fatal("Can't register handler", err)
	}
	if len(server.subHandlers) != 1 {
		t.Fatal("subHandler not registered")
	}

	err = server.RegisterSubHandler(uri, testSubHandler, nil, nil)
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
	err = server.RegisterSubHandler(uri, testSubHandler, nil, func(_uri string, event interface{}, extra interface{}) (bool, interface{}) {
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

func TestClientConnectingAndRPC(t *testing.T) {
	path := "/wamp-client-connecting"
	server := createWampServer(path)
	server.RegisterRPCHandler("wango.test", testRPCHandlerForClient)
	server.RegisterRPCHandler("wango.test-error", testRPCHandlerWithErrorReturn)
	client, err := Connect(url+path, origin)
	if err != nil {
		t.Fatal("Can't connect to server", err.Error())
	}
	res, err := client.Call("wango.test", "call")
	if err != nil {
		t.Fatal("Can't call")
	}
	expected := "test-call"
	if res == nil {
		t.Fatal("Res is nil")
	}
	if res.(string) != expected {
		t.Fatal("RPC failed", res, "!=", expected)
	}

	_, err = client.Call("wango.test-error")
	if err == nil {
		t.Fatal("Can't call")
	}
}

func TestClientConnectingToInvalidURI(t *testing.T) {
	_, err := Connect("invalid-uri", origin)
	if err == nil {
		t.Fatal("Must not connect to server", err.Error())
	}
}

func TestClientConnectingAndPubSub(t *testing.T) {
	path := "/wamp-client-sub"
	subUri := "wango.sub"
	server := createWampServer(path)
	testRPCHandlerWithPub := func(c *Conn, uri string, args ...interface{}) (interface{}, error) {
		pubUri := args[0].(string)
		pubData := args[1]
		server.Publish(pubUri, pubData)
		return nil, nil
	}
	server.RegisterRPCHandler("wango.test", testRPCHandlerWithPub)
	server.RegisterSubHandler(subUri, func(c *Conn, uri string, args ...interface{}) (interface{}, error) {
		return nil, nil
	}, nil, nil)

	url := "localhost:1243"
	client, err := Connect(url+path, origin)
	if err != nil {
		t.Fatal("Can't connect to server", err.Error())
	}

	resChan := make(chan bool)
	var counter int
	err = client.Subscribe(subUri, func(uri string, event interface{}) {
		if uri != subUri {
			t.Fatal("Invalid URI")
		}
		if event != nil && event.(string) != "test" {
			t.Fatal("Invalid event")
		}
		if counter > 1 {
			resChan <- true
		}
		if event != nil {
			counter++
		}
	})
	if err != nil {
		t.Fatal("Can't subscribe", err)
	}

	err = client.Subscribe("invalid."+subUri, func(uri string, event interface{}) {})
	if err == nil {
		t.Fatal("Subscribe to invalid uri must returns error", err)
	}

	err = client.Subscribe(subUri, func(uri string, event interface{}) {}, "12312312")
	if err == nil {
		t.Fatal("Subscribe to invalid connID must returns error", err)
	}

	err = client.Subscribe("", func(uri string, event interface{}) {})
	if err == nil {
		t.Fatal("Subscribe to empty uri must returns error", err)
	}

	_, err = client.Call("wango.test", subUri, "test")
	if err != nil {
		t.Fatal("Can't call")
	}

	_, err = client.Call("wango.test", subUri, "test")
	if err != nil {
		t.Fatal("Can't call")
	}

	server.Publish(subUri, "test")

	time.Sleep(time.Millisecond)

	err = client.Unsubscribe(subUri)
	if err != nil {
		t.Fatal("Can't unsubscribe")
	}

	err = client.Unsubscribe(subUri)
	if err == nil {
		t.Fatal("Not subscriber URI must not unsubscribe", err.Error())
	}

	err = client.Unsubscribe("")
	if err == nil {
		t.Fatal("Empty URI must not unsubscribe", err.Error())
	}

	err = client.Unsubscribe("", "12312")
	if err == nil {
		t.Fatal("Invalid connID must not unsubscribe", err.Error())
	}

	timer := time.NewTimer(time.Millisecond * 200)
	select {
	case <-timer.C:
		t.Fatal("Time is gone")
	case <-resChan:
	}
}

func TestSessionOpenSessionCloseHandlers(t *testing.T) {
	path := "/wamp-client-handlers"
	server := createWampServer(path)
	var totalClients int
	sessionOpenCallback := func(c *Conn) {
		totalClients++
	}
	sessionCloseCallback := func(c *Conn) {
		totalClients--
	}
	server.SetSessionOpenCallback(sessionOpenCallback)
	server.SetSessionCloseCallback(sessionCloseCallback)

	url := "localhost:1243"
	maxConnects := 20
	clients := make([]*Wango, maxConnects)
	for i := 0; i < maxConnects; i++ {
		client, err := Connect(url+path, origin)
		if err != nil {
			t.Fatal("Can't connect to server", err.Error())
		}

		if totalClients != i+1 {
			t.Fatal("Open handler is not working")
		}
		clients[i] = client
	}
	for i := 0; i < maxConnects; i++ {
		client := clients[i]
		client.Disconnect()
		time.Sleep(time.Millisecond)
		if totalClients != maxConnects-i-1 {
			t.Fatal("Close handler is not working", totalClients)
		}
	}
}

func TestConnectionStatus(t *testing.T) {
	path := "/wamp-client-status"
	server := createWampServer(path)
	client, err := Connect(url+path, origin)
	if err != nil {
		t.Fatal("Can't connect to server", err.Error())
	}
	server.RegisterRPCHandler("test", func(c *Conn, uri string, args ...interface{}) (interface{}, error) {
		if !c.Connected() {
			t.Fatal("Should be connected")
		}
		time.Sleep(time.Millisecond * 100)
		if c.Connected() {
			t.Fatal("Should not be connected")
		}
		return nil, nil
	})
	go client.Call("test")
	time.Sleep(time.Millisecond * 50)
	client.Disconnect()
}

func TestClientOpenCloseCallbacks(t *testing.T) {
	path := "/wamp-client-callbacks"
	createWampServer(path)
	client, err := Connect(url+path, origin)
	if err != nil {
		t.Fatal("Can't connect to server", err.Error())
	}
	var closeCallbackCalled bool
	client.SetSessionCloseCallback(func(c *Conn) {
		closeCallbackCalled = true
	})
	if client.closeCB == nil {
		t.Fatal("Close callback was not set")
	}
	client.Disconnect()
	time.Sleep(time.Millisecond * 50)
	if !closeCallbackCalled {
		t.Fatal("Close callback was not called")
	}

}

func testRPCHandlerForClient(c *Conn, uri string, args ...interface{}) (interface{}, error) {
	res := "test-"
	if len(args) > 0 {
		if _arg, ok := args[0].(string); ok {
			res += _arg
		}
	}
	return res, nil
}

func testRPCHandler(c *Conn, uri string, args ...interface{}) (interface{}, error) {
	return "test-" + uri, nil
}

func testRPCHandlerWithErrorReturn(c *Conn, uri string, args ...interface{}) (interface{}, error) {
	return nil, errors.New("RPC error")
}

func testSubHandler(c *Conn, uri string, args ...interface{}) (interface{}, error) {
	if strings.Contains(uri, "error") {
		return nil, errors.New("403")
	}
	return nil, nil
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
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()
	msgId := newUUIDv4()
	message, _ := createMessage(msgCall, msgId, uri)
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
		if message[0].(string) == msgIntTypes[msgCallResult] && message[1].(string) == msgId {
			return message[2], nil
		}
		if message[0].(string) == msgIntTypes[msgCallError] && message[1].(string) == msgId {
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
		for {
			var msg string
			err := websocket.Message.Receive(ws, &msg)
			if err != nil {
				break
			}
			ch <- msg
		}
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
			if message[0].(string) == msgIntTypes[msgWelcome] {
				continue
			}
			if message[1].(string) == uri {
				if message[0].(string) == msgIntTypes[msgSubscribed] {
					return
				}
				if message[0].(string) == msgIntTypes[msgSubscribeError] {
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
				if message[0].(string) == msgIntTypes[msgEvent] {
					return message[2], nil
				}
				if message[0].(string) == msgIntTypes[msgSubscribeError] {
					t.Fatal(message[2])
				}
			}
		case <-timer.C:
			return nil, errors.New("Time is gone")
		}
	}
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

func createWampServer(path string) *Wango {
	wampServer := New()
	http.Handle(path, websocket.Handler(func(ws *websocket.Conn) {
		wampServer.WampHandler(ws, nil)
	}))
	return wampServer
}

func init() {
	go func() {
		err := http.ListenAndServe(":1243", nil)
		if err != nil {
			panic("ListenAndServe: " + err.Error())
		}
	}()
}
