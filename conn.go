package nfs4go

import (
	"bufio"
	"context"
	"net"
	"sync"

	"github.com/kuleuven/nfs4go/auth"
	"github.com/kuleuven/nfs4go/clients"
	"github.com/kuleuven/nfs4go/logger"
	"github.com/kuleuven/nfs4go/worker"
	"go.uber.org/multierr"
)

// Conn represents an NFS connection
type Conn struct {
	Conn    net.Conn
	Clients *clients.Clients

	FS func(creds *auth.Creds, sessionID [16]byte) *worker.Worker

	Request  chan Request
	Response chan Response

	wg  sync.WaitGroup
	err error
	sync.Mutex
}

func (c *Conn) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)

	c.wg.Add(3)

	go c.ReceiveRequests(ctx)
	go c.RunMux()
	go c.SendReplies(cancel)

	c.wg.Wait()

	return c.err
}

func (c *Conn) ServeLinear(ctx context.Context) error {
	conn := NewCtxReader(ctx, c.Conn)

	defer close(c.Request)

	mux4 := &Muxv4{
		Clients: c.Clients,
		FS:      c.FS,
		Logger:  logger.Logger.WithField("remote", c.Conn.RemoteAddr().String()),
	}
	muxOther := &MuxMismatch{}

	response := make(chan Response, 1)

	for {
		header, data, err := ReceiveCall(conn)
		if err != nil {
			return err
		}

		request := Request{
			Header: header,
			Data:   data,
		}

		switch header.Vers {
		case 4:
			mux4.Handle(request, response)
		default:
			muxOther.Handle(request, response)
		}

		resp := <-response

		if resp.Error != nil {
			return resp.Error
		}

		err = SendReply(c.Conn, resp.Reply, resp.Data)
		if err != nil {
			return err
		}
	}
}

type Mux interface {
	Handle(request Request, response chan<- Response)
}

func (c *Conn) RunMux() {
	defer c.wg.Done()

	defer close(c.Response)

	mux4 := &Muxv4{
		Clients: c.Clients,
		FS:      c.FS,
		Logger:  logger.Logger.WithField("remote", c.Conn.RemoteAddr().String()),
	}
	muxOther := &MuxMismatch{}

	var muxwg sync.WaitGroup

	for request := range c.Request {
		muxwg.Add(1)

		go func(request Request) {
			defer muxwg.Done()

			switch request.Header.Vers {
			case 4:
				mux4.Handle(request, c.Response)
			default:
				muxOther.Handle(request, c.Response)
			}
		}(request)
	}

	muxwg.Wait()
}

func (c *Conn) ReceiveRequests(ctx context.Context) {
	defer c.wg.Done()

	defer close(c.Request)

	r := bufio.NewReaderSize(c.Conn, 10*65536)

	intermediate := make(chan Request, 1)

	// Spawn a possibly blocking thread.
	// It will block when no message is received.
	go func() {
		defer close(intermediate)

		for ctx.Err() == nil {
			header, data, err := ReceiveCall(r)
			if err != nil {
				c.appendError(err)

				return
			}

			intermediate <- Request{
				Header: header,
				Data:   data,
			}
		}
	}()

	// Proxy the requests and don't block if the context is cancelled
	for {
		select {
		case <-ctx.Done():
			return
		case req, ok := <-intermediate:
			if !ok {
				return
			}

			c.Request <- req
		}
	}
}

func (c *Conn) SendReplies(cancel context.CancelFunc) {
	defer c.wg.Done()

	w := bufio.NewWriterSize(c.Conn, 10*65536)

	for {
		if len(c.Response) == 0 {
			if err := w.Flush(); err != nil {
				cancel()

				c.appendError(err)
			}
		}

		resp, ok := <-c.Response
		if !ok {
			return
		}

		if resp.Error != nil {
			cancel()

			c.appendError(resp.Error)

			continue
		}

		if err := SendReply(w, resp.Reply, resp.Data); err != nil {
			cancel()

			c.appendError(resp.Error)
		}
	}
}

func (c *Conn) appendError(err error) {
	c.Lock()
	defer c.Unlock()

	c.err = multierr.Append(c.err, err)
}
