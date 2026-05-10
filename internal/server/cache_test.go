package server

import (
	"net/netip"
	"testing"
)

func TestCacheDiffAggregation(t *testing.T) {
	c := newCache()
	c.serial = 10

	roa1 := ROA{Prefix: netip.MustParsePrefix("1.1.1.0/24"), ASN: 1, MaxMask: 24}
	roa2 := ROA{Prefix: netip.MustParsePrefix("2.2.2.0/24"), ASN: 2, MaxMask: 24}
	roa3 := ROA{Prefix: netip.MustParsePrefix("3.3.3.0/24"), ASN: 3, MaxMask: 24}

	// Update 1: 10 -> 11 (add roa1)
	c.updateDiffs([]ROA{roa1}, []ROA{roa1}, nil, nil, nil, nil)
	c.serial = 11

	// Update 2: 11 -> 12 (add roa2)
	c.updateDiffs([]ROA{roa1, roa2}, []ROA{roa2}, nil, nil, nil, nil)
	c.serial = 12

	// Update 3: 12 -> 13 (add roa3, del roa1)
	c.updateDiffs([]ROA{roa2, roa3}, []ROA{roa3}, []ROA{roa1}, nil, nil, nil)
	c.serial = 13

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
	// Aggregated: add roa1, roa2, roa3; del roa1
	if len(add) != 3 || len(del) != 1 {
		t.Errorf("Expected 3 additions and 1 deletion, got %d and %d", len(add), len(del))
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
}
