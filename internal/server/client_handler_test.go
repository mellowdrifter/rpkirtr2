package server

import (
	"bufio"
	"net"
	"testing"

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
