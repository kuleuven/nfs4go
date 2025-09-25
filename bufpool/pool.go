package bufpool

import (
	"sync"

	"github.com/sirupsen/logrus"
)

type Pool struct {
	bufs []Bytes
	sync.Mutex
}

func (p *Pool) Get() Bytes {
	p.Lock()
	defer p.Unlock()

	if len(p.bufs) > 0 {
		buf := p.bufs[0]
		p.bufs = p.bufs[1:]

		return buf
	}

	logrus.Info("creating new buffer")

	return New(make([]byte, 1024))
}

func (p *Pool) Put(buf Bytes) {
	p.Lock()
	defer p.Unlock()

	buf.Reset()

	p.bufs = append(p.bufs, buf)
}

func (p *Pool) New(buf []byte) Bytes {
	return &Buf{
		buf:  buf,
		pool: p,
	}
}

var GlobalPool Pool

func Get() Bytes {
	return GlobalPool.Get()
}

func Put(buf Bytes) {
	GlobalPool.Put(buf)
}

func New(buf []byte) Bytes {
	return GlobalPool.New(buf)
}
