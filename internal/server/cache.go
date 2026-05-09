package server

import (
	"context"
	"sync"
	"time"
)

type cache struct {
	mu sync.RWMutex
	//TODO: Why not just store the ROAs as prefix PDUs?
	roas    []ROA
	diffs   diffs
	serial  uint32
	session uint16
}

type diffs struct {
	delRoa []ROA
	addRoa []ROA
	diff   bool
}

func newCache() *cache {
	return &cache{
		diffs:   diffs{},
		serial:  1,
		session: uint16(time.Now().Unix() & 0xFFFF),
	}
}

func (c *cache) replaceRoas(roas []ROA) {
	c.roas = roas
}

func (c *cache) updateDiffs(roas, addRoa, delRoa []ROA) {
	c.roas = roas
	c.diffs.addRoa = addRoa
	c.diffs.delRoa = delRoa
	c.diffs.diff = (len(addRoa) > 0 || len(delRoa) > 0)
}

func (c *cache) count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.roas)
}

func (c *cache) incrementSerial() {
	c.serial += 1
}

func (c *cache) isDiffs() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return (len(c.diffs.addRoa) > 0 || len(c.diffs.delRoa) > 0)
}

func (c *cache) getDiffs() (addRoa, delRoa []ROA) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.diffs.addRoa, c.diffs.delRoa
}

type cacheState struct {
	serial  uint32
	session uint16
	addRoa  []ROA
	delRoa  []ROA
	roas    []ROA
}

func (c *cache) getState() cacheState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cacheState{
		serial:  c.serial,
		session: c.session,
		addRoa:  c.diffs.addRoa,
		delRoa:  c.diffs.delRoa,
		roas:    c.roas,
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
	if diff.diff {
		s.logger.Debugf("The following ROAs were added: %v", diff.addRoa)
		s.logger.Debugf("The following ROAs were deleted: %v", diff.delRoa)
		s.lock()
		s.cache.updateDiffs(newROAs, diff.addRoa, diff.delRoa)
		s.cache.incrementSerial()
		s.unlock()
		s.notifyClients()
	} else {
		s.logger.Debugf("no diffs in ROAs. New ROA length is %d", len(newROAs))
	}
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
