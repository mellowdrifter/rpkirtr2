package clienttest

import (
	"encoding/binary"
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
	// Read the full PDU (expecting Error Report)
	resp, err := client.Receive(4096)
	if err != nil {
		t.Fatalf("Failed to read Error Report: %v", err)
	}

	pduType := resp[1]
	if pduType != 10 {
		t.Fatalf("Expected Error Report (type 10), got type: %d", pduType)
	}

	if len(resp) < 16 {
		t.Fatalf("Error Report PDU too short: %x", resp)
	}

	// Error code should be 3 (invalid request)
	errorCode := binary.BigEndian.Uint16(resp[2:4])
	if errorCode != 3 {
		t.Errorf("Expected error code 3 (unsupported version), got: %d", errorCode)
	}

	// Confirm connection was closed
	_, err = client.Receive(4096)
	if err == nil {
		t.Errorf("Expected connection to close after error, but read succeeded")
	}
}
