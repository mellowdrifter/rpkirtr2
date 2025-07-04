package server

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/mellowdrifter/rpkirtr2/internal/protocol"

	"go.uber.org/zap"
)

type Client struct {
	conn      net.Conn
	reader    *bufio.Reader
	writer    *bufio.Writer
	logger    *zap.SugaredLogger
	id        string
	closeOnce sync.Once
	handler   *protocol.ProtocolHandler
}

// NewClient wraps a new connection into a Client instance.
func NewClient(conn net.Conn, baseLogger *zap.SugaredLogger) *Client {
	remote := conn.RemoteAddr().String()
	logger := baseLogger.With("client", remote)

	return &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
		logger: logger,
		id:     remote,
	}
}

// ID returns the unique identifier for the client (IP:Port).
func (c *Client) ID() string {
	return c.id
}

// Handle manages the full lifecycle of the client connection.
func (c *Client) Handle() error {
	defer c.Close()

	c.logger.Info("Client session started")

	// Step 1: Version negotiation
	ver, err := protocol.Negotiate(c.reader)
	if err != nil {
		c.logger.Warnf("Negotiation failed: %v", err)
		c.sendAndCloseError("NEGOTIATION_FAILED")
		return err
	}
	c.handler = protocol.NewProtocolHandler(ver) // Initialize protocol handler

	// Step 2: Send protocol-specific initial response(s)
	if err := c.handler.SendInitialMessages(c.writer); err != nil {
		c.logger.Warnf("Failed to send initial messages: %v", err)
		c.sendAndCloseError("INIT_FAILED")
		return err
	}

	// Step 3: Main read-process loop
	for {
		msg, err := c.handler.ReadMessage(c.reader)
		if err != nil {
			if isDisconnectError(err) {
				c.logger.Info("Client disconnected")
				return nil
			}
			c.logger.Warnf("Read error: %v", err)
			c.sendAndCloseError("READ_ERROR")
			return err
		}

		if err := c.handler.HandleMessage(msg, c.writer); err != nil {
			c.logger.Warnf("Message handling error: %v", err)
			c.sendAndCloseError("BAD_REQUEST")
			return err
		}
	}
}

// sendAndCloseError sends a protocol error PDU and closes the connection.
func (c *Client) sendAndCloseError(msg string) {
	if c.writer == nil {
		return
	}
	pdu := protocol.PDU{
		Type:    protocol.PDUTypeError,
		Payload: []byte(msg),
	}
	data, err := pdu.Marshal()
	if err == nil {
		_, _ = c.writer.Write(data)
		_ = c.writer.Flush()
	}
	_ = c.conn.Close()
}

// isDisconnectError checks whether an error is due to client disconnection.
func isDisconnectError(err error) bool {
	return errors.Is(err, io.EOF) ||
		strings.Contains(err.Error(), "use of closed network connection") ||
		strings.Contains(err.Error(), "connection reset by peer")
}

// Close terminates the client connection and logs the cleanup step.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		c.logger.Infof("Closing connection to client: %s", c.id)

		if c.conn != nil {
			_ = c.conn.Close()
		}

		// TODO: Cleanup other state if needed
	})
}
