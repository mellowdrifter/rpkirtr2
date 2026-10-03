package server

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/mellowdrifter/rpkirtr2/internal/protocol"
	"go.uber.org/zap"
)

func TestClientHandleSerialQuery(t *testing.T) {
	// Setup mock server components
	logger := zap.NewNop().Sugar()
	c := newCache()
	c.session = 1234
	c.serial = 10

	// Create a pipe to simulate network connection
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	client := NewClient(serverConn, logger, c)
	client.version = 1

	// Setup some diffs in the cache
	client.cache.mu.Lock()
	r1 := ROA{ASN: 300, MaxMask: 24, Prefix: netip.MustParsePrefix("1.1.1.0/24")}
	r2 := ROA{ASN: 301, MaxMask: 24, Prefix: netip.MustParsePrefix("2.2.2.0/24")}
	client.cache.updateDiffs([]ROA{r1, r2}, []ROA{r1, r2}, nil, nil, nil, nil)
	client.cache.serial = 11
	client.cache.mu.Unlock()

	// In a goroutine, send a Serial Query from the "router"
	go func() {
		sq := protocol.NewSerialQueryPDU(1, 1234, 10)
		sq.Write(clientConn)
	}()

	// Run handleSerialQuery
	go func() {
		pdu, _ := protocol.GetPDU(bufio.NewReader(serverConn))
		client.dispatchPDU(pdu)
	}()

	// Read responses from clientConn
	respReader := bufio.NewReader(clientConn)

	// 1. Should receive Cache Response
	pdu, err := protocol.GetPDU(respReader)
	if err != nil {
		t.Fatalf("Failed to read Cache Response: %v", err)
	}
	if pdu.Type() != protocol.CacheResponse {
		t.Errorf("Expected Cache Response, got %v", pdu.Type())
	}

	// 2. Should receive Prefix PDUs (2 additions)
	for i := 0; i < 2; i++ {
		pdu, err = protocol.GetPDU(respReader)
		if err != nil {
			t.Fatalf("Failed to read Prefix PDU %d: %v", i, err)
		}
		if pdu.Type() != protocol.Ipv4Prefix {
			t.Errorf("Expected Ipv4Prefix, got %v", pdu.Type())
		}
	}

	// 3. Should receive End of Data
	pdu, err = protocol.GetPDU(respReader)
	if err != nil {
		t.Fatalf("Failed to read End of Data: %v", err)
	}
	if pdu.Type() != protocol.EndOfData {
		t.Errorf("Expected End of Data, got %v", pdu.Type())
	}
}

func TestClientHandleResetQuery(t *testing.T) {
	logger := zap.NewNop().Sugar()
	cache := newCache()
	cache.session = 5678
	cache.serial = 100

	r1 := ROA{ASN: 300, MaxMask: 24, Prefix: netip.MustParsePrefix("1.1.1.0/24")}
	cache.replaceRoas([]ROA{r1})

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	client := NewClient(serverConn, logger, cache)
	client.version = 1

	go func() {
		rq := protocol.NewResetQueryPDU(1)
		rq.Write(clientConn)
	}()

	go func() {
		pdu, _ := protocol.GetPDU(bufio.NewReader(serverConn))
		client.dispatchPDU(pdu)
	}()

	respReader := bufio.NewReader(clientConn)

	pdu, _ := protocol.GetPDU(respReader)
	if pdu.Type() != protocol.CacheResponse {
		t.Errorf("Expected Cache Response, got %v", pdu.Type())
	}

	pdu, _ = protocol.GetPDU(respReader)
	if pdu.Type() != protocol.Ipv4Prefix {
		t.Errorf("Expected Ipv4Prefix, got %v", pdu.Type())
	}

	pdu, _ = protocol.GetPDU(respReader)
	if pdu.Type() != protocol.EndOfData {
		t.Errorf("Expected End of Data, got %v", pdu.Type())
	}
}

func TestSendAndCloseError(t *testing.T) {
	logger := zap.NewNop().Sugar()
	serverConn, clientConn := net.Pipe()

	client := NewClient(serverConn, logger, nil)
	client.version = 1

	go client.sendAndCloseError("test error", protocol.InternalError)

	respReader := bufio.NewReader(clientConn)
	pdu, err := protocol.GetPDU(respReader)
	if err != nil {
		t.Fatalf("Failed to read Error Report: %v", err)
	}
	if pdu.Type() != protocol.ErrorReport {
		t.Errorf("Expected Error Report, got %v", pdu.Type())
	}

	// Connection should be closed
	_, err = respReader.ReadByte()
	if err == nil {
		t.Error("Expected connection to be closed")
	}
}

func TestNotify(t *testing.T) {
	logger := zap.NewNop().Sugar()
	c := newCache()
	c.session = 1111
	c.serial = 2222

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	client := NewClient(serverConn, logger, c)
	client.version = 1

	go client.notify()

	respReader := bufio.NewReader(clientConn)
	pdu, err := protocol.GetPDU(respReader)
	if err != nil {
		t.Fatalf("Failed to read Serial Notify: %v", err)
	}
	if pdu.Type() != protocol.SerialNotify {
		t.Errorf("Expected Serial Notify, got %v", pdu.Type())
	}

	sn := pdu.(*protocol.SerialNotifyPDU)
	if sn.Serial() != 2222 {
		t.Errorf("Expected serial 2222, got %d", sn.Serial())
	}
}

func TestIsDisconnectError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "EOF",
			err:      io.EOF,
			expected: true,
		},
		{
			name:     "ErrUnexpectedEOF",
			err:      io.ErrUnexpectedEOF,
			expected: true,
		},
		{
			name:     "net.ErrClosed",
			err:      net.ErrClosed,
			expected: true,
		},
		{
			name: "net.OpError with timeout",
			err: &net.OpError{
				Op:  "read",
				Net: "tcp",
				Err: os.ErrDeadlineExceeded,
			},
			expected: false,
		},
		{
			name: "net.OpError without timeout (ECONNRESET)",
			err: &net.OpError{
				Op:  "read",
				Net: "tcp",
				Err: syscall.ECONNRESET,
			},
			expected: true,
		},
		{
			name:     "arbitrary error",
			err:      errors.New("some application error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDisconnectError(tt.err)
			if got != tt.expected {
				t.Errorf("isDisconnectError(%v) = %v; want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestClientIdleTimeout(t *testing.T) {
	logger := zap.NewNop().Sugar()
	c := newCache()
	c.session = 100
	c.serial = 1

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	client := NewClient(serverConn, logger, c)
	// Override idleTimeout to a short duration for test
	client.intervals.idleTimeout = 100 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- client.Handle()
	}()

	// Send Initial Reset Query (version is peeked from this PDU)
	rq := protocol.NewResetQueryPDU(1)
	if err := rq.Write(clientConn); err != nil {
		t.Fatalf("Failed to write Reset Query: %v", err)
	}

	// Read responses until End of Data
	respReader := bufio.NewReader(clientConn)
	for {
		pdu, err := protocol.GetPDU(respReader)
		if err != nil {
			t.Fatalf("Failed to read PDU: %v", err)
		}
		if pdu.Type() == protocol.EndOfData {
			break
		}
	}

	// 3. Client sits idle past the 100ms idleTimeout
	select {
	case err := <-done:
		// Should exit with timeout error
		var netErr net.Error
		if !errors.As(err, &netErr) || !netErr.Timeout() {
			t.Errorf("Expected timeout error after idleTimeout, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Handle() did not exit after idle timeout")
	}
}
