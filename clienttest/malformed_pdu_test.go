package clienttest

import (
	"testing"
	"time"
)

func TestMalformedPDU(t *testing.T) {
	client, err := NewRTRClient("localhost:8282", 2*time.Second)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	err = client.Send(BuildMalformedPDU())
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	_, err = client.Receive(4096)
	if err != nil {
		t.Logf("Expected failure or close: %v", err)
	} else {
		t.Errorf("Malformed PDU did not cause an error")
	}
}
