package vfs

import (
	"errors"
	"io"

	"github.com/kuleuven/vfs/io/readerat"
	"github.com/kuleuven/vfs/io/writerat"
)

func ReadFile(fs FS, path string) ([]byte, error) {
	f, err := FileReadSeekCloser(fs, path)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	return io.ReadAll(f)
}

func FileReadSeekCloser(fs FS, path string) (io.ReadSeekCloser, error) {
	readerAt, err := fs.FileRead(path)
	if err != nil {
		return nil, err
	}

	reader := readerat.Reader(readerAt, 0, -1)

	return struct {
		io.ReadSeeker
		io.Closer
	}{
		ReadSeeker: reader,
		Closer:     readerAt,
	}, nil
}

func WriteFile(fs FS, path string, data []byte, flags int) error {
	f, err := FileWriteCloser(fs, path, flags)
	if err != nil {
		return err
	}

	defer f.Close()

	for len(data) > 0 {
		n, err := f.Write(data)
		if err != nil && !errors.Is(err, io.ErrShortWrite) {
			return err
		}

		data = data[n:]
	}

	return nil
}

func FileWriteCloser(fs FS, path string, flags int) (io.WriteCloser, error) {
	writerAt, err := fs.FileWrite(path, flags)
	if err != nil {
		return nil, err
	}

	writer := writerat.Writer(writerAt, 0, -1)

	return struct {
		io.Writer
		io.Closer
	}{
		Writer: writer,
		Closer: writerAt,
	}, nil
}
