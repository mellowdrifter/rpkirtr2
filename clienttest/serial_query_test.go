package clienttest

import (
	"testing"
	"time"
)


func TestSerialQueryWrongSession(t *testing.T) {
	addr := SetupTestServer(t)

	client, err := NewRTRClient(addr, 1*time.Second)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	// 1. Send Reset Query to get valid session ID
	if err := client.Send(BuildResetQuery(1)); err != nil {
		t.Fatalf("Send Reset Query failed: %v", err)
	}

	// 2. Read Cache Response to get session ID
	pdu, err := ReadNextPDU(client.conn)
	if err != nil {
		t.Fatalf("Read Cache Response failed: %v", err)
	}
	if pdu.Type != CacheResponse {
		t.Fatalf("Expected Cache Response, got type %d", pdu.Type)
	}
	sessionID := pdu.SessionID

	// 3. Consume remaining PDUs (Prefixes + EOD)
	_, _, err = client.CollectPrefixes()
	if err != nil {
		t.Fatalf("CollectPrefixes failed: %v", err)
	}

	// 4. Send Serial Query with a WRONG session ID
	wrongSession := int(sessionID) + 1
	if err := client.Send(BuildSerialQuery(1, wrongSession, 1)); err != nil {
		t.Fatalf("Send Serial Query failed: %v", err)
	}

	// 5. Expect Cache Reset
	pdu, err = ReadNextPDU(client.conn)
	if err != nil {
		t.Fatalf("Read response failed: %v", err)
	}
	if pdu.Type != CacheReset {
		t.Errorf("Expected Cache Reset PDU for wrong session ID, got type %d", pdu.Type)
	}
}

func TestSerialQueryZero(t *testing.T) {
	addr := SetupTestServer(t)

	client, err := NewRTRClient(addr, 1*time.Second)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	// 1. Send Reset Query to get valid session ID
	if err := client.Send(BuildResetQuery(1)); err != nil {
		t.Fatalf("Send Reset Query failed: %v", err)
	}

	// 2. Read Cache Response to get session ID
	pdu, err := ReadNextPDU(client.conn)
	if err != nil {
		t.Fatalf("Read Cache Response failed: %v", err)
	}
	if pdu.Type != CacheResponse {
		t.Fatalf("Expected Cache Response, got type %d", pdu.Type)
	}
	sessionID := pdu.SessionID

	// 3. Consume remaining PDUs (Prefixes + EOD)
	_, _, err = client.CollectPrefixes()
	if err != nil {
		t.Fatalf("CollectPrefixes failed: %v", err)
	}

	// 4. Send Serial Query with serial 0
	if err := client.Send(BuildSerialQuery(1, int(sessionID), 0)); err != nil {
		t.Fatalf("Send Serial Query failed: %v", err)
	}

	// 5. Expect Cache Reset
	pdu, err = ReadNextPDU(client.conn)
	if err != nil {
		t.Fatalf("Read response failed: %v", err)
	}
	if pdu.Type != CacheReset {
		t.Errorf("Expected Cache Reset PDU for serial 0, got type %d", pdu.Type)
	}
}
