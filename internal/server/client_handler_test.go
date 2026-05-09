package server

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/mellowdrifter/rpkirtr2/internal/protocol"
	"go.uber.org/zap"
)

func TestHandleSerialQuerySessionValidation(t *testing.T) {
	// Setup a pipe for the client connection
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Initialize server cache with a known session ID
	sessionID := uint16(0xABCD)
	c := &cache{
		session: sessionID,
		serial:  10,
	}

	// Create a client
	logger := zap.NewNop().Sugar()
	client := &Client{
		conn:    serverConn,
		reader:  bufio.NewReader(serverConn),
		writer:  bufio.NewWriter(serverConn),
		logger:  logger,
		id:      "test-client",
		cache:   c,
		version: 1,
	}

	// Test case 1: Mismatched session ID
	mismatchedSession := uint16(0x1234)
	sqPDU := protocol.NewSerialQueryPDU(1, mismatchedSession, 5)

	// We need a way to check if Cache Reset was sent.
	// handleSerialQuery calls sendCacheReset, which writes to client.writer.
	
	// Run handleSerialQuery in a goroutine because it might block on flush if we don't read from clientConn
	errChan := make(chan error, 1)
	go func() {
		errChan <- client.handleSerialQuery(sqPDU)
	}()

	// Read from clientConn to see what the server sent
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

	// Test case 2: Matching session ID
	matchingSQ := protocol.NewSerialQueryPDU(1, sessionID, 10)
	go func() {
		errChan <- client.handleSerialQuery(matchingSQ)
	}()

	responsePDU, err = protocol.GetPDU(clientConn)
	if err != nil {
		t.Fatalf("Failed to read response PDU: %v", err)
	}

	// For matching serial, it should send Cache Response followed by End of Data
	if responsePDU.Type() != protocol.CacheResponse {
		t.Errorf("Expected CacheResponse PDU, got %v", responsePDU.Type())
	}

	responsePDU, err = protocol.GetPDU(clientConn)
	if err != nil {
		t.Fatalf("Failed to read response PDU: %v", err)
	}
	if responsePDU.Type() != protocol.EndOfData {
		t.Errorf("Expected EndOfData PDU, got %v", responsePDU.Type())
	}

	if err := <-errChan; err != nil {
		t.Errorf("handleSerialQuery returned error: %v", err)
	}
}

func TestHandleReadTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer ln.Close()

	// Client side
	errChan := make(chan error, 1)
	go func() {
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			errChan <- err
			return
		}
		// Don't send anything and just wait
		time.Sleep(200 * time.Millisecond)
		conn.Close()
		errChan <- nil
	}()

	// Server side
	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("Failed to accept: %v", err)
	}
	defer conn.Close()

	c := &cache{}
	logger := zap.NewNop().Sugar()
	client := &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
		logger: logger,
		id:     "test-client",
		cache:  c,
		cfg: cfg{
			readTimeout: 50 * time.Millisecond,
		},
	}

	// Handle should return an error due to timeout
	err = client.Handle()
	if err == nil {
		t.Error("Expected Handle to return an error due to timeout, got nil")
	}

	// Check if the error is a timeout
	if !strings.Contains(err.Error(), "i/o timeout") {
		t.Errorf("Expected timeout error, got: %v", err)
	}

	if err := <-errChan; err != nil {
		t.Errorf("Client error: %v", err)
	}
}
