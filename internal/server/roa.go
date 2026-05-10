package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
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
	Prefix string `json:"prefix"`
	Mask   uint8  `json:"maxLength"`
	ASN    any    `json:"asn"`
	Expires int64 `json:"expires"`
}

type roas struct {
	Roas []JSONROA `json:"roas"`
}

type rpkiResponse struct {
	roas
}

func (r ROA) Key() string {
	return fmt.Sprintf("%s/%d|%d|%d", r.Prefix.Addr().String(), r.Prefix.Bits(), r.MaxMask, r.ASN)
}

// GetSetOfValidatedROAs returns a slice of ROAs with no duplicates.
// It only appends if the ROA is valid
func GetSetOfValidatedROAs(roas []ROA) []ROA {
	u := make([]ROA, 0, len(roas))
	m := make(map[ROA]struct{})
	for _, roa := range roas {
		if _, ok := m[roa]; !ok {
			m[roa] = struct{}{}
			if roa.isValid() {
				u = append(u, roa)
			}
		}
	}
	return u
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
	newMap := make(map[string]ROA, len(new))
	oldMap := make(map[string]ROA, len(old))

	for _, r := range new {
		newMap[r.Key()] = r
	}
	for _, r := range old {
		oldMap[r.Key()] = r
	}

	var addROA, delROA []ROA

	for k, r := range newMap {
		if _, exists := oldMap[k]; !exists {
			addROA = append(addROA, r)
		}
	}

	for k, r := range oldMap {
		if _, exists := newMap[k]; !exists {
			delROA = append(delROA, r)
		}
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

	// Decode JSON array
	var r rpkiResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("failed to decode json: %w", err)
	}

	// Convert JSON ROAs to internal ROA type
	roas := make([]ROA, 0, len(r.Roas))
	for _, r := range r.Roas {
		// Parse prefix string to netip.Prefix
		prefix, err := netip.ParsePrefix(r.Prefix)
		if err != nil {
			return nil, fmt.Errorf("invalid prefix %q: %w", r.Prefix, err)
		}
		roas = append(roas, ROA{
			Prefix:  prefix,
			MaxMask: r.Mask,
			ASN:     decodeASN(r),
			Expires: r.Expires,
		})
	}

	return roas, nil
}

func filterExpired(roas []ROA, now time.Time) []ROA {
	out := make([]ROA, 0, len(roas))
	for _, r := range roas {
		if r.Expires == 0 || time.Unix(r.Expires, 0).After(now) {
			out = append(out, r)
		}
	}
	return out
}

// Some URLs have the AS Number as a number while others as a string.
func decodeASN(data JSONROA) uint32 {
	switch atype := data.ASN.(type) {
	case string:
		return asnToUint32(atype)
	case float64:
		return uint32(atype)
	}
	return 0
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

	combined := []ROA{}
	for r := range roasCh {
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

	var r aspaResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("failed to decode json: %w", err)
	}

	aspas := make([]ASPA, 0, len(r.Aspas))
	for _, a := range r.Aspas {
		providers := make([]uint32, len(a.Providers))
		for i, p := range a.Providers {
			providers[i] = p.ASN
		}
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

	combined := []ASPA{}
	for a := range aspaCh {
		combined = append(combined, a...)
	}

	filteredASPAs := filterExpiredASPAs(combined, time.Now())
	return filteredASPAs, nil
}
