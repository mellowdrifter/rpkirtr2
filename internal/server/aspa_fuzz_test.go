package server

import (
	"bytes"
	"testing"
)

func FuzzDecodeASPAsJSON(f *testing.F) {
	// Seed corpus
	f.Add([]byte(`{"aspa": [{"customer": 1234, "providers": [{"asn": 100}, {"asn": 200}], "expires": 1234567890}]}`))
	f.Add([]byte(`{"aspa": []}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`invalid json`))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := bytes.NewReader(data)
		_, _ = decodeASPAsJSON(r)
	})
}
