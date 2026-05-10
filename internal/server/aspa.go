package server

import (
	"sort"
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
	Customer  uint32         `json:"customer"`
	Providers []JSONProvider `json:"providers"`
	Expires   int64          `json:"expires"`
}

type JSONProvider struct {
	ASN uint32 `json:"asn"`
}

// Less reports whether this ASPA should sort before the other.
func (a ASPA) Less(other ASPA) bool {
	if a.CustomerASN != other.CustomerASN {
		return a.CustomerASN < other.CustomerASN
	}
	if len(a.ProviderASNs) != len(other.ProviderASNs) {
		return len(a.ProviderASNs) < len(other.ProviderASNs)
	}
	for i := range a.ProviderASNs {
		if a.ProviderASNs[i] != other.ProviderASNs[i] {
			return a.ProviderASNs[i] < other.ProviderASNs[i]
		}
	}
	return false
}

// DeduplicateASPAsInPlace sorts and deduplicates the provided slice in-place.
func DeduplicateASPAsInPlace(aspas []ASPA) []ASPA {
	if len(aspas) == 0 {
		return aspas
	}

	sort.Slice(aspas, func(i, j int) bool {
		return aspas[i].Less(aspas[j])
	})

	i := 0
	for j := 1; j < len(aspas); j++ {
		// Compare CustomerASN and ProviderASNs
		if aspas[j].CustomerASN != aspas[i].CustomerASN || len(aspas[j].ProviderASNs) != len(aspas[i].ProviderASNs) {
			i++
			aspas[i] = aspas[j]
			continue
		}
		match := true
		for k := range aspas[j].ProviderASNs {
			if aspas[j].ProviderASNs[k] != aspas[i].ProviderASNs[k] {
				match = false
				break
			}
		}
		if !match {
			i++
			aspas[i] = aspas[j]
		}
	}
	return aspas[:i+1]
}

// filterExpiredASPAs removes ASPA objects that have already expired, in-place.
func filterExpiredASPAs(aspas []ASPA, now time.Time) []ASPA {
	i := 0
	for _, a := range aspas {
		if a.Expires == 0 || time.Unix(a.Expires, 0).After(now) {
			aspas[i] = a
			i++
		}
	}
	return aspas[:i]
}
