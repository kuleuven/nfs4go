//go:build !linux
// +build !linux

package nativefs

import (
	"os"

	"github.com/kuleuven/vfs"
)

func NumLinks(_ os.FileInfo) uint64 {
	return 1
}

func Inode(_ os.FileInfo) uint64 {
	return 0
}

func FindByInodes(root string, handle []byte) (string, error) {
	return "", vfs.ErrNotSupported
}

func SystemPermissions(path string) (*vfs.Permissions, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}

	return &vfs.Permissions{
		Read:             true,
		Write:            true,
		Delete:           true,
		Own:              true,
		GetExtendedAttrs: true,
		SetExtendedAttrs: true,
	}, nil
}
