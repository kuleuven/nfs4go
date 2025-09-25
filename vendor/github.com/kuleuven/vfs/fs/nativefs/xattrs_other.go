//go:build !linux
// +build !linux

package nativefs

import (
	"os"

	"github.com/kuleuven/vfs"
)

func SetExtendedAttrs(name string, attrs vfs.Attributes) error {
	return nil
}

func LSetExtendedAttrs(name string, attrs vfs.Attributes) error {
	return nil
}

func SetExtendedAttr(path, name string, value []byte) error {
	return nil
}

func LSetExtendedAttr(path, name string, value []byte) error {
	return nil
}

func UnsetExtendedAttr(path, name string) error {
	return nil
}

func LUnsetExtendedAttr(path, name string) error {
	return nil
}

func GetExtendedAttrs(name string) (vfs.Attributes, error) {
	return nil, nil
}

func LGetExtendedAttrs(name string) (vfs.Attributes, error) {
	return nil, nil
}

func PackExtendedAttrs(fi os.FileInfo, name string) *ExtendedFileInfo {
	return &ExtendedFileInfo{
		FileInfo: fi,
	}
}

func LPackExtendedAttrs(fi os.FileInfo, name string) *ExtendedFileInfo {
	return &ExtendedFileInfo{
		FileInfo: fi,
	}
}

func (fi *ExtendedFileInfo) Uid() uint32 { //nolint:staticcheck
	return 0
}

func (fi *ExtendedFileInfo) Gid() uint32 { //nolint:staticcheck
	return 0
}

func (fi *ExtendedFileInfo) NumLinks() uint64 {
	return 1
}
