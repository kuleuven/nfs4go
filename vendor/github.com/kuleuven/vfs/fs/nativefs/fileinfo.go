package nativefs

import (
	"os"

	"github.com/kuleuven/vfs"
)

type ExtendedFileInfo struct {
	os.FileInfo
	extendedAttrs func() (vfs.Attributes, error)
	permissions   func() (*vfs.Permissions, error)
}

func (fi *ExtendedFileInfo) Extended() (vfs.Attributes, error) {
	if fi.extendedAttrs == nil {
		return vfs.Attributes{}, nil
	}

	return fi.extendedAttrs()
}

func (fi *ExtendedFileInfo) Permissions() (*vfs.Permissions, error) {
	if fi.permissions == nil {
		return &vfs.Permissions{
			Read:             true,
			Write:            true,
			Delete:           true,
			Own:              true,
			GetExtendedAttrs: true,
			SetExtendedAttrs: true,
		}, nil
	}

	return fi.permissions()
}
