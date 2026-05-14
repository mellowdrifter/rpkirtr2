package server

import (
	"reflect"
	"testing"
	"time"
)

func TestASPAIsValid(t *testing.T) {
	tests := []struct {
		name string
		aspa ASPA
		want bool
	}{
		{
			name: "valid ASPA",
			aspa: ASPA{
				CustomerASN:  65001,
				ProviderASNs: []uint32{65002},
			},
			want: true,
		},
		{
			name: "invalid: CustomerASN is 0",
			aspa: ASPA{
				CustomerASN:  0,
				ProviderASNs: []uint32{65002},
			},
			want: false,
		},
		{
			name: "invalid: empty ProviderASNs",
			aspa: ASPA{
				CustomerASN:  65001,
				ProviderASNs: []uint32{},
			},
			want: false,
		},
		{
			name: "invalid: nil ProviderASNs",
			aspa: ASPA{
				CustomerASN:  65001,
				ProviderASNs: nil,
			},
			want: false,
		},
		{
			name: "invalid: both CustomerASN 0 and empty providers",
			aspa: ASPA{
				CustomerASN:  0,
				ProviderASNs: []uint32{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.aspa.isValid(); got != tt.want {
				t.Errorf("ASPA.isValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeduplicateASPAsInPlace(t *testing.T) {
	tests := []struct {
		name string
		in   []ASPA
		want []ASPA
	}{
		{
			name: "nil slice",
			in:   nil,
			want: nil,
		},
		{
			name: "empty slice",
			in:   []ASPA{},
			want: []ASPA{},
		},
		{
			name: "all invalid",
			in: []ASPA{
				{CustomerASN: 0, ProviderASNs: []uint32{100}},
				{CustomerASN: 200, ProviderASNs: nil},
			},
			want: []ASPA{},
		},
		{
			name: "first element invalid",
			in: []ASPA{
				{CustomerASN: 0, ProviderASNs: []uint32{100}},
				{CustomerASN: 65001, ProviderASNs: []uint32{100}},
			},
			want: []ASPA{
				{CustomerASN: 65001, ProviderASNs: []uint32{100}},
			},
		},
		{
			name: "duplicate identical",
			in: []ASPA{
				{CustomerASN: 65001, ProviderASNs: []uint32{100, 200}},
				{CustomerASN: 65001, ProviderASNs: []uint32{100, 200}},
			},
			want: []ASPA{
				{CustomerASN: 65001, ProviderASNs: []uint32{100, 200}},
			},
		},
		{
			name: "same CustomerASN different providers (should keep one)",
			in: []ASPA{
				{CustomerASN: 65001, ProviderASNs: []uint32{100, 200}},
				{CustomerASN: 65001, ProviderASNs: []uint32{100, 300}},
			},
			want: []ASPA{
				{CustomerASN: 65001, ProviderASNs: []uint32{100, 200}},
			},
		},
		{
			name: "mixed valid and invalid and duplicates",
			in: []ASPA{
				{CustomerASN: 0, ProviderASNs: []uint32{100}},
				{CustomerASN: 65001, ProviderASNs: []uint32{100}},
				{CustomerASN: 65001, ProviderASNs: []uint32{100}},
				{CustomerASN: 65002, ProviderASNs: []uint32{200}},
				{CustomerASN: 65001, ProviderASNs: nil},
			},
			want: []ASPA{
				{CustomerASN: 65001, ProviderASNs: []uint32{100}},
				{CustomerASN: 65002, ProviderASNs: []uint32{200}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeduplicateASPAsInPlace(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("%s: got len %d, want %d", tt.name, len(got), len(tt.want))
			}
			for i := range got {
				if !aspasEqual(got[i], tt.want[i]) {
					t.Errorf("%s at index %d: got %v, want %v", tt.name, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestFilterExpiredASPAs(t *testing.T) {
	now := time.Unix(1000, 0)

	tests := []struct {
		name string
		in   []ASPA
		want []ASPA
	}{
		{
			name: "all valid",
			in: []ASPA{
				{CustomerASN: 65001, Expires: 2000},
				{CustomerASN: 65002, Expires: 3000},
			},
			want: []ASPA{
				{CustomerASN: 65001, Expires: 2000},
				{CustomerASN: 65002, Expires: 3000},
			},
		},
		{
			name: "all expired",
			in: []ASPA{
				{CustomerASN: 65001, Expires: 500},
				{CustomerASN: 65002, Expires: 600},
			},
			want: []ASPA{},
		},
		{
			name: "mixed valid/expired",
			in: []ASPA{
				{CustomerASN: 65001, Expires: 2000},
				{CustomerASN: 65002, Expires: 500},
			},
			want: []ASPA{
				{CustomerASN: 65001, Expires: 2000},
			},
		},
		{
			name: "expires 0 passthrough",
			in: []ASPA{
				{CustomerASN: 65001, Expires: 0},
				{CustomerASN: 65002, Expires: 2000},
			},
			want: []ASPA{
				{CustomerASN: 65001, Expires: 0},
				{CustomerASN: 65002, Expires: 2000},
			},
		},
		{
			name: "exactly at boundary (should be expired)",
			in: []ASPA{
				{CustomerASN: 65001, Expires: 1000},
			},
			want: []ASPA{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterExpiredASPAs(tt.in, now)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterExpiredASPAs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMakeASPADiff(t *testing.T) {
	aspa1 := ASPA{CustomerASN: 65001, ProviderASNs: []uint32{100, 200}}
	aspa2 := ASPA{CustomerASN: 65002, ProviderASNs: []uint32{300}}
	aspa3 := ASPA{CustomerASN: 65003, ProviderASNs: []uint32{400}}
	aspa1Mod := ASPA{CustomerASN: 65001, ProviderASNs: []uint32{100, 300}}

	tests := []struct {
		name    string
		old     []ASPA
		new     []ASPA
		wantAdd []ASPA
		wantDel []ASPA
	}{
		{
			name:    "no diff",
			old:     []ASPA{aspa1, aspa2},
			new:     []ASPA{aspa1, aspa2},
			wantAdd: nil,
			wantDel: nil,
		},
		{
			name:    "add one",
			old:     []ASPA{aspa1},
			new:     []ASPA{aspa1, aspa2},
			wantAdd: []ASPA{aspa2},
			wantDel: nil,
		},
		{
			name:    "delete one",
			old:     []ASPA{aspa1, aspa2},
			new:     []ASPA{aspa1},
			wantAdd: nil,
			wantDel: []ASPA{aspa2},
		},
		{
			name:    "modify one (same ASN, different providers)",
			old:     []ASPA{aspa1, aspa2},
			new:     []ASPA{aspa1Mod, aspa2},
			wantAdd: []ASPA{aspa1Mod},
			wantDel: []ASPA{aspa1},
		},
		{
			name:    "mixed operations",
			old:     []ASPA{aspa1, aspa2},
			new:     []ASPA{aspa2, aspa3},
			wantAdd: []ASPA{aspa3},
			wantDel: []ASPA{aspa1},
		},
		{
			name:    "empty old, all add",
			old:     nil,
			new:     []ASPA{aspa1, aspa2},
			wantAdd: []ASPA{aspa1, aspa2},
			wantDel: nil,
		},
		{
			name:    "empty new, all delete",
			old:     []ASPA{aspa1, aspa2},
			new:     nil,
			wantAdd: nil,
			wantDel: []ASPA{aspa1, aspa2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeASPADiff(tt.new, tt.old)
			if !reflect.DeepEqual(got.addAspa, tt.wantAdd) {
				t.Errorf("%s: addAspa = %v, want %v", tt.name, got.addAspa, tt.wantAdd)
			}
			if !reflect.DeepEqual(got.delAspa, tt.wantDel) {
				t.Errorf("%s: delAspa = %v, want %v", tt.name, got.delAspa, tt.wantDel)
			}
		})
	}
}
