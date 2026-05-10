package server

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/mellowdrifter/rpkirtr2/internal/protocol"

	"go.uber.org/zap"
)

const (
	// Intervals are the default intervals in seconds if no specific value is configured
	DefaultRefreshInterval = uint32(3600) // 1 - 86400
	DefaultRetryInterval   = uint32(600)  // 1 - 7200
	DefaultExpireInterval  = uint32(7200) // 600 - 172800
	DefaultReadTimeout     = 2 * time.Minute
)

type Client struct {
	conn      net.Conn
	reader    *bufio.Reader
	writer    *bufio.Writer
	writeMu   sync.Mutex
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
	readTimeout     time.Duration
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
		readTimeout:     DefaultReadTimeout,
	}
}

// Handle manages the full lifecycle of the client connection.
func (c *Client) Handle() error {
	defer c.Close()

	c.logger.Info("Client session started")

	// Step 1: Version negotiation
	c.conn.SetReadDeadline(time.Now().Add(c.cfg.readTimeout))
	ver, err := protocol.Negotiate(c.reader)
	if err != nil {
		c.logger.Warnf("Negotiation failed: %v", err)
		c.sendAndCloseError("NEGOTIATION_FAILED", protocol.UnsupportedVersion)
		return err
	}

	c.logger.Infof("Negotiated version: %d", ver)
	c.version = ver

	// Step 2: Client MUST send either a Reset Query or a Serial Query PDU
	c.conn.SetReadDeadline(time.Now().Add(c.cfg.readTimeout))
	pdu, err := protocol.GetPDU(c.reader)
	if err != nil {
		c.logger.Warnf("Failed to read initial PDU: %v", err)
		c.sendAndCloseError("INVALID_REQUEST", protocol.InvalidRequest)
		return err
	}
	if err := c.dispatchPDU(pdu); err != nil {
		c.logger.Warnf("Failed to dispatch initial PDU: %v", err)
		return err
	}

	// Step 3: Main read-process loop
	for {
		c.conn.SetReadDeadline(time.Now().Add(c.cfg.readTimeout))
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
		if err := c.dispatchPDU(pdu); err != nil {
			return err
		}
	}
}

func (c *Client) dispatchPDU(pdu protocol.PDU) error {
	if c.version != pdu.Version() {
		c.logger.Warnf("Version mismatch: expected %d, got %d", c.version, pdu.Version())
		c.sendAndCloseError("VERSION_MISMATCH", protocol.UnexpectedVersion)
		return errors.New("version mismatch")
	}

	switch pdu.Type() {
	case protocol.ResetQuery:
		c.logger.Info("Received Reset Query PDU")
		state := c.cache.getState()
		c.sendAllData(state.roas, state.aspas, state.session, state.serial)
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
		c.logger.Warnf("Unexpected PDU type: %d", pdu.Type())
		c.Close()
		return nil
	}
	return nil
}

func (c *Client) handleSerialQuery(pdu *protocol.SerialQueryPDU) error {
	c.logger.Info("Handling Serial Query PDU")
	serial := pdu.Serial()
	state := c.cache.getState()

	if pdu.Session() != state.session {
		c.logger.Infof("Client session ID %d does not match server session ID %d. Sending cache reset.", pdu.Session(), state.session)
		c.sendCacheReset()
		return nil
	}

	if serial == 0 {
		c.logger.Infof("Client requested serial 0, so sending cache reset PDU")
		c.sendCacheReset()
		return nil
	}

	addRoa, delRoa, addAspa, delAspa, found := c.cache.getDiffsFrom(serial)
	if !found {
		c.logger.Infof("Client requested serial %d, current serial is %d. Serial too old or unknown. Sending cache reset.", serial, state.serial)
		c.sendCacheReset()
		return nil
	}

	c.logger.Infof("Client requested serial %d, current serial is %d. Sending %d ROA additions, %d ROA deletions, %d ASPA additions, %d ASPA deletions.", serial, state.serial, len(addRoa), len(delRoa), len(addAspa), len(delAspa))
	c.sendDiffs(addRoa, delRoa, addAspa, delAspa, state.session, state.serial)

	return nil
}

func (c *Client) writePDUUnsafe(pdu protocol.PDU) error {
	if err := pdu.Write(c.writer); err != nil {
		return err
	}
	return c.writer.Flush()
}

func (c *Client) sendDiffs(add, del []ROA, addAspa, delAspa []ASPA, session uint16, serial uint32) {
	c.logger.Info("Sending diffs to client")

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// 1. Send Cache Response
	cpdu := protocol.NewCacheResponsePDU(c.version, session)
	if err := cpdu.Write(c.writer); err != nil {
		c.logger.Errorf("Failed to write Cache Response PDU: %v", err)
		return
	}

	// 2. Send all ROA additions
	for _, ROA := range add {
		var err error
		if ROA.Prefix.Addr().Is4() {
			err = protocol.WriteIpv4Prefix(c.writer, c.version, protocol.Announce, uint8(ROA.Prefix.Bits()), ROA.MaxMask, ROA.Prefix.Addr().As4(), ROA.ASN)
		} else {
			err = protocol.WriteIpv6Prefix(c.writer, c.version, protocol.Announce, uint8(ROA.Prefix.Bits()), ROA.MaxMask, ROA.Prefix.Addr().As16(), ROA.ASN)
		}
		if err != nil {
			c.logger.Errorf("Failed to write prefix PDU: %v", err)
			return
		}
	}

	// 3. Send all ASPA additions
	if c.version >= 1 {
		for _, aspa := range addAspa {
			pdu := protocol.NewAspaPDU(c.version, protocol.Announce, aspa.CustomerASN, aspa.ProviderASNs)
			if err := pdu.Write(c.writer); err != nil {
				c.logger.Errorf("Failed to write ASPA PDU: %v", err)
				return
			}
		}
	}

	// 4. Send all ROA deletions
	for _, ROA := range del {
		var err error
		if ROA.Prefix.Addr().Is4() {
			err = protocol.WriteIpv4Prefix(c.writer, c.version, protocol.Withdraw, uint8(ROA.Prefix.Bits()), ROA.MaxMask, ROA.Prefix.Addr().As4(), ROA.ASN)
		} else {
			err = protocol.WriteIpv6Prefix(c.writer, c.version, protocol.Withdraw, uint8(ROA.Prefix.Bits()), ROA.MaxMask, ROA.Prefix.Addr().As16(), ROA.ASN)
		}
		if err != nil {
			c.logger.Errorf("Failed to write prefix PDU: %v", err)
			return
		}
	}

	// 5. Send all ASPA deletions
	if c.version >= 1 {
		for _, aspa := range delAspa {
			pdu := protocol.NewAspaPDU(c.version, protocol.Withdraw, aspa.CustomerASN, aspa.ProviderASNs)
			if err := pdu.Write(c.writer); err != nil {
				c.logger.Errorf("Failed to write ASPA PDU: %v", err)
				return
			}
		}
	}

	// 6. Send End of Data
	epdu := protocol.NewEndOfDataPDU(c.version, session, serial, DefaultRefreshInterval, DefaultRetryInterval, DefaultExpireInterval)
	if err := epdu.Write(c.writer); err != nil {
		c.logger.Errorf("Failed to write End of Data PDU: %v", err)
		return
	}

	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer: %v", err)
		return
	}
}

func (c *Client) sendCacheReset() {
	c.logger.Info("Sending Cache Reset PDU to client")
	rpdu := protocol.NewCacheResetPDU(c.version)

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := c.writePDUUnsafe(rpdu); err != nil {
		c.logger.Errorf("Failed to write Cache Reset PDU: %v", err)
	}
}

func (c *Client) sendAllData(roas []ROA, aspas []ASPA, session uint16, serial uint32) {
	c.logger.Info("Sending all ROAs and ASPAs to client")

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	// 1. Cache Response
	cpdu := protocol.NewCacheResponsePDU(c.version, session)
	if err := cpdu.Write(c.writer); err != nil {
		c.logger.Errorf("Failed to write Cache Response PDU: %v", err)
		return
	}

	// 2. Prefix PDUs
	for _, ROA := range roas {
		var err error
		if ROA.Prefix.Addr().Is4() {
			err = protocol.WriteIpv4Prefix(c.writer, c.version, protocol.Announce, uint8(ROA.Prefix.Bits()), ROA.MaxMask, ROA.Prefix.Addr().As4(), ROA.ASN)
		} else {
			err = protocol.WriteIpv6Prefix(c.writer, c.version, protocol.Announce, uint8(ROA.Prefix.Bits()), ROA.MaxMask, ROA.Prefix.Addr().As16(), ROA.ASN)
		}
		if err != nil {
			c.logger.Errorf("Failed to write prefix PDU: %v", err)
			return
		}
	}

	// 3. ASPA PDUs
	if c.version >= 1 {
		for _, aspa := range aspas {
			pdu := protocol.NewAspaPDU(c.version, protocol.Announce, aspa.CustomerASN, aspa.ProviderASNs)
			if err := pdu.Write(c.writer); err != nil {
				c.logger.Errorf("Failed to write ASPA PDU: %v", err)
				return
			}
		}
	}

	// 4. End of Data
	epdu := protocol.NewEndOfDataPDU(c.version, session, serial, DefaultRefreshInterval, DefaultRetryInterval, DefaultExpireInterval)
	if err := epdu.Write(c.writer); err != nil {
		c.logger.Errorf("Failed to write End of Data PDU: %v", err)
		return
	}

	if err := c.writer.Flush(); err != nil {
		c.logger.Errorf("Failed to flush writer: %v", err)
		return
	}
}

func (c *Client) sendAndCloseError(msg string, code protocol.ErrorCode) {
	version := c.version
	if version == 0 {
		version = 2
	}
	pdu := protocol.NewErrorReportPDU(version, code, []byte(msg), msg)

	c.writeMu.Lock()
	// No defer unlock because we might close the connection
	_ = c.writePDUUnsafe(pdu)
	c.writeMu.Unlock()

	c.Close()
}

func isDisconnectError(err error) bool {
	return errors.Is(err, io.EOF) ||
		strings.Contains(err.Error(), "use of closed network connection") ||
		strings.Contains(err.Error(), "connection reset by peer")
}

func (c *Client) Close() {
	c.closeOnce.Do(func() {
		c.logger.Infof("Closing connection to client: %s", c.id)
		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
}

func (c *Client) notify() {
	pdu := protocol.NewSerialNotifyPDU(c.version, c.getSession(), c.getSerial())

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := c.writePDUUnsafe(pdu); err != nil {
		c.logger.Errorf("Failed to write Serial Notify PDU: %v", err)
	}
}

func (c *Client) getSerial() uint32 {
	c.cache.mu.RLock()
	defer c.cache.mu.RUnlock()
	return c.cache.serial
}

func (c *Client) getSession() uint16 {
	c.cache.mu.RLock()
	defer c.cache.mu.RUnlock()
	return c.cache.session
}
