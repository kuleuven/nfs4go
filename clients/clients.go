package clients

import (
	"bytes"
	"sync"
	"time"

	"github.com/kuleuven/nfs4go/auth"
	"github.com/kuleuven/nfs4go/bufpool"
	"github.com/kuleuven/nfs4go/clock"
	"github.com/kuleuven/nfs4go/msg"
	"github.com/sirupsen/logrus"
)

type Clients struct {
	clients map[uint64]*Client
	sync.Mutex
}

func New() *Clients {
	c := &Clients{
		clients: map[uint64]*Client{},
	}

	go c.cleanup()

	return c
}

// Add a client, used for EXCHANGE_ID and SET_CLIENTID.
func (x *Clients) Add(client Client) (clientID, confirmValue uint64, seqID uint32, err error) { //nolint:nonamedreturns
	x.Lock()
	defer x.Unlock()

	prev := map[uint64]*Client{}

	for id, stored := range x.clients {
		if !stored.confirmed {
			continue
		}

		if !bytes.Equal(stored.Name, client.Name) {
			continue
		}

		if !stored.Creds.Equal(client.Creds) {
			return 0, 0, 0, msg.Error(msg.NFS4ERR_CLID_INUSE)
		}

		prev[id] = stored
	}

	for id, stored := range prev {
		if stored.Verifier != client.Verifier || !stored.confirmed {
			continue
		}

		// There is a update of the current client
		stored.confirmValue = randUint64()
		stored.seqID++
		stored.lastSeen = clock.Now()

		return id, stored.confirmValue, stored.seqID, nil
	}

	// Generate new id
	id := randUint64()

	for _, ok := x.clients[id]; ok; _, ok = x.clients[id] {
		id = randUint64()
	}

	// Remove other unconfirmed clients for the same Name
	for o, stored := range x.clients {
		if stored.confirmed || !bytes.Equal(stored.Name, client.Name) {
			continue
		}

		delete(x.clients, o)
	}

	// Store client
	client.clientID = id
	client.confirmValue = randUint64()
	client.seqID = 1
	client.lastSeen = clock.Now()
	client.lock = &x.Mutex

	x.clients[id] = &client

	return id, client.confirmValue, client.seqID, nil
}

// Confirm a client, used for SET_CLIENTID_CONFIRM
func (x *Clients) Confirm(clientID, confirmValue uint64, creds *auth.Creds) (*Client, error) {
	x.Lock()
	defer x.Unlock()

	client, ok := x.clients[clientID]
	if !ok || client.confirmValue != confirmValue {
		return nil, msg.Error(msg.NFS4ERR_STALE_CLIENTID)
	}

	if !client.Creds.Equal(creds) {
		return nil, msg.Error(msg.NFS4ERR_CLID_INUSE)
	}

	client.confirmed = true
	client.lastSeen = clock.Now()

	return client, nil
}

// Confirm a client, used for CREATE_SESSION
func (x *Clients) Confirm41(clientID uint64, seqID uint32, creds *auth.Creds) (*Client, error) {
	x.Lock()
	defer x.Unlock()

	client, ok := x.clients[clientID]
	if !ok {
		return nil, msg.Error(msg.NFS4ERR_STALE_CLIENTID)
	}

	if client.confirmed {
		if seqID == 0 && client.seqID != 0 { // First session after confirmation
			client.lastSeen = clock.Now()
			client.seqID = 0

			return client, nil
		}

		if seqID < client.seqID {
			return nil, msg.Error(msg.NFS4ERR_SEQ_MISORDERED)
		}

		if client.seqID == seqID { // Replay of previous CREATE_SESSION
			return client, nil
		}

		client.lastSeen = clock.Now()
		client.seqID = seqID

		return client, nil
	}

	if client.seqID != seqID {
		return nil, msg.Error(msg.NFS4ERR_STALE_CLIENTID)
	}

	if !client.Creds.Equal(creds) {
		return nil, msg.Error(msg.NFS4ERR_CLID_INUSE)
	}

	client.confirmed = true
	client.lastSeen = clock.Now()

	return client, nil
}

// Get a client, used for RENEW
func (x *Clients) Get(clientID uint64) (*Client, bool) {
	x.Lock()
	defer x.Unlock()

	c, ok := x.clients[clientID]

	if !ok || !c.confirmed {
		return nil, false
	}

	c.lastSeen = clock.Now()

	return c, true
}

// Get a client by its name, used for EXCHANGE_ID
func (x *Clients) GetByName(name []byte, verifier uint64, creds *auth.Creds) (uint64, bool) {
	x.Lock()
	defer x.Unlock()

	for clientID, c := range x.clients {
		if !bytes.Equal(c.Name, name) || c.Verifier != verifier || !c.Creds.Equal(creds) || !c.confirmed {
			continue
		}

		c.lastSeen = clock.Now()

		return clientID, true
	}

	return 0, false
}

func (x *Clients) RemoveClient(clientID uint64) error {
	x.Lock()
	defer x.Unlock()

	c, ok := x.clients[clientID]

	if !ok || !c.confirmed {
		return msg.Error(msg.NFS4ERR_STALE_CLIENTID)
	}

	if c.busy > 0 {
		return msg.Error(msg.NFS4ERR_CLIENTID_BUSY)
	}

	delete(x.clients, clientID)

	return nil
}

func (x *Clients) RemoveExpiredClients(expiration time.Duration) []uint64 {
	x.Lock()
	defer x.Unlock()

	now := clock.Now()

	var removed []uint64

	for index, client := range x.clients {
		if now.Sub(client.lastSeen) <= expiration || client.busy > 0 {
			continue
		}

		logrus.Infof("removing expired client %d: %s", index, client.Creds.Hostname)

		delete(x.clients, index)

		removed = append(removed, index)

		for _, slots := range client.sessions {
			for _, slot := range slots {
				if slot.Buf != nil {
					bufpool.Put(slot.Buf)
				}
			}
		}
	}

	return removed
}

var ClientExpiration = 5 * time.Minute

func (x *Clients) cleanup() {
	ticker := time.NewTicker(ClientExpiration / 2)

	defer ticker.Stop()

	for range ticker.C {
		x.RemoveExpiredClients(ClientExpiration)
	}
}
