package server

import (
	"context"
	"sync"
	"time"
)

type cache struct {
	mu sync.RWMutex
	//TODO: Why not just store the ROAs as prefix PDUs?
	roas    []roa
	diffs   diffs
	serial  uint32
	session uint16
}

type diffs struct {
	delRoa []roa
	addRoa []roa
	diff   bool
}

func newCache() *cache {
	return &cache{
		diffs:   diffs{},
		serial:  0,
		session: uint16(time.Now().Unix() & 0xFFFF),
	}
}

func (c *cache) replaceRoas(roas []roa) {
	c.roas = roas
}

func (c *cache) updateDiffs(roas, addRoa, delRoa []roa) {
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
	return (len(c.diffs.addRoa) > 0 || len(c.diffs.delRoa) > 0)
}

func (c *cache) getDiffs() (addRoa, delRoa []roa) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.diffs.addRoa, c.diffs.delRoa
}

func (c *cache) getRoas() []roa {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.roas
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

			s.rlock()
			diff := makeDiff(newROAs, s.cache.roas)
			s.runlock()
			if s.cache.isDiffs() {
				s.logger.Infof("The following ROAs were added: %v", diff.addRoa)
				s.logger.Infof("The following ROAs were deleted: %v", diff.delRoa)
				s.lock()
				s.cache.updateDiffs(newROAs, diff.addRoa, diff.delRoa)
				s.cache.incrementSerial()
				s.unlock()
				for _, client := range s.clients {
					s.logger.Infof("Notifying client %s of new serial %d", client.ID(), s.getSerial())
					client.notify()
				}
			}
		}
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
	return s.cache.serial
}
