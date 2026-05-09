package server

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestCacheRace(t *testing.T) {
	logger := zap.NewNop().Sugar()
	c := newCache()

	// Initial ROAs
	c.replaceRoas([]ROA{
		{ASN: 100, MaxMask: 24},
		{ASN: 200, MaxMask: 32},
	})

	var wg sync.WaitGroup
	numClients := 50
	stop := make(chan struct{})

	// Start many clients
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			// Use net.Pipe to simulate connection
			serverConn, clientConn := net.Pipe()
			defer serverConn.Close()
			defer clientConn.Close()

			client := &Client{
				conn:    serverConn,
				reader:  bufio.NewReader(serverConn),
				writer:  bufio.NewWriter(serverConn),
				logger:  logger,
				id:      fmt.Sprintf("client-%d", id),
				cache:   c,
				version: 1,
			}

			// Handle client in a goroutine
			go func() {
				// We don't call client.Handle() because it loops forever.
				// Instead we just simulate requests.
				for {
					select {
					case <-stop:
						return
					default:
						// Send a Reset Query
						// In this test, we call the internal methods directly to stress the cache access
						state := client.cache.getState()
						client.sendAllROAS(state.roas, state.session, state.serial)
						time.Sleep(time.Millisecond * 2)
					}
				}
			}()

			// Read from clientConn to prevent blocking
			go func() {
				for {
					buf := make([]byte, 4096)
					_, err := clientConn.Read(buf)
					if err != nil {
						return
					}
				}
			}()

			<-stop
		}(i)
	}

	// Rapidly update cache
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			select {
			case <-stop:
				return
			default:
				c.mu.Lock()
				c.serial++
				c.roas = append(c.roas, ROA{ASN: uint32(i + 1000), MaxMask: 24})
				c.mu.Unlock()
				time.Sleep(time.Millisecond)
			}
		}
	}()

	time.Sleep(500 * time.Millisecond)
	close(stop)
	wg.Wait()
}
