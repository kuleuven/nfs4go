package nfs4go

import (
	"context"
	"crypto/md5" //nolint:gosec
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/kuleuven/nfs4go/auth"
	"github.com/kuleuven/nfs4go/bufpool"
	"github.com/kuleuven/nfs4go/clients"
	"github.com/kuleuven/nfs4go/logger"
	"github.com/kuleuven/nfs4go/msg"
	"github.com/kuleuven/nfs4go/worker"
	"github.com/kuleuven/vfs"
	"github.com/kuleuven/vfs/fs/errorfs"
	"github.com/sirupsen/logrus"
)

// A Server represents the NFS server. It should be created using Listen or New.
type Server struct {
	listener net.Listener
	loader   RootLoader

	clients *clients.Clients
	workers map[[16]byte]map[uint32]*worker.Worker
	wg      sync.WaitGroup
	lock    sync.Mutex
}

// Listen creates a new Server listening on the specified address and using the provided RootLoader.
func Listen(address string, loader RootLoader) (*Server, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("net.Listen: %w", err)
	}

	return New(ln, loader)
}

// RootLoader is a function that loads a root filesystem for a given connection and credentials.
type RootLoader func(ctx context.Context, conn net.Conn, creds *auth.Creds) (vfs.AdvancedLinkFS, error)

// New returns a new server with the given listener (e.g. net.Listen, tls.Listen, etc.)
func New(l net.Listener, loader RootLoader) (*Server, error) {
	return &Server{
		listener: l,
		loader:   loader,
		clients:  clients.New(),
		workers:  make(map[[16]byte]map[uint32]*worker.Worker),
	}, nil
}

// Serve serves the NFS requests using the provided context.
// It accepts incoming connections and handles them asynchronously.
// Returns an error if the server fails to start or encounters an issue during operation.
func (s *Server) Serve(ctx context.Context) error {
	defer s.wg.Wait()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Close listener on context cancel
	go func() {
		<-ctx.Done()

		s.listener.Close()
	}()

	logger.Logger.Infof("Serving NFS at %s ...", s.listener.Addr())

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				logger.Logger.Errorf("accept error: %s", err)
				time.Sleep(10 * time.Millisecond)

				continue
			}
		}

		// Disallow unprivileged ports
		if tcp, ok := conn.RemoteAddr().(*net.TCPAddr); !ok {
			logrus.Error("only TCP connections are allowed, but got: ", conn.RemoteAddr())
			conn.Close()

			continue
		} else if tcp.Port >= 1024 {
			s.wg.Add(1)

			go s.HandleTrap(ctx, conn)

			continue
		}

		s.wg.Add(1)

		go s.HandleConn(ctx, conn)
	}
}

func (s *Server) HandleTrap(ctx context.Context, conn net.Conn) {
	defer s.wg.Done()

	defer conn.Close()

	buf := make([]byte, 128)

	n, err := NewCtxReader(ctx, conn).Read(buf)
	if n == 0 && errors.Is(err, io.EOF) {
		return
	}

	if err != nil {
		logger.Logger.Errorf("failed to read trap: %v", err)

		return
	}

	logrus.Errorf("received data from unprivileged port: %d bytes from %s", n, conn.RemoteAddr())
}

type Request struct {
	Header *msg.RPCMsgCall
	Data   Bytes
}

type Response struct {
	Reply *msg.RPCMsgReply
	Data  Bytes
	Error error
}

type Bytes interface {
	bufpool.Bytes
}

// HandleConn handles the connection with the given context and network connection.
func (s *Server) HandleConn(ctx context.Context, conn net.Conn) {
	defer s.wg.Done()

	defer conn.Close()

	sess := &Conn{
		Conn:    conn,
		Clients: s.clients,
		FS: func(creds *auth.Creds, sessionID [16]byte) *worker.Worker {
			return s.GetWorker(ctx, conn, creds, sessionID)
		},
		Request:  make(chan Request, 50),
		Response: make(chan Response, 50),
	}

	if err := sess.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		var ip string

		if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
			ip = tcpAddr.IP.String()
		}

		logger.Logger.WithFields(logrus.Fields{
			"remote_ip": ip,
		}).Errorf("session failed: %v", err)
	}
}

func (s *Server) Close() error {
	return s.listener.Close()
}

func (s *Server) GetWorker(ctx context.Context, conn net.Conn, creds *auth.Creds, sessionID [16]byte) *worker.Worker {
	s.lock.Lock()
	defer s.lock.Unlock()

	// If a protocol before 4.1 is used, the session ID is not set and we need to generate one based on the client IP
	if sessionID == [16]byte{} {
		h := md5.New() //nolint:gosec

		if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
			h.Write([]byte(tcpAddr.IP.String()))
		}

		copy(sessionID[:], h.Sum(nil))
	}

	w, ok := s.workers[sessionID][creds.UID]

	switch {
	case ok && !w.Creds.Equal(creds):
		logger.Logger.Warnf("discarding old worker for %d because credentials changed: %s -> %s", creds.UID, w.Creds, creds)

		w.Discard()

	case ok:
		if err := w.Use(); err == nil {
			return w
		}
	}

	s.wg.Add(1)

	newFS := func(creds *auth.Creds) vfs.AdvancedLinkFS {
		fs, err := s.loader(ctx, conn, creds)
		if err != nil {
			logger.Logger.Errorf("failed to load root filesystem: %v", err)

			fs = errorfs.New(err)
		}

		return fs
	}

	w = worker.New(ctx, creds, newFS, s.wg.Done)

	if _, ok := s.workers[sessionID]; !ok {
		s.workers[sessionID] = make(map[uint32]*worker.Worker)
	}

	s.workers[sessionID][creds.UID] = w

	return w
}
