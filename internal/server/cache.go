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
	aspas      []ASPA
	history    []diffRecord
	serial     uint32
	session    uint16
	lastUpdate time.Time
}

type diffRecord struct {
	from    uint32
	to      uint32
	add     []ROA
	del     []ROA
	addAspa []ASPA
	delAspa []ASPA
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

func (c *cache) replaceAspas(aspas []ASPA) {
	c.aspas = aspas
}

func (c *cache) updateDiffs(roas []ROA, addRoa, delRoa []ROA, aspas []ASPA, addAspa, delAspa []ASPA) {
	c.roas = roas
	c.aspas = aspas
	newDiff := diffRecord{
		from:    c.serial,
		to:      c.serial + 1,
		add:     addRoa,
		del:     delRoa,
		addAspa: addAspa,
		delAspa: delAspa,
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

func (c *cache) getDiffsFrom(serial uint32) ([]ROA, []ROA, []ASPA, []ASPA, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if serial == c.serial {
		return nil, nil, nil, nil, true
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
		return nil, nil, nil, nil, false
	}

	// Aggregate all diffs from startIdx to the end
	var allAdd, allDel []ROA
	var allAddAspa, allDelAspa []ASPA
	for i := startIdx; i < len(c.history); i++ {
		allAdd = append(allAdd, c.history[i].add...)
		allDel = append(allDel, c.history[i].del...)
		allAddAspa = append(allAddAspa, c.history[i].addAspa...)
		allDelAspa = append(allDelAspa, c.history[i].delAspa...)
	}

	return allAdd, allDel, allAddAspa, allDelAspa, true
}


type cacheState struct {
	serial     uint32
	session    uint16
	roas       []ROA
	aspas      []ASPA
	lastUpdate time.Time
}

func (c *cache) getState() cacheState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cacheState{
		serial:     c.serial,
		session:    c.session,
		roas:       c.roas,
		aspas:      c.aspas,
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
	newASPAs, err := s.loadASPAs(ctx)
	if err != nil {
		s.logger.Warnf("failed to refresh ASPAs: %v", err)
	}
	s.updateCache(newROAs, newASPAs)
	return nil
}

func (s *Server) updateCache(newROAs []ROA, newASPAs []ASPA) {
	newROAs = filterExpired(newROAs, time.Now())
	newASPAs = filterExpiredASPAs(newASPAs, time.Now())

	s.rlock()
	roaDiff := makeDiff(newROAs, s.cache.roas)
	aspaDiff := makeASPADiff(newASPAs, s.cache.aspas)
	s.runlock()

	if len(roaDiff.addRoa) > 0 || len(roaDiff.delRoa) > 0 || len(aspaDiff.addAspa) > 0 || len(aspaDiff.delAspa) > 0 {
		s.logger.Debugf("ROA diff: %d added, %d deleted", len(roaDiff.addRoa), len(roaDiff.delRoa))
		s.logger.Debugf("ASPA diff: %d added, %d deleted", len(aspaDiff.addAspa), len(aspaDiff.delAspa))
		s.lock()
		s.cache.updateDiffs(newROAs, roaDiff.addRoa, roaDiff.delRoa, newASPAs, aspaDiff.addAspa, aspaDiff.delAspa)
		s.cache.incrementSerial()
		s.cache.lastUpdate = time.Now()
		s.unlock()
		s.notifyClients()
	} else {
		s.logger.Debugf("no diffs in ROAs or ASPAs.")
	}
}

type aspaDiffResult struct {
	addAspa []ASPA
	delAspa []ASPA
}

func makeASPADiff(new, old []ASPA) aspaDiffResult {
	newMap := make(map[string]ASPA, len(new))
	oldMap := make(map[string]ASPA, len(old))

	for _, a := range new {
		newMap[a.Key()] = a
	}
	for _, a := range old {
		oldMap[a.Key()] = a
	}

	var addASPA, delASPA []ASPA

	for k, a := range newMap {
		if _, exists := oldMap[k]; !exists {
			addASPA = append(addASPA, a)
		}
	}

	for k, a := range oldMap {
		if _, exists := newMap[k]; !exists {
			delASPA = append(delASPA, a)
		}
	}

	return aspaDiffResult{
		addAspa: addASPA,
		delAspa: delASPA,
	}
}

// UpdateROAs manually triggers a cache update with the provided ROAs,
// generating diffs and incrementing the serial number. This is primarily for testing.
func (s *Server) UpdateROAs(roas []ROA) {
	s.updateCache(roas, s.cache.aspas)
}

// UpdateASPAs manually triggers a cache update with the provided ASPAs.
func (s *Server) UpdateASPAs(aspas []ASPA) {
	s.updateCache(s.cache.roas, aspas)
}

func (s *Server) notifyClients() {
	s.clientsMu.RLock()
	clients := make([]*Client, 0, len(s.clients))
	for _, client := range s.clients {
		clients = append(clients, client)
	}
	s.clientsMu.RUnlock()

	if len(clients) == 0 {
		return
	}

	// Notify clients in the background to avoid blocking the server's update loop
	// if a client is slow or dead.
	go func() {
		for _, client := range clients {
			s.logger.Infof("Notifying client %s of new serial %d", client.ID(), s.getSerial())
			client.notify()
		}
	}()
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
