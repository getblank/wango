package wango

import "testing"

var validMessage = `["CALL","0f2ac8b4-0d18-c6a0-acf1-a338f53a7f2e","com.state"]`
var invalidRPCMessage = `[2,"0f2ac8b4-0d18-c6a0-acf1-a338f53a7f2e"]`
var invalidMessage = `["CALL","0f2ac8b4-0d18-c6a0-acf1-a338f53a7f2e","com.state"`

func TestParseValidMessage(t *testing.T) {
	callType, msg, err := parseMessage(validMessage)
	if err != nil {
		t.Fatal("Invalid validMessage", err)
	}
	if callType != 2 {
		t.Fatal("Invalid callType", callType)
	}
	if len(msg) != 3 {
		t.Fatal("Invalid count element msg", msg)
	}
}

func TestParseinvalidRPCMessage(t *testing.T) {
	_, msg, err := parseMessage(invalidRPCMessage)
	if err != nil {
		t.Fatal("Invalid invalidMessage", err)
	}
	_, err = parseCallMessage(msg)

	if err == nil {
		t.Fatal("RPC args length error not handled", err)
	}
}

func TestParseInvalidMessage(t *testing.T) {
	_, _, err := parseMessage(invalidMessage)
	if err == nil {
		t.Fatal("Invalid invalidMessage", err)
	}
}
