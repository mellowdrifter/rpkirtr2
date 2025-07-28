package clienttest

import (
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestStressResetQueries(t *testing.T) {
	client, err := NewRTRClient("localhost:8282", 2*time.Second)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer client.Close()

	for i := range 10 {
		err := client.Send(BuildResetQuery(2))
		if err != nil {
			t.Fatalf("Send failed at %d: %v", i, err)
		}
		if i%10_000 == 0 {
			t.Logf("Sent %d queries", i)
		}
	}
}

func TestStressNewClients(t *testing.T) {
	var clients []RTRClient
	defer func() {
		for _, client := range clients {
			client.Close()
		}
	}()

	// Create 100 clients
	for i := range 100 {
		client, err := NewRTRClient("localhost:8282", 2*time.Second)
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
