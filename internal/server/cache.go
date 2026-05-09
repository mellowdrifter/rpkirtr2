package server

import (
	"context"
	"sync"
	"time"
)

const maxHistory = 10

type cache struct {
	mu sync.RWMutex
	//TODO: Why not just store the ROAs as prefix PDUs?
	roas       []ROA
	history    []diffRecord
	serial     uint32
	session    uint16
	lastUpdate time.Time
}

type diffRecord struct {
	from uint32
	to   uint32
	add  []ROA
	del  []ROA
}

func newCache() *cache {
	return &cache{
		history: make([]diffRecord, 0, maxHistory),
		serial:  1,
		session: uint16(time.Now().Unix() & 0xFFFF),
	}
}

func (c *cache) replaceRoas(roas []ROA) {
	c.roas = roas
}

func (c *cache) updateDiffs(roas, addRoa, delRoa []ROA) {
	c.roas = roas
	newDiff := diffRecord{
		from: c.serial,
		to:   c.serial + 1,
		add:  addRoa,
		del:  delRoa,
	}
	c.history = append(c.history, newDiff)
	if len(c.history) > maxHistory {
		c.history = c.history[1:]
	}
}

func (c *cache) count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.roas)
}

func (c *cache) incrementSerial() {
	c.serial += 1
}

func (c *cache) getDiffsFrom(serial uint32) ([]ROA, []ROA, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if serial == c.serial {
		return nil, nil, true
	}

	// Find the sequence of diffs starting from 'serial'
	var startIdx = -1
	for i, d := range c.history {
		if d.from == serial {
			startIdx = i
			break
		}
	}

	if startIdx == -1 {
		return nil, nil, false
	}

	// Aggregate all diffs from startIdx to the end
	var allAdd, allDel []ROA
	for i := startIdx; i < len(c.history); i++ {
		allAdd = append(allAdd, c.history[i].add...)
		allDel = append(allDel, c.history[i].del...)
	}

	return allAdd, allDel, true
}


type cacheState struct {
	serial     uint32
	session    uint16
	roas       []ROA
	lastUpdate time.Time
}

func (c *cache) getState() cacheState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cacheState{
		serial:     c.serial,
		session:    c.session,
		roas:       c.roas,
		lastUpdate: c.lastUpdate,
	}
}

func (c *cache) getRoas() []ROA {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy instead of the same slice reference
	roasCopy := make([]ROA, len(c.roas))
	copy(roasCopy, c.roas)
	return roasCopy
}

func (s *Server) periodicROAUpdater(ctx context.Context) {
	ticker := time.NewTicker(refreshROA)
	if s.cfg.LogLevel == "debug" {
		ticker = time.NewTicker(1 * time.Minute)
	}
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.logger.Info("Checking for ROA updates...")
			if err := s.TriggerRefresh(ctx); err != nil {
				s.logger.Errorf("failed to update ROAs: %v", err)
			}
		}
	}
}

// TriggerRefresh forces a reload of ROAs from all configured URLs.
func (s *Server) TriggerRefresh(ctx context.Context) error {
	newROAs, err := s.loadROAs(ctx)
	if err != nil {
		return err
	}
	s.updateCache(newROAs)
	return nil
}

func (s *Server) updateCache(newROAs []ROA) {
	newROAs = filterExpired(newROAs, time.Now())

	s.rlock()
	diff := makeDiff(newROAs, s.cache.roas)
	s.runlock()
	if len(diff.addRoa) > 0 || len(diff.delRoa) > 0 {
		s.logger.Debugf("The following ROAs were added: %v", diff.addRoa)
		s.logger.Debugf("The following ROAs were deleted: %v", diff.delRoa)
		s.lock()
		s.cache.updateDiffs(newROAs, diff.addRoa, diff.delRoa)
		s.cache.incrementSerial()
		s.cache.lastUpdate = time.Now()
		s.unlock()
		s.notifyClients()
	} else {
		s.logger.Debugf("no diffs in ROAs. New ROA length is %d", len(newROAs))
	}
}

// UpdateROAs manually triggers a cache update with the provided ROAs,
// generating diffs and incrementing the serial number. This is primarily for testing.
func (s *Server) UpdateROAs(roas []ROA) {
	s.updateCache(roas)
}

func (s *Server) notifyClients() {
	s.clientsMu.RLock()
	clients := make([]*Client, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}
	s.clientsMu.RUnlock()

	for _, client := range clients {
		s.logger.Infof("Notifying client %s of new serial %d", client.ID(), s.getSerial())
		client.notify()
	}
}

func (s *Server) lock() {
	s.cache.mu.Lock()
}

func (s *Server) unlock() {
	s.cache.mu.Unlock()
}

func (s *Server) rlock() {
	s.cache.mu.RLock()
}

func (s *Server) runlock() {
	s.cache.mu.RUnlock()
}

func (s *Server) getSerial() uint32 {
	s.cache.mu.RLock()
	defer s.cache.mu.RUnlock()
	return s.cache.serial
}

func (s *Server) getSession() uint16 {
	s.cache.mu.RLock()
	defer s.cache.mu.RUnlock()
	return s.cache.session
}
