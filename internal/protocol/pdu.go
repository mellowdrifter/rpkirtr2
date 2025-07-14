package protocol

import (
	"io"
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
		length:  serialNotifyLength,
		serial:  serial,
	}
}

func (s *SerialNotifyPDU) Type() PDUType {
	return s.ptype
}

func (s *SerialNotifyPDU) Serial() uint32 {
	return s.serial
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
		length:  serialQueryLength,
		serial:  serial,
	}
}

func (s *SerialQueryPDU) Type() PDUType {
	return s.ptype
}

func (s *SerialQueryPDU) Serial() uint32 {
	return s.serial
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
		length:  resetQueryLength,
	}
}

func (r *ResetQueryPDU) Type() PDUType {
	return r.ptype
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
		length:  cacheResponseLength,
	}
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
		length:  ipv4Length,
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
	length  uint32
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
		length:  ipv6Length,
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
		length:  EndOfDataLength,
		serial:  serial,
		refresh: refresh,
		retry:   retry,
		expire:  expire,
	}
}

func (e *EndOfDataPDU) Type() PDUType {
	return e.ptype
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
		length:  cacheResetLength,
	}
}

func (c *cacheResetPDU) Type() PDUType {
	return c.ptype
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

func NewErrorReportPDU(ver Version, code uint16, offendingPDU []byte, text string) *ErrorReportPDU {
	pduLen := uint32(len(offendingPDU))
	textBytes := []byte(text)
	textLen := uint32(len(textBytes))
	totalLen := 12 + len(offendingPDU) + 4 + len(textBytes)

	return &ErrorReportPDU{
		verion:  ver,
		ptype:   ErrorReport,
		code:    code,
		length:  uint32(totalLen),
		pduLen:  pduLen,
		pdu:     offendingPDU,
		textLen: textLen,
		text:    textBytes,
	}
}

func (e *ErrorReportPDU) Type() PDUType {
	return e.ptype
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

func NewAspaPDU(ver Version, flags uint8, casn uint32, pasn []uint32) *AspaPDU {
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
