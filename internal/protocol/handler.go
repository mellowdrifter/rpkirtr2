package protocol

import (
	"bufio"
	"fmt"
	"strings"
)

// ProtocolHandler handles all protocol versions (v1, v2).
type ProtocolHandler struct {
	version Version
}

// NewProtocolHandler creates a new handler for the given version.
func NewProtocolHandler(version Version) *ProtocolHandler {
	return &ProtocolHandler{version: version}
}

// SendInitialMessages sends the initial PDUs after negotiation.
func (h *ProtocolHandler) SendInitialMessages(w *bufio.Writer) error {
	pdu := &GenericPDU{
		VersionField: h.version,
		TypeField:    PDUTypeHello,
		Payload:      []byte("Welcome! Using protocol " + h.version),
	}
	return MarshalAndSend(pdu, w)
}

// ReadMessage reads and parses a PDU from the client.
func (h *ProtocolHandler) ReadMessage(r *bufio.Reader) (PDU, error) {
	pdu, err := ReadRawPDU(r)
	if err != nil {
		return nil, err
	}

	// Accept only supported versions
	if pdu.Version() != "v1" && pdu.Version() != "v2" {
		return nil, fmt.Errorf("unsupported protocol version: %s", pdu.Version())
	}

	return pdu, nil
}

// HandleMessage processes the PDU and optionally sends response(s).
func (h *ProtocolHandler) HandleMessage(pdu PDU, w *bufio.Writer) error {
	switch pdu.Type() {
	case PDUTypeHello:
		// Example: Echo back a hello ack
		resp := &GenericPDU{
			VersionField: h.version,
			TypeField:    PDUTypeAck,
			Payload:      []byte("Hello received"),
		}
		return MarshalAndSend(resp, w)

	case PDUTypeData:
		// Process data (example: just acknowledge)
		resp := &GenericPDU{
			VersionField: h.version,
			TypeField:    PDUTypeAck,
			Payload:      []byte("Data received"),
		}
		return MarshalAndSend(resp, w)

	case PDUTypeError:
		// Log or handle error PDUs if necessary
		return fmt.Errorf("received error PDU: %s", strings.TrimSpace(string(pdu.(*GenericPDU).Payload)))

	default:
		return fmt.Errorf("unknown PDU type: %d", pdu.Type())
	}
}
