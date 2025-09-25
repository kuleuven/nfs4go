package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kuleuven/nfs4go/auth"
	"github.com/kuleuven/nfs4go/clock"
	"github.com/kuleuven/vfs"

	"github.com/sirupsen/logrus"
	"go.uber.org/multierr"
)

type Worker struct {
	vfs.AdvancedLinkFS
	Cache       *Cache
	Creds       *auth.Creds // Credentials for this worker
	SessionID   uint64
	Listers     map[uint64]*Lister  // Don't access without locking
	Files       map[uint64]*File    // Don't access without locking
	ClosedFiles map[uint64]struct{} // Don't access without locking

	inUse         int  // Number of times the worker is in use
	discarded     bool // Whether the worker is discarded
	lastUse       time.Time
	afterCleanup  func() // Function called after clean up of worker
	discardTicker *time.Ticker
	cleanupTicker *time.Ticker

	sync.Mutex
}

type NewFS func(creds *auth.Creds) vfs.AdvancedLinkFS

func New(ctx context.Context, creds *auth.Creds, newfs NewFS, afterCleanup func()) *Worker {
	w := &Worker{
		AdvancedLinkFS: newfs(creds),
		Cache:          NewCache(DefaultTimeout),
		Creds:          creds,
		Listers:        make(map[uint64]*Lister),
		Files:          make(map[uint64]*File),
		SessionID:      randUint64(),
		ClosedFiles:    make(map[uint64]struct{}),
		discardTicker:  time.NewTicker(CleanupInterval),
		cleanupTicker:  time.NewTicker(CleanupInterval),
		inUse:          1,
		lastUse:        clock.Now(),
		afterCleanup:   afterCleanup,
	}

	go w.autoDiscard(ctx)
	go w.cleanupListers()

	return w
}

var ErrWorkerDiscarded = errors.New("worker discarded")

// Use marks that the worker is in use by the current goroutine.
func (w *Worker) Use() error {
	w.Lock()
	defer w.Unlock()

	if w.discarded {
		return ErrWorkerDiscarded
	}

	w.inUse++
	w.lastUse = clock.Now()

	return nil
}

// Discard marks the worker discarded, so new goroutines will not use it.
// The worker still might be used by other goroutines.
func (w *Worker) Discard() {
	w.Lock()
	defer w.Unlock()

	if w.discarded {
		return
	}

	w.discarded = true

	if w.inUse > 0 {
		return
	}

	go w.cleanup()
}

// Close marks that the worker is no longer used by the current goroutine.
func (w *Worker) Close() error {
	w.Lock()
	defer w.Unlock()

	w.inUse--

	if w.discarded && w.inUse == 0 {
		go w.cleanup()
	}

	return nil
}

func (w *Worker) cleanup() {
	defer w.afterCleanup()

	var err error

	for _, l := range w.Listers {
		err = multierr.Append(err, l.Lister.Close())
	}

	for _, f := range w.Files {
		err = multierr.Append(err, f.File.Close())

		f.Client.Done()
	}

	w.Listers = make(map[uint64]*Lister)
	w.Files = make(map[uint64]*File)
	w.ClosedFiles = make(map[uint64]struct{})

	err = multierr.Append(err, w.AdvancedLinkFS.Close())
	if err != nil {
		logrus.Errorf("failed to close worker: %v", err)
	}
}

var CleanupInterval = 30 * time.Second

var IdleTimeout = 5 * time.Minute

func (w *Worker) autoDiscard(ctx context.Context) {
	defer w.discardTicker.Stop()

	for {
		var mustDiscard bool

		select {
		case <-ctx.Done():
			mustDiscard = true
		case <-w.discardTicker.C:
		}

		w.Lock()

		if w.discarded {
			w.Unlock()

			return
		}

		if clock.Since(w.lastUse) <= IdleTimeout && !mustDiscard {
			w.Unlock()

			continue
		}

		w.discarded = true

		if w.inUse == 0 {
			go w.cleanup()
		}

		w.Unlock()

		return
	}
}
