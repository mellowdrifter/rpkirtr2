package server

import (
	"bufio"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/mellowdrifter/rpkirtr2/internal/protocol"
	"go.uber.org/zap"
)

func setupTestClient(serverConn net.Conn) *Client {
	logger := zap.NewNop().Sugar()
	c := &cache{
		session: 0xABCD,
		serial:  10,
	}
	return &Client{
		conn:    serverConn,
		reader:  bufio.NewReader(serverConn),
		writer:  bufio.NewWriter(serverConn),
		logger:  logger,
		id:      "test-client",
		cache:   c,
		cfg: cfg{
			readTimeout: 100 * time.Millisecond,
		},
	}
}

func TestHandleSerialQuerySessionValidation(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	client := setupTestClient(serverConn)
	client.version = 1

	// Test case 1: Mismatched session ID
	mismatchedSession := uint16(0x1234)
	sqPDU := protocol.NewSerialQueryPDU(1, mismatchedSession, 5)

	errChan := make(chan error, 1)
	go func() {
		errChan <- client.handleSerialQuery(sqPDU)
	}()

	responsePDU, err := protocol.GetPDU(clientConn)
	if err != nil {
		t.Fatalf("Failed to read response PDU: %v", err)
	}
	if responsePDU.Type() != protocol.CacheReset {
		t.Errorf("Expected CacheReset PDU, got %v", responsePDU.Type())
	}
	if err := <-errChan; err != nil {
		t.Errorf("handleSerialQuery returned error: %v", err)
	}
}

func TestHandleSerialQueryDiffs(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	client := setupTestClient(serverConn)
	client.version = 1

	// Setup some diffs in the cache
	client.cache.mu.Lock()
	client.cache.serial = 11
	client.cache.diffs.addRoa = []roa{
		{
			ASN:     300,
			MaxMask: 24,
			Prefix:  netip.MustParsePrefix("1.1.1.0/24"),
		},
	}
	client.cache.mu.Unlock()

	// Client requests serial 10 (current is 11)
	sqPDU := protocol.NewSerialQueryPDU(1, 0xABCD, 10)

	go func() {
		client.handleSerialQuery(sqPDU)
	}()

	// Should receive: Cache Response, then IPv4 Prefix (Announce), then End of Data
	p1, _ := protocol.GetPDU(clientConn)
	if p1.Type() != protocol.CacheResponse {
		t.Errorf("Expected CacheResponse, got %v", p1.Type())
	}

	p2, _ := protocol.GetPDU(clientConn)
	if p2.Type() != protocol.Ipv4Prefix {
		t.Errorf("Expected Ipv4Prefix, got %v", p2.Type())
	}

	p3, _ := protocol.GetPDU(clientConn)
	if p3.Type() != protocol.EndOfData {
		t.Errorf("Expected EndOfData, got %v", p3.Type())
	}
}


func TestHandleReadTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, _ := net.Dial("tcp", ln.Addr().String())
		time.Sleep(200 * time.Millisecond)
		conn.Close()
	}()

	conn, _ := ln.Accept()
	defer conn.Close()

	client := setupTestClient(conn)
	client.cfg.readTimeout = 50 * time.Millisecond

	err = client.Handle()
	if err == nil || !strings.Contains(err.Error(), "i/o timeout") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

func TestHandleUnexpectedVersionAfterNegotiation(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	client := setupTestClient(serverConn)
	
	errChan := make(chan error, 1)
	go func() {
		errChan <- client.Handle()
	}()

	// 1. Send ResetQuery(v1) to negotiate version 1
	protocol.NewResetQueryPDU(1).Write(clientConn)

	// 2. Consume responses (CacheResponse + EndOfData)
	p1, _ := protocol.GetPDU(clientConn)
	p2, _ := protocol.GetPDU(clientConn)
	if p1 == nil || p2 == nil {
		t.Fatalf("Failed to read initial responses")
	}

	// 3. Send SerialQuery(v2) -> should trigger UnexpectedVersion
	protocol.NewSerialQueryPDU(2, 0xABCD, 10).Write(clientConn)

	resp, err := protocol.GetPDU(clientConn)
	if err != nil {
		t.Fatalf("Failed to read error report: %v", err)
	}

	if resp.Type() != protocol.ErrorReport {
		t.Fatalf("Expected ErrorReport, got %v", resp.Type())
	}

	errReport := resp.(*protocol.ErrorReportPDU)
	if errReport.Code() != protocol.UnexpectedVersion {
		t.Errorf("Expected code 8, got %d", errReport.Code())
	}

	select {
	case err := <-errChan:
		if err == nil || !strings.Contains(err.Error(), "version mismatch") {
			t.Errorf("Expected version mismatch error, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("Timed out waiting for Handle to return")
	}
}

func TestHandleMalformedPDU(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	client := setupTestClient(serverConn)

	go func() {
		client.Handle()
	}()

	// 1. Send valid ResetQuery to negotiate and enter main loop
	protocol.NewResetQueryPDU(1).Write(clientConn)
	protocol.GetPDU(clientConn) // CacheResponse
	protocol.GetPDU(clientConn) // EndOfData

	// 2. Send malformed PDU header (length 4 is invalid)
	clientConn.Write([]byte{1, 1, 0, 0, 0, 0, 0, 4})

	resp, err := protocol.GetPDU(clientConn)
	if err != nil {
		t.Fatalf("Failed to read error report: %v", err)
	}

	if resp.Type() != protocol.ErrorReport {
		t.Fatalf("Expected ErrorReport, got %v", resp.Type())
	}

	errReport := resp.(*protocol.ErrorReportPDU)
	if errReport.Code() != protocol.CorruptData {
		t.Errorf("Expected code 0 (CorruptData), got %d", errReport.Code())
	}
}
