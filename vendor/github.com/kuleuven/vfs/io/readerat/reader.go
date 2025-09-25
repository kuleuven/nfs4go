package readerat

import (
	"errors"
	"io"
	"os"
	"sync"
)

// ReaderFromReaderAt creates an OffsetReader from an io.ReaderAt
func Reader(r io.ReaderAt, offset, length int64) OffsetReader {
	return &readerFromReaderAt{
		ReaderAt: r,
		offset:   offset,
		length:   length,
	}
}

func ReaderAt(r io.ReadSeeker) io.ReaderAt {
	return &readerAtFromReadSeeker{
		ReadSeeker: r,
	}
}

type OffsetReader interface {
	io.Reader
	io.Seeker
	Offset() int64
}

type readerFromReaderAt struct {
	io.ReaderAt
	offset int64
	length int64
	sync.Mutex
}

func (r *readerFromReaderAt) Read(buf []byte) (int, error) {
	r.Lock()
	defer r.Unlock()

	n, err := r.ReadAt(buf, r.offset)
	if err == nil || errors.Is(err, io.EOF) {
		r.offset += int64(n)
	}

	return n, err
}

func (r *readerFromReaderAt) Offset() int64 {
	r.Lock()
	defer r.Unlock()

	return r.offset
}

func (r *readerFromReaderAt) Seek(offset int64, whence int) (int64, error) {
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

type readerAtFromReadSeeker struct {
	io.ReadSeeker
	offset int64
	sync.Mutex
}

func (r *readerAtFromReadSeeker) ReadAt(buf []byte, offset int64) (int, error) {
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

	n, err := r.Read(buf)
	if err == nil || errors.Is(err, io.EOF) {
		r.offset += int64(n)
	}

	return n, err
}
