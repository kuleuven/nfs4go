package vfs

import (
	"os"
	"time"
)

// Base for partial implementations
type NotImplementedFS struct{}

type NotImplementedAdvancedFS struct {
	NotImplementedFS
}

type NotImplementedRootFS struct {
	NotImplementedAdvancedFS
}

var _ FS = NotImplementedFS{}

var _ AdvancedFS = NotImplementedAdvancedFS{}

var _ RootFS = NotImplementedRootFS{}

func (n NotImplementedFS) Stat(path string) (FileInfo, error) {
	return nil, ErrNotImplemented
}

func (n NotImplementedFS) List(path string) (ListerAt, error) {
	return nil, ErrNotImplemented
}

func (n NotImplementedFS) Mkdir(path string, perm os.FileMode) error {
	return ErrNotImplemented
}

func (n NotImplementedFS) Remove(path string) error {
	return ErrNotImplemented
}

func (n NotImplementedFS) Rmdir(path string) error {
	return ErrNotImplemented
}

func (n NotImplementedFS) FileRead(path string) (ReaderAt, error) {
	return nil, ErrNotImplemented
}

func (n NotImplementedFS) FileWrite(path string, flag int) (WriterAt, error) {
	return nil, ErrNotImplemented
}

func (n NotImplementedFS) Chmod(path string, mode os.FileMode) error {
	return ErrNotImplemented
}

func (n NotImplementedFS) Chown(path string, uid, gid int) error {
	return ErrNotImplemented
}

func (n NotImplementedFS) Chtimes(path string, atime, mtime time.Time) error {
	return ErrNotImplemented
}

func (n NotImplementedFS) Truncate(path string, size int64) error {
	return ErrNotImplemented
}

func (n NotImplementedFS) SetExtendedAttr(path, name string, value []byte) error {
	return ErrNotImplemented
}

func (n NotImplementedFS) UnsetExtendedAttr(path, name string) error {
	return ErrNotImplemented
}

func (n NotImplementedFS) SetExtendedAttrs(path string, attrs Attributes) error {
	return ErrNotImplemented
}

func (n NotImplementedFS) Rename(oldpath, newpath string) error {
	return ErrNotImplemented
}

func (n NotImplementedFS) Close() error {
	return nil
}

func (n NotImplementedAdvancedFS) Handle(path string) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (n NotImplementedAdvancedFS) Path(handle []byte) (string, error) {
	return "", ErrNotImplemented
}

func (n NotImplementedAdvancedFS) Open(path string) (File, error) {
	return nil, ErrNotImplemented
}

func (n NotImplementedAdvancedFS) OpenFile(path string, mode int, perm os.FileMode) (File, error) {
	return nil, ErrNotImplemented
}

func (n NotImplementedAdvancedFS) Walk(path string, fn WalkFunc) error {
	return ErrNotImplemented
}

func (n NotImplementedRootFS) Lstat(path string) (FileInfo, error) {
	return nil, ErrNotImplemented
}

func (n NotImplementedRootFS) Link(path, target string) error {
	return ErrNotImplemented
}

func (n NotImplementedRootFS) Symlink(path, target string) error {
	return ErrNotImplemented
}

func (n NotImplementedRootFS) Readlink(path string) (string, error) {
	return "", ErrNotImplemented
}

func (n NotImplementedRootFS) RealPath(path string) (string, error) {
	return "", ErrNotImplemented
}

func (n NotImplementedRootFS) Mount(path string, fs FS, index byte) error {
	return ErrNotImplemented
}
