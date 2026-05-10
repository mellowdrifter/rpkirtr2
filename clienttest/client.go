package clienttest

import (
	"fmt"
	"net"
	"time"
)

type RTRClient struct {
	conn net.Conn
}

func NewRTRClient(address string, timeout time.Duration) (*RTRClient, error) {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, err
	}
	return &RTRClient{conn: conn}, nil
}

func (c *RTRClient) Send(data []byte) error {
	_, err := c.conn.Write(data)
	return err
}

func (c *RTRClient) Receive(maxLen int) ([]byte, error) {
	c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, maxLen)
	n, err := c.conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (c *RTRClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *RTRClient) CollectPrefixes() ([]ReceivedROA, error) {
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer c.conn.SetReadDeadline(time.Time{}) // clear deadline after

	var out []ReceivedROA
	for {
		pdu, err := ReadNextPDU(c.conn)
		if err != nil {
			return nil, err
		}
		switch pdu.Type {
		case Ipv4Prefix, Ipv6Prefix:
			r, err := parsePrefix(pdu)
			if err != nil {
				return nil, err
			}
			out = append(out, r)
		case EndOfDataType:
			return out, nil
		case CacheReset:
			return nil, fmt.Errorf("received unexpected Cache Reset")
		case CacheResponse, SerialNotify:
			// Skip and keep reading.
			continue
		default:
			return nil, fmt.Errorf("unexpected PDU type %d while collecting prefixes", pdu.Type)
		}
	}
}
