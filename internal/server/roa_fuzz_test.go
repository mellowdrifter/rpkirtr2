package server

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

func FuzzGetSetOfValidatedROAs(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		const roaSize = 16 + 1 + 4 + 1 // addr(16) + bits(1) + asn(4) + maxmask(1)
		if len(data) < roaSize {
			return
		}

		numROAs := len(data) / roaSize
		if numROAs > 100 {
			numROAs = 100
		}

		roas := make([]ROA, numROAs)
		for i := 0; i < numROAs; i++ {
			offset := i * roaSize
			ipBytes := data[offset : offset+16]
			bits := int(data[offset+16] % 129)
			asn := binary.BigEndian.Uint32(data[offset+17 : offset+21])
			maxMask := data[offset+21]

			addr := netip.AddrFrom16([16]byte(ipBytes))
			// Randomly decide between IPv4 and IPv6
			if data[offset]%2 == 0 {
				addr = netip.AddrFrom4([4]byte(ipBytes[:4]))
				if bits > 32 {
					bits = 32
				}
			}

			roas[i] = ROA{
				Prefix:  netip.PrefixFrom(addr, bits),
				ASN:     asn,
				MaxMask: maxMask,
			}
		}

		result := GetSetOfValidatedROAs(roas)

		// Invariants
		if len(result) > numROAs {
			t.Errorf("len(result) %d > numROAs %d", len(result), numROAs)
		}
		for i := 0; i < len(result); i++ {
			if !result[i].isValid() {
				t.Errorf("result[%d] is invalid: %+v", i, result[i])
			}
			if i > 0 {
				if !result[i-1].key().Less(result[i].key()) {
					t.Errorf("result is not strictly sorted or has duplicates: %v vs %v", result[i-1].key(), result[i].key())
				}
			}
		}
	})
}
