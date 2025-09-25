package vfs

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"

	"github.com/kuleuven/iron/api"
	"github.com/kuleuven/vfs/io/readerat"
)

type SetExtendedAttrFS interface {
	Stat(path string) (FileInfo, error)
	SetExtendedAttr(path string, name string, value []byte) error
	UnsetExtendedAttr(path string, name string) error
}

func SetExtendedAttrs(fs SetExtendedAttrFS, path string, attrs Attributes) error {
	if efs, ok := fs.(SetExtendedAttrsFS); ok {
		return efs.SetExtendedAttrs(path, attrs)
	}

	fi, err := fs.Stat(path)
	if err != nil {
		return err
	}

	curr, err := fi.Extended()
	if err != nil {
		return err
	}

	add, remove := extendedAttrsDifference(curr, attrs)

	for attr := range remove {
		if err := fs.UnsetExtendedAttr(path, attr); err != nil {
			return err
		}
	}

	for attr, value := range add {
		if err := fs.SetExtendedAttr(path, attr, value); err != nil {
			return err
		}
	}

	return nil
}

func extendedAttrsDifference(oldAttrs, newAttrs Attributes) (Attributes, Attributes) {
	add := Attributes{}

	for attr, value := range newAttrs {
		if oldValue, ok := oldAttrs[attr]; ok && bytes.Equal(oldValue, value) {
			continue
		}

		add[attr] = value
	}

	remove := Attributes{}

	for attr, value := range oldAttrs {
		if _, ok := newAttrs[attr]; ok {
			continue
		}

		remove[attr] = value
	}

	return add, remove
}

type MkdirFS interface {
	Stat(path string) (FileInfo, error)
	Mkdir(path string, perm os.FileMode) error
}

func MkdirAll(fs MkdirFS, path string, perm os.FileMode) error {
	fi, err := fs.Stat(path)

	switch {
	case err == nil && fi.IsDir():
		return nil
	case err == nil:
		return syscall.ENOTDIR
	case errors.Is(err, os.ErrNotExist):
		break
	default:
		return err
	}

	// Create the parent first
	parent := Dir(Clean(path))

	if parent == path {
		return nil // Avoid infinite recursion
	}

	if err := MkdirAll(fs, parent, perm); err != nil {
		return err
	}

	return fs.Mkdir(path, perm)
}

type WalkableFS interface {
	Stat(path string) (FileInfo, error)
	List(path string) (ListerAt, error)
}

type WalkFunc func(path string, info FileInfo, err error) error

var (
	SkipDir     = filepath.SkipDir //nolint:errname
	SkipAll     = filepath.SkipAll //nolint:errname
	SkipSubDirs = api.SkipSubDirs  //nolint:errname
)

func Walk(fs WalkableFS, root string, fn WalkFunc) error {
	if walkFS, ok := fs.(WalkFS); ok {
		return walkFS.Walk(root, fn)
	}

	var (
		info FileInfo
		err  error
	)

	if symlinkFS, ok := fs.(SymlinkFS); ok {
		info, err = symlinkFS.Lstat(root)
	} else {
		info, err = fs.Stat(root)
	}

	if err != nil {
		err = fn(root, nil, err)
	} else {
		err = walk(fs, root, info, fn, false)
	}

	if err == SkipAll || err == SkipDir || err == SkipSubDirs {
		return nil
	}

	return err
}

type WalkRelFunc func(path, rel string, info FileInfo, err error) error

func walk(fs WalkableFS, path string, info FileInfo, walkFn WalkFunc, mustSkipDirs bool) error {
	if !info.IsDir() || mustSkipDirs {
		return walkFn(path, info, nil)
	}

	entries, err := ReadDir(fs, path)
	err1 := walkFn(path, info, err)
	// If err != nil, walk can't walk into this directory.
	// err1 != nil means walkFn want walk to skip this directory or stop walking.
	// Therefore, if one of err and err1 isn't nil, walk will return.
	if err != nil || (err1 != nil && err1 != SkipSubDirs) {
		// The caller's behavior is controlled by the return value, which is decided
		// by walkFn. walkFn may ignore err and return nil.
		// If walkFn returns SkipDir or SkipAll, it will be handled by the caller.
		// So walk should return whatever walkFn returns.
		return err1
	}

	for _, entry := range entries {
		filename := Join(path, entry.Name())

		err = walk(fs, filename, entry, walkFn, err1 == SkipSubDirs)
		if err != nil && err != SkipDir && err != SkipSubDirs {
			return err
		}
	}

	return nil
}

const ReadDirBufSize = 512

// ReadDir reads the directory named by dirname and returns
// a sorted list of directory entries.
func ReadDir(fs WalkableFS, dirname string) ([]FileInfo, error) {
	f, err := fs.List(dirname)
	if err != nil {
		return nil, err
	}

	buf := make([]FileInfo, ReadDirBufSize)
	entries := []FileInfo{}

	var (
		offset int64
		EOF    bool
	)

	for !EOF {
		n, err := f.ListAt(buf, offset)
		if errors.Is(err, io.EOF) {
			EOF = true
		} else if err != nil {
			return entries, err
		}

		offset += int64(n)
		entries = append(entries, buf[:n]...)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	return entries, nil
}

// WalkRel is like Walk, but the paths are relative to root.
func WalkRel(fs WalkableFS, root string, fn WalkRelFunc) error {
	var prefix string

	if strings.HasSuffix(root, string(Separator)) {
		root = Clean(root)
		prefix = root + string(Separator)
	} else {
		root = Clean(root)
		prefix = Dir(root)

		if prefix != string(Separator) {
			prefix += string(Separator)
		}
	}

	return Walk(fs, root, func(path string, info FileInfo, err error) error {
		name := strings.TrimPrefix(path, prefix)

		if path+string(Separator) == prefix {
			name = ""
		}

		return fn(path, name, info, err)
	})
}

type WalkRemoveFS interface {
	WalkableFS
	Remove(path string) error
}

func RemoveAll(fs WalkRemoveFS, path string) error {
	dirs := []string{}

	err := Walk(fs, path, func(path string, info FileInfo, err error) error {
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			dirs = append(dirs, path)

			return nil
		}

		return fs.Remove(path)
	})
	if err != nil {
		return err
	}

	slices.Reverse(dirs)

	for _, dir := range dirs {
		if err := fs.Remove(dir); err != nil {
			return err
		}
	}

	return nil
}

type OpenableFS interface {
	Stat(path string) (FileInfo, error)
	FileRead(path string) (ReaderAt, error)
	List(path string) (ListerAt, error)
}

func Open(fs OpenableFS, path string) (File, error) {
	fi, err := fs.Stat(path)
	if err != nil {
		return nil, err
	}

	if fi.IsDir() {
		return &roDirectory{
			fi:   fi,
			path: path,
			fs:   fs,
		}, nil
	}

	readerAt, err := fs.FileRead(path)
	if err != nil {
		return nil, err
	}

	return &roFile{
		fi:         fi,
		path:       path,
		ReadSeeker: readerat.Reader(readerAt, 0, fi.Size()),
		Closer:     readerAt,
	}, nil
}

type roDirectory struct {
	fi     FileInfo
	path   string
	fs     OpenableFS
	offset int64
}

func (r *roDirectory) Name() string {
	return r.path
}

func (r *roDirectory) Read([]byte) (int, error) {
	return 0, syscall.EISDIR
}

func (r *roDirectory) ReadAt([]byte, int64) (int, error) {
	return 0, syscall.EISDIR
}

func (r *roDirectory) Write([]byte) (int, error) {
	return 0, syscall.EISDIR
}

func (r *roDirectory) WriteAt([]byte, int64) (int, error) {
	return 0, syscall.EISDIR
}

func (r *roDirectory) Truncate(int64) error {
	return syscall.EISDIR
}

func (r *roDirectory) Seek(int64, int) (int64, error) {
	return 0, syscall.EISDIR
}

func (r *roDirectory) Readdir(count int) ([]FileInfo, error) {
	if count < 0 {
		return r.readAll()
	}

	lister, err := r.fs.List(r.path)
	if err != nil {
		return nil, err
	}

	buf := make([]FileInfo, count)

	n, err := lister.ListAt(buf, r.offset)

	r.offset += int64(n)

	if errors.Is(err, io.EOF) && n > 0 {
		err = nil
	}

	return buf[:n], err
}

const ListBufSize = 512

func (r *roDirectory) readAll() ([]FileInfo, error) {
	var result []FileInfo

	for {
		batch, err := r.Readdir(ListBufSize)

		result = append(result, batch...)

		if errors.Is(err, io.EOF) || (err == nil && len(batch) < ListBufSize) {
			return result, nil
		} else if err != nil {
			return nil, err
		}
	}
}

func (r *roDirectory) Stat() (FileInfo, error) {
	return r.fi, nil
}

func (r *roDirectory) Close() error {
	return nil
}

type roFile struct {
	fi   FileInfo
	path string
	io.ReadSeeker
	io.ReaderAt
	io.Closer
}

func (r *roFile) Name() string {
	return r.path
}

func (r *roFile) Write([]byte) (int, error) {
	return 0, os.ErrPermission
}

func (r *roFile) WriteAt([]byte, int64) (int, error) {
	return 0, os.ErrPermission
}

func (r *roFile) Truncate(int64) error {
	return os.ErrPermission
}

func (r *roFile) Readdir(count int) ([]FileInfo, error) {
	return nil, syscall.ENOTDIR
}

func (r *roFile) Stat() (FileInfo, error) {
	return r.fi, nil
}
