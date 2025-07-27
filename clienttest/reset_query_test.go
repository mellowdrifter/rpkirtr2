package clienttest

import (
	"testing"
	"time"
)

func TestResetQuery(t *testing.T) {
	client, err := NewRTRClient("localhost:8282", 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	// Send Reset Query
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
}
