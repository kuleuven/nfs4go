package errorfs

import (
	"os"

	"github.com/kuleuven/vfs"
)

var _ vfs.RootFS = &ErrorFS{}

func New(err error) *ErrorFS {
	return &ErrorFS{Error: err}
}

type ErrorFS struct {
	vfs.NotImplementedFS
	Error error
}

func (efs *ErrorFS) Stat(path string) (vfs.FileInfo, error) {
	return nil, efs.Error
}

func (efs *ErrorFS) Lstat(path string) (vfs.FileInfo, error) {
	return nil, efs.Error
}

func (efs *ErrorFS) List(path string) (vfs.ListerAt, error) {
	return nil, efs.Error
}

func (efs *ErrorFS) Link(path, target string) error {
	return efs.Error
}

func (efs *ErrorFS) Symlink(path, target string) error {
	return efs.Error
}

func (efs *ErrorFS) Readlink(path string) (string, error) {
	return "", efs.Error
}

func (efs *ErrorFS) FileRead(path string) (vfs.ReaderAt, error) {
	return nil, efs.Error
}

func (efs *ErrorFS) Handle(path string) ([]byte, error) {
	return nil, efs.Error
}

func (efs *ErrorFS) Path(handle []byte) (string, error) {
	return "", efs.Error
}

func (efs *ErrorFS) Open(path string) (vfs.File, error) {
	return nil, efs.Error
}

func (efs *ErrorFS) OpenFile(path string, mode int, perm os.FileMode) (vfs.File, error) {
	return nil, efs.Error
}

func (efs *ErrorFS) RealPath(path string) (string, error) {
	return "", efs.Error
}

func (efs *ErrorFS) Mount(path string, fs vfs.FS, index byte) error {
	return efs.Error
}

func (efs *ErrorFS) Walk(path string, walkFn vfs.WalkFunc) error {
	return efs.Error
}
