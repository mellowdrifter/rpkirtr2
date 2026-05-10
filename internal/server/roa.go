package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ROA struct {
	Prefix  netip.Prefix
	ASN     uint32
	MaxMask uint8
	Expires int64 // Unix timestamp; 0 means no expiry information
}

type JSONROA struct {
	Prefix  string  `json:"prefix"`
	Mask    uint8   `json:"maxLength"`
	ASN     jsonASN `json:"asn"`
	Expires int64   `json:"expires"`
}

type jsonASN uint32

func (a *jsonASN) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*a = jsonASN(asnToUint32(s))
		return nil
	}
	var n uint32
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*a = jsonASN(n)
	return nil
}



type roaKey struct {
	Prefix  netip.Prefix
	ASN     uint32
	MaxMask uint8
}

func (rk roaKey) Less(other roaKey) bool {
	if c := rk.Prefix.Addr().Compare(other.Prefix.Addr()); c != 0 {
		return c < 0
	}
	if rk.Prefix.Bits() != other.Prefix.Bits() {
		return rk.Prefix.Bits() < other.Prefix.Bits()
	}
	if rk.ASN != other.ASN {
		return rk.ASN < other.ASN
	}
	return rk.MaxMask < other.MaxMask
}

func (r ROA) key() roaKey {
	return roaKey{
		Prefix:  r.Prefix,
		ASN:     r.ASN,
		MaxMask: r.MaxMask,
	}
}

// GetSetOfValidatedROAs returns a slice of ROAs with no duplicates, in-place.
// It only appends if the ROA is valid.
func GetSetOfValidatedROAs(roas []ROA) []ROA {
	if len(roas) == 0 {
		return nil
	}

	// Sort first to allow easy deduplication
	sort.Slice(roas, func(i, j int) bool {
		return roas[i].key().Less(roas[j].key())
	})

	i := 0
	for j := 0; j < len(roas); j++ {
		if !roas[j].isValid() {
			continue
		}
		// If it's the first kept element or different from the previous one, keep it
		if i == 0 || roas[j].key() != roas[i-1].key() {
			roas[i] = roas[j]
			i++
		}
	}
	return roas[:i]
}

// https://datatracker.ietf.org/doc/html/rfc6482#section-3.3
func (roa *ROA) isValid() bool {
	// MaxLength cannot be zero or negative
	// MaxMask is a uint8 so cannot be negative
	if roa.MaxMask == 0 {
		return false
	}

	// MaxLength cannot be smaller than prefix length
	if roa.MaxMask < uint8(roa.Prefix.Bits()) {
		return false
	}

	// MaxLength cannot be larger than the max allowed for that address family
	if roa.Prefix.Addr().Is4() && roa.MaxMask > 32 {
		return false
	} else if roa.MaxMask > 128 {
		return false
	}

	return true
}

type diffResult struct {
	addRoa []ROA
	delRoa []ROA
}

func makeDiff(new, old []ROA) diffResult {
	// Slices are already sorted by loadROAs and previous updateCache

	var addROA, delROA []ROA
	i, j := 0, 0
	for i < len(new) && j < len(old) {
		nk := new[i].key()
		ok := old[j].key()

		if nk == ok {
			// Same ROA, skip both
			i++
			j++
		} else if nk.Less(ok) {
			// New ROA is smaller, so it must be added
			addROA = append(addROA, new[i])
			i++
		} else {
			// Old ROA is smaller, so it must be deleted
			delROA = append(delROA, old[j])
			j++
		}
	}

	// Append remaining new ROAs
	for ; i < len(new); i++ {
		addROA = append(addROA, new[i])
	}
	// Append remaining old ROAs
	for ; j < len(old); j++ {
		delROA = append(delROA, old[j])
	}

	return diffResult{
		addRoa: addROA,
		delRoa: delROA,
	}
}

// TODO: Any improvements in JSON 1.25 Go?
func (s *Server) fetchROAsFromURL(ctx context.Context, url string) ([]ROA, error) {
	// Create HTTP request with context for cancellation/timeouts
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

	return decodeROAsJSON(resp.Body)
}

func decodeROAsJSON(r io.Reader) ([]ROA, error) {
	// Use streaming decoder to avoid loading entire JSON into memory
	dec := json.NewDecoder(r)

	// Seek to the "roas" field
	// Expected format: { "roas": [ ... ] }
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
		if t == "roas" {
			break
		}
	}

	t, err = dec.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to read start of roas array: %w", err)
	}
	if t != json.Delim('[') {
		return nil, fmt.Errorf("expected '[', got %v", t)
	}

	var roas []ROA
	for dec.More() {
		var r JSONROA
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("failed to decode roa: %w", err)
		}

		prefix, err := netip.ParsePrefix(r.Prefix)
		if err != nil {
			return nil, fmt.Errorf("invalid prefix %q: %w", r.Prefix, err)
		}
		roas = append(roas, ROA{
			Prefix:  prefix,
			MaxMask: r.Mask,
			ASN:     uint32(r.ASN),
			Expires: r.Expires,
		})
	}

	return roas, nil
}

func filterExpired(roas []ROA, now time.Time) []ROA {
	i := 0
	for _, r := range roas {
		if r.Expires == 0 || time.Unix(r.Expires, 0).After(now) {
			roas[i] = r
			i++
		}
	}
	return roas[:i]
}


// Some json VRPs contain ASXXX instead of just XXX as the ASN
// TODO: Use a regex to remove letter instead of assuming its the first two
func asnToUint32(a string) uint32 {
	n, err := strconv.Atoi(strings.TrimLeft(a, "ASas"))
	if err != nil {
		return 0
	}

	return uint32(n)
}

func (s *Server) loadROAs(ctx context.Context) ([]ROA, error) {
	var wg sync.WaitGroup
	roasCh := make(chan []ROA, len(s.urls))
	errsCh := make(chan error, len(s.urls))

	fetch := func(url string) {
		defer wg.Done()
		s.logger.Debugf("Fetching ROAs from %s", url)
		roas, err := s.fetchROAsFromURL(ctx, url)
		
		s.upstreamsMu.Lock()
		stats := &UpstreamStatus{
			LastFetchTime: time.Now(),
		}
		if err != nil {
			stats.LastFetchSuccess = false
			stats.ErrorMessage = err.Error()
			errsCh <- err
		} else {
			stats.LastFetchSuccess = true
			roasCh <- roas
		}
		s.upstreams[url] = stats
		s.upstreamsMu.Unlock()
		
		if err == nil {
			s.logger.Debugf("Roas retrieved from %s", url)
		}
	}

	wg.Add(len(s.urls))
	for _, url := range s.urls {
		go fetch(url)
	}
	wg.Wait()
	close(roasCh)
	close(errsCh)

	// Log any errors that occurred
	for err := range errsCh {
		s.logger.Errorf("failed to fetch ROAs from upstream: %v", err)
	}

	var allSlices [][]ROA
	total := 0
	for r := range roasCh {
		allSlices = append(allSlices, r)
		total += len(r)
	}

	combined := make([]ROA, 0, total)
	for _, r := range allSlices {
		combined = append(combined, r...)
	}

	// If we have no ROAs but we did have URLs configured, something went wrong.
	if len(combined) == 0 && len(s.urls) > 0 {
		return nil, fmt.Errorf("failed to fetch ROAs from any configured URL")
	}

	validRoas := GetSetOfValidatedROAs(combined)
	filteredRoas := filterExpired(validRoas, time.Now())

	return filteredRoas, nil
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
		if t == "aspa" {
			break
		}
	}

	t, err = dec.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to read start of aspa array: %w", err)
	}
	if t != json.Delim('[') {
		return nil, fmt.Errorf("expected '[', got %v", t)
	}

	var aspas []ASPA
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
