package clienttest

import (
	"testing"
	"time"
)

func TestMalformedMidSession(t *testing.T) {
	addr := SetupTestServer(t)

	client, err := NewRTRClient(addr, 1*time.Second)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	// 1. Successfully negotiate and complete a Reset Query
	if err := client.Send(BuildResetQuery(1)); err != nil {
		t.Fatalf("Send Reset Query failed: %v", err)
	}

	// Read Cache Response
	pdu, err := ReadNextPDU(client.conn)
	if err != nil {
		t.Fatalf("Read Cache Response failed: %v", err)
	}
	if pdu.Type != CacheResponse {
		t.Fatalf("Expected Cache Response, got type %d", pdu.Type)
	}

	// Read everything until EOD
	_, _, err = client.CollectPrefixes()
	if err != nil {
		t.Fatalf("Initial Reset Query failed: %v", err)
	}

	// 2. Send a garbage PDU
	if err := client.Send(BuildMalformedPDU()); err != nil {
		t.Fatalf("Send malformed PDU failed: %v", err)
	}

	// 3. Server should respond with Error Report
	pdu, err = ReadNextPDU(client.conn)
	if err != nil {
		t.Fatalf("Read response failed: %v", err)
	}
	if pdu.Type != ErrorReport {
		t.Errorf("Expected Error Report PDU, got type %d", pdu.Type)
	}

	// 4. Server should close the connection
	// Reading again should return EOF or error
	_, err = ReadNextPDU(client.conn)
	if err == nil {
		t.Errorf("Expected connection to be closed, but it's still open")
	}
}
