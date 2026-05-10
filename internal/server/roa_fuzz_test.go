package server

import (
	"bytes"
	"testing"
)

func FuzzDecodeROAsJSON(f *testing.F) {
	// Seed corpus
	f.Add([]byte(`{"roas": [{"prefix": "1.1.1.0/24", "maxLength": 24, "asn": 1234, "expires": 1234567890}]}`))
	f.Add([]byte(`{"roas": [{"prefix": "2001:db8::/32", "maxLength": 48, "asn": "AS1234"}]}`))
	f.Add([]byte(`{"not_roas": []}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`invalid json`))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := bytes.NewReader(data)
		_, _ = decodeROAsJSON(r)
	})
}
