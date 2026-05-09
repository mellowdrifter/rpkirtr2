package clienttest

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

const (
	SerialNotify  = 0
	SerialQuery   = 1
	ResetQuery    = 2
	CacheResponse = 3
	Ipv4Prefix    = 4
	Ipv6Prefix    = 6
	EndOfDataType = 7
	CacheReset    = 8
)

type ReceivedROA struct {
	Prefix  string
	ASN     uint32
	MaxMask uint8
	Flags   uint8 // 0 = withdraw, 1 = announce
}

func parsePrefix(pdu *PDU) (ReceivedROA, error) {
	if pdu.Type == Ipv4Prefix {
		if len(pdu.Body) < 12 {
			return ReceivedROA{}, fmt.Errorf("ipv4 prefix body too short")
		}
		flags := pdu.Body[0]
		mask := pdu.Body[1]
		maxMask := pdu.Body[2]
		// skip pdu.Body[3] which is zero
		ip := net.IP(pdu.Body[4:8])
		asn := binary.BigEndian.Uint32(pdu.Body[8:12])
		return ReceivedROA{
			Prefix:  fmt.Sprintf("%s/%d", ip.String(), mask),
			ASN:     asn,
			MaxMask: maxMask,
			Flags:   flags,
		}, nil
	} else if pdu.Type == Ipv6Prefix {
		if len(pdu.Body) < 24 {
			return ReceivedROA{}, fmt.Errorf("ipv6 prefix body too short")
		}
		flags := pdu.Body[0]
		mask := pdu.Body[1]
		maxMask := pdu.Body[2]
		// skip pdu.Body[3] which is zero
		ip := net.IP(pdu.Body[4:20])
		asn := binary.BigEndian.Uint32(pdu.Body[20:24])
		return ReceivedROA{
			Prefix:  fmt.Sprintf("%s/%d", ip.String(), mask),
			ASN:     asn,
			MaxMask: maxMask,
			Flags:   flags,
		}, nil
	}
	return ReceivedROA{}, fmt.Errorf("not a prefix PDU")
}

type PDU struct {
	Version   uint8
	Type      uint8
	SessionID uint16
	Length    uint32
	Body      []byte
}

func ReadNextPDU(conn net.Conn) (*PDU, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, fmt.Errorf("reading PDU header: %w", err)
	}

	version := header[0]
	pduType := header[1]
	sessionID := binary.BigEndian.Uint16(header[2:4])
	length := binary.BigEndian.Uint32(header[4:8])

	if length < 8 {
		return nil, fmt.Errorf("invalid PDU length: %d", length)
	}

	bodyLen := length - 8
	body := make([]byte, bodyLen)
	if bodyLen > 0 {
		if _, err := io.ReadFull(conn, body); err != nil {
			return nil, fmt.Errorf("reading PDU body: %w", err)
		}
	}

	return &PDU{
		Version:   version,
		Type:      pduType,
		SessionID: sessionID,
		Length:    length,
		Body:      body,
	}, nil
}

type EndOfData struct {
	SerialNumber    uint32
	RefreshInterval uint32
	RetryInterval   uint32
	ExpireInterval  uint32
}

func parseEndOfData(pdu *PDU) (*EndOfData, error) {
	if len(pdu.Body) < 16 {
		return nil, fmt.Errorf("end of data PDU body too short: got %d bytes", len(pdu.Body))
	}

	return &EndOfData{
		SerialNumber:    binary.BigEndian.Uint32(pdu.Body[0:4]),
		RefreshInterval: binary.BigEndian.Uint32(pdu.Body[4:8]),
		RetryInterval:   binary.BigEndian.Uint32(pdu.Body[8:12]),
		ExpireInterval:  binary.BigEndian.Uint32(pdu.Body[12:16]),
	}, nil
}

func BuildResetQuery(version int) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint8(version))
	binary.Write(buf, binary.BigEndian, uint8(ResetQuery))
	binary.Write(buf, binary.BigEndian, uint16(0)) // reserved
	binary.Write(buf, binary.BigEndian, uint32(8)) // length
	return buf.Bytes()
}

func BuildSerialQuery(version, session, serial int) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint8(version))
	binary.Write(buf, binary.BigEndian, uint8(SerialQuery))
	binary.Write(buf, binary.BigEndian, uint16(session))
	binary.Write(buf, binary.BigEndian, uint32(12)) // length
	binary.Write(buf, binary.BigEndian, uint32(serial))
	return buf.Bytes()
}

func BuildMalformedPDU() []byte {
	// Bad version and short length
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint8(2))
	binary.Write(buf, binary.BigEndian, uint8(99)) // Unknown PDU type
	binary.Write(buf, binary.BigEndian, uint16(0))
	binary.Write(buf, binary.BigEndian, uint32(4)) // Invalid length
	return buf.Bytes()
}
