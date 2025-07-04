package server

import (
	"bufio"
	"fmt"
	"net"
	"strings"

	"github.com/mellowdrifter/rpkirtr2/internal/protocol"

	"go.uber.org/zap"
)

type Client struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	logger *zap.SugaredLogger
	id     string
	proto  protocol.Handler
}

// NewClient creates a new client connection wrapper
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

// ID returns the unique ID for this client (IP:port)
func (c *Client) ID() string {
	return c.id
}

// Handle runs the client session: negotiates version, then processes messages
func (c *Client) Handle() error {
	c.logger.Info("Starting session")

	// Step 1: Protocol version negotiation
	handler, err := protocol.Negotiate(c.reader, c.writer)
	if err != nil {
		return fmt.Errorf("version negotiation failed: %w", err)
	}
	c.proto = handler
	c.logger.Infof("Negotiated protocol: %T", handler)

	// Step 2: Read-process loop
	for {
		msg, err := c.proto.ReadMessage(c.reader)
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				c.logger.Info("Client disconnected")
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}

		if err := c.proto.HandleMessage(msg, c.writer); err != nil {
			return fmt.Errorf("message handling error: %w", err)
		}
	}
}
