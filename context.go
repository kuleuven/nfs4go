package nfs4go

import (
	"context"
	"io"
)

type ctxReader struct {
	ctx context.Context //nolint:containedctx
	r   io.ReadCloser
}

type ioret struct {
	n   int
	err error
}

func NewCtxReader(ctx context.Context, r io.ReadCloser) *ctxReader {
	return &ctxReader{ctx: ctx, r: r}
}

func (r *ctxReader) Read(buf []byte) (int, error) {
	buf2 := make([]byte, len(buf))

	c := make(chan ioret, 1)

	go func() {
		n, err := r.r.Read(buf2)
		c <- ioret{n, err}

		close(c)
	}()

	select {
	case ret := <-c:
		copy(buf, buf2)
		return ret.n, ret.err
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	}
}

func (r *ctxReader) Close() error {
	return r.r.Close()
}
