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

const (
	// Intervals are the default intervals in seconds if no specific value is configured
	DefaultRefreshInterval = uint32(3600) // 1 - 86400
	DefaultRetryInterval   = uint32(600)  // 1 - 7200
	DefaultExpireInterval  = uint32(7200) // 600 - 172800

)

type Client struct {
	conn      net.Conn
	reader    *bufio.Reader
	writer    *bufio.Writer
	logger    *zap.SugaredLogger
	id        string
	closeOnce sync.Once
	version   protocol.Version
	cache     *cache
	cfg       cfg
}

type cfg struct {
	refreshInterval uint32
	retryInterval   uint32
	expireInterval  uint32
}

// NewClient wraps a new connection into a Client instance.
func NewClient(conn net.Conn, baseLogger *zap.SugaredLogger, c *cache) *Client {
	remote := conn.RemoteAddr().String()
	logger := baseLogger.With("client", remote)

	return &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
		logger: logger,
		id:     remote,
		cache:  c,
		cfg:    *newCfg(),
	}
}

// ID returns the unique identifier for the client (IP:Port).
func (c *Client) ID() string {
	return c.id
}

func newCfg() *cfg {
	return &cfg{
		refreshInterval: DefaultRefreshInterval,
		retryInterval:   DefaultRetryInterval,
		expireInterval:  DefaultExpireInterval,
	}
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

	c.logger.Infof("Negotiated version: %d", ver)
	c.version = ver

	// Step 2: Client MUST send either a Reset Query or a Serial Query PDU
	pdu, err := protocol.GetPDU(c.reader)
	if err != nil {
		c.logger.Warnf("Failed to read initial PDU: %v", err)
		c.sendAndCloseError("INITIAL_PDU_READ_FAILED")
		return err
	}
	if err := c.sendInitialResponse(pdu); err != nil {
		c.logger.Warnf("Failed to send initial messages: %v", err)
		c.sendAndCloseError("INIT_FAILED")
		return err
	}

	// Step 3: Main read-process loop
	for {
		_, err := protocol.GetPDU(c.reader)
		if err != nil {
			if isDisconnectError(err) {
				c.logger.Info("Client disconnected")
				return nil
			}
			c.logger.Warnf("Read error: %v", err)
			c.sendAndCloseError("READ_ERROR")
			return err
		}
		switch pdu.Type() {
		case protocol.ResetQuery:
			c.logger.Info("Received Reset Query PDU")
			c.sendCacheResponse()
			c.sendAllROAS()
		case protocol.SerialQuery:
			c.logger.Info("Received Serial Query PDU")
			sqPDU, ok := pdu.(*protocol.SerialQueryPDU)
			if !ok {
				c.logger.Warnf("Failed to cast PDU to *SerialQueryPDU")
				c.sendAndCloseError("SERIAL_QUERY_CAST_ERROR")
				return errors.New("failed to cast PDU to *SerialQueryPDU")
			}
			if err := c.handleSerialQuery(sqPDU); err != nil {
				c.logger.Warnf("Failed to handle Serial Query PDU: %v", err)
				c.sendAndCloseError("SERIAL_QUERY_ERROR")
				return err
			}
			// TODO: Handle errors and whatever other PDUs the client might send
		default:
			c.logger.Warnf("Unexpected PDU type: %s", pdu.Type())
			c.logger.Infof("Going to end the session")
			c.Close()
			return nil
		}
	}
}

// TODO: A lot of overlap with the main loop here...
func (c *Client) sendInitialResponse(pdu protocol.PDU) error {

	switch pdu.Type() {
	case protocol.ResetQuery:
		c.logger.Info("Received Reset Query PDU")
		c.sendCacheResponse()
		c.sendAllROAS()
	case protocol.SerialQuery:
		c.logger.Info("Received Serial Query PDU")
		sqPDU, ok := pdu.(*protocol.SerialQueryPDU)
		if !ok {
			c.logger.Warnf("Failed to cast PDU to *SerialQueryPDU")
			c.sendAndCloseError("SERIAL_QUERY_CAST_ERROR")
			return errors.New("failed to cast PDU to *SerialQueryPDU")
		}
		if err := c.handleSerialQuery(sqPDU); err != nil {
			c.logger.Warnf("Failed to handle Serial Query PDU: %v", err)
			c.sendAndCloseError("SERIAL_QUERY_ERROR")
			return err
		}
	default:
		c.logger.Warnf("Unexpected PDU type: %s", pdu.Type())
		c.logger.Infof("Going to end the session")
		c.Close()
		return nil
	}
	return nil
}

func (c *Client) handleSerialQuery(pdu *protocol.SerialQueryPDU) error {
	c.logger.Info("Handling Serial Query PDU")

	// If the client sends nil, it's new and therefore just send a reset so it'll ask for everything
	if pdu.Serial() == 0 {
		c.logger.Infof("Client requested serial 0, so sending cache reset PDU")
		c.sendCacheReset()
		return nil
	}

	serial := c.cache.getSerial()

	// Cache can only deal with the current or previous serial number
	if pdu.Serial() != serial && pdu.Serial() != serial-1 {
		c.logger.Infof("Client requested serial %d, current serial is %d", pdu.Serial(), serial)
		// Send a reset to the client, and it'll then request the entire cache
		c.sendCacheReset()
		return nil
	}

	// If the serials match, send a Cache Response PDU
	if pdu.Serial() == serial {
		c.logger.Infof("Client requested current serial %d", pdu.Serial())
		c.sendCacheResponse()
	}

	// If the serial is one less than the current, and there are diffs, send the diffs
	if pdu.Serial() == serial-1 && c.cache.isDiffs() {
		c.sendCacheResponse()
		c.sendDiffs()
	}

	// Notify the client of the current serial number
	c.sendEndOfDataPDU(c.cache.session, serial)

	return nil

}

func (c *Client) sendDiffs() {
	c.logger.Info("Sending diffs to client")

	// Send all ROAs that were added
	add, del := c.cache.getDiffs()
	for _, roa := range add {
		var pdu protocol.PDU
		if roa.Prefix.Addr().Is4() {
			pdu = protocol.NewIpv4PrefixPDU(
				c.version,
				protocol.Announce,
				uint8(roa.Prefix.Bits()),
				roa.MaxMask,
				roa.Prefix.Addr().As4(),
				roa.ASN,
			)
		} else {
			pdu = protocol.NewIpv6PrefixPDU(
				c.version,
				protocol.Announce,
				uint8(roa.Prefix.Bits()),
				roa.MaxMask,
				roa.Prefix.Addr().As16(),
				roa.ASN,
			)
		}
		if err := pdu.Write(c.writer); err != nil {
			c.logger.Errorf("Failed to write PDU for added ROA: %v", err)
			c.sendAndCloseError("WRITE_ERROR")
			return
		}
	}
	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer after sending PDU for added ROA: %v", err)
		c.sendAndCloseError("FLUSH_ERROR")
		return
	}

	for _, roa := range del {
		var pdu protocol.PDU
		if roa.Prefix.Addr().Is4() {
			pdu = protocol.NewIpv4PrefixPDU(
				c.version,
				protocol.Withdraw,
				uint8(roa.Prefix.Bits()),
				roa.MaxMask,
				roa.Prefix.Addr().As4(),
				roa.ASN,
			)
		} else {
			pdu = protocol.NewIpv6PrefixPDU(
				c.version,
				protocol.Withdraw,
				uint8(roa.Prefix.Bits()),
				roa.MaxMask,
				roa.Prefix.Addr().As16(),
				roa.ASN,
			)
		}
		if err := pdu.Write(c.writer); err != nil {
			c.logger.Errorf("Failed to write PDU for deleted ROA: %v", err)
			c.sendAndCloseError("WRITE_ERROR")
			return
		}
	}
	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer after sending PDU for deleted ROA: %v", err)
		c.sendAndCloseError("FLUSH_ERROR")
		return
	}
}

func (c *Client) sendCacheReset() {
	c.logger.Info("Sending Cache Reset PDU to client")
	rpdu := protocol.NewResetQueryPDU(c.version)
	if err := rpdu.Write(c.writer); err != nil {
		c.logger.Errorf("Failed to write Cache Reset PDU: %v", err)
		c.sendAndCloseError("WRITE_ERROR")
		return
	}
	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer after sending Cache Reset PDU: %v", err)
		c.sendAndCloseError("FLUSH_ERROR")
		return
	}
	c.logger.Info("Cache Reset PDU sent successfully")
}

func (c *Client) sendEndOfDataPDU(session uint16, serial uint32) {
	c.logger.Info("Sending End of Data PDU to client")
	// TODO: Use the actual values from the client if they are set
	edpu := protocol.NewEndOfDataPDU(
		c.version,
		session,
		serial,
		DefaultRefreshInterval,
		DefaultRetryInterval,
		DefaultExpireInterval,
	)

	if err := edpu.Write(c.writer); err != nil {
		c.logger.Errorf("Failed to write End of Data PDU: %v", err)
		c.sendAndCloseError("WRITE_ERROR")
		return
	}

	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer after sending End of Data PDU: %v", err)
		c.sendAndCloseError("FLUSH_ERROR")
		return
	}
	c.logger.Info("End of Data PDU sent successfully")
}

func (c *Client) sendCacheResponse() {
	c.logger.Info("Sending Cache Response PDU to client")
	cpdu := protocol.NewCacheResponsePDU(c.version, c.cache.getSession())
	if err := cpdu.Write(c.writer); err != nil {
		c.logger.Errorf("Failed to write Cache Response PDU: %v", err)
		c.sendAndCloseError("WRITE_ERROR")
		return
	}

	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer after sending Cache Response PDU: %v", err)
		c.sendAndCloseError("FLUSH_ERROR")
		return
	}
	c.logger.Info("Cache Response PDU sent successfully")
}

func (c *Client) sendAllROAS() {
	c.logger.Info("Sending all ROAs to client")

	// Buffer writer so we send multiple PDUs per TCP packet
	buf := bufio.NewWriter(c.conn)

	roas := c.cache.getRoas()
	for _, roa := range roas {
		var pdu protocol.PDU
		if roa.Prefix.Addr().Is4() {
			pdu = protocol.NewIpv4PrefixPDU(
				c.version,
				protocol.Announce,
				uint8(roa.Prefix.Bits()),
				roa.MaxMask,
				roa.Prefix.Addr().As4(),
				roa.ASN,
			)
		} else {
			pdu = protocol.NewIpv6PrefixPDU(
				c.version,
				protocol.Announce,
				uint8(roa.Prefix.Bits()),
				roa.MaxMask,
				roa.Prefix.Addr().As16(),
				roa.ASN,
			)
		}
		if err := pdu.Write(buf); err != nil {
			c.logger.Errorf("Failed to write prefix PDUs: %v", err)
			c.sendAndCloseError("WRITE_ERROR")
			return
		}
	}
	// Compact all the ROA updates into the TCP stream, instead of sending tiny packets
	if err := buf.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer: %v", err)
		c.sendAndCloseError("FLUSH_ERROR")
		return
	}

	c.logger.Infof("Sent all ROAs to client %s", c.id)
	c.sendEndOfDataPDU(c.cache.getSession(), c.cache.getSerial())
}

// sendAndCloseError sends a protocol error PDU and closes the connection.
func (c *Client) sendAndCloseError(msg string) {
	// TODO: Figure out error code mapping
	// Also fix the version field
	pdu := protocol.NewErrorReportPDU(2, 10, nil, []byte(msg))
	pdu.Write(c.writer)
	if err := c.writer.Flush(); err != nil {
		c.logger.Warnf("Failed to send error PDU: %v", err)
	}
	c.logger.Warnf("Closing connection due to error: %s", msg)
	if c.conn != nil {
		c.logger.Infof("Closing connection to client: %s", c.id)

		_ = c.conn.Close()
	}
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

// notify sends a notification to the client with the new serial number.
func (c *Client) notify() {
	serial := c.cache.getSerial()

	pdu := protocol.NewSerialNotifyPDU(c.version, c.cache.getSession(), serial)
	if err := pdu.Write(c.writer); err != nil {
		c.logger.Errorf("Failed to write Serial Notify PDU: %v", err)
		return
	}

	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer after sending Serial Notify PDU: %v", err)
	}
	c.logger.Infof("Sent Serial Notify PDU with serial %d to client %s", serial, c.id)
}
