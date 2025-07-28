package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
)

func writeFull(w io.Writer, buf []byte) error {
	total := 0
	for total < len(buf) {
		n, err := w.Write(buf[total:])
		if err != nil {
			return fmt.Errorf("write error after %d bytes (wanted %d): %w", total, len(buf), err)
		}
		if n == 0 {
			return fmt.Errorf("short write: wrote 0 bytes after %d", total)
		}
		total += n
	}
	return nil
}

func (s *SerialNotifyPDU) Write(w io.Writer) error {
	buf := make([]byte, 12) // fixed-size PDU

	buf[0] = byte(s.version)
	buf[1] = byte(s.ptype)
	binary.BigEndian.PutUint16(buf[2:], s.session)
	binary.BigEndian.PutUint32(buf[4:], s.length)
	binary.BigEndian.PutUint32(buf[8:], s.serial)

	if err := writeFull(w, buf); err != nil {
		return fmt.Errorf("failed to write SerialNotifyPDU: %w", err)
	}
	return nil
}

func (s *SerialQueryPDU) Write(w io.Writer) error {
	buf := make([]byte, 12) // fixed-size PDU

	buf[0] = byte(s.version)
	buf[1] = byte(s.ptype)
	binary.BigEndian.PutUint16(buf[2:], s.session)
	binary.BigEndian.PutUint32(buf[4:], s.length)
	binary.BigEndian.PutUint32(buf[8:], s.serial)
	if err := writeFull(w, buf); err != nil {
		return fmt.Errorf("failed to write SerialQueryPDU: %w", err)
	}
	return nil
}

func (r *ResetQueryPDU) Write(w io.Writer) error {
	buf := make([]byte, 8) // fixed-size PDU

	buf[0] = byte(r.version)
	buf[1] = byte(r.ptype)
	buf[2] = r.zero
	binary.BigEndian.PutUint32(buf[4:], r.length)

	if err := writeFull(w, buf); err != nil {
		return fmt.Errorf("failed to write ResetQueryPDU: %w", err)
	}
	return nil
}

func (c *CacheResponsePDU) Write(w io.Writer) error {
	buf := make([]byte, 8) // fixed-size PDU

	buf[0] = byte(c.version)
	buf[1] = byte(c.ptype)
	binary.BigEndian.PutUint16(buf[2:], c.session)
	binary.BigEndian.PutUint32(buf[4:], c.length)

	if err := writeFull(w, buf); err != nil {
		return fmt.Errorf("failed to write CacheResponsePDU: %w", err)
	}
	return nil
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

	if err := writeFull(w, buf); err != nil {
		log.Printf("Failed to write Ipv4PrefixPDU: %v", buf)
		return fmt.Errorf("failed to write Ipv4PrefixPDU: %w", err)
	}
	return nil
}

func (i *Ipv6PrefixPDU) Write(w io.Writer) error {
	buf := make([]byte, 32) // fixed-size PDU
	buf[0] = byte(i.version)
	buf[1] = byte(i.ptype)
	binary.BigEndian.PutUint16(buf[2:], i.zero1)
	binary.BigEndian.PutUint32(buf[4:], 32) // length of the PDU
	buf[8] = i.flags
	buf[9] = i.min
	buf[10] = i.max
	buf[11] = i.zero2
	copy(buf[12:28], i.prefix[:])               // 16 bytes for IPv6 prefix
	binary.BigEndian.PutUint32(buf[28:], i.asn) // 4 bytes for AS Number

	if err := writeFull(w, buf); err != nil {
		return fmt.Errorf("failed to write Ipv6PrefixPDU: %w", err)
	}
	return nil
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

	if err := writeFull(w, buf); err != nil {
		return fmt.Errorf("failed to write EndOfDataPDU: %w", err)
	}
	return nil
}

func (c *cacheResetPDU) Write(w io.Writer) error {
	buf := make([]byte, 8) // fixed-size PDU

	buf[0] = byte(c.version)
	buf[1] = byte(c.ptype)
	binary.BigEndian.PutUint16(buf[2:], c.zero)
	binary.BigEndian.PutUint32(buf[4:], c.length)

	if err := writeFull(w, buf); err != nil {
		return fmt.Errorf("failed to write cacheResetPDU: %w", err)
	}
	return nil
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

	if err := writeFull(w, buf); err != nil {
		return fmt.Errorf("failed to write RouterKeyPDU: %w", err)
	}
	return nil
}

func (e *ErrorReportPDU) Write(w io.Writer) error {
	// Validate lengths to avoid panics
	if int(e.pduLen) > len(e.pdu) {
		return fmt.Errorf("ErrorReportPDU.Write: pduLen (%d) exceeds pdu size (%d)", e.pduLen, len(e.pdu))
	}
	if int(e.textLen) > len(e.text) {
		return fmt.Errorf("ErrorReportPDU.Write: textLen (%d) exceeds text size (%d)", e.textLen, len(e.text))
	}

	bufLen := 12 + int(e.pduLen) + 4 + int(e.textLen) // 4 bytes for textLen field
	buf := make([]byte, bufLen)

	buf[0] = byte(e.verion)
	buf[1] = byte(e.ptype)
	binary.BigEndian.PutUint16(buf[2:], uint16(e.code))
	binary.BigEndian.PutUint32(buf[4:], uint32(bufLen))
	binary.BigEndian.PutUint32(buf[8:], e.pduLen)

	copy(buf[12:12+e.pduLen], e.pdu[:e.pduLen])

	offset := 12 + int(e.pduLen)
	binary.BigEndian.PutUint32(buf[offset:], e.textLen)
	copy(buf[offset+4:], e.text[:e.textLen])

	if err := writeFull(w, buf); err != nil {
		return fmt.Errorf("failed to write ErrorReportPDU: %w", err)
	}
	return nil
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

	if err := writeFull(w, buf); err != nil {
		return fmt.Errorf("failed to write AspaPDU: %w", err)
	}
	return nil
}
