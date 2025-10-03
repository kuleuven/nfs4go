package nfs4go

import (
	"os"

	"github.com/kuleuven/vfs"
)

func NopWriterAt(r vfs.ReaderAt) vfs.WriterAtReaderAt {
	return &nopWriterAt{
		ReaderAt: r,
	}
}

type nopWriterAt struct {
	vfs.ReaderAt
}

func (r *nopWriterAt) WriteAt([]byte, int64) (int, error) {
	return 0, os.ErrPermission
}

func NopReaderAt(w vfs.WriterAt) vfs.WriterAtReaderAt {
	return &nopReaderAt{
		WriterAt: w,
	}
}

type nopReaderAt struct {
	vfs.WriterAt
}

func (r *nopReaderAt) ReadAt([]byte, int64) (int, error) {
	return 0, os.ErrPermission
}
