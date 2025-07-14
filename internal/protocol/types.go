package protocol

type PDUType uint8
type Version uint8
type Flags uint8
type ErrorCode uint16

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

	// Error codes
	// https://www.ietf.org/archive/id/draft-ietf-sidrops-8210bis-21.html#name-error-codes
	CorruptData        ErrorCode = 0
	InternalError      ErrorCode = 1
	NoData             ErrorCode = 2
	InvalidRequest     ErrorCode = 3
	UnsupportedVersion ErrorCode = 4
	UnsupportedPDU     ErrorCode = 5
	UnknownWithdrawal  ErrorCode = 6
	Duplicate          ErrorCode = 7
	UnexpectedVersion  ErrorCode = 8
	ASPAListError      ErrorCode = 9
	TransportError     ErrorCode = 10

	// lengths
	minPDULength        = 8
	maxPDULength        = 65535
	headPDULength       = 2
	serialNotifyLength  = 12
	serialQueryLength   = 12
	resetQueryLength    = 8
	cacheResponseLength = 8
	ipv4Length          = 20
	ipv6Length          = 32
	EndOfDataLength     = 24
	cacheResetLength    = 8

	// flags
	Withdraw uint8 = 0
	Announce uint8 = 1
)
