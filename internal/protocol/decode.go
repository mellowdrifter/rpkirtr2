package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

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
		return nil, fmt.Errorf("failed to read PDU header: %w", err)
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
	if len(data) < 2 {
		return nil, fmt.Errorf("data too short to contain PDU type: %d bytes", len(data))
	}

	ptype := PDUType(data[1])

	switch ptype {

	// SerialQuery asks for diffs of ROAs from last serial number.
	case SerialQuery:
		if len(data) < 12 {
			return nil, fmt.Errorf("SerialQueryPDU too short: %d bytes", len(data))
		}
		sqPDU := NewSerialQueryPDU(
			Version(data[0]),
			binary.BigEndian.Uint16(data[2:4]),
			binary.BigEndian.Uint32(data[8:12]),
		)
		return sqPDU, nil

	// ResetQuery asks for all ROAs.
	case ResetQuery:
		if len(data) < 8 {
			return nil, fmt.Errorf("ResetQueryPDU too short: %d bytes", len(data))
		}
		rqPDU := NewResetQueryPDU(
			Version(data[0]),
		)
		return rqPDU, nil

	case ErrorReport:
		if len(data) < 12 {
			return nil, fmt.Errorf("ErrorReportPDU too short: %d bytes", len(data))
		}
		pduLen := binary.BigEndian.Uint32(data[8:12])

		// Check pduLen does not cause overflow or slice bounds error
		if pduLen > uint32(len(data)) || int(12+pduLen+4) > len(data) {
			return nil, fmt.Errorf("ErrorReportPDU invalid pduLen: %d", pduLen)
		}

		textLen := binary.BigEndian.Uint32(data[12+pduLen : 12+pduLen+4])

		if textLen > uint32(len(data)) || int(12+pduLen+4+textLen) > len(data) {
			return nil, fmt.Errorf("ErrorReportPDU invalid textLen: %d", textLen)
		}

		return &ErrorReportPDU{
			verion:  Version(data[0]),
			ptype:   ptype,
			code:    binary.BigEndian.Uint16(data[2:4]),
			length:  binary.BigEndian.Uint32(data[4:8]),
			pduLen:  pduLen,
			pdu:     data[12 : 12+pduLen],
			textLen: textLen,
			text:    data[12+pduLen+4 : 12+pduLen+4+textLen],
		}, nil

		// Cache server should only ever receive the above three PDUs.
	default:
		return nil, fmt.Errorf("unsupported PDU type: %d", ptype)
	}
}
