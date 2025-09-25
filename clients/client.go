package clients

import (
	"encoding/binary"
	"sync"
	"time"

	"github.com/kuleuven/nfs4go/auth"
	"github.com/kuleuven/nfs4go/bufpool"
)

type Client struct {
	Name     []byte
	Verifier uint64
	Creds    *auth.Creds

	clientID     uint64
	lastSeen     time.Time
	confirmed    bool
	confirmValue uint64
	seqID        uint32
	busy         int
	lock         *sync.Mutex

	sessions map[uint64]Session
}

type Session []*Slot

type Slot struct {
	SlotID       uint32
	SequenceID   uint32
	ContainsData bool
	Buf          bufpool.Bytes
}

// The maximum slot id for use in a session (slots start at 0)
var MaxSlotID = uint32(15)

// Mark that the client is in use by a file
func (c *Client) Add(n int) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.busy += n
}

func (c *Client) Done() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.busy--
}

func (c *Client) BuildSession(persist bool) [16]byte {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.sessions == nil {
		c.sessions = make(map[uint64]Session)
	}

	cacheID := randUint64()

	for _, ok := c.sessions[cacheID]; ok; _, ok = c.sessions[cacheID] {
		cacheID = randUint64()
	}

	buf := [16]byte{}

	binary.BigEndian.PutUint64(buf[:8], c.clientID)
	binary.BigEndian.PutUint64(buf[8:], cacheID)

	c.sessions[cacheID] = make([]*Slot, MaxSlotID+1)

	for i := uint32(0); i <= MaxSlotID; i++ {
		c.sessions[cacheID][i] = &Slot{
			SlotID: i,
		}

		if persist {
			c.sessions[cacheID][i].Buf = bufpool.Get()
		}
	}

	return buf
}

func (c *Client) GetSlot(sessionID [16]byte, slotID uint32) *Slot {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.sessions == nil {
		return nil
	}

	cacheID := CacheIDFromSessionID(sessionID)

	if _, ok := c.sessions[cacheID]; !ok {
		return nil
	}

	if len(c.sessions[cacheID]) <= int(slotID) {
		return nil
	}

	return c.sessions[cacheID][slotID]
}

func (c *Client) RemoveSession(sessionID [16]byte) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.sessions == nil {
		return
	}

	cacheID := CacheIDFromSessionID(sessionID)

	if _, ok := c.sessions[cacheID]; !ok {
		return
	}

	for _, slot := range c.sessions[cacheID] {
		if slot.Buf != nil {
			bufpool.Put(slot.Buf)
		}
	}

	delete(c.sessions, cacheID)
}

func ClientIDFromSessionID(sessionID [16]byte) uint64 {
	return binary.BigEndian.Uint64(sessionID[:8])
}

func CacheIDFromSessionID(sessionID [16]byte) uint64 {
	return binary.BigEndian.Uint64(sessionID[8:])
}
