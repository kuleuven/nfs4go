package emptyfs

import (
	"io"
	"os"
	"syscall"
	"time"

	"github.com/kuleuven/vfs"
)

func New() *Empty {
	return &Empty{}
}

type Empty struct {
	vfs.NotImplementedRootFS
}

var _ vfs.RootFS = Empty{}

func (Empty) Stat(path string) (vfs.FileInfo, error) {
	if path != "/" {
		return nil, os.ErrNotExist
	}

	return EmptyDirStat{}, nil
}

func (e Empty) Lstat(path string) (vfs.FileInfo, error) {
	return e.Stat(path)
}

func (Empty) List(path string) (vfs.ListerAt, error) {
	if path != "/" {
		return nil, os.ErrNotExist
	}

	return EmptyDirList{}, nil
}

func (Empty) Walk(path string, walkFn vfs.WalkFunc) error {
	if path != "/" {
		return os.ErrNotExist
	}

	err := walkFn("/", EmptyDirStat{}, nil)
	if err == vfs.SkipAll || err == vfs.SkipDir {
		return nil
	}

	return err
}

func (Empty) Open(path string) (vfs.File, error) {
	if path != "/" {
		return nil, os.ErrNotExist
	}

	return EmptyDirFile{}, nil
}

func (Empty) Handle(path string) ([]byte, error) {
	if path != "/" {
		return nil, os.ErrNotExist
	}

	return []byte{0}, nil
}

func (Empty) Path(handle []byte) (string, error) {
	if len(handle) != 1 || handle[0] != 0 {
		return "", os.ErrNotExist
	}

	return "/", nil
}

type EmptyDirStat struct{}

func (fi EmptyDirStat) Name() string {
	return "."
}

func (fi EmptyDirStat) Size() int64 {
	return 0
}

func (fi EmptyDirStat) Mode() os.FileMode {
	return 0o755 | os.ModeDir
}

func (fi EmptyDirStat) ModTime() time.Time {
	return time.Time{}
}

func (fi EmptyDirStat) IsDir() bool {
	return true
}

func (fi EmptyDirStat) Sys() interface{} {
	return nil
}

func (fi EmptyDirStat) NumLinks() uint64 {
	return 1
}

func (fi EmptyDirStat) Extended() (vfs.Attributes, error) {
	return vfs.Attributes{}, nil
}

func (fi EmptyDirStat) Permissions() (*vfs.Permissions, error) {
	return &vfs.Permissions{
		Read:             true,
		GetExtendedAttrs: true,
	}, nil
}

func (fi EmptyDirStat) Uid() uint32 { //nolint:staticcheck
	return 0
}

func (fi EmptyDirStat) Gid() uint32 { //nolint:staticcheck
	return 0
}

type EmptyDirList struct{}

func (EmptyDirList) Close() error {
	return nil
}

func (EmptyDirList) ListAt(buf []vfs.FileInfo, offset int64) (int, error) {
	return 0, io.EOF
}

type EmptyDirFile struct{}

func (EmptyDirFile) Close() error {
	return nil
}

func (EmptyDirFile) Stat() (vfs.FileInfo, error) {
	return EmptyDirStat{}, nil
}

func (EmptyDirFile) Name() string {
	return "/"
}

func (EmptyDirFile) Readdir(n int) ([]vfs.FileInfo, error) {
	return nil, io.EOF
}

func (EmptyDirFile) Read([]byte) (int, error) {
	return 0, syscall.EOPNOTSUPP
}

func (EmptyDirFile) Write([]byte) (int, error) {
	return 0, syscall.EOPNOTSUPP
}

func (EmptyDirFile) Seek(int64, int) (int64, error) {
	return 0, syscall.EOPNOTSUPP
}

func (EmptyDirFile) Truncate(int64) error {
	return syscall.EOPNOTSUPP
}

func (EmptyDirFile) WriteAt([]byte, int64) (int, error) {
	return 0, syscall.EOPNOTSUPP
}

func (EmptyDirFile) ReadAt([]byte, int64) (int, error) {
	return 0, syscall.EOPNOTSUPP
}
