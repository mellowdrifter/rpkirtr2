package clienttest

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestVersionMismatch(t *testing.T) {
	client, err := NewRTRClient("localhost:8282", 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Send Reset Query with version 2
	err = client.Send(BuildResetQuery(2))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	var (
		seenCacheResponse bool
		seenEndOfData     bool
		prefixCount       int
	)

	for {
		pdu, err := ReadNextPDU(client.conn)
		if err != nil {
			t.Fatalf("Failed to read PDU: %v", err)
		}

		switch pdu.Type {
		case 3: // Cache Response
			if seenCacheResponse {
				t.Errorf("Received multiple Cache Response PDUs")
			}
			seenCacheResponse = true
			t.Log("✅ Received Cache Response PDU")

		case 4, 6: // IPv4 or IPv6 Prefix
			prefixCount++

		case 7: // End of Data
			if seenEndOfData {
				t.Errorf("Received multiple End of Data PDUs")
			}
			seenEndOfData = true

			t.Logf("✅ Received End of Data PDU after %d prefix PDUs", prefixCount)
			eod, err := parseEndOfData(pdu)
			if err != nil {
				t.Errorf("Failed to parse End of Data: %v", err)
			} else {
				t.Logf("✅ End of Data: Session ID: %d, Serial Number: %d, Refresh: %d, Retry: %d, Expire: %d",
					pdu.SessionID, eod.SerialNumber, eod.RefreshInterval, eod.RetryInterval, eod.ExpireInterval)
			}

		default:
			t.Errorf("❌ Unexpected PDU type received: %d", pdu.Type)
		}

		if seenEndOfData {
			break
		}
	}

	if !seenCacheResponse {
		t.Error("❌ Did not receive Cache Response PDU")
	}
	if !seenEndOfData {
		t.Error("❌ Did not receive End of Data PDU")
	}
	if prefixCount == 0 {
		t.Error("❌ No prefix PDUs received")
	}

	// Now send Reset Query with version 1
	err = client.Send(BuildResetQuery(1))
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

	// Error code should be 8 (unexpected version)
	errorCode := binary.BigEndian.Uint16(resp[2:4])
	if errorCode != 8 {
		t.Errorf("Expected error code 8 (unexpected version), got: %d", errorCode)
	}

	// Confirm connection was closed
	_, err = client.Receive(4096)
	if err == nil {
		t.Errorf("Expected connection to close after error, but read succeeded")
	}
}
