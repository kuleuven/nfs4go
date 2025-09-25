package vfs

import (
	"errors"
	"io"
)

type FileInfoListerAt []FileInfo

// Modeled after strings.Reader's ReadAt() implementation
func (f FileInfoListerAt) ListAt(ls []FileInfo, offset int64) (int, error) {
	var n int

	if offset >= int64(len(f)) {
		return 0, io.EOF
	}

	n = copy(ls, f[offset:])

	if n < len(ls) {
		return n, io.EOF
	}

	return n, nil
}

func (f FileInfoListerAt) Close() error {
	return nil
}

func ListAll(lister ListerAt) ([]FileInfo, error) {
	buf := make([]FileInfo, 100)

	var (
		result []FileInfo
		offset int64
	)

	for {
		n, err := lister.ListAt(buf, offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}

		result = append(result, buf[:n]...)

		if errors.Is(err, io.EOF) {
			return result, nil
		}

		offset += int64(n)
	}
}
