package server

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

func FuzzMakeDiff(f *testing.F) {
	// Seed with some basic valid ROA data pairs
	f.Add([]byte{
		// ROA 1: 10.0.0.0/24, ASN 1, MaxMask 32
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 10, 0, 0, 0, 24, 0, 0, 0, 1, 32,
		// ROA 2: 10.0.1.0/24, ASN 2, MaxMask 32
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 10, 0, 1, 0, 24, 0, 0, 0, 2, 32,
	})
	f.Add([]byte{
		// ROA 1: 1.1.1.0/24, ASN 100, MaxMask 24
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 1, 1, 1, 0, 24, 0, 0, 0, 100, 24,
		// ROA 2: 2.2.2.0/24, ASN 200, MaxMask 24
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 2, 2, 2, 0, 24, 0, 0, 0, 200, 24,
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		const roaSize = 16 + 1 + 4 + 1
		if len(data) < roaSize*2 {
			return
		}

		totalROAs := len(data) / roaSize
		if totalROAs > 50 {
			totalROAs = 50
		}
		
		// Split data into two sets
		splitIdx := totalROAs / 2
		
		genROAs := func(count int, d []byte) []ROA {
			roas := make([]ROA, count)
			for i := 0; i < count; i++ {
				offset := i * roaSize
				ipBytes := d[offset : offset+16]
				bits := int(d[offset+16] % 129)
				asn := binary.BigEndian.Uint32(d[offset+17 : offset+21])
				maxMask := d[offset+21]
				addr := netip.AddrFrom16([16]byte(ipBytes))
				if d[offset]%2 == 0 {
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
			return GetSetOfValidatedROAs(roas)
		}

		newROAs := genROAs(splitIdx, data[:splitIdx*roaSize])
		oldROAs := genROAs(totalROAs-splitIdx, data[splitIdx*roaSize:])

		diff := makeDiff(newROAs, oldROAs)

		// Invariants
		newMap := make(map[roaKey]bool)
		for _, r := range newROAs {
			newMap[r.key()] = true
		}
		oldMap := make(map[roaKey]bool)
		for _, r := range oldROAs {
			oldMap[r.key()] = true
		}

		for _, r := range diff.addRoa {
			if !newMap[r.key()] {
				t.Errorf("Added ROA %v not in new set", r)
			}
			if oldMap[r.key()] {
				t.Errorf("Added ROA %v was already in old set", r)
			}
		}
		for _, r := range diff.delRoa {
			if !oldMap[r.key()] {
				t.Errorf("Deleted ROA %v not in old set", r)
			}
			if newMap[r.key()] {
				t.Errorf("Deleted ROA %v still in new set", r)
			}
		}
		
		// Check overlap between add and del
		addMap := make(map[roaKey]bool)
		for _, r := range diff.addRoa {
			addMap[r.key()] = true
		}
		for _, r := range diff.delRoa {
			if addMap[r.key()] {
				t.Errorf("ROA %v is in both add and del", r)
			}
		}
	})
}

func FuzzMakeASPADiff(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		// Each ASPA needs: 4 (casn) + 1 (count) + count*4 (pasns)
		// To simplify, let's say 4 (casn) + 4 (pasn1) + 4 (pasn2) = 12 bytes
		const aspaSize = 12
		if len(data) < aspaSize*2 {
			return
		}

		totalASPAs := len(data) / aspaSize
		if totalASPAs > 50 {
			totalASPAs = 50
		}
		splitIdx := totalASPAs / 2

		genASPAs := func(count int, d []byte) []ASPA {
			aspas := make([]ASPA, count)
			for i := 0; i < count; i++ {
				offset := i * aspaSize
				casn := binary.BigEndian.Uint32(d[offset : offset+4])
				if casn == 0 {
					casn = 1
				}
				pasn1 := binary.BigEndian.Uint32(d[offset+4 : offset+8])
				pasn2 := binary.BigEndian.Uint32(d[offset+8 : offset+12])
				aspas[i] = ASPA{
					CustomerASN:  casn,
					ProviderASNs: []uint32{pasn1, pasn2},
				}
			}
			return DeduplicateASPAsInPlace(aspas)
		}

		newASPAs := genASPAs(splitIdx, data[:splitIdx*aspaSize])
		oldASPAs := genASPAs(totalASPAs-splitIdx, data[splitIdx*aspaSize:])

		diff := makeASPADiff(newASPAs, oldASPAs)

		// Invariants
		newMap := make(map[uint32]ASPA)
		for _, a := range newASPAs {
			newMap[a.CustomerASN] = a
		}
		oldMap := make(map[uint32]ASPA)
		for _, a := range oldASPAs {
			oldMap[a.CustomerASN] = a
		}

		for _, a := range diff.addAspa {
			na, ok := newMap[a.CustomerASN]
			if !ok {
				t.Errorf("Added ASPA %v not in new set", a)
			}
			if !aspasEqual(a, na) {
				t.Errorf("Added ASPA %v mismatch with new set %v", a, na)
			}
			
			oa, ok := oldMap[a.CustomerASN]
			if ok && aspasEqual(a, oa) {
				t.Errorf("Added ASPA %v was already identical in old set", a)
			}
		}
		for _, a := range diff.delAspa {
			oa, ok := oldMap[a.CustomerASN]
			if !ok {
				t.Errorf("Deleted ASPA %v not in old set", a)
			}
			if !aspasEqual(a, oa) {
				t.Errorf("Deleted ASPA %v mismatch with old set %v", a, oa)
			}
			
			na, ok := newMap[a.CustomerASN]
			if ok && aspasEqual(a, na) {
				t.Errorf("Deleted ASPA %v is still identical in new set", a)
			}
		}
	})
}
