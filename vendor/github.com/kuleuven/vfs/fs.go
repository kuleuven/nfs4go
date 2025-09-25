package vfs

import (
	"context"
	"io"
	"os"
	"time"
)

type ContextKey string

var (
	// Log context key to specify which logger.
	// Set a context value either to a *logrus.Logger or a *logrus.Entry to use a custom logger.
	// Defaults to logrus.StandardLogger
	Log = ContextKey("logger")

	// Boolean indicating whether List and Walk methods should return extended attributes.
	// If false, implementations may choose to not include them if Extended() is called on the returned file infos.
	ListWithXattrs = ContextKey("list-with-xattrs")

	// String indicating which local storage can be used as persistent storage for the inode handle database.
	PersistentStorage = ContextKey("persistent-storage")

	// Boolean indicating whether or not server inodes should be used when exposing a native posix file system.
	UseServerInodes = ContextKey("use-serverino")

	// Boolean indicating whether or not chown is supported when exposing a native posix file system.
	AllowServerChown = ContextKey("allow-chown")
)

// Bool returns the boolean value associated with the given context key.
// If the key is not present in the context, it returns false.
func Bool(ctx context.Context, key ContextKey) bool {
	b, ok := ctx.Value(key).(bool)
	if !ok {
		return false
	}

	return b
}

// String returns the string value associated with the given context key.
// If the key is not present in the context, it returns an empty string.
func String(ctx context.Context, key ContextKey) string {
	s, ok := ctx.Value(key).(string)
	if !ok {
		return ""
	}

	return s
}

// Int returns the int value associated with the given context key.
// If the key is not present in the context, it returns 0.
func Int(ctx context.Context, key ContextKey) int {
	i, ok := ctx.Value(key).(int)
	if !ok {
		return 0
	}

	return i
}

// Duration returns the time.Duration value associated with the given context key.
// If the key is not present in the context, it returns 0.
func Duration(ctx context.Context, key ContextKey) time.Duration {
	d, ok := ctx.Value(key).(time.Duration)
	if !ok {
		return 0
	}

	return d
}

// FS interface
// Implementing file systems should implement all methods.
type FS interface {
	Stat(path string) (FileInfo, error)
	List(path string) (ListerAt, error)
	FileRead(path string) (ReaderAt, error)
	FileWrite(path string, flags int) (WriterAt, error)
	Chmod(path string, mode os.FileMode) error
	Chown(path string, uid, gid int) error
	Chtimes(path string, atime, mtime time.Time) error
	Truncate(path string, size int64) error
	SetExtendedAttr(path string, name string, value []byte) error
	UnsetExtendedAttr(path string, name string) error
	Rename(oldpath, newpath string) error
	Rmdir(path string) error
	Remove(path string) error
	Mkdir(path string, perm os.FileMode) error
	Close() error
}

type FileInfo interface {
	os.FileInfo
	Uid() uint32
	Gid() uint32
	NumLinks() uint64
	Extended() (Attributes, error)
	Permissions() (*Permissions, error) // Returns an indicative permission set to indicate which permissions the client has
}

type Attributes map[string][]byte

func (a Attributes) Get(name string) ([]byte, bool) {
	if a == nil {
		return nil, false
	}

	v, ok := a[name]

	return v, ok
}

func (a Attributes) GetString(name string) (string, bool) {
	if a == nil {
		return "", false
	}

	v, ok := a[name]

	return string(v), ok
}

func (a Attributes) Set(name string, value []byte) {
	if a == nil {
		a = Attributes{}
	}

	a[name] = value
}

func (a Attributes) SetString(name, value string) {
	a.Set(name, []byte(value))
}

func (a Attributes) Delete(name string) {
	if a == nil {
		return
	}

	delete(a, name)
}

type Permissions struct {
	Read             bool // List, FileRead
	Write            bool // FileWrite, Chtimes, Truncate
	Delete           bool // Rmdir, Remove
	Own              bool // Chmod, Chown (depends on target user/group), Chtimes
	GetExtendedAttrs bool // Stat returns extended attributes
	SetExtendedAttrs bool // SetExtendedAttrs
}

type ListerAt interface {
	ListAt(buf []FileInfo, offset int64) (int, error)
	Close() error
}

type HandleFS interface {
	FS
	Handle(path string) ([]byte, error)
}

type HandleResolveFS interface {
	HandleFS
	Path(handle []byte) (string, error)
}

type OpenFileFS interface {
	FS
	OpenFile(path string, flag int, perm os.FileMode) (File, error)
}

type File interface {
	Name() string
	Stat() (FileInfo, error)
	Truncate(size int64) error
	Readdir(count int) ([]FileInfo, error)
	io.ReadWriteCloser
	WriterAtReaderAt
	io.Seeker
}

type SymlinkFS interface {
	FS
	Lstat(path string) (FileInfo, error)
	Symlink(target, link string) error
	Readlink(path string) (string, error)
}

type LinkFS interface {
	FS
	Link(oldname, newname string) error
}

type WalkFS interface {
	FS
	Walk(path string, walkFn WalkFunc) error
}

type SetExtendedAttrsFS interface {
	FS
	SetExtendedAttrs(path string, attrs Attributes) error
}

type AdvancedFS interface {
	OpenFileFS
	HandleResolveFS
	SetExtendedAttrsFS
}

type AdvancedLinkFS interface {
	AdvancedFS
	SymlinkFS
	LinkFS
	RealPath(path string) (string, error)
}

type RootFS interface {
	AdvancedLinkFS
	WalkFS
	Open(path string) (File, error)
	Mount(path string, fs FS, index byte) error
}

type ReaderAt interface {
	io.ReaderAt
	io.Closer
}

type WriterAt interface {
	io.WriterAt
	io.Closer
}

type WriterAtReaderAt interface {
	io.WriterAt
	io.ReaderAt
	io.Closer
}
