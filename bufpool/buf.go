package bufpool

type Bytes interface {
	Bytes() []byte
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	SeekWrite(n int) int
	Allocate(n int) []byte
	Commit(n int)
	Reset()
	Discard()
	Copy(target Bytes)
}

type Buf struct {
	buf  []byte
	r, w int
	pool *Pool
}

func (b *Buf) Write(p []byte) (int, error) {
	if b.w+len(p) > len(b.buf) {
		b.Grow(b.w + len(p))
	}

	n := copy(b.buf[b.w:], p)

	b.w += n

	return n, nil
}

func (b *Buf) Read(p []byte) (int, error) {
	n := copy(p, b.buf[b.r:b.w])

	b.r += n

	return n, nil
}

func (b *Buf) Bytes() []byte {
	return b.buf[b.r:b.w]
}

func (b *Buf) Allocate(n int) []byte {
	if b.w+n > len(b.buf) {
		b.Grow(b.w + n)
	}

	return b.buf[b.w : b.w+n]
}

func (b *Buf) Commit(n int) {
	b.w += n
}

func (b *Buf) SeekWrite(n int) int {
	old := b.w

	b.w = n

	return old
}

func (b *Buf) Reset() {
	b.r = 0
	b.w = 0
}

func (b *Buf) Grow(n int) {
	if n <= len(b.buf) {
		return
	}

	newbuf := make([]byte, n)

	copy(newbuf, b.buf)

	b.buf = newbuf
}

func (b *Buf) Discard() {
	b.pool.Put(b)
}

func (b *Buf) Len() int {
	return b.w - b.r
}

func (b *Buf) Copy(other Bytes) {
	target := other.Allocate(b.Len())

	n := copy(target, b.Bytes())

	other.Commit(n)
}
