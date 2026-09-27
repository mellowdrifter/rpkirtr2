package clienttest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGracefulShutdownWithClients(t *testing.T) {
	// 1. Setup a mock ROA server with a HUGE response to ensure long streaming
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"roas": [`)
		for i := 0; i < 200000; i++ {
			if i > 0 {
				fmt.Fprintf(w, ",")
			}
			fmt.Fprintf(w, `{"prefix": "10.%d.%d.%d/32", "maxLength": 32, "asn": 1}`, (i>>16)&0xFF, (i>>8)&0xFF, i&0xFF)
		}
		fmt.Fprintf(w, `]}`)
	}))
	defer ts.Close()

	addr, srv := SetupTestServerWithURLs(t, []string{ts.URL})

	// 2. Connect multiple clients and start Reset Queries
	numClients := 10
	clients := make([]*RTRClient, numClients)
	for i := 0; i < numClients; i++ {
		c, err := NewRTRClient(addr, 1*time.Second)
		if err != nil {
			t.Fatalf("Connect failed at %d: %v", i, err)
		}
		clients[i] = c
		if err := c.Send(BuildResetQuery(1)); err != nil {
			t.Fatalf("Send failed at %d: %v", i, err)
		}
	}

	// Wait a bit to ensure they are mid-stream
	time.Sleep(50 * time.Millisecond)

	// 3. Trigger shutdown
	shutdownErr := make(chan error, 1)
	go func() {
		srv.Stop(500 * time.Millisecond)
		shutdownErr <- nil
	}()

	// 4. Verify clean exit
	select {
	case <-shutdownErr:
		// Success
	case <-time.After(2 * time.Second):
		t.Errorf("Server.Stop() timed out or hung")
	}

	// Clean up clients
	for _, c := range clients {
		c.Close()
	}
}

func TestGracefulShutdownWithIdleClients(t *testing.T) {
	addr, srv := SetupTestServerWithURLs(t, nil)

	// Connect clients that complete handshake and initial query, then go idle waiting
	numClients := 5
	clients := make([]*RTRClient, numClients)
	for i := 0; i < numClients; i++ {
		c, err := NewRTRClient(addr, 1*time.Second)
		if err != nil {
			t.Fatalf("Connect failed at %d: %v", i, err)
		}
		clients[i] = c
		// Send reset query to complete handshake and get initial data
		if err := c.Send(BuildResetQuery(1)); err != nil {
			t.Fatalf("Send failed at %d: %v", i, err)
		}
		// Collect prefixes until End of Data, leaving the client idle in the read loop
		if _, _, err := c.CollectPrefixes(); err != nil {
			t.Fatalf("CollectPrefixes failed at %d: %v", i, err)
		}
	}

	// Trigger shutdown - should complete in well under 1 second without timing out
	shutdownDone := make(chan error, 1)
	start := time.Now()
	go func() {
		shutdownDone <- srv.Stop(1 * time.Second)
	}()

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Errorf("srv.Stop() returned error: %v", err)
		}
		duration := time.Since(start)
		if duration > 1*time.Second {
			t.Errorf("srv.Stop() took too long: %v (expected < 1s)", duration)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("Server.Stop() hung with idle clients")
	}

	for _, c := range clients {
		c.Close()
	}
}
