package writerat

import (
	"errors"
	"io"
	"os"
	"sync"
)

// WriterFromWriterAt returns an io.WriterAt that writes to the given io.WriterAt starting at the given offset
func Writer(r io.WriterAt, offset, length int64) OffsetWriter {
	return &writerFromWriterAt{
		WriterAt: r,
		offset:   offset,
		length:   length,
	}
}

func WriterAt(w io.WriteSeeker) io.WriterAt {
	return &writerAtFromWriteSeeker{
		WriteSeeker: w,
	}
}

type OffsetWriter interface {
	io.Writer
	io.Seeker
	Offset() int64
}

type writerFromWriterAt struct {
	io.WriterAt
	offset int64
	length int64
	sync.Mutex
}

func (r *writerFromWriterAt) Write(buf []byte) (int, error) {
	r.Lock()
	defer r.Unlock()

	n, err := r.WriteAt(buf, r.offset)
	if err == nil || errors.Is(err, io.EOF) {
		r.offset += int64(n)
	}

	if r.offset > r.length {
		r.length = r.offset
	}

	return n, err
}

func (r *writerFromWriterAt) Offset() int64 {
	r.Lock()
	defer r.Unlock()

	return r.offset
}

func (r *writerFromWriterAt) Seek(offset int64, whence int) (int64, error) {
	r.Lock()
	defer r.Unlock()

	switch whence {
	case io.SeekStart:
		r.offset = offset
	case io.SeekCurrent:
		r.offset += offset
	case io.SeekEnd:
		if r.length < 0 {
			return 0, os.ErrInvalid
		}

		r.offset = r.length + offset
	}

	return r.offset, nil
}

type writerAtFromWriteSeeker struct {
	io.WriteSeeker
	offset int64
	sync.Mutex
}

func (r *writerAtFromWriteSeeker) WriteAt(buf []byte, offset int64) (int, error) {
	r.Lock()
	defer r.Unlock()

	if offset != r.offset {
		if newOffset, err := r.Seek(offset, io.SeekStart); err != nil {
			return 0, err
		} else if newOffset != offset {
			return 0, io.EOF
		}

		r.offset = offset
	}

	n, err := r.Write(buf)
	if err == nil || errors.Is(err, io.EOF) {
		r.offset += int64(n)
	}

	return n, err
}
