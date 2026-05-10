package clienttest

import (
	"testing"
	"time"
)

func TestIntegrationBasic(t *testing.T) {
	addr := SetupTestServer(t)
	
	client, err := NewRTRClient(addr, 1*time.Second)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	// Just check if we can connect and receive anything
	err = client.Send(BuildResetQuery(1))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
}
