package clienttest

import (
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
