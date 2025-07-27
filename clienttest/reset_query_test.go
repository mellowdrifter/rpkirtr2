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

	err = client.Send(BuildResetQuery(1))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	resp, err := client.Receive(4096)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	t.Logf("Received %d bytes: %x", len(resp), resp)
}
