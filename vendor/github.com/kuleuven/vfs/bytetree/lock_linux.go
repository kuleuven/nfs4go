//go:build linux
// +build linux

package bytetree

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"syscall"
)

func TryLock(file *os.File) (*FileLock, error) {
	dir, name := filepath.Split(file.Name())
	path := filepath.Join(dir, fmt.Sprintf(".%s.lock%x", name, rand.Uint64())) //nolint:gosec

	if err := os.Link(file.Name(), path); err != nil {
		return nil, err
	}

	fi, err := file.Stat()
	if err != nil {
		os.Remove(path)

		return nil, err
	}

	s, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		os.Remove(path)

		return nil, ErrTypeAssertion
	}

	if s.Nlink != 2 {
		os.Remove(path)

		return nil, ErrLockHeld
	}

	return &FileLock{
		path: path,
	}, nil
}
