package protocol

import "testing"

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
