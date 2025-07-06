package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

type roa struct {
	Prefix  netip.Prefix
	ASN     uint32
	MaxMask uint8
}

type diffs struct {
	old    uint32
	new    uint32
	delRoa []roa
	addRoa []roa
	diff   bool
}

type Jsonroa struct {
	Prefix string `json:"prefix"`
	Mask   uint8  `json:"maxLength"`
	ASN    any    `json:"asn"`
}

type roas struct {
	Roas []Jsonroa `json:"roas"`
}

type rpkiResponse struct {
	roas
}

func (r roa) Key() string {
	return fmt.Sprintf("%s/%d|%d|%d", r.Prefix.Addr().String(), r.Prefix.Bits(), r.MaxMask, r.ASN)
}

// GetSetOfValidatedROAs returns a slice of ROAs with no duplicates.
// It only appends if the ROA is valid
func GetSetOfValidatedROAs(roas []roa) []roa {
	u := make([]roa, 0, len(roas))
	m := make(map[roa]bool)
	for _, roa := range roas {
		if _, ok := m[roa]; !ok {
			m[roa] = true
			if roa.isValid() {
				u = append(u, roa)
			}
		}
	}
	return u
}

// https://datatracker.ietf.org/doc/html/rfc6482#section-3.3
func (roa *roa) isValid() bool {
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

func makeDiff(new, old []roa, serial uint32) diffs {
	newMap := make(map[string]roa, len(new))
	oldMap := make(map[string]roa, len(old))

	for _, r := range new {
		newMap[r.Key()] = r
	}
	for _, r := range old {
		oldMap[r.Key()] = r
	}

	var addROA, delROA []roa

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

	return diffs{
		old:    serial,
		new:    serial + 1,
		addRoa: addROA,
		delRoa: delROA,
		diff:   len(addROA) > 0 || len(delROA) > 0,
	}
}

// TODO: Any improvements in JSON 1.25 Go?
func fetchROAsFromURL(ctx context.Context, url string) ([]roa, error) {
	// Create HTTP request with context for cancellation/timeouts
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use a client with timeout
	client := http.Client{
		Timeout: 1 * time.Minute,
	}

	resp, err := client.Do(req)
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

	// Convert JSON ROAs to internal roa type
	var roas = make([]roa, len(r.Roas))
	for _, r := range r.Roas {
		// Parse prefix string to netip.Prefix
		prefix, err := netip.ParsePrefix(r.Prefix)
		if err != nil {
			return nil, fmt.Errorf("invalid prefix %q: %w", r.Prefix, err)
		}
		roas = append(roas, roa{
			Prefix:  prefix,
			MaxMask: r.Mask,
			ASN:     decodeASN(r),
		})
	}

	return roas, nil
}

// Some URLs have the AS Number as a number while others as a string.
func decodeASN(data Jsonroa) uint32 {
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
	n, err := strconv.Atoi(a[2:])
	if err != nil {
		return 0
	}

	return uint32(n)
}

func (s *Server) loadROAs(ctx context.Context) ([]roa, error) {
	var wg sync.WaitGroup
	roasCh := make(chan []roa, len(s.urls))
	errsCh := make(chan error, len(s.urls))

	fetch := func(url string) {
		defer wg.Done()
		s.logger.Debugf("Fetching ROAs from %s", url)
		roas, err := fetchROAsFromURL(ctx, url)
		if err != nil {
			errsCh <- err
			return
		}
		roasCh <- roas
	}

	wg.Add(len(s.urls))
	for _, url := range s.urls {
		go fetch(url)
	}
	wg.Wait()
	close(roasCh)
	close(errsCh)

	if len(errsCh) > 0 {
		return nil, <-errsCh
	}

	combined := []roa{}
	for r := range roasCh {
		combined = append(combined, r...)
	}

	validRoas := GetSetOfValidatedROAs(combined)

	return validRoas, nil
}

func (s *Server) periodicROAUpdater(ctx context.Context) {
	ticker := time.NewTicker(refreshROA)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.logger.Info("Checking for ROA updates...")
			newROAs, err := s.loadROAs(ctx)
			if err != nil {
				s.logger.Errorf("failed to update ROAs: %v", err)
				continue
			}

			s.mu.Lock()
			diff := makeDiff(newROAs, s.roas, s.serial)
			if diff.diff {
				s.logger.Infof("The following ROAs were added: %v", diff.addRoa)
				s.logger.Infof("The following ROAs were deleted: %v", diff.delRoa)
				s.roas = newROAs
				s.serial++
				for _, client := range s.clients {
					s.logger.Infof("Notifying client %s of new serial %d", client.ID(), s.serial)
					client.notify(s.serial, s.session)
				}
			}
			s.mu.Unlock()
		}
	}
}
