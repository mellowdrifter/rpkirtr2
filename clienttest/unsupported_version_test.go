package clienttest

import (
	"encoding/binary"
	"fmt"
	"slices"
	"testing"
	"time"
)

var supportedVersions = []int{1, 2}

func TestUnsupportedVersionsResetQuery(t *testing.T) {
	for i := 0; i <= 256; i++ {
		if slices.Contains(supportedVersions, i) {
			continue // Skip supported versions
		}
		t.Run(fmt.Sprintf("Testing version %d", i), func(t *testing.T) {
			client, err := NewRTRClient("localhost:8282", 2*time.Second)
			if err != nil {
				t.Fatalf("Connect failed: %v", err)
			}
			defer client.Close()

			err = client.Send(BuildResetQuery(i))
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

			// Error code should be 4 (unsupported version)
			errorCode := binary.BigEndian.Uint16(resp[2:4])
			if errorCode != 4 {
				t.Errorf("Expected error code 4 (unsupported version), got: %d", errorCode)
			}

			// Confirm connection was closed
			_, err = client.Receive(4096)
			if err == nil {
				t.Errorf("Expected connection to close after error, but read succeeded")
			}
		})
	}
}

func TestUnsupportedVersionsSerialQuery(t *testing.T) {
	for i := 0; i <= 256; i++ {
		if slices.Contains(supportedVersions, i) {
			continue // Skip supported versions
		}
		t.Run(fmt.Sprintf("Testing version %d", i), func(t *testing.T) {
			client, err := NewRTRClient("localhost:8282", 2*time.Second)
			if err != nil {
				t.Fatalf("Connect failed: %v", err)
			}
			defer client.Close()

			err = client.Send(BuildSerialQuery(i, 0, 0))
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

			// Error code should be 4 (unsupported version)
			errorCode := binary.BigEndian.Uint16(resp[2:4])
			if errorCode != 4 {
				t.Errorf("Expected error code 4 (unsupported version), got: %d", errorCode)
			}

			// Confirm connection was closed
			_, err = client.Receive(4096)
			if err == nil {
				t.Errorf("Expected connection to close after error, but read succeeded")
			}

		})
	}
}
