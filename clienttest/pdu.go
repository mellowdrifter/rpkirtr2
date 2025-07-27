package clienttest

import (
	"bytes"
	"encoding/binary"
)

const (
	SerialQuery = 1
	ResetQuery  = 2
)

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
	binary.Write(buf, binary.BigEndian, uint8(0))  // Invalid version
	binary.Write(buf, binary.BigEndian, uint8(99)) // Unknown PDU type
	binary.Write(buf, binary.BigEndian, uint16(0))
	binary.Write(buf, binary.BigEndian, uint32(4)) // Invalid length
	return buf.Bytes()
}
