package server

import (
	"net/netip"
	"testing"
)

func TestHistoricalDiffs(t *testing.T) {
	c := newCache()
	c.serial = 10

	roa1 := ROA{Prefix: netip.MustParsePrefix("1.1.1.0/24"), ASN: 1, MaxMask: 24}
	roa2 := ROA{Prefix: netip.MustParsePrefix("2.2.2.0/24"), ASN: 2, MaxMask: 24}
	roa3 := ROA{Prefix: netip.MustParsePrefix("3.3.3.0/24"), ASN: 3, MaxMask: 24}

	// Update 1: 10 -> 11 (add roa1)
	c.updateDiffs([]ROA{roa1}, []ROA{roa1}, nil, nil, nil, nil)
	c.incrementSerial()

	// Update 2: 11 -> 12 (add roa2)
	c.updateDiffs([]ROA{roa1, roa2}, []ROA{roa2}, nil, nil, nil, nil)
	c.incrementSerial()

	// Update 3: 12 -> 13 (add roa3, del roa1)
	c.updateDiffs([]ROA{roa2, roa3}, []ROA{roa3}, []ROA{roa1}, nil, nil, nil)
	c.incrementSerial()

	// Test 1: Get diffs from 12 (one generation)
	add, del, _, _, ok := c.getDiffsFrom(12)
	if !ok {
		t.Fatal("Expected diff from 12 to be found")
	}
	if len(add) != 1 || add[0] != roa3 {
		t.Errorf("Expected add [roa3], got %v", add)
	}
	if len(del) != 1 || del[0] != roa1 {
		t.Errorf("Expected del [roa1], got %v", del)
	}

	// Test 2: Get diffs from 11 (two generations)
	add, del, _, _, ok = c.getDiffsFrom(11)
	if !ok {
		t.Fatal("Expected diff from 11 to be found")
	}
	// Aggregated: add roa2, roa3; del roa1
	if len(add) != 2 || len(del) != 1 {
		t.Errorf("Expected 2 additions and 1 deletion, got %d and %d", len(add), len(del))
	}

	// Test 3: Get diffs from 10 (three generations)
	add, del, _, _, ok = c.getDiffsFrom(10)
	if !ok {
		t.Fatal("Expected diff from 10 to be found")
	}
	// Aggregated: roa1 was added then deleted, so net is: add roa2, roa3
	if len(add) != 2 || len(del) != 0 {
		t.Errorf("Expected 2 additions and 0 deletions, got %d and %d", len(add), len(del))
	}

	// Test 4: Get diffs from 9 (too old)
	_, _, _, _, ok = c.getDiffsFrom(9)
	if ok {
		t.Error("Expected diff from 9 NOT to be found")
	}
}

func TestCacheRotation(t *testing.T) {
	c := newCache()
	c.serial = 100

	// Push maxHistory + 5 updates
	for i := 0; i < maxHistory+5; i++ {
		c.updateDiffs(nil, nil, nil, nil, nil, nil)
		c.serial++
	}

	// Should still have maxHistory entries
	if len(c.history) != maxHistory {
		t.Errorf("Expected %d history entries, got %d", maxHistory, len(c.history))
	}

	// The first 5 entries should be gone. 100 to 104 should be evicted.
	_, _, _, _, ok := c.getDiffsFrom(100)
	if ok {
		t.Error("Expected serial 100 to have been evicted from history")
	}
}

func TestDiffCancellation(t *testing.T) {
	c := newCache()
	c.serial = 10

	roa1 := ROA{Prefix: netip.MustParsePrefix("1.1.1.0/24"), ASN: 1, MaxMask: 24}

	// 10 -> 11: Add ROA1
	c.updateDiffs([]ROA{roa1}, []ROA{roa1}, nil, nil, nil, nil)
	c.incrementSerial()

	// 11 -> 12: Del ROA1
	c.updateDiffs(nil, nil, []ROA{roa1}, nil, nil, nil)
	c.incrementSerial()

	// Request diff from 10. Aggregated should be empty.
	add, del, _, _, ok := c.getDiffsFrom(10)
	if !ok {
		t.Fatal("Expected diff from 10 to be found")
	}

	if len(add) != 0 {
		t.Errorf("Expected 0 additions, got %d: %v", len(add), add)
	}
	if len(del) != 0 {
		t.Errorf("Expected 0 deletions, got %d: %v", len(del), del)
	}
}

func TestASPADiffCancellation(t *testing.T) {
	c := newCache()
	c.serial = 10

	aspa1 := ASPA{CustomerASN: 1, ProviderASNs: []uint32{10, 20}}

	// 10 -> 11: Add ASPA1
	c.updateDiffs(nil, nil, nil, []ASPA{aspa1}, []ASPA{aspa1}, nil)
	c.incrementSerial()

	// 11 -> 12: Del ASPA1
	c.updateDiffs(nil, nil, nil, nil, nil, []ASPA{aspa1})
	c.incrementSerial()

	// Request diff from 10. Aggregated should be empty.
	_, _, add, del, ok := c.getDiffsFrom(10)
	if !ok {
		t.Fatal("Expected diff from 10 to be found")
	}

	if len(add) != 0 {
		t.Errorf("Expected 0 additions, got %d: %v", len(add), add)
	}
	if len(del) != 0 {
		t.Errorf("Expected 0 deletions, got %d: %v", len(del), del)
	}
}

func TestHistorySizeInvariant(t *testing.T) {
	c := newCache()
	for i := 0; i < maxHistory*3; i++ {
		c.updateDiffs(nil, nil, nil, nil, nil, nil)
		if len(c.history) > maxHistory {
			t.Fatalf("At step %d, history size %d exceeded maxHistory %d", i, len(c.history), maxHistory)
		}
	}
}

func TestGetDiffsFromInvariants(t *testing.T) {
	c := newCache()
	c.serial = 100
	roa1 := ROA{Prefix: netip.MustParsePrefix("1.1.1.0/24"), ASN: 1, MaxMask: 24}

	// 1. Add roa1
	c.updateDiffs([]ROA{roa1}, []ROA{roa1}, nil, nil, nil, nil)
	c.serial++

	// 2. Delete roa1
	c.updateDiffs(nil, nil, []ROA{roa1}, nil, nil, nil)
	c.serial++

	// 3. Add roa1 again
	c.updateDiffs([]ROA{roa1}, []ROA{roa1}, nil, nil, nil, nil)
	c.serial++

	// Diff from 100 should only have roa1 in addRoa, and nothing in delRoa
	add, del, _, _, ok := c.getDiffsFrom(100)
	if !ok {
		t.Fatal("expected diff")
	}

	for _, a := range add {
		for _, d := range del {
			if a.key() == d.key() {
				t.Errorf("ROA %v is in both add and del", a)
			}
		}
	}

	// For roa1, it was added (100-101), deleted (101-102), added (102-103)
	// Net should be added.
	found := false
	for _, r := range add {
		if r.key() == roa1.key() {
			found = true
			break
		}
	}
	if !found {
		t.Error("roa1 should be in aggregated additions")
	}
	if len(del) != 0 {
		t.Errorf("expected 0 deletions, got %d", len(del))
	}
}
