package server

import "testing"

// sampleROA returns a deterministic roa object for test purposes
func sampleROA(prefix string, asn uint32) roa {
	return roa{
		Prefix: prefix,
		ASN:    asn,
	}
}

func BenchmarkMakeDiff(b *testing.B) {
	sizes := []int{100, 1000, 5000, 10000}

	for _, size := range sizes {
		b.Run("size_"+itoa(size), func(b *testing.B) {
			old := make([]roa, 0, size)
			new := make([]roa, 0, size)

			// Fill `old` with 0..size-1
			for i := 0; i < size; i++ {
				old = append(old, sampleROA("10.0.0.0/24", uint32(i)))
			}

			// Fill `new` with 0..size/2-1 (shared) + size..size+size/2-1 (new)
			for i := 0; i < size/2; i++ {
				new = append(new, sampleROA("10.0.0.0/24", uint32(i)))      // overlap
				new = append(new, sampleROA("10.0.0.0/24", uint32(size+i))) // new entries
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = makeDiff(new, old, 100)
			}
		})
	}
}

// fast int to string helper for benchmark names
func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	buf := make([]byte, 0, 8)
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	return string(buf)
}
