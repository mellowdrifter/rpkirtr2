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
	c.updateDiffs([]ROA{roa1}, []ROA{roa1}, nil)
	c.serial = 11

	// Update 2: 11 -> 12 (add roa2)
	c.updateDiffs([]ROA{roa1, roa2}, []ROA{roa2}, nil)
	c.serial = 12

	// Update 3: 12 -> 13 (add roa3, del roa1)
	c.updateDiffs([]ROA{roa2, roa3}, []ROA{roa3}, []ROA{roa1})
	c.serial = 13

	// Test 1: Get diffs from 12 (one generation)
	add, del, found := c.getDiffsFrom(12)
	if !found {
		t.Fatal("Expected diff from 12 to be found")
	}
	if len(add) != 1 || add[0] != roa3 {
		t.Errorf("Expected add [roa3], got %v", add)
	}
	if len(del) != 1 || del[0] != roa1 {
		t.Errorf("Expected del [roa1], got %v", del)
	}

	// Test 2: Get diffs from 11 (two generations)
	add, del, found = c.getDiffsFrom(11)
	if !found {
		t.Fatal("Expected diff from 11 to be found")
	}
	// Aggregated: add roa2, roa3; del roa1
	if len(add) != 2 {
		t.Errorf("Expected 2 additions, got %d", len(add))
	}
	if len(del) != 1 || del[0] != roa1 {
		t.Errorf("Expected del [roa1], got %v", del)
	}

	// Test 3: Get diffs from 10 (three generations)
	add, del, found = c.getDiffsFrom(10)
	if !found {
		t.Fatal("Expected diff from 10 to be found")
	}
	// Aggregated: add roa1, roa2, roa3; del roa1
	if len(add) != 3 {
		t.Errorf("Expected 3 additions, got %d", len(add))
	}
	if len(del) != 1 {
		t.Errorf("Expected 1 deletion, got %d", len(del))
	}

	// Test 4: Get diffs from 9 (too old)
	_, _, found = c.getDiffsFrom(9)
	if found {
		t.Error("Expected diff from 9 NOT to be found")
	}
}

func TestRingBufferTrimming(t *testing.T) {
	c := newCache()
	c.serial = 100

	// Push maxHistory + 5 updates
	for i := 0; i < maxHistory+5; i++ {
		c.updateDiffs(nil, nil, nil)
		c.serial++
	}

	if len(c.history) != maxHistory {
		t.Errorf("Expected history length %d, got %d", maxHistory, len(c.history))
	}

	// The oldest serial in history should be 100 + 5
	expectedOldest := uint32(100 + 5)
	if c.history[0].from != expectedOldest {
		t.Errorf("Expected oldest serial %d, got %d", expectedOldest, c.history[0].from)
	}
}
