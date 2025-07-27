package clienttest

import (
	"testing"
	"time"
)

func TestStressResetQueries(t *testing.T) {
	client, err := NewRTRClient("localhost:8282", 2*time.Second)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	for i := 0; i < 100_000; i++ {
		err := client.Send(BuildResetQuery(1))
		if err != nil {
			t.Fatalf("Send failed at %d: %v", i, err)
		}
		if i%10_000 == 0 {
			t.Logf("Sent %d queries", i)
		}
	}
}
