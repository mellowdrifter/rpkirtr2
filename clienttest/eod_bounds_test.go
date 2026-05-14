package clienttest

import (
	"testing"
	"time"
)

func TestEndOfDataIntervals(t *testing.T) {
	addr := SetupTestServer(t)

	client, err := NewRTRClient(addr, 1*time.Second)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	if err := client.Send(BuildResetQuery(1)); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Read until EOD
	for {
		pdu, err := ReadNextPDU(client.conn)
		if err != nil {
			t.Fatalf("ReadNextPDU failed: %v", err)
		}
		if pdu.Type == EndOfDataType {
			eod, err := parseEndOfData(pdu)
			if err != nil {
				t.Fatalf("parseEndOfData failed: %v", err)
			}

			// Validate RFC 8210 bounds
			// Refresh: 1 to 86400 seconds
			if eod.RefreshInterval < 1 || eod.RefreshInterval > 86400 {
				t.Errorf("RefreshInterval %d out of RFC 8210 bounds (1-86400)", eod.RefreshInterval)
			}
			// Retry: 1 to 7200 seconds
			if eod.RetryInterval < 1 || eod.RetryInterval > 7200 {
				t.Errorf("RetryInterval %d out of RFC 8210 bounds (1-7200)", eod.RetryInterval)
			}
			// Expire: 600 to 172800 seconds
			if eod.ExpireInterval < 600 || eod.ExpireInterval > 172800 {
				t.Errorf("ExpireInterval %d out of RFC 8210 bounds (600-172800)", eod.ExpireInterval)
			}
			break
		}
	}
}
