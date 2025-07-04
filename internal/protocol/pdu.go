package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

type PDUType uint8

type Version uint8

const (
	// PDU Types
	serialNotify  PDUType = 0
	serialQuery   PDUType = 1
	resetQuery    PDUType = 2
	cacheResponse PDUType = 3
	ipv4Prefix    PDUType = 4
	ipv6Prefix    PDUType = 6
	endOfData     PDUType = 7
	cacheReset    PDUType = 8
	routerKey     PDUType = 9
	errorReport   PDUType = 10
	aspa          PDUType = 11

	// protocol versions
	version1 uint8 = 1
	version2 uint8 = 2

	minPDULength  = 8
	maxPDULength  = 65535
	headPDULength = 2

	// flags
	withdraw uint8 = 0
	announce uint8 = 1
)

// PDU represents a generic protocol data unit
type PDU interface {
	Type() PDUType
	Marshal() ([]byte, error)
}

// headerPDU is used to extract the header of each incoming PDU
type headerPDU struct {
	Version uint8
	Ptype   uint8
}

var supportedVersions = []uint8{
	version1,
	version2,
}

type serialNotifyPDU struct {
	/*
		0          8          16         24        31
		.-------------------------------------------.
		| Protocol |   PDU    |                     |
		| Version  |   Type   |     Session ID      |
		|    X     |    0     |                     |
		+-------------------------------------------+
		|                                           |
		|                Length=12                  |
		|                                           |
		+-------------------------------------------+
		|                                           |
		|               Serial Number               |
		|                                           |
		`-------------------------------------------'
	*/
	Session uint16
	Serial  uint32
}

type serialQueryPDU struct {
	/*
		0          8          16         24        31
		.-------------------------------------------.
		| Protocol |   PDU    |                     |
		| Version  |   Type   |     Session ID      |
		|    X     |    1     |                     |
		+-------------------------------------------+
		|                                           |
		|                 Length=12                 |
		|                                           |
		+-------------------------------------------+
		|                                           |
		|               Serial Number               |
		|                                           |
		`-------------------------------------------'
	*/
	Session uint16
	Serial  uint32
}

type resetQueryPDU struct {
	/*
		0          8          16         24        31
		.-------------------------------------------.
		| Protocol |   PDU    |                     |
		| Version  |   Type   |         zero        |
		|    X     |    2     |                     |
		+-------------------------------------------+
		|                                           |
		|                 Length=8                  |
		|                                           |
		`-------------------------------------------'
	*/
}

type cacheResponsePDU struct {
	/*
		0          8          16         24        31
		.-------------------------------------------.
		| Protocol |   PDU    |                     |
		| Version  |   Type   |     Session ID      |
		|    X     |    3     |                     |
		+-------------------------------------------+
		|                                           |
		|                 Length=8                  |
		|                                           |
		`-------------------------------------------'
	*/
	Session uint16
}

func (c *cacheResponsePDU) Type() PDUType {
	return cacheResponse
}

func (c *cacheResponsePDU) Marshal() ([]byte, error) {
	buf := new(bytes.Buffer)
	// Write PDU type (1 byte)
	if err := binary.Write(buf, binary.BigEndian, cacheResponse); err != nil {
		return nil, fmt.Errorf("failed to write PDU type: %w", err)
	}
	// Write payload length (2 bytes, max 65535)
	payloadLen := uint16(8) // Length of the cacheResponsePDU
	if err := binary.Write(buf, binary.BigEndian, payloadLen); err != nil {
		return nil, fmt.Errorf("failed to write payload length: %w", err)
	}
	// Write session ID (2 bytes)
	if err := binary.Write(buf, binary.BigEndian, c.Session); err != nil {
		return nil, fmt.Errorf("failed to write session ID: %w", err)
	}
	// Pad the rest of the PDU with zeros to make it 8 bytes long
	for buf.Len() < 8 {
		if err := buf.WriteByte(0); err != nil {
			return nil, fmt.Errorf("failed to write padding byte: %w", err)
		}
	}
	return buf.Bytes(), nil
}

type ipv4PrefixPDU struct {
	/*
		0          8          16         24        31
		.-------------------------------------------.
		| Protocol |   PDU    |                     |
		| Version  |   Type   |         zero        |
		|    X     |    4     |                     |
		+-------------------------------------------+
		|                                           |
		|                 Length=20                 |
		|                                           |
		+-------------------------------------------+
		|          |  Prefix  |   Max    |          |
		|  Flags   |  Length  |  Length  |   zero   |
		|          |   0..32  |   0..32  |          |
		+-------------------------------------------+
		|                                           |
		|                IPv4 Prefix                |
		|                                           |
		+-------------------------------------------+
		|                                           |
		|         Autonomous System Number          |
		|                                           |
		`-------------------------------------------'
	*/
	Flags  uint8
	Min    uint8
	Max    uint8
	Prefix [4]byte
	Asn    uint32
}

type ipv6PrefixPDU struct {
	/*
		0          8          16         24        31
		.-------------------------------------------.
		| Protocol |   PDU    |                     |
		| Version  |   Type   |         zero        |
		|    X     |    6     |                     |
		+-------------------------------------------+
		|                                           |
		|                 Length=32                 |
		|                                           |
		+-------------------------------------------+
		|          |  Prefix  |   Max    |          |
		|  Flags   |  Length  |  Length  |   zero   |
		|          |  0..128  |  0..128  |          |
		+-------------------------------------------+
		|                                           |
		+---                                     ---+
		|                                           |
		+---            IPv6 Prefix              ---+
		|                                           |
		+---                                     ---+
		|                                           |
		+-------------------------------------------+
		|                                           |
		|         Autonomous System Number          |
		|                                           |
		`-------------------------------------------'
	*/
	Flags  uint8
	Min    uint8
	Max    uint8
	Prefix [16]byte
	Asn    uint32
}

type endOfDataPDU struct {
	/*
		0          8          16         24        31
		.-------------------------------------------.
		| Protocol |   PDU    |                     |
		| Version  |   Type   |     Session ID      |
		|    X     |    7     |                     |
		+-------------------------------------------+
		|                                           |
		|                 Length=24                 |
		|                                           |
		+-------------------------------------------+
		|                                           |
		|               Serial Number               |
		|                                           |
		+-------------------------------------------+
		|                                           |
		|              Refresh Interval             |
		|                                           |
		+-------------------------------------------+
		|                                           |
		|               Retry Interval              |
		|                                           |
		+-------------------------------------------+
		|                                           |
		|              Expire Interval              |
		|                                           |
		`-------------------------------------------'
	*/
	Session uint16
	Serial  uint32
	Refresh uint32
	Retry   uint32
	Expire  uint32
}

type cacheResetPDU struct {
	/*
		0          8          16         24        31
		.-------------------------------------------.
		| Protocol |   PDU    |                     |
		| Version  |   Type   |         zero        |
		|    X     |    8     |                     |
		+-------------------------------------------+
		|                                           |
		|                 Length=8                  |
		|                                           |
		`-------------------------------------------'
	*/
}

type routerKeyPDU struct {
	/*
		   	0          8          16         24        31
			.-------------------------------------------.
			| Protocol |   PDU    |                     |
			| Version  |   Type   |     Session ID      |
			|    X     |    9     |                     |
			+-------------------------------------------+
			|                                           |
			|                  Length                   |
			|                                           |
			+-------------------------------------------+
			|                                           |
		    +---                                     ---+
		    |          Subject Key Identifier           |
			+---                                     ---+
			|                                           |
			+---                                     ---+
			|                (20 octets)                |
			+---                                     ---+
			|                                           |
			+-------------------------------------------+
			|                                           |
			|                 AS Number                 |
			|                                           |
			+-------------------------------------------+
			|                                           |
			~          Subject Public Key Info          ~
			|                                           |
			`-------------------------------------------'
	*/
}

type errorReportPDU struct {
	/*
		0          8          16         24        31
		.-------------------------------------------.
		| Protocol |   PDU    |                     |
		| Version  |   Type   |     Error Code      |
		|    X     |    10    |                     |
		+-------------------------------------------+
		|                                           |
		|                  Length                   |
		|                                           |
		+-------------------------------------------+
		|                                           |
		|       Length of Encapsulated PDU          |
		|                                           |
		+-------------------------------------------+
		|                                           |
		~               Erroneous PDU               ~
		|                                           |
		+-------------------------------------------+
		|                                           |
		|           Length of Error Text            |
		|                                           |
		+-------------------------------------------+
		|                                           |
		|              Arbitrary Text               |
		|                    of                     |
		~          Error Diagnostic Message         ~
		|                                           |
		`-------------------------------------------'
	*/
	Code   uint16
	Report string
	Pdu    []byte
}

type aspaPDU struct {
	/*
	   0          8          16         24        31
	   .-------------------------------------------.
	   | Protocol |   PDU    |          |          |
	   | Version  |   Type   |   Flags  |   zero   |
	   |    2     |    11    |          |          |
	   +-------------------------------------------+
	   |                                           |
	   |                 Length                    |
	   |                                           |
	   +-------------------------------------------+
	   |                                           |
	   |    Customer Autonomous System Number      |
	   |                                           |
	   +-------------------------------------------+
	   |                                           |
	   ~    Provider Autonomous System Numbers     ~
	   |                                           |
	   `-------------------------------------------'
	*/
}

// Marshal encodes the PDU into bytes
func (p *PDU) Marshal() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write PDU type (1 byte)
	if err := binary.Write(buf, binary.BigEndian, p.Type); err != nil {
		return nil, fmt.Errorf("failed to write PDU type: %w", err)
	}

	// Write payload length (2 bytes, max 65535)
	payloadLen := uint16(len(p.Payload))
	if err := binary.Write(buf, binary.BigEndian, payloadLen); err != nil {
		return nil, fmt.Errorf("failed to write payload length: %w", err)
	}

	// Write payload
	if payloadLen > 0 {
		if _, err := buf.Write(p.Payload); err != nil {
			return nil, fmt.Errorf("failed to write payload: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// GetPDU reads from the provided io.Reader and returns a PDU.
func GetPDU(r io.Reader) (*PDU, error) {
	bytes, err := getPDUBytes(r)
	if err != nil {
		return nil, fmt.Errorf("failed to get PDU bytes: %w", err)
	}
	pdu, err := unmarshal(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal PDU: %w", err)
	}
	return pdu, nil
}

// getPDUBytes will return a byte slice which contains a PDU.
func getPDUBytes(r io.Reader) ([]byte, error) {
	/*
		0          8          16         24        31
		.-------------------------------------------.
		| Protocol |   PDU    |                     |
		| Version  |   Type   |     Session ID      |
		+-------------------------------------------+
		|                                           |
		|                 Length                    |
		|                                           |
		`-------------------------------------------'
	*/
	buf := make([]byte, minPDULength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	// Read the rest of the PDU, minus the header.
	length := binary.BigEndian.Uint32(buf[4:8]) - 8
	if length > 0 {
		lr := io.LimitReader(r, int64(length))
		data := make([]byte, length)
		if _, err := io.ReadFull(lr, data); err != nil {
			return nil, err
		}
		buf = append(buf, data...)
	}
	return buf, nil
}

// unmarshal parses bytes into a PDU struct
func unmarshal(data []byte) (*PDU, error) {
	if len(data) < 3 {
		return nil, fmt.Errorf("data too short to unmarshal PDU")
	}

	p := &PDU{}

	p.Type = PDUType(data[0])

	payloadLen := binary.BigEndian.Uint16(data[1:3])

	if len(data) < int(3+payloadLen) {
		return nil, fmt.Errorf("payload length mismatch")
	}

	if payloadLen > 0 {
		p.Payload = data[3 : 3+payloadLen]
	} else {
		p.Payload = nil
	}

	return p, nil
}
