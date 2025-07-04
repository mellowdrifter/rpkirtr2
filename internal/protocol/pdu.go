package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// PDUType represents the type of PDU message
type PDUType uint8

const (
	PDUTypeHello PDUType = iota + 1
	PDUTypeVersionNegotiation
	PDUTypeData
	PDUTypeAck
	PDUTypeError
	// Add other PDU types here
)

// PDU represents a generic protocol data unit
type PDU struct {
	Type    PDUType
	Payload []byte
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

// Unmarshal parses bytes into a PDU struct
func Unmarshal(data []byte) (*PDU, error) {
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
