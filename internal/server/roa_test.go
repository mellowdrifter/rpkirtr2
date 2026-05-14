package server

import (
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
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

			isDiff := len(got.addRoa) > 0 || len(got.delRoa) > 0
			if isDiff != tt.wantDiff {
				t.Errorf("diff = %v, want %v", isDiff, tt.wantDiff)
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
		{"AS0", "AS0", 0},
		{"overflow uint32", "4294967296", 0},
		{"large value", "5000000000", 0},
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

func TestDecodeROAsJSON(t *testing.T) {
	jsonStr := `{
		"roas": [
			{
				"prefix": "1.1.1.0/24",
				"maxLength": 24,
				"asn": "AS1",
				"expires": 1715594400
			},
			{
				"prefix": "2.2.2.0/24",
				"maxLength": 32,
				"asn": 2,
				"expires": 1715594400
			}
		]
	}`

	roas, err := decodeROAsJSON(strings.NewReader(jsonStr))
	if err != nil {
		t.Fatalf("decodeROAsJSON failed: %v", err)
	}

	if len(roas) != 2 {
		t.Fatalf("Expected 2 ROAs, got %d", len(roas))
	}

	expected := []ROA{
		{Prefix: mustPrefix("1.1.1.0/24"), MaxMask: 24, ASN: 1, Expires: 1715594400},
		{Prefix: mustPrefix("2.2.2.0/24"), MaxMask: 32, ASN: 2, Expires: 1715594400},
	}

	if !reflect.DeepEqual(roas, expected) {
		t.Errorf("Decoded ROAs don't match expected.\nGot: %+v\nWant: %+v", roas, expected)
	}
}

func TestROAIsValid(t *testing.T) {
	tests := []struct {
		name string
		roa  ROA
		want bool
	}{
		{
			name: "valid ipv4",
			roa:  ROA{Prefix: mustPrefix("1.1.1.0/24"), MaxMask: 24, ASN: 1},
			want: true,
		},
		{
			name: "valid ipv4 with maxmask",
			roa:  ROA{Prefix: mustPrefix("1.1.1.0/24"), MaxMask: 32, ASN: 1},
			want: true,
		},
		{
			name: "valid ipv6",
			roa:  ROA{Prefix: mustPrefix("2001:db8::/32"), MaxMask: 48, ASN: 1},
			want: true,
		},
		{
			name: "maxmask < prefix length",
			roa:  ROA{Prefix: mustPrefix("1.1.1.0/24"), MaxMask: 16, ASN: 1},
			want: false,
		},
		{
			name: "ipv4 maxmask > 32",
			roa:  ROA{Prefix: mustPrefix("1.1.1.0/24"), MaxMask: 33, ASN: 1},
			want: false,
		},
		{
			name: "ipv6 maxmask > 128",
			roa:  ROA{Prefix: mustPrefix("2001:db8::/32"), MaxMask: 129, ASN: 1},
			want: false,
		},
		{
			name: "maxmask == 0",
			roa:  ROA{Prefix: mustPrefix("1.1.1.0/24"), MaxMask: 0, ASN: 1},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.roa.isValid(); got != tt.want {
				t.Errorf("ROA.isValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSetOfValidatedROAs(t *testing.T) {
	roa1 := ROA{Prefix: mustPrefix("1.1.1.0/24"), MaxMask: 24, ASN: 1}
	roa1Dup := ROA{Prefix: mustPrefix("1.1.1.0/24"), MaxMask: 24, ASN: 1}
	roa2 := ROA{Prefix: mustPrefix("2.2.2.0/24"), MaxMask: 24, ASN: 2}
	invalidRoa := ROA{Prefix: mustPrefix("3.3.3.0/24"), MaxMask: 16, ASN: 3}

	tests := []struct {
		name string
		roas []ROA
		want []ROA
	}{
		{
			name: "deduplication",
			roas: []ROA{roa1, roa1Dup, roa2},
			want: []ROA{roa1, roa2},
		},
		{
			name: "filtering invalid",
			roas: []ROA{roa1, invalidRoa, roa2},
			want: []ROA{roa1, roa2},
		},
		{
			name: "empty input",
			roas: nil,
			want: nil,
		},
		{
			name: "all invalid",
			roas: []ROA{invalidRoa},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSetOfValidatedROAs(tt.roas)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetSetOfValidatedROAs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterExpired(t *testing.T) {
	now := time.Unix(1000, 0)

	tests := []struct {
		name string
		roas []ROA
		want []ROA
	}{
		{
			name: "all valid",
			roas: []ROA{
				{Prefix: mustPrefix("1.1.1.0/24"), Expires: 2000},
				{Prefix: mustPrefix("2.2.2.0/24"), Expires: 3000},
			},
			want: []ROA{
				{Prefix: mustPrefix("1.1.1.0/24"), Expires: 2000},
				{Prefix: mustPrefix("2.2.2.0/24"), Expires: 3000},
			},
		},
		{
			name: "all expired",
			roas: []ROA{
				{Prefix: mustPrefix("1.1.1.0/24"), Expires: 500},
				{Prefix: mustPrefix("2.2.2.0/24"), Expires: 600},
			},
			want: []ROA{},
		},
		{
			name: "mixed valid/expired",
			roas: []ROA{
				{Prefix: mustPrefix("1.1.1.0/24"), Expires: 2000},
				{Prefix: mustPrefix("2.2.2.0/24"), Expires: 500},
			},
			want: []ROA{
				{Prefix: mustPrefix("1.1.1.0/24"), Expires: 2000},
			},
		},
		{
			name: "expires 0 passthrough",
			roas: []ROA{
				{Prefix: mustPrefix("1.1.1.0/24"), Expires: 0},
				{Prefix: mustPrefix("2.2.2.0/24"), Expires: 2000},
			},
			want: []ROA{
				{Prefix: mustPrefix("1.1.1.0/24"), Expires: 0},
				{Prefix: mustPrefix("2.2.2.0/24"), Expires: 2000},
			},
		},
		{
			name: "exactly at boundary (should be expired)",
			roas: []ROA{
				{Prefix: mustPrefix("1.1.1.0/24"), Expires: 1000},
			},
			want: []ROA{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterExpired(tt.roas, now)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}
