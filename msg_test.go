package wango

import (
	"strconv"
	"testing"
)

func TestCreateMessage(t *testing.T) {
	msg, err := createMessage(1, "com", "wango", 2)
	if err != nil {
		t.Fatal(err)
	}
	expected := `[1,"com","wango",2]`
	if string(msg) != expected {
		t.Fatal("Invalid createMessage", string(msg), "!=", expected)
	}
}

func TestCreateHeartbeatEvent(t *testing.T) {
	msg, err := createHeartbeatEvent(1)
	if err != nil {
		t.Fatal(err)
	}
	expected := `[` + strconv.Itoa(msgHeartbeat) + `,1]`
	if string(msg) != expected {
		t.Fatal("Invalid createHeartbeatEvent", string(msg), "!=", expected)
	}
}

func TestCreateHeartbeatTxtEvent(t *testing.T) {
	msg, err := createHeartbeatTxtEvent(1)
	if err != nil {
		t.Fatal(err)
	}
	expected := `["HB",1]`
	if string(msg) != expected {
		t.Fatal("Invalid createHeartbeatEvent", string(msg), "!=", expected)
	}
}

func TestCreateWelcomeMessage(t *testing.T) {
	msg, err := createWelcomeMessage("1")
	if err != nil {
		t.Fatal(err)
	}
	expected := `["` + msgIntTypes[msgWelcome] + `","1",1,"` + identity + `"]`
	if string(msg) != expected {
		t.Fatal("Invalid createWelcomeMessage", string(msg), "!=", expected)
	}
}
