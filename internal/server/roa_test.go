package server

import (
	"net/netip"
	"reflect"
	"strconv"
	"testing"
)

func mustPrefix(s string) netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		panic(err)
	}
	return p
}

func TestMakeDiff2(t *testing.T) {
	roa1 := ROA{Prefix: mustPrefix("10.0.0.0/24"), ASN: 1, MaxMask: 24}
	roa2 := ROA{Prefix: mustPrefix("10.0.1.0/24"), ASN: 2, MaxMask: 24}
	roa3 := ROA{Prefix: mustPrefix("10.0.2.0/24"), ASN: 3, MaxMask: 24}

	tests := []struct {
		name     string
		old      []ROA
		new      []ROA
		serial   uint32
		wantAdd  []ROA
		wantDel  []ROA
		wantDiff bool
	}{
		{
			name:     "no diff",
			old:      []ROA{roa1, roa2},
			new:      []ROA{roa1, roa2},
			serial:   10,
			wantAdd:  nil,
			wantDel:  nil,
			wantDiff: false,
		},
		{
			name:     "add one",
			old:      []ROA{roa1},
			new:      []ROA{roa1, roa2},
			serial:   20,
			wantAdd:  []ROA{roa2},
			wantDel:  nil,
			wantDiff: true,
		},
		{
			name:     "delete one",
			old:      []ROA{roa1, roa2},
			new:      []ROA{roa1},
			serial:   30,
			wantAdd:  nil,
			wantDel:  []ROA{roa2},
			wantDiff: true,
		},
		{
			name:     "add and delete",
			old:      []ROA{roa1, roa2},
			new:      []ROA{roa1, roa3},
			serial:   40,
			wantAdd:  []ROA{roa3},
			wantDel:  []ROA{roa2},
			wantDiff: true,
		},
		{
			name:     "empty old, all add",
			old:      nil,
			new:      []ROA{roa1, roa2},
			serial:   50,
			wantAdd:  []ROA{roa1, roa2},
			wantDel:  nil,
			wantDiff: true,
		},
		{
			name:     "empty new, all delete",
			old:      []ROA{roa1, roa2},
			new:      nil,
			serial:   60,
			wantAdd:  nil,
			wantDel:  []ROA{roa1, roa2},
			wantDiff: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeDiff(tt.new, tt.old)
			if len(got.addRoa) != len(tt.wantAdd) {
				t.Errorf("addRoa length = %d, want %d", len(got.addRoa), len(tt.wantAdd))
			}
			if len(got.delRoa) != len(tt.wantDel) {
				t.Errorf("delRoa length = %d, want %d", len(got.delRoa), len(tt.wantDel))
			}
			
			// Simple check for presence without order dependency
			for _, r := range got.addRoa {
				found := false
				for _, wr := range tt.wantAdd {
					if reflect.DeepEqual(r, wr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("addRoa contains unexpected ROA: %v", r)
				}
			}
			for _, wr := range tt.wantAdd {
				found := false
				for _, r := range got.addRoa {
					if reflect.DeepEqual(r, wr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("addRoa missing expected ROA: %v", wr)
				}
			}

			if got.diff != tt.wantDiff {
				t.Errorf("diff = %v, want %v", got.diff, tt.wantDiff)
			}
		})
	}
}

// mockKey returns a reproducible key for test ROAs.
func mockKey(prefix string, maxMask uint8, asn uint32) string {
	return prefix + "|" + strconv.Itoa(int(maxMask)) + "|" + strconv.FormatUint(uint64(asn), 10)
}

// helper to generate dummy ROAs
func generateROAs(n int, offset int) []ROA {
	roas := make([]ROA, 0, n)
	for i := 0; i < n; i++ {
		prefix, _ := netip.ParsePrefix("192.0." + strconv.Itoa((i+offset)%256) + ".0/24")
		roas = append(roas, ROA{
			Prefix:  prefix,
			MaxMask: 24,
			ASN:     64512 + uint32(i),
		})
	}
	return roas
}

func BenchmarkMakeDiff(b *testing.B) {
	const count = 10000

	oldROAs := generateROAs(count, 0)
	newROAs := generateROAs(count, 500) // slight offset so some ROAs differ

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = makeDiff(newROAs, oldROAs)
	}
}

func TestAsnToUint32(t *testing.T) {
tests := []struct {
name     string
input    string
expected uint32
}{
{"AS65000", "AS65000", 65000},
{"as65000", "as65000", 65000},
{"pure number", "65000", 65000},
{"short", "A", 0},
{"empty", "", 0},
{"garbage", "ASabc", 0},
{"just AS", "AS", 0},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
got := asnToUint32(tt.input)
if got != tt.expected {
t.Errorf("asnToUint32(%q) = %d, want %d", tt.input, got, tt.expected)
}
})
}
}
