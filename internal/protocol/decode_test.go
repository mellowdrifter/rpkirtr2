package protocol

import (
	"bytes"
	"encoding/binary"
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
	// Invalid or short PDU
	f.Add([]byte{1})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Panic safety: your func should never panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("decipherPDU panicked: %v", r)
			}
		}()

		_, _ = decipherPDU(data)
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
