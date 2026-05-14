package clienttest

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestStressNewClients(t *testing.T) {
	addr := SetupTestServer(t)
	var clients []RTRClient
	defer func() {
		for _, client := range clients {
			client.Close()
		}
	}()

	// Create 100 clients
	for i := range 100 {
		client, err := NewRTRClient(addr, 2*time.Second)
		if err != nil {
			t.Fatalf("Connect failed at %d: %v", i, err)
		}
		clients = append(clients, *client)
	}

	// Create 10 goroutines, each handling 10 clients
	var wg sync.WaitGroup
	numGoroutines := 10
	clientsPerGoroutine := 10

	// Use a channel to synchronize the start of all goroutines
	startSignal := make(chan struct{})

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			// Wait for start signal to synchronize all goroutines
			<-startSignal

			// Calculate which clients this goroutine handles
			startIdx := goroutineID * clientsPerGoroutine
			endIdx := startIdx + clientsPerGoroutine

			t.Logf("Goroutine %d handling clients %d-%d", goroutineID, startIdx, endIdx-1)

			// Process clients assigned to this goroutine
			for i := startIdx; i < endIdx; i++ {
				client := clients[i]

				// Each client gets its own state variables
				var seenCacheResponse bool
				var seenEndOfData bool
				var prefixCount int

				err := client.Send(BuildResetQuery(rand.Intn(2) + 1))
				if err != nil {
					t.Errorf("Goroutine %d, Client %d: Send failed: %v", goroutineID, i, err)
					continue
				}

				for {
					pdu, err := ReadNextPDU(client.conn)
					if err != nil {
						t.Errorf("Goroutine %d, Client %d: Failed to read PDU: %v", goroutineID, i, err)
						break
					}

					switch pdu.Type {
					case 3: // Cache Response
						if seenCacheResponse {
							t.Errorf("Goroutine %d, Client %d: Received multiple Cache Response PDUs", goroutineID, i)
						}
						seenCacheResponse = true
						t.Logf("Goroutine %d, Client %d: ✅ Received Cache Response PDU", goroutineID, i)

					case 4, 6: // IPv4 or IPv6 Prefix
						prefixCount++

					case 7: // End of Data
						if seenEndOfData {
							t.Errorf("Goroutine %d, Client %d: Received multiple End of Data PDUs", goroutineID, i)
						}
						seenEndOfData = true

						t.Logf("Goroutine %d, Client %d: ✅ Received End of Data PDU after %d prefix PDUs", goroutineID, i, prefixCount)
						eod, err := parseEndOfData(pdu)
						if err != nil {
							t.Errorf("Goroutine %d, Client %d: Failed to parse End of Data: %v", goroutineID, i, err)
						} else {
							t.Logf("Goroutine %d, Client %d: ✅ End of Data: Session ID: %d, Serial Number: %d, Refresh: %d, Retry: %d, Expire: %d",
								goroutineID, i, pdu.SessionID, eod.SerialNumber, eod.RefreshInterval, eod.RetryInterval, eod.ExpireInterval)
						}

					default:
						t.Errorf("Goroutine %d, Client %d: ❌ Unexpected PDU type received: %d", goroutineID, i, pdu.Type)
					}

					if seenEndOfData {
						break
					}
				}
			}

			t.Logf("Goroutine %d completed all clients", goroutineID)
		}(g)
	}

	// Signal all goroutines to start roughly at the same time
	close(startSignal)

	// Wait for all goroutines to complete
	wg.Wait()

	t.Log("All goroutines completed")
}

func TestConcurrentUpdateAndResetQuery(t *testing.T) {
	// 1. Setup a mock ROA server with a large-ish response to prolong streaming
	var mu sync.Mutex
	asn := 1
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		currentASN := asn
		mu.Unlock()

		// Send 1000 ROAs
		fmt.Fprintf(w, `{"roas": [`)
		for i := 0; i < 1000; i++ {
			if i > 0 {
				fmt.Fprintf(w, ",")
			}
			fmt.Fprintf(w, `{"prefix": "10.0.%d.%d/32", "maxLength": 32, "asn": %d}`, i/256, i%256, currentASN)
		}
		fmt.Fprintf(w, `]}`)
	}))
	defer ts.Close()

	addr, srv := SetupTestServerWithURLs(t, []string{ts.URL})

	// 2. Start a client and send Reset Query
	client, err := NewRTRClient(addr, 1*time.Second)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	if err := client.Send(BuildResetQuery(1)); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// 3. In parallel, trigger multiple cache updates
	done := make(chan bool)
	go func() {
		for i := 0; i < 5; i++ {
			mu.Lock()
			asn++
			mu.Unlock()
			if err := srv.TriggerRefresh(context.Background()); err != nil {
				t.Errorf("TriggerRefresh failed: %v", err)
			}
			time.Sleep(5 * time.Millisecond)
		}
		done <- true
	}()

	// 4. Collect data from the client
	prefixes, _, err := client.CollectPrefixes()
	if err != nil {
		t.Errorf("CollectPrefixes failed: %v", err)
	}
	if len(prefixes) != 1000 {
		t.Errorf("Expected 1000 prefixes, got %d", len(prefixes))
	}

	<-done
}
