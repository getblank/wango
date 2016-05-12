package wango

import (
	"log"
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

var origin = "http://localhost/"
var url = "ws://localhost:1234"

func TestAcceptingConcurrentConnections(t *testing.T) {
	path := "/wamp1"
	server := httpServer(path)
	numberConnections := 100
	for i := 0; i < numberConnections; i++ {
		go clientConnect(path)
	}
	time.Sleep(time.Millisecond * 200)

	totalConnections := len(server.connections)
	if totalConnections != numberConnections {
		t.Fatal("Invalid connections number when connecting", totalConnections)
	}
}

func TestClosingConcurrentConnections(t *testing.T) {
	path := "/wamp2"
	server := httpServer(path)
	numberConnections := 100
	for i := 0; i < numberConnections; i++ {
		go clientConnect(path)
	}

	time.Sleep(time.Second * 2)

	totalConnections := len(server.connections)
	if totalConnections != 0 {
		t.Fatal("Invalid connections number when closing", totalConnections)
	}
}

func clientConnect(path string) {
	ws, err := websocket.Dial(url+path, "", origin)
	if err != nil {
		log.Fatal(err)
	}

	message := []byte("ping")
	_, err = ws.Write(message)
	if err != nil {
		log.Fatal(err)
	}

	var msg = make([]byte, 512)
	_, err = ws.Read(msg)
	if err != nil {
		log.Fatal(err)
	}
	// fmt.Printf("Receive: %s\n", msg)
	time.Sleep(time.Second * 1)
	ws.Close()
	// fmt.Printf("Closed")
}

func httpServer(path string) *WS {
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
