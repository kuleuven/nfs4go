//go:build !linux
// +build !linux

package bytetree

import (
	"fmt"
	"os"
	"path/filepath"
)

func TryLock(file *os.File) (*FileLock, error) {
	dir, name := filepath.Split(file.Name())
	path := filepath.Join(dir, fmt.Sprintf("%s.%s.lock", dir, name))

	if err := os.Symlink(file.Name(), path); err != nil {
		return nil, err
	}

	return &FileLock{
		path: path,
	}, nil
}
