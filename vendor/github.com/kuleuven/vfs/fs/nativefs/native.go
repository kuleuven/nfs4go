package nativefs

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kuleuven/vfs"
	"github.com/kuleuven/vfs/fs/errorfs"
	"github.com/kuleuven/vfs/runas"
)

var (
	_ vfs.OpenFileFS      = &NativeFS{}
	_ vfs.SymlinkFS       = &NativeFS{}
	_ vfs.LinkFS          = &NativeFS{}
	_ vfs.HandleResolveFS = &NativeServerInodeFS{}
)

func New(ctx context.Context, path string) vfs.FS {
	allowChown, _ := ctx.Value(vfs.AllowServerChown).(bool)

	fs := &NativeFS{
		Root:       path,
		Context:    runas.RunAsCurrentUser(),
		AllowChown: allowChown,
	}

	if serverino, _ := ctx.Value(vfs.UseServerInodes).(bool); serverino {
		return &NativeServerInodeFS{fs}
	}

	return fs
}

func NewAsUser(ctx context.Context, path string, u *runas.User) vfs.FS {
	c, err := runas.RunAs(u)
	if err != nil {
		return errorfs.New(err)
	}

	allowChown, _ := ctx.Value(vfs.AllowServerChown).(bool)

	fs := &NativeFS{
		Root:       path,
		Context:    c,
		AllowChown: allowChown,
	}

	if serverino, _ := ctx.Value(vfs.UseServerInodes).(bool); serverino {
		return &NativeServerInodeFS{fs}
	}

	return fs
}

type NativeFS struct {
	Root       string
	Context    runas.Context
	AllowChown bool
}

type NativeServerInodeFS struct {
	*NativeFS
}

func (m *NativeFS) BuildPath(path string) string {
	return filepath.Join(m.Root, filepath.FromSlash(path))
}

func (m *NativeFS) Stat(path string) (vfs.FileInfo, error) {
	rpath := m.BuildPath(path)

	var fi vfs.FileInfo

	err := m.Context.Run(func() error {
		stat, err := os.Stat(rpath)
		if err != nil {
			return err
		}

		fi = PackExtendedAttrs(stat, rpath)

		return nil
	})

	return fi, err
}

func (m *NativeFS) Lstat(path string) (vfs.FileInfo, error) {
	rpath := m.BuildPath(path)

	var fi vfs.FileInfo

	err := m.Context.Run(func() error {
		stat, err := os.Lstat(rpath)
		if err != nil {
			return err
		}

		fi = LPackExtendedAttrs(stat, rpath)

		return nil
	})

	return fi, err
}

func (m *NativeFS) List(path string) (vfs.ListerAt, error) {
	rpath := m.BuildPath(path)

	var result vfs.ListerAt

	err := m.Context.Run(func() error {
		f, err := os.Open(rpath)
		if err != nil {
			return err
		}

		defer f.Close()

		files, err := f.Readdir(-1)
		if err != nil {
			return err
		}

		list := make([]*ExtendedFileInfo, len(files))

		for i, f := range files {
			list[i] = PackExtendedAttrs(f, filepath.Join(rpath, f.Name()))
		}

		result = XattrsExtendedListerAt(list)

		return nil
	})

	return result, err
}

func (m *NativeFS) Mkdir(path string, perm os.FileMode) error {
	rpath := m.BuildPath(path)

	return m.Context.Run(func() error {
		return os.Mkdir(rpath, perm)
	})
}

func (m *NativeFS) Readlink(path string) (string, error) {
	rpath := m.BuildPath(path)

	var target string

	err := m.Context.Run(func() error {
		var err error

		target, err = os.Readlink(rpath)

		return err
	})
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(target) {
		return target, nil
	}

	// Check the target against the root to see if it's valid
	againstRoot, err := filepath.Rel(m.Root, target)
	if err != nil {
		return "", err
	}

	if len(againstRoot) > 1 && againstRoot[0] == '.' {
		return "", &os.LinkError{Op: "readlink", Err: syscall.EINVAL}
	}

	// Return absolute path
	if againstRoot == "." {
		return "/", nil
	}

	return filepath.ToSlash(string(os.PathSeparator) + againstRoot), nil
}

func (m *NativeFS) Remove(path string) error {
	rpath := m.BuildPath(path)

	return m.Context.Run(func() error {
		// IEEE 1003.1 remove explicitly can unlink files and remove empty directories.
		// We use instead here the semantics of unlink, which is allowed to be restricted against directories.
		stat, err := os.Lstat(rpath)
		if err != nil {
			return err
		} else if stat.IsDir() {
			return syscall.EISDIR
		}

		return os.Remove(rpath)
	})
}

func (m *NativeFS) Rmdir(path string) error {
	rpath := m.BuildPath(path)

	return m.Context.Run(func() error {
		stat, err := os.Lstat(rpath)
		if err != nil {
			return err
		} else if !stat.IsDir() {
			return syscall.ENOTDIR
		}

		return os.Remove(m.BuildPath(path))
	})
}

func (m *NativeFS) FileRead(path string) (vfs.ReaderAt, error) {
	return m.openFile(m.BuildPath(path), os.O_RDONLY, 0o640)
}

func (m *NativeFS) FileWrite(path string, flag int) (vfs.WriterAt, error) {
	return m.openFile(m.BuildPath(path), flag, 0o640)
}

func (m *NativeFS) Open(path string) (vfs.File, error) {
	var f *os.File

	err := m.Context.Run(func() error {
		var err error

		f, err = os.Open(m.BuildPath(path))

		return err
	})
	if err != nil {
		return nil, err
	}

	return &wrapFile{f, path, m.Context}, nil
}

func (m *NativeFS) OpenFile(path string, flag int, perm os.FileMode) (vfs.File, error) {
	return m.openFile(m.BuildPath(path), flag, perm)
}

func (m *NativeFS) openFile(path string, flag int, perm os.FileMode) (*wrapFile, error) {
	var f *os.File

	err := m.Context.Run(func() error {
		var err error

		f, err = os.OpenFile(path, flag, perm)

		return err
	})
	if err != nil {
		return nil, err
	}

	return &wrapFile{f, path, m.Context}, nil
}

func (m *NativeFS) Symlink(target, path string) error {
	absolute := filepath.IsAbs(target)

	if !absolute {
		target = vfs.Join(vfs.Dir(path), target)
	}

	path = m.BuildPath(path)
	target = m.BuildPath(target)

	if !absolute {
		var err error

		target, err = filepath.Rel(filepath.Dir(path), target)
		if err != nil {
			return err
		}
	}

	return m.Context.Run(func() error {
		return os.Symlink(target, path)
	})
}

func (m *NativeFS) Link(target, path string) error {
	absolute := filepath.IsAbs(target)

	if !absolute {
		target = vfs.Join(vfs.Dir(path), target)
	}

	path = m.BuildPath(path)
	target = m.BuildPath(target)

	return m.Context.Run(func() error {
		return os.Link(target, path)
	})
}

func (m *NativeFS) Chmod(path string, mode os.FileMode) error {
	return m.Context.Run(func() error {
		return os.Chmod(m.BuildPath(path), mode)
	})
}

func (m *NativeFS) Chown(path string, uid, gid int) error {
	if !m.AllowChown {
		return os.ErrPermission
	}

	return m.Context.Run(func() error {
		return os.Chown(m.BuildPath(path), uid, gid)
	})
}

func (m *NativeFS) Chtimes(path string, atime, mtime time.Time) error {
	return m.Context.Run(func() error {
		return os.Chtimes(m.BuildPath(path), atime, mtime)
	})
}

func (m *NativeFS) Truncate(path string, size int64) error {
	return m.Context.Run(func() error {
		return os.Truncate(m.BuildPath(path), size)
	})
}

func (m *NativeFS) SetExtendedAttr(path, name string, value []byte) error {
	return m.Context.Run(func() error {
		return SetExtendedAttr(m.BuildPath(path), name, value)
	})
}

func (m *NativeFS) UnsetExtendedAttr(path, name string) error {
	return m.Context.Run(func() error {
		return UnsetExtendedAttr(m.BuildPath(path), name)
	})
}

func (m *NativeFS) SetExtendedAttrs(path string, attrs vfs.Attributes) error {
	return m.Context.Run(func() error {
		return SetExtendedAttrs(m.BuildPath(path), attrs)
	})
}

func (m *NativeFS) Rename(oldpath, newpath string) error {
	target := m.BuildPath(newpath)

	return m.Context.Run(func() error {
		// SFTP-v2: "It is an error if there already exists a file with the name specified by newpath."
		// This varies from the POSIX specification, which allows limited replacement of target files.
		if _, err := os.Lstat(target); err != nil && !os.IsNotExist(err) {
			return err
		} else if err == nil {
			return os.ErrExist
		}

		if err := os.Rename(m.BuildPath(oldpath), target); err != nil {
			return err
		}

		return nil
	})
}

func (m *NativeFS) Close() error {
	return m.Context.Close()
}

func (m *NativeServerInodeFS) Handle(path string) ([]byte, error) {
	var handle []byte

	err := m.Context.Run(func() error {
		var err error

		handle, err = InodeTrail(m.Root, m.BuildPath(path))

		return err
	})

	return handle, err
}

func (m *NativeServerInodeFS) Path(handle []byte) (string, error) {
	var path string

	err := m.Context.Run(func() error {
		var err error

		path, err = FindByInodes(m.Root, handle)

		return err
	})
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(m.Root, path)
	if err != nil {
		return "", err
	}

	if rel == "." {
		return "/", nil
	}

	if rel == "" || rel[0] == '.' {
		return "", syscall.EINVAL
	}

	return filepath.ToSlash(string(os.PathSeparator) + rel), nil
}

type wrapFile struct {
	*os.File
	OrigPath string
	Context  runas.Context
}

func (w *wrapFile) Name() string {
	return w.OrigPath
}

func (w *wrapFile) Stat() (vfs.FileInfo, error) {
	var fi vfs.FileInfo

	err := w.Context.Run(func() error {
		stat, err := w.File.Stat()
		if err != nil {
			return err
		}

		fi = PackExtendedAttrs(stat, w.File.Name())

		return nil
	})

	return fi, err
}

func (w *wrapFile) Truncate(size int64) error {
	return w.Context.Run(func() error {
		return w.File.Truncate(size)
	})
}

func (w *wrapFile) Read(p []byte) (int, error) {
	var n int

	err := w.Context.Run(func() error {
		var err error

		n, err = w.File.Read(p)

		return err
	})

	return n, err
}

func (w *wrapFile) Write(p []byte) (int, error) {
	var n int

	err := w.Context.Run(func() error {
		var err error

		n, err = w.File.Write(p)

		return err
	})

	return n, err
}

func (w *wrapFile) Seek(offset int64, whence int) (int64, error) {
	err := w.Context.Run(func() error {
		var err error

		offset, err = w.File.Seek(offset, whence)

		return err
	})

	return offset, err
}

func (w *wrapFile) Close() error {
	return w.Context.Run(func() error {
		return w.File.Close()
	})
}

func (w *wrapFile) Readdir(count int) ([]vfs.FileInfo, error) {
	var result []vfs.FileInfo

	err := w.Context.Run(func() error {
		orig, err := w.File.Readdir(count)
		if err != nil {
			return err
		}

		result = make([]vfs.FileInfo, len(orig))

		for i, v := range orig {
			result[i] = PackExtendedAttrs(v, filepath.Join(w.File.Name(), v.Name()))
		}

		return nil
	})

	return result, err
}
