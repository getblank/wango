package wango

import "testing"

var validMessage = `["CALL","0f2ac8b4-0d18-c6a0-acf1-a338f53a7f2e","com.state"]`
var invalidRPCMessage = `[2,"0f2ac8b4-0d18-c6a0-acf1-a338f53a7f2e"]`
var invalidMessage = `["CALL","0f2ac8b4-0d18-c6a0-acf1-a338f53a7f2e","com.state"`
var validSubMessage = `[5,"com.uri", 6]`

func TestParseValidMessage(t *testing.T) {
	callType, msg, err := parseMessage(validMessage)
	if err != nil {
		t.Fatal("Invalid validMessage", err)
	}
	if callType != msgCall {
		t.Fatal("Invalid callType", callType)
	}
	if len(msg) != 3 {
		t.Fatal("Invalid count element msg", msg)
	}
}

func TestParseValidSubMessage(t *testing.T) {
	callType, msg, err := parseMessage(validSubMessage)
	if err != nil {
		t.Fatal("Invalid validMessage", err)
	}
	if callType != msgSubscribe {
		t.Fatal("Invalid callType", callType)
	}
	if len(msg) != 3 {
		t.Fatal("Invalid count element msg", msg)
	}

	res, err := parseWampMessage(msgSubscribe, msg)
	if err != nil {
		t.Fatal("Can't parse sub message", err)
	}
	if res.URI != "com.uri" {
		t.Fatal("Invalid uri parsed", res.URI, "!=", "com.uri")
	}
}

func TestParseInvalidRPCMessage(t *testing.T) {
	_, msg, err := parseMessage(invalidRPCMessage)
	if err != nil {
		t.Fatal("Invalid invalidMessage", err)
	}
	_, err = parseWampMessage(msgCall, msg)

	if err == nil {
		t.Fatal("InvalidMessage parsed without error", err)
	}
}

func TestParseInvalidMessage(t *testing.T) {
	_, _, err := parseMessage(invalidMessage)
	if err == nil {
		t.Fatal("Invalid invalidMessage", err)
	}
}
