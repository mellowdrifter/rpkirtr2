package server

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ASPA represents an Autonomous System Provider Authorization object.
type ASPA struct {
	CustomerASN  uint32
	ProviderASNs []uint32
	Expires      int64 // Unix timestamp; 0 means no expiry information
}

// JSONASPA represents the JSON structure of an ASPA object as provided by collectors like rpki-client.
type JSONASPA struct {
	Customer  uint32          `json:"customer"`
	Providers []JSONProvider  `json:"providers"`
	Expires   int64           `json:"expires"`
}

type JSONProvider struct {
	ASN uint32 `json:"asn"`
}



// Key returns a unique string identifier for an ASPA object, used for diffing.
// We sort ProviderASNs to ensure the key is deterministic regardless of JSON order.
func (a ASPA) Key() string {
	providers := make([]uint32, len(a.ProviderASNs))
	copy(providers, a.ProviderASNs)
	sort.Slice(providers, func(i, j int) bool {
		return providers[i] < providers[j]
	})
	
	providerStrs := make([]string, len(providers))
	for i, p := range providers {
		providerStrs[i] = fmt.Sprint(p)
	}
	
	return fmt.Sprintf("%d|%s", a.CustomerASN, strings.Join(providerStrs, ","))
}

// filterExpiredASPAs removes ASPA objects that have already expired.
func filterExpiredASPAs(aspas []ASPA, now time.Time) []ASPA {
	out := make([]ASPA, 0, len(aspas))
	for _, a := range aspas {
		if a.Expires == 0 || time.Unix(a.Expires, 0).After(now) {
			out = append(out, a)
		}
	}
	return out
}
