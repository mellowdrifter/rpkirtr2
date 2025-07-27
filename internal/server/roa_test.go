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
	roa1 := roa{Prefix: mustPrefix("10.0.0.0/24"), ASN: 1, MaxMask: 24}
	roa2 := roa{Prefix: mustPrefix("10.0.1.0/24"), ASN: 2, MaxMask: 24}
	roa3 := roa{Prefix: mustPrefix("10.0.2.0/24"), ASN: 3, MaxMask: 24}

	tests := []struct {
		name     string
		old      []roa
		new      []roa
		serial   uint32
		wantAdd  []roa
		wantDel  []roa
		wantDiff bool
	}{
		{
			name:     "no diff",
			old:      []roa{roa1, roa2},
			new:      []roa{roa1, roa2},
			serial:   10,
			wantAdd:  nil,
			wantDel:  nil,
			wantDiff: false,
		},
		{
			name:     "add one",
			old:      []roa{roa1},
			new:      []roa{roa1, roa2},
			serial:   20,
			wantAdd:  []roa{roa2},
			wantDel:  nil,
			wantDiff: true,
		},
		{
			name:     "delete one",
			old:      []roa{roa1, roa2},
			new:      []roa{roa1},
			serial:   30,
			wantAdd:  nil,
			wantDel:  []roa{roa2},
			wantDiff: true,
		},
		{
			name:     "add and delete",
			old:      []roa{roa1, roa2},
			new:      []roa{roa1, roa3},
			serial:   40,
			wantAdd:  []roa{roa3},
			wantDel:  []roa{roa2},
			wantDiff: true,
		},
		{
			name:     "empty old, all add",
			old:      nil,
			new:      []roa{roa1, roa2},
			serial:   50,
			wantAdd:  []roa{roa1, roa2},
			wantDel:  nil,
			wantDiff: true,
		},
		{
			name:     "empty new, all delete",
			old:      []roa{roa1, roa2},
			new:      nil,
			serial:   60,
			wantAdd:  nil,
			wantDel:  []roa{roa1, roa2},
			wantDiff: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeDiff(tt.new, tt.old)
			if !reflect.DeepEqual(got.addRoa, tt.wantAdd) {
				t.Errorf("addRoa = %v, want %v", got.addRoa, tt.wantAdd)
			}
			if !reflect.DeepEqual(got.delRoa, tt.wantDel) {
				t.Errorf("delRoa = %v, want %v", got.delRoa, tt.wantDel)
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
func generateROAs(n int, offset int) []roa {
	roas := make([]roa, 0, n)
	for i := 0; i < n; i++ {
		prefix, _ := netip.ParsePrefix("192.0." + strconv.Itoa((i+offset)%256) + ".0/24")
		roas = append(roas, roa{
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
