package protocol

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func FuzzDecipherPDU(f *testing.F) {
	// Add a few valid seed inputs (optional but helps fuzzing)
	f.Add([]byte{
		1, byte(SerialNotify),
		0, 1, // session
		0, 0, 0, 12, // length
		0, 0, 0, 42, // serial
	})
	f.Add([]byte{
		1, byte(SerialQuery),
		0, 1,
		0, 0, 0, 12,
		0, 0, 0, 99,
	})
	// Aspa PDU - valid
	f.Add([]byte{
		1, byte(Aspa),
		1, 0, // flags (announce), zero
		0, 0, 0, 16, // length (8+4+4)
		0, 0, 4, 210, // casn 1234
		0, 0, 0, 100, // pasn 100
	})
	// Aspa PDU - 0 providers
	f.Add([]byte{
		1, byte(Aspa),
		1, 0,
		0, 0, 0, 12,
		0, 0, 4, 210,
	})
	// Aspa PDU - mismatched length (declared length > actual bytes)
	f.Add([]byte{
		1, byte(Aspa),
		1, 0,
		0, 0, 0, 20, // declared 20, but only 16 bytes provided
		0, 0, 4, 210,
		0, 0, 0, 100,
	})
	// RouterKey PDU
	f.Add([]byte{
		1, byte(RouterKey),
		0, 1, // session
		0, 0, 0, 32, // length
		0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, // ski
		0, 0, 4, 210, // asn 1234
	})
	// ErrorReport PDU
	f.Add([]byte{
		1, byte(ErrorReport),
		0, 1, // error code
		0, 0, 0, 16, // total length
		0, 0, 0, 0, // embedded pdu length
		0, 0, 0, 0, // text length
	})
	// Invalid or short PDU
	f.Add([]byte{1})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Panic safety: your func should never panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("decipherPDU panicked: %v", r)
			}
		}()

		pdu, err := decipherPDU(data)
		if err != nil {
			return
		}

		// Round-trip check: ensure Write doesn't panic and produces reasonable output
		var buf bytes.Buffer
		if err := pdu.Write(&buf); err != nil {
			t.Errorf("failed to write decoded PDU: %v", err)
		}

		// Verify the written length matches the expected length from header
		if buf.Len() >= 8 {
			gotLen := binary.BigEndian.Uint32(buf.Bytes()[4:8])
			if int(gotLen) != buf.Len() {
				t.Errorf("PDU length mismatch: header says %d, wrote %d", gotLen, buf.Len())
			}
		}
	})
}

func TestSerialQueryRoundTrip(t *testing.T) {
	orig := NewSerialQueryPDU(1, 100, 12345)

	var buf bytes.Buffer
	require.NoError(t, orig.Write(&buf))

	got, err := GetPDU(&buf)
	require.NoError(t, err)
	require.Equal(t, orig, got)
}

func TestSerialQuerySession(t *testing.T) {
	session := uint16(1234)
	pdu := NewSerialQueryPDU(1, session, 42)
	require.Equal(t, session, pdu.Session())
}

func TestDecipherPDU(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		wantErr  bool
		wantType PDUType // expected Type() value from the resulting PDU
	}{
		{
			name: "valid SerialQuery",
			input: func() []byte {
				buf := make([]byte, 12)
				buf[0] = 1                                   // Version
				buf[1] = byte(SerialQuery)                   // PDU Type
				binary.BigEndian.PutUint16(buf[2:4], 0x1234) // Session ID
				binary.BigEndian.PutUint32(buf[4:8], 12)     // Length
				binary.BigEndian.PutUint32(buf[8:12], 42)    // Serial
				return buf
			}(),
			wantErr:  false,
			wantType: SerialQuery,
		},
		{
			name: "valid ResetQuery",
			input: func() []byte {
				buf := make([]byte, 8)
				buf[0] = 1
				buf[1] = byte(ResetQuery)
				binary.BigEndian.PutUint32(buf[4:8], 8) // Length
				return buf
			}(),
			wantErr:  false,
			wantType: ResetQuery,
		},
		{
			name: "valid ErrorReport",
			input: func() []byte {
				pdu := []byte{0xde, 0xad, 0xbe, 0xef}
				text := []byte("something went wrong")

				header := make([]byte, 12)
				header[0] = 1
				header[1] = byte(ErrorReport)
				binary.BigEndian.PutUint16(header[2:4], 0x0001)                          // Error code
				binary.BigEndian.PutUint32(header[4:8], uint32(12+len(pdu)+4+len(text))) // total length (optional)
				binary.BigEndian.PutUint32(header[8:12], uint32(len(pdu)))               // embedded PDU length

				textLen := make([]byte, 4)
				binary.BigEndian.PutUint32(textLen, uint32(len(text)))

				return append(append(append(header, pdu...), textLen...), text...)
			}(),
			wantErr:  false,
			wantType: ErrorReport,
		},
		{
			name:    "too short",
			input:   []byte{1},
			wantErr: true,
		},
		{
			name: "unsupported PDU type",
			input: func() []byte {
				buf := make([]byte, 8)
				buf[0] = 1
				buf[1] = 99 // not in defined types
				return buf
			}(),
			wantErr: true,
		},
		{
			name: "error report with invalid pduLen",
			input: func() []byte {
				buf := make([]byte, 12)
				buf[0] = 1
				buf[1] = byte(ErrorReport)
				binary.BigEndian.PutUint32(buf[8:12], 999999) // absurd length
				return buf
			}(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decipherPDU(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("unexpected error state: got err=%v, wantErr=%v", err, tt.wantErr)
			}
			if err == nil && got.Type() != tt.wantType {
				t.Errorf("got Type() = %v, want %v", got.Type(), tt.wantType)
			}
		})
	}
}

func FuzzProtocolRoundTrip(f *testing.F) {
	// Seed with some valid PDU bytes for various types
	// SerialQuery (Version 1, Session 1, Serial 1)
	f.Add([]byte{1, 1, 0, 1, 0, 0, 0, 12, 0, 0, 0, 1})
	// ResetQuery (Version 1)
	f.Add([]byte{1, 2, 0, 0, 0, 0, 0, 8})
	// Ipv4Prefix (Version 1, Prefix 1.2.3.0/24, ASN 100, MaxMask 24)
	f.Add([]byte{1, 4, 0, 0, 0, 0, 0, 20, 1, 24, 24, 0, 1, 2, 3, 0, 0, 0, 0, 100})
	// EndOfData (Version 1, Session 1, Serial 1, Refresh 3600, Retry 600, Expire 7200)
	f.Add([]byte{1, 7, 0, 1, 0, 0, 0, 24, 0, 0, 0, 1, 0, 0, 14, 16, 0, 0, 2, 88, 0, 0, 28, 32})
	// Aspa (Version 1, Flags 1, CASN 1234, PASN 100)
	f.Add([]byte{1, 11, 1, 0, 0, 0, 0, 16, 0, 0, 4, 210, 0, 0, 0, 100})

	f.Fuzz(func(t *testing.T, data []byte) {
		// 1. Try to decode the PDU from input data
		pdu, err := decipherPDU(data)
		if err != nil {
			// Not a valid PDU, skip
			return
		}

		// 2. Marshal the PDU back to bytes
		var buf bytes.Buffer
		if err := pdu.Write(&buf); err != nil {
			t.Fatalf("Failed to write PDU type %d: %v", pdu.Type(), err)
		}

		// 3. Decode the marshaled bytes
		pdu2, err := decipherPDU(buf.Bytes())
		if err != nil {
			t.Fatalf("Failed to decode marshaled PDU type %d: %v", pdu.Type(), err)
		}

		// 4. Compare original and second PDU
		// reflect.DeepEqual works well here because decipherPDU and Write are symmetric.
		if !reflect.DeepEqual(pdu, pdu2) {
			t.Errorf("Round-trip mismatch for PDU type %d\nOriginal: %+v\nDecoded:  %+v", pdu.Type(), pdu, pdu2)
		}

		// 5. Verify the marshaled length matches the PDU length header
		if buf.Len() >= 8 {
			gotLen := binary.BigEndian.Uint32(buf.Bytes()[4:8])
			if int(gotLen) != buf.Len() {
				t.Errorf("PDU length header %d does not match written length %d for type %d", gotLen, buf.Len(), pdu.Type())
			}
		}
	})
}
