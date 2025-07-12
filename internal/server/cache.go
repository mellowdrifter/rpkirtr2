package server

import (
	"sync"
	"time"
)

type cache struct {
	mu      sync.RWMutex
	roas    []roa
	diffs   diffs
	serial  uint32
	session uint16
}

type diffs struct {
	delRoa []roa
	addRoa []roa
	// diff indicates if there are any changes between the two serial numbers
	diff bool
}

func newCache() *cache {
	return &cache{
		diffs:   diffs{},
		serial:  0,
		session: uint16(time.Now().Unix() & 0xFFFF),
	}
}

func (c *cache) replaceRoas(roas []roa) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.roas = roas
}

func (c *cache) updateDiffs(roas, addRoa, delRoa []roa) {
	c.mu.Lock()
	defer c.mu.Unlock()
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

func (c *cache) getSerial() uint32 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.serial
}

func (c *cache) incrementSerial() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.serial += 1
}

func (c *cache) getSession() uint16 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

func (c *cache) isDiffs() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

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
