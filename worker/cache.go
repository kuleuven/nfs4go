package worker

import (
	"sync"
	"time"

	"github.com/kuleuven/nfs4go/clock"
	"github.com/kuleuven/vfs"
)

type Cache struct {
	Timeout time.Duration
	cache   map[string]savedEntry
	sync.Mutex
}

type Entry struct {
	Path string
	vfs.FileInfo
}

type savedEntry struct {
	time time.Time
	Entry
}

var DefaultTimeout = 30 * time.Second

func NewCache(timeout time.Duration) *Cache {
	c := &Cache{
		Timeout: timeout,
		cache:   map[string]savedEntry{},
	}

	go c.gc()

	return c
}

func (c *Cache) Get(handle []byte) (Entry, bool) {
	c.Lock()
	defer c.Unlock()

	entry, ok := c.cache[string(handle)]
	if !ok {
		return Entry{}, false
	}

	if clock.Since(entry.time) > c.Timeout {
		return Entry{}, false
	}

	return entry.Entry, true
}

func (c *Cache) Put(handle []byte, entry Entry) {
	c.Lock()
	defer c.Unlock()

	c.cache[string(handle)] = savedEntry{
		time:  clock.Now(),
		Entry: entry,
	}
}

func (c *Cache) Invalidate(handle []byte) {
	c.Lock()
	defer c.Unlock()

	delete(c.cache, string(handle))
}

func (c *Cache) gc() {
	for {
		c.Lock()

		for handle, entry := range c.cache {
			if clock.Since(entry.time) > c.Timeout {
				delete(c.cache, handle)
			}
		}

		c.Unlock()

		time.Sleep(c.Timeout)
	}
}
