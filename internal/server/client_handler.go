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
		c.sendAndCloseError("NEGOTIATION_FAILED", protocol.UnsupportedVersion)
		return err
	}

	c.logger.Infof("Negotiated version: %d", ver)
	c.version = ver

	// Step 2: Client MUST send either a Reset Query or a Serial Query PDU
	pdu, err := protocol.GetPDU(c.reader)
	if err != nil {
		c.logger.Warnf("Failed to read initial PDU: %v", err)
		c.sendAndCloseError("INVALID_REQUEST", protocol.InvalidRequest)
		return err
	}
	if err := c.sendInitialResponse(pdu); err != nil {
		c.logger.Warnf("Failed to send initial messages: %v", err)
		c.sendAndCloseError("INIT_FAILED", protocol.InternalError)
		return err
	}

	// Step 3: Main read-process loop
	for {
		pdu, err := protocol.GetPDU(c.reader)
		if err != nil {
			if isDisconnectError(err) {
				c.logger.Info("Client disconnected")
				return nil
			}
			c.logger.Warnf("Read error: %v", err)
			c.sendAndCloseError("READ_ERROR", protocol.CorruptData)
			return err
		}
		switch pdu.Type() {
		case protocol.ResetQuery:
			c.logger.Info("Received Reset Query PDU")
			c.logger.Debugf("Reset Query PDU: %+v", pdu)
			c.sendCacheResponse()
			c.sendAllROAS()
		case protocol.SerialQuery:
			c.logger.Info("Received Serial Query PDU")
			sqPDU, ok := pdu.(*protocol.SerialQueryPDU)
			if !ok {
				c.logger.Warnf("Failed to cast PDU to *SerialQueryPDU")
				c.sendAndCloseError("SERIAL_QUERY_CAST_ERROR", protocol.InternalError)
				return errors.New("failed to cast PDU to *SerialQueryPDU")
			}
			if err := c.handleSerialQuery(sqPDU); err != nil {
				c.logger.Warnf("Failed to handle Serial Query PDU: %v", err)
				c.sendAndCloseError("SERIAL_QUERY_ERROR", protocol.InternalError)
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
		c.logger.Debugf("Reset Query PDU: %+v", pdu)
		c.sendCacheResponse()
		c.sendAllROAS()
	case protocol.SerialQuery:
		c.logger.Info("Received Serial Query PDU")
		sqPDU, ok := pdu.(*protocol.SerialQueryPDU)
		if !ok {
			c.logger.Warnf("Failed to cast PDU to *SerialQueryPDU")
			c.sendAndCloseError("SERIAL_QUERY_CAST_ERROR", protocol.InternalError)
			return errors.New("failed to cast PDU to *SerialQueryPDU")
		}
		if err := c.handleSerialQuery(sqPDU); err != nil {
			c.logger.Warnf("Failed to handle Serial Query PDU: %v", err)
			c.sendAndCloseError("SERIAL_QUERY_ERROR", protocol.InternalError)
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

	// Cache can only deal with the current or previous serial number
	if pdu.Serial() != c.getSerial() && pdu.Serial() != c.getSerial()-1 {
		c.logger.Infof("Client requested serial %d, current serial is %d", pdu.Serial(), c.getSerial())
		// Send a reset to the client, and it'll then request the entire cache
		c.sendCacheReset()
		return nil
	}

	// If the serials match, send a Cache Response PDU
	if pdu.Serial() == c.getSerial() {
		c.logger.Infof("Client requested current serial %d", pdu.Serial())
		c.sendCacheResponse()
	}

	// If the serial is one less than the current, and there are diffs, send the diffs
	if pdu.Serial() == c.getSerial()-1 && c.cache.isDiffs() {
		c.sendCacheResponse()
		c.sendDiffs()
	}

	// Notify the client of the current serial number
	c.sendEndOfDataPDU(c.getSession(), c.getSerial())

	return nil

}

func (c *Client) sendDiffs() {
	c.rlock()
	defer c.runlock()

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
			c.sendAndCloseError("WRITE_ERROR", protocol.InternalError)
			return
		}
	}
	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer after sending PDU for added ROA: %v", err)
		c.sendAndCloseError("FLUSH_ERROR", protocol.InternalError)
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
			c.sendAndCloseError("WRITE_ERROR", protocol.InternalError)
			return
		}
	}
	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer after sending PDU for deleted ROA: %v", err)
		c.sendAndCloseError("FLUSH_ERROR", protocol.InternalError)
		return
	}
}

func (c *Client) sendCacheReset() {
	c.logger.Info("Sending Cache Reset PDU to client")
	rpdu := protocol.NewCacheResetPDU(c.version)
	if err := rpdu.Write(c.writer); err != nil {
		c.logger.Errorf("Failed to write Cache Reset PDU: %v", err)
		c.sendAndCloseError("WRITE_ERROR", protocol.InternalError)
		return
	}
	c.logger.Debugf("cache reset PDU: %+v", rpdu)
	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer after sending Cache Reset PDU: %v", err)
		c.sendAndCloseError("FLUSH_ERROR", protocol.InternalError)
		return
	}
	c.logger.Info("Cache Reset PDU sent successfully")
}

func (c *Client) sendEndOfDataPDU(session uint16, serial uint32) {
	c.rlock()
	defer c.runlock()

	c.logger.Info("Sending End of Data PDU to client")
	// TODO: Use the actual values from the client if they are set
	epdu := protocol.NewEndOfDataPDU(
		c.version,
		session,
		serial,
		DefaultRefreshInterval,
		DefaultRetryInterval,
		DefaultExpireInterval,
	)

	c.logger.Debugf("end of data pdu: %+v", epdu)

	if err := epdu.Write(c.writer); err != nil {
		c.logger.Errorf("Failed to write End of Data PDU: %v", err)
		c.sendAndCloseError("WRITE_ERROR", protocol.InternalError)
		return
	}

	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer after sending End of Data PDU: %v", err)
		c.sendAndCloseError("FLUSH_ERROR", protocol.InternalError)
		return
	}
	c.logger.Info("End of Data PDU sent successfully")
}

func (c *Client) sendCacheResponse() {
	c.rlock()
	defer c.runlock()

	c.logger.Info("Sending Cache Response PDU to client")
	cpdu := protocol.NewCacheResponsePDU(c.getVersion(), c.getSession())
	if err := cpdu.Write(c.writer); err != nil {
		c.logger.Errorf("Failed to write Cache Response PDU: %v", err)
		c.sendAndCloseError("WRITE_ERROR", protocol.InternalError)
		return
	}

	c.logger.Debugf("cache response PDU: %+v", cpdu)

	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer after sending Cache Response PDU: %v", err)
		c.sendAndCloseError("FLUSH_ERROR", protocol.InternalError)
		return
	}
	c.logger.Info("Cache Response PDU sent successfully")
}

func (c *Client) sendAllROAS() {
	c.rlock()
	defer c.runlock()

	c.logger.Info("Sending all ROAs to client")

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
		if err := pdu.Write(c.writer); err != nil {
			c.logger.Errorf("Failed to write prefix PDUs: %v", err)
			c.sendAndCloseError("WRITE_ERROR", protocol.InternalError)
			return
		}
	}
	// Compact all the ROA updates into the TCP stream, instead of sending tiny packets
	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer: %v", err)
		c.sendAndCloseError("FLUSH_ERROR", protocol.InternalError)
		return
	}

	c.logger.Infof("Sent all ROAs to client %s", c.id)
	c.sendEndOfDataPDU(c.getSession(), c.getSerial())
}

// sendAndCloseError sends a protocol error PDU and closes the connection.
func (c *Client) sendAndCloseError(msg string, code protocol.ErrorCode) {
	// TODO: Figure out error code mapping
	// Also fix the version field
	// TODO: There should be two error functions, one that takes in PDUs and another that doesn't
	// Adding bytes of msg as a temp holder
	pdu := protocol.NewErrorReportPDU(2, code, []byte(msg), msg)
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

	pdu := protocol.NewSerialNotifyPDU(c.version, c.getSession(), c.getSerial())
	if err := pdu.Write(c.writer); err != nil {
		c.logger.Errorf("Failed to write Serial Notify PDU: %v", err)
		return
	}

	c.logger.Debugf("serial notify PDU: %+v", pdu)

	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer after sending Serial Notify PDU: %v", err)
	}
	c.logger.Infof("Sent Serial Notify PDU with serial %d to client %s", c.getSerial(), c.id)
}

func (c *Client) getSerial() uint32 {
	return c.cache.serial
}

func (c *Client) getSession() uint16 {
	return c.cache.session
}

func (c *Client) rlock() {
	c.cache.mu.RLock()
}

func (c *Client) runlock() {
	c.cache.mu.RUnlock()
}

func (c *Client) getVersion() protocol.Version {
	return c.version
}
