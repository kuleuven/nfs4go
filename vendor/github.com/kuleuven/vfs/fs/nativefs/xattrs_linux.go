//go:build linux
// +build linux

package nativefs

import (
	"errors"
	"os"
	"syscall"

	"github.com/kuleuven/vfs"
	"github.com/pkg/xattr"
	"github.com/sirupsen/logrus"
)

func SetExtendedAttrs(path string, attrs vfs.Attributes) error {
	return setExtendedAttrs(path, attrs, xattr.List, xattr.Set, xattr.Remove)
}

func LSetExtendedAttrs(path string, attrs vfs.Attributes) error {
	return setExtendedAttrs(path, attrs, xattr.LList, xattr.LSet, xattr.LRemove)
}

func setExtendedAttrs(path string, attrs vfs.Attributes, list func(string) ([]string, error), set func(string, string, []byte) error, remove func(string, string) error) error {
	attrnames, err := list(path)
	if errors.Is(err, syscall.EOPNOTSUPP) {
		logrus.Warnf("xattr not supported on %s", path)

		return nil
	} else if err != nil {
		return err
	}

	todelete := map[string]bool{}

	for _, name := range attrnames {
		todelete[name] = true
	}

	if attrs == nil {
		attrs = vfs.Attributes{}
	}

	for attr, value := range attrs {
		err = set(path, attr, value)
		if errors.Is(err, syscall.EOPNOTSUPP) {
			logrus.Warnf("set xattr %s not supported on %s", attr, path)

			continue
		} else if err != nil {
			return err
		}

		delete(todelete, attr)
	}

	for attr := range todelete {
		err = remove(path, attr)
		if errors.Is(err, syscall.EOPNOTSUPP) {
			logrus.Warnf("remove xattr %s not supported on %s", attr, path)

			return nil
		} else if err != nil {
			return err
		}
	}

	return nil
}

func SetExtendedAttr(path, name string, value []byte) error {
	return setExtendedAttr(path, name, value, xattr.Set)
}

func LSetExtendedAttr(path, name string, value []byte) error {
	return setExtendedAttr(path, name, value, xattr.LSet)
}

func setExtendedAttr(path, name string, value []byte, set func(string, string, []byte) error) error {
	return set(path, name, value)
}

func UnsetExtendedAttr(path, name string) error {
	return unsetExtendedAttr(path, name, xattr.Remove)
}

func LUnsetExtendedAttr(path, name string) error {
	return unsetExtendedAttr(path, name, xattr.LRemove)
}

func unsetExtendedAttr(path, name string, remove func(string, string) error) error {
	return remove(path, name)
}

func deferredExtendedAttrs(path string, list func(string) ([]string, error), get func(string, string) ([]byte, error)) func() (vfs.Attributes, error) {
	return func() (vfs.Attributes, error) {
		return getExtendedAttrs(path, list, get)
	}
}

func getExtendedAttrs(path string, list func(string) ([]string, error), get func(string, string) ([]byte, error)) (vfs.Attributes, error) {
	var value []byte

	attrnames, err := list(path)
	if errors.Is(err, syscall.EOPNOTSUPP) || errors.Is(err, os.ErrPermission) {
		return vfs.Attributes{}, nil
	} else if err != nil {
		return nil, err
	}

	attrs := vfs.Attributes{}

	for _, attr := range attrnames {
		value, err = get(path, attr)
		if errors.Is(err, syscall.EOPNOTSUPP) {
			return vfs.Attributes{}, nil
		} else if err != nil {
			return nil, err
		}

		attrs.Set(attr, value)
	}

	return attrs, nil
}

func deferredSystemPermissions(path string) func() (*vfs.Permissions, error) {
	return func() (*vfs.Permissions, error) {
		return SystemPermissions(path)
	}
}

func GetExtendedAttrs(path string) (vfs.Attributes, error) {
	return getExtendedAttrs(path, xattr.List, xattr.Get)
}

func LGetExtendedAttrs(path string) (vfs.Attributes, error) {
	return getExtendedAttrs(path, xattr.LList, xattr.LGet)
}

func PackExtendedAttrs(fi os.FileInfo, path string) *ExtendedFileInfo {
	return &ExtendedFileInfo{
		fi,
		deferredExtendedAttrs(path, xattr.List, xattr.Get),
		deferredSystemPermissions(path),
	}
}

func LPackExtendedAttrs(fi os.FileInfo, path string) *ExtendedFileInfo {
	return &ExtendedFileInfo{
		fi,
		deferredExtendedAttrs(path, xattr.LList, xattr.LGet),
		deferredSystemPermissions(path),
	}
}

func (fi *ExtendedFileInfo) Uid() uint32 { //nolint:staticcheck
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}

	return stat.Uid
}

func (fi *ExtendedFileInfo) Gid() uint32 { //nolint:staticcheck
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}

	return stat.Gid
}

func (fi *ExtendedFileInfo) NumLinks() uint64 {
	if s, ok := fi.Sys().(*syscall.Stat_t); ok {
		return s.Nlink
	}

	return 1
}
