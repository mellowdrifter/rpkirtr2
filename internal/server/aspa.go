package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
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

func aspasEqual(a, b ASPA) bool {
	if a.CustomerASN != b.CustomerASN {
		return false
	}
	if len(a.ProviderASNs) != len(b.ProviderASNs) {
		return false
	}
	for i := range a.ProviderASNs {
		if a.ProviderASNs[i] != b.ProviderASNs[i] {
			return false
		}
	}
	return true
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

func (s *Server) fetchASPAsFromURL(ctx context.Context, url string) ([]ASPA, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	return decodeASPAsJSON(resp.Body)
}

func decodeASPAsJSON(r io.Reader) ([]ASPA, error) {
	dec := json.NewDecoder(r)

	// Seek to the "aspa" field
	t, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to read start of JSON: %w", err)
	}
	if t != json.Delim('{') {
		return nil, fmt.Errorf("expected '{', got %v", t)
	}

	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to read token: %w", err)
		}
		key, ok := t.(string)
		if !ok {
			continue
		}
		if key == "aspa" {
			break
		}
		// Skip this key's value to stay in sync
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, fmt.Errorf("failed to skip value for key %q: %w", key, err)
		}
	}

	t, err = dec.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to read start of aspa array: %w", err)
	}
	if t != json.Delim('[') {
		return nil, fmt.Errorf("expected '[', got %v", t)
	}

	aspas := make([]ASPA, 0, 10_000)
	for dec.More() {
		var a JSONASPA
		if err := dec.Decode(&a); err != nil {
			return nil, fmt.Errorf("failed to decode aspa: %w", err)
		}

		providers := make([]uint32, len(a.Providers))
		for i, p := range a.Providers {
			providers[i] = p.ASN
		}
		sort.Slice(providers, func(i, j int) bool {
			return providers[i] < providers[j]
		})
		aspas = append(aspas, ASPA{
			CustomerASN:  a.Customer,
			ProviderASNs: providers,
			Expires:      a.Expires,
		})
	}

	return aspas, nil
}

func (s *Server) loadASPAs(ctx context.Context) ([]ASPA, error) {
	if len(s.aspaURLs) == 0 {
		return nil, nil
	}

	var wg sync.WaitGroup
	aspaCh := make(chan []ASPA, len(s.aspaURLs))
	errsCh := make(chan error, len(s.aspaURLs))

	fetch := func(url string) {
		defer wg.Done()
		s.logger.Debugf("Fetching ASPAs from %s", url)
		aspas, err := s.fetchASPAsFromURL(ctx, url)

		s.upstreamsMu.Lock()
		stats, ok := s.upstreams[url]
		if !ok {
			stats = &UpstreamStatus{}
		}
		stats.LastFetchTime = time.Now()
		if err != nil {
			stats.LastFetchSuccess = false
			stats.ErrorMessage = err.Error()
			errsCh <- err
		} else {
			stats.LastFetchSuccess = true
			aspaCh <- aspas
		}
		s.upstreams[url] = stats
		s.upstreamsMu.Unlock()
	}

	wg.Add(len(s.aspaURLs))
	for _, url := range s.aspaURLs {
		go fetch(url)
	}
	wg.Wait()
	close(aspaCh)
	close(errsCh)

	for err := range errsCh {
		s.logger.Errorf("failed to fetch ASPAs from upstream: %v", err)
	}

	var allASPASlices [][]ASPA
	totalASPA := 0
	for a := range aspaCh {
		allASPASlices = append(allASPASlices, a)
		totalASPA += len(a)
	}
	combined := make([]ASPA, 0, totalASPA)
	for _, a := range allASPASlices {
		combined = append(combined, a...)
	}

	validASPAs := DeduplicateASPAsInPlace(combined)
	filteredASPAs := filterExpiredASPAs(validASPAs, time.Now())
	return filteredASPAs, nil
}
