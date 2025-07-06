package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

type PDUType uint8

type Version uint8

const (
	// PDU Types
	SerialNotify  PDUType = 0
	SerialQuery   PDUType = 1
	ResetQuery    PDUType = 2
	CacheResponse PDUType = 3
	Ipv4Prefix    PDUType = 4
	Ipv6Prefix    PDUType = 6
	EndOfData     PDUType = 7
	CacheReset    PDUType = 8
	RouterKey     PDUType = 9
	ErrorReport   PDUType = 10
	Aspa          PDUType = 11

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
	Write(w io.Writer) error
}

type SerialNotifyPDU struct {
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
	version Version
	ptype   PDUType
	session uint16
	length  uint32
	serial  uint32
}

func NewSerialNotifyPDU(ver Version, session uint16, serial uint32) *SerialNotifyPDU {
	return &SerialNotifyPDU{
		version: ver,
		ptype:   SerialNotify,
		session: session,
		length:  12,
		serial:  serial,
	}
}

func (s *SerialNotifyPDU) Type() PDUType {
	return s.ptype
}

func (s *SerialNotifyPDU) Write(w io.Writer) error {
	buf := make([]byte, 12) // fixed-size PDU

	buf[0] = byte(s.version)
	buf[1] = byte(s.ptype)
	binary.BigEndian.PutUint16(buf[2:], s.session)
	binary.BigEndian.PutUint32(buf[4:], s.length)
	binary.BigEndian.PutUint32(buf[8:], s.serial)

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write SerialNotifyPDU: %w", err)
	}
	return nil
}

type SerialQueryPDU struct {
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
	version Version
	ptype   PDUType
	session uint16
	length  uint32
	serial  uint32
}

func NewSerialQueryPDU(ver Version, session uint16, serial uint32) *SerialQueryPDU {
	return &SerialQueryPDU{
		version: ver,
		ptype:   SerialQuery,
		session: session,
		length:  12,
		serial:  serial,
	}
}

func (s *SerialQueryPDU) Type() PDUType {
	return s.ptype
}

func (s *SerialQueryPDU) Write(w io.Writer) error {
	buf := make([]byte, 12) // fixed-size PDU

	buf[0] = byte(s.version)
	buf[1] = byte(s.ptype)
	binary.BigEndian.PutUint16(buf[2:], s.session)
	binary.BigEndian.PutUint32(buf[4:], s.length)
	binary.BigEndian.PutUint32(buf[8:], s.serial)
	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write SerialQueryPDU: %w", err)
	}
	return nil
}

type ResetQueryPDU struct {
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
	version Version
	ptype   PDUType
	zero    uint8
	length  uint32
}

func NewResetQueryPDU(ver Version) *ResetQueryPDU {
	return &ResetQueryPDU{
		version: ver,
		ptype:   ResetQuery,
		zero:    0,
		length:  8,
	}
}

func (r *ResetQueryPDU) Type() PDUType {
	return r.ptype
}

func (r *ResetQueryPDU) Write(w io.Writer) error {
	buf := make([]byte, 8) // fixed-size PDU

	buf[0] = byte(r.version)
	buf[1] = byte(r.ptype)
	buf[2] = r.zero
	binary.BigEndian.PutUint32(buf[4:], r.length)

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write ResetQueryPDU: %w", err)
	}
	return nil
}

type CacheResponsePDU struct {
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
	version Version
	ptype   PDUType
	session uint16
	length  uint32
}

func (c *CacheResponsePDU) Type() PDUType {
	return c.ptype
}

func NewCacheResponsePDU(ver Version, session uint16) *CacheResponsePDU {
	return &CacheResponsePDU{
		version: ver,
		ptype:   CacheResponse,
		session: session,
		length:  8, // fixed size for this PDU
	}
}

func (c *CacheResponsePDU) Write(w io.Writer) error {
	buf := make([]byte, 8) // fixed-size PDU

	buf[0] = byte(c.version)
	buf[1] = byte(c.ptype)
	binary.BigEndian.PutUint16(buf[2:], c.session)
	binary.BigEndian.PutUint32(buf[4:], c.length)

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write CacheResponsePDU: %w", err)
	}
	return nil
}

type Ipv4PrefixPDU struct {
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
	version Version
	ptype   PDUType
	zero1   uint16
	length  uint32
	flags   uint8
	min     uint8
	max     uint8
	zero2   uint8
	prefix  [4]byte
	asn     uint32
}

func NewIpv4PrefixPDU(ver Version, flags, min, max uint8, prefix [4]byte, asn uint32) *Ipv4PrefixPDU {
	return &Ipv4PrefixPDU{
		version: ver,
		ptype:   Ipv4Prefix,
		zero1:   0,
		length:  20,
		flags:   flags,
		min:     min,
		max:     max,
		zero2:   0,
		prefix:  prefix,
		asn:     asn,
	}
}

func (i *Ipv4PrefixPDU) Type() PDUType {
	return i.ptype
}

func (i *Ipv4PrefixPDU) Write(w io.Writer) error {
	buf := make([]byte, 20) // fixed-size PDU

	buf[0] = byte(i.version)
	buf[1] = byte(i.ptype)
	binary.BigEndian.PutUint16(buf[2:], i.zero1)
	binary.BigEndian.PutUint32(buf[4:], i.length)
	buf[8] = i.flags
	buf[9] = i.min
	buf[10] = i.max
	buf[11] = i.zero2
	copy(buf[12:16], i.prefix[:])
	binary.BigEndian.PutUint32(buf[16:], i.asn)

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write Ipv4PrefixPDU: %w", err)
	}
	return nil
}

type Ipv6PrefixPDU struct {
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
	version Version
	ptype   PDUType
	zero1   uint16
	flags   uint8
	min     uint8
	max     uint8
	zero2   uint8
	prefix  [16]byte
	asn     uint32
}

func NewIpv6PrefixPDU(ver Version, flags, min, max uint8, prefix [16]byte, asn uint32) *Ipv6PrefixPDU {
	return &Ipv6PrefixPDU{
		version: ver,
		ptype:   Ipv6Prefix,
		zero1:   0,
		flags:   flags,
		min:     min,
		max:     max,
		zero2:   0,
		prefix:  prefix,
		asn:     asn,
	}
}

func (i *Ipv6PrefixPDU) Type() PDUType {
	return i.ptype
}

type EndOfDataPDU struct {
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
	version Version
	ptype   PDUType
	session uint16
	length  uint32
	serial  uint32
	refresh uint32
	retry   uint32
	expire  uint32
}

func NewEndOfDataPDU(ver Version, session uint16, serial, refresh, retry, expire uint32) *EndOfDataPDU {
	return &EndOfDataPDU{
		version: ver,
		ptype:   EndOfData,
		session: session,
		length:  24,
		serial:  serial,
		refresh: refresh,
		retry:   retry,
		expire:  expire,
	}
}

func (e *EndOfDataPDU) Type() PDUType {
	return e.ptype
}

func (e *EndOfDataPDU) Write(w io.Writer) error {
	buf := make([]byte, 24) // fixed-size PDU

	buf[0] = byte(e.version)
	buf[1] = byte(e.ptype)
	binary.BigEndian.PutUint16(buf[2:], e.session)
	binary.BigEndian.PutUint32(buf[4:], e.length)
	binary.BigEndian.PutUint32(buf[8:], e.serial)
	binary.BigEndian.PutUint32(buf[12:], e.refresh)
	binary.BigEndian.PutUint32(buf[16:], e.retry)
	binary.BigEndian.PutUint32(buf[20:], e.expire)

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write EndOfDataPDU: %w", err)
	}
	return nil
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

	version Version
	ptype   PDUType
	zero    uint16
	length  uint32
}

func NewCacheResetPDU(ver Version) *cacheResetPDU {
	return &cacheResetPDU{
		version: ver,
		ptype:   CacheReset,
		zero:    0,
		length:  8,
	}
}

func (c *cacheResetPDU) Type() PDUType {
	return c.ptype
}

func (c *cacheResetPDU) Write(w io.Writer) error {
	buf := make([]byte, 8) // fixed-size PDU

	buf[0] = byte(c.version)
	buf[1] = byte(c.ptype)
	binary.BigEndian.PutUint16(buf[2:], c.zero)
	binary.BigEndian.PutUint32(buf[4:], c.length)

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write cacheResetPDU: %w", err)
	}
	return nil
}

type RouterKeyPDU struct {
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
	version Version
	ptype   PDUType
	session uint16
	length  uint32
	ski     [20]byte // Subject Key Identifier
	asn     uint32   // Autonomous System Number
	skiInfo []byte   // Subject Public Key Info, variable length
}

func NewRouterKeyPDU(ver Version, session uint16, ski [20]byte, asn uint32, skiInfo []byte) *RouterKeyPDU {
	return &RouterKeyPDU{
		version: ver,
		ptype:   RouterKey,
		session: session,
		length:  uint32(24 + len(skiInfo)), // 24 bytes for header and SKI, plus variable length for skiInfo
		ski:     ski,
		asn:     asn,
		skiInfo: skiInfo,
	}

}

func (r *RouterKeyPDU) Type() PDUType {
	return r.ptype
}

func (r *RouterKeyPDU) Write(w io.Writer) error {
	buf := make([]byte, 24+len(r.skiInfo)) // fixed-size PDU

	buf[0] = byte(r.version)
	buf[1] = byte(r.ptype)
	binary.BigEndian.PutUint16(buf[2:], r.session)
	binary.BigEndian.PutUint32(buf[4:], r.length)
	copy(buf[8:28], r.ski[:])                   // 20 bytes for SKI
	binary.BigEndian.PutUint32(buf[28:], r.asn) // 4 bytes for AS Number
	if len(r.skiInfo) > 0 {
		copy(buf[32:], r.skiInfo) // variable length for Subject Public Key Info
	}
	_, err := w.Write(buf)

	if err != nil {
		return fmt.Errorf("failed to write RouterKeyPDU: %w", err)
	}
	return nil
}

type ErrorReportPDU struct {
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
	verion  Version
	ptype   PDUType
	code    uint16
	length  uint32
	pduLen  uint32 // Length of the encapsulated PDU
	pdu     []byte // Erroneous PDU
	textLen uint32 // Length of the error text
	text    []byte // Arbitrary text of the error diagnostic message
}

func NewErrorReportPDU(ver Version, code uint16, pdu []byte, text []byte) *ErrorReportPDU {
	return &ErrorReportPDU{
		verion:  ver,
		ptype:   ErrorReport,
		code:    code,
		length:  uint32(12 + len(pdu) + len(text)), // 12 bytes for header and lengths
		pduLen:  uint32(len(pdu)),
		pdu:     pdu,
		textLen: uint32(len(text)),
		text:    text,
	}
}

func (e *ErrorReportPDU) Type() PDUType {
	return e.ptype
}

func (e *ErrorReportPDU) Write(w io.Writer) error {
	buf := make([]byte, 12+len(e.pdu)+len(e.text)) // fixed-size PDU

	buf[0] = byte(e.verion)
	buf[1] = byte(e.ptype)
	binary.BigEndian.PutUint16(buf[2:], e.code)
	binary.BigEndian.PutUint32(buf[4:], e.length)
	binary.BigEndian.PutUint32(buf[8:], e.pduLen)
	if len(e.pdu) > 0 {
		copy(buf[12:12+len(e.pdu)], e.pdu) // copy erroneous PDU
	}
	if len(e.text) > 0 {
		binary.BigEndian.PutUint32(buf[12+len(e.pdu):], e.textLen) // length of error text
		copy(buf[16+len(e.pdu):16+len(e.pdu)+len(e.text)], e.text) // copy error text
	}

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write ErrorReportPDU: %w", err)
	}
	return nil
}

type AspaPDU struct {
	/*
	   0          8          16         24        31
	   .-------------------------------------------.
	   | Protocol |   PDU    |          |          |
	   | Version  |   Type   |   Flags  |   zero   |
	   |    x     |    11    |          |          |
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
	version Version
	ptype   PDUType
	flags   uint8    // Flags for the PDU
	zero    uint8    // Reserved, should be zero
	length  uint32   // Total length of the PDU
	casn    uint32   // Customer Autonomous System Number
	pasn    []uint32 // Provider Autonomous System Numbers, variable length
}

func NewASPA(ver Version, flags uint8, casn uint32, pasn []uint32) *AspaPDU {
	return &AspaPDU{
		version: ver,
		ptype:   Aspa,
		flags:   flags,
		zero:    0,
		length:  uint32(12 + len(pasn)*4), // 12 bytes
		casn:    casn,
		pasn:    pasn,
	}
}

func (a *AspaPDU) Type() PDUType {
	return a.ptype
}

func (a *AspaPDU) Write(w io.Writer) error {
	buf := make([]byte, 12+len(a.pasn)*4) // fixed-size PDU

	buf[0] = byte(a.version)
	buf[1] = byte(a.ptype)
	buf[2] = a.flags
	buf[3] = a.zero
	binary.BigEndian.PutUint32(buf[4:], a.length)
	binary.BigEndian.PutUint32(buf[8:], a.casn)
	for i, pasn := range a.pasn {
		binary.BigEndian.PutUint32(buf[12+i*4:], pasn) // 4 bytes for each Provider AS Number
	}

	_, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("failed to write AspaPDU: %w", err)
	}
	return nil
}

// GetPDU reads from the provided io.Reader and returns a PDU.
func GetPDU(r io.Reader) (PDU, error) {
	bytes, err := getPDUBytes(r)
	if err != nil {
		return nil, fmt.Errorf("failed to get PDU bytes: %w", err)
	}
	pdu, err := decipherPDU(bytes)
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
	// Read the first 8 bytes to get the PDU header
	buf := make([]byte, minPDULength)
	if _, err := io.ReadFull(r, buf); err != nil {
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, fmt.Errorf("failed to read PDU header: %w", err)
		}
	}

	// Check the full length of the PDU
	length := binary.BigEndian.Uint32(buf[4:8])
	if length < minPDULength || length > maxPDULength {
		return nil, fmt.Errorf("invalid PDU length: %d", length)
	}

	// If there is payload, read it
	payloadLen := int(length) - minPDULength
	if payloadLen > 0 {
		data := make([]byte, payloadLen)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, fmt.Errorf("failed to read PDU payload: %w", err)
		}
		buf = append(buf, data...)
	}

	return buf, nil

}

func decipherPDU(data []byte) (PDU, error) {

	ptype := PDUType(data[1])

	switch ptype {
	case SerialNotify:
		if len(data) < 12 {
			return nil, fmt.Errorf("SerialNotifyPDU too short: %d bytes", len(data))
		}
		return &SerialNotifyPDU{
			version: Version(data[0]),
			ptype:   ptype,
			session: binary.BigEndian.Uint16(data[2:4]),
			length:  binary.BigEndian.Uint32(data[4:8]),
			serial:  binary.BigEndian.Uint32(data[8:12]),
		}, nil

	case SerialQuery:
		if len(data) < 12 {
			return nil, fmt.Errorf("SerialQueryPDU too short: %d bytes", len(data))
		}
		return &SerialQueryPDU{
			version: Version(data[0]),
			ptype:   ptype,
			session: binary.BigEndian.Uint16(data[2:4]),
			length:  binary.BigEndian.Uint32(data[4:8]),
			serial:  binary.BigEndian.Uint32(data[8:12]),
		}, nil

	case ErrorReport:
		if len(data) < 12 {
			return nil, fmt.Errorf("ErrorReportPDU too short: %d bytes", len(data))
		}
		pduLen := binary.BigEndian.Uint32(data[8:12])
		textLen := binary.BigEndian.Uint32(data[12+pduLen:])
		if len(data) < int(12+pduLen+textLen) {
			return nil, fmt.Errorf("ErrorReportPDU too short for pdu and text: %d bytes", len(data))
		}
		return &ErrorReportPDU{
			verion:  Version(data[0]),
			ptype:   ptype,
			code:    binary.BigEndian.Uint16(data[2:4]),
			length:  binary.BigEndian.Uint32(data[4:8]),
			pduLen:  pduLen,
			pdu:     data[12 : 12+pduLen],
			textLen: textLen,
			text:    data[12+pduLen : 12+pduLen+textLen],
		}, nil

		// Cache server should only ever receive the above three PDUs.
	default:
		return nil, fmt.Errorf("unsupported PDU type: %d", ptype)
	}
}
