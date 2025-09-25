package nativefs

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/kuleuven/vfs"
)

var ErrPathIsNoChild = errors.New("path is not a child")

func InodeTrail(root, path string) ([]byte, error) {
	// For lookup purposes, keep a list of all traversed inodes
	var inodes []uint64

	for path != root {
		stat, statErr := os.Lstat(path)
		if statErr != nil {
			return nil, statErr
		}

		inodes = append(inodes, Inode(stat))

		parent := filepath.Dir(path)

		if len(parent) >= len(path) {
			return nil, ErrPathIsNoChild
		}

		path = parent
	}

	handle := make([]byte, 8*len(inodes))

	for i := range inodes {
		binary.LittleEndian.PutUint64(handle[8*i:], inodes[len(inodes)-i-1])
	}

	return handle, nil
}

type XattrsExtendedListerAt []*ExtendedFileInfo

// Modeled after strings.Reader's ReadAt() implementation
func (f XattrsExtendedListerAt) ListAt(ls []vfs.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(f)) {
		return 0, io.EOF
	}

	for i, v := range f[offset:] {
		ls[i] = v

		if i == len(ls)-1 {
			return len(ls), nil
		}
	}

	return len(f[offset:]), io.EOF
}

func (f XattrsExtendedListerAt) Close() error {
	return nil
}
