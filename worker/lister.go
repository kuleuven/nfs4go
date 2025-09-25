package worker

import (
	"time"

	"github.com/kuleuven/nfs4go/clock"
	"github.com/kuleuven/vfs"
	"github.com/sirupsen/logrus"
)

type Lister struct {
	Lister   vfs.ListerAt
	lastSeen time.Time
}

var EOFLister = uint64(0xffffffffffffffff)

var ListerExpiration = time.Minute

func (w *Worker) AddLister(lister *Lister) uint64 {
	w.Lock()
	defer w.Unlock()

	var index uint64

	for _, ok := w.Listers[index]; ok || index == EOFLister; _, ok = w.Listers[index] {
		index = randUint64()
	}

	lister.lastSeen = clock.Now()

	w.Listers[index] = lister

	return index
}

func (w *Worker) GetLister(index uint64) (*Lister, bool) {
	w.Lock()
	defer w.Unlock()

	lister, ok := w.Listers[index]
	if !ok {
		return nil, false
	}

	lister.lastSeen = clock.Now()

	return lister, ok
}

func (w *Worker) CloseLister(index uint64) error {
	w.Lock()
	defer w.Unlock()

	lister, ok := w.Listers[index]
	if !ok {
		return nil
	}

	delete(w.Listers, index)

	return lister.Lister.Close()
}

func (w *Worker) cleanupListers() {
	defer w.cleanupTicker.Stop()

	for range w.cleanupTicker.C {
		w.Lock()

		if w.discarded {
			w.Unlock()

			return
		}

		for index, lister := range w.Listers {
			if clock.Since(lister.lastSeen) <= ListerExpiration {
				continue
			}

			delete(w.Listers, index)

			if err := lister.Lister.Close(); err != nil {
				logrus.Errorf("failed to close lister: %v", err)
			}
		}

		w.Unlock()
	}
}
