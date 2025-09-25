package clock

import (
	"sync"
	"time"
)

type Clock struct {
	init   sync.Once
	lock   sync.Mutex
	ticker *time.Ticker
	now    time.Time
}

// Now returns the current time of the clock.
func (c *Clock) Now() time.Time {
	c.init.Do(c.start)
	c.lock.Lock()

	defer c.lock.Unlock()

	return c.now
}

// Since returns the time elapsed since t.
func (c *Clock) Since(t time.Time) time.Duration {
	c.init.Do(c.start)
	c.lock.Lock()

	defer c.lock.Unlock()

	return c.now.Sub(t)
}

// MustIncrement returns the actual time, but makes sure
// that it is later than prev. If needed, the clock will be
// incremented by one second.
func (c *Clock) MustIncrement(prev time.Time) time.Time {
	c.init.Do(c.start)
	c.lock.Lock()

	defer c.lock.Unlock()

	now := c.now

	if now.Before(prev) {
		now = now.Add(time.Second)
	}

	return now
}

func (c *Clock) start() {
	c.now = time.Now()
	c.ticker = time.NewTicker(time.Second)

	go func() {
		for t := range c.ticker.C {
			c.set(t)
		}
	}()
}

func (c *Clock) set(t time.Time) {
	c.lock.Lock()

	defer c.lock.Unlock()

	c.now = t
}
