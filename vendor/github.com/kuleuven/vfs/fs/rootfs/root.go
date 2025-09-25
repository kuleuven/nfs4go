package rootfs

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/kuleuven/vfs"
	"github.com/kuleuven/vfs/fs/rootfs/handledb"
	"github.com/sirupsen/logrus"
	"go.uber.org/multierr"
)

var _ vfs.RootFS = &Root{}

var _ vfs.WalkFS = &Root{}

type Root struct {
	Context context.Context //nolint:containedctx
	mounts  []*Mount
}

func New(ctx context.Context) *Root {
	return &Root{
		Context: ctx,
	}
}

// Logger returns a logger for the current session.
func (r *Root) Logger() *logrus.Entry {
	return vfs.Logger(r.Context)
}

// Mount adds a mountpoint to the root FS.
// It is an helper function that can be used in an implementation of HandlerFromEnv.
// A mountpoint can only be added if it either replaces a previously mounted path,
// or if the parent path exists and is a directory, and does not yet contain a
// file with the same name. If not, an error is returned.
func (r *Root) Mount(path string, fs vfs.FS, index byte) error {
	path = vfs.Clean(path)

	if !vfs.IsAbs(path) {
		return fmt.Errorf("mount path %q is not absolute", path)
	}

	// Allow to remount at a certain path
	for i, mp := range r.mounts {
		if mp.Mountpoint == path {
			r.mounts[i] = r.prepareMount(path, fs, index)

			return nil
		}
	}

	// Check that path does not exist
	if _, err := r.Stat(path); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Check that parent path exists and is a directory
	if path != "/" {
		if fi, err := r.Stat(vfs.Dir(path)); err != nil {
			return err
		} else if !fi.IsDir() {
			return syscall.ENOTDIR
		}
	}

	// Add mount
	r.mounts = append(r.mounts, r.prepareMount(path, fs, index))

	sort.Slice(r.mounts, func(i, j int) bool {
		return r.mounts[i].Mountpoint > r.mounts[j].Mountpoint
	})

	return nil
}

// MustMount attempts to mount the filesystem at the specified path
// and panics if the operation fails. It is a convenience function
// for scenarios where failure to mount is considered a critical
// error and should halt execution.
func (r *Root) MustMount(path string, fs vfs.FS, index byte) {
	if err := r.Mount(path, fs, index); err != nil {
		panic(err)
	}
}

// AddMountNoCheck adds a mountpoint to the root FS without any checks.
// This should only be used if it is absolute sure that the required
// conditions are met: the parent path must exist and be a directory, and
// there must not yet be a file, directory or mount with the same name.
func (r *Root) AddMountNoCheck(path string, fs vfs.FS, index byte) {
	// Add mount
	r.mounts = append(r.mounts, r.prepareMount(path, fs, index))

	sort.Slice(r.mounts, func(i, j int) bool {
		return r.mounts[i].Mountpoint > r.mounts[j].Mountpoint
	})
}

func (r *Root) prepareMount(path string, fs vfs.FS, index byte) *Mount {
	mount := &Mount{
		Index:      index,
		Mountpoint: path,
		FS:         fs,
	}

	if _, ok := mount.FS.(vfs.HandleResolveFS); ok {
		return mount
	}

	storage, ok := r.Context.Value(vfs.PersistentStorage).(string)
	if !ok || storage == "" {
		r.Logger().Warnf("Cannot load HandleDB for %s, no persistent storage configured", mount.Mountpoint)

		return mount
	}

	db, err := handledb.New(filepath.Join(storage, fmt.Sprintf("%02x", mount.Index)))
	if err != nil {
		r.Logger().Warnf("Cannot load HandleDB for %s: %v", mount.Mountpoint, err)

		return mount
	}

	mount.HandleDB = db

	return mount
}

type Mount struct {
	Index      byte
	Mountpoint string
	vfs.FS
	HandleDB *handledb.DB
}

func (m Mount) Contains(path string) bool {
	return path == m.Mountpoint || strings.HasPrefix(path, m.Mountpoint+string(vfs.Separator))
}

// Below checks whether the mountpoint is below the given path,
// as a strict ancestor.
func (m Mount) Below(path string) bool {
	return strings.HasPrefix(m.Mountpoint, path+string(vfs.Separator))
}

func (r *Root) Close() error {
	r.Logger().Trace("Close()")

	var result error

	for _, mp := range r.mounts {
		if mp.FS == nil {
			continue
		}

		if err := mp.Close(); err != nil {
			result = multierr.Append(result, err)
		}

		if mp.HandleDB == nil {
			continue
		}

		if err := mp.HandleDB.Close(); err != nil {
			result = multierr.Append(result, err)
		}
	}

	return result
}

// ResolvePath resolves all symlinks in the directory path, but does not
// check whether the file actually exists, i.e. the file name is left
// unaltered.
func (r *Root) ResolvePath(path string) (*Mount, string, error) {
	// If path is a mountpoint, return the mountpoint
	for _, mp := range r.mounts {
		if path == mp.Mountpoint {
			return mp, "/", nil
		}
	}

	// Handle the case when nothing is mounted on /
	if path == "/" {
		return nil, "", os.ErrNotExist
	}

	if path == "" || !vfs.IsPathSeparator(path[0]) {
		return nil, "", os.ErrNotExist
	}

	// Look up the parent
	dir, file := vfs.Split(path)

	// Resolve all symlinks for the parent
	fs, fsdir, err := r.followSymlinks(dir, false, 16)
	if err != nil {
		return nil, "", err
	}

	r.Logger().Tracef("ResolvePath(%q) -> (%q, %q)", path, fs.Mountpoint, vfs.Join(fsdir, file))

	return fs, vfs.Join(fsdir, file), nil
}

// FollowSymlinks resolves all symlinks in the full path, including the
// link possibly defined by the file name. If the path leads to an unexisting
// file, we fallback to ResolvePath.
func (r *Root) FollowSymlinks(path string) (*Mount, string, error) {
	fs, fspath, err := r.followSymlinks(path, true, 16)
	if err != nil {
		return nil, "", err
	}

	r.Logger().Tracef("FollowSymlinks(%q) -> (%q, %q)", path, fs.Mountpoint, fspath)

	return fs, fspath, nil
}

// FollowSymlinksNoDangling resolves all symlinks in the full path, including the
// link possibly defined by the file name. If the path leads to an unexisting
// file, we return ErrNotExist.
func (r *Root) FollowSymlinksNoDangling(path string) (*Mount, string, error) {
	fs, fspath, err := r.followSymlinks(path, false, 16)
	if err != nil {
		return nil, "", err
	}

	r.Logger().Tracef("FollowSymlinksNoDangling(%q) -> (%q, %q)", path, fs.Mountpoint, fspath)

	return fs, fspath, nil
}

// FollowSymlinks resolves all symlinks in the full path, including the
// link possibly defined by the file name.
func (r *Root) followSymlinks(path string, acceptDangling bool, budget int) (*Mount, string, error) {
	// If path is a mountpoint, return the mountpoint
	for _, mp := range r.mounts {
		if path == mp.Mountpoint {
			return mp, "/", nil
		}
	}

	// Handle the case when nothing is mounted on /
	if path == "/" {
		return nil, "", os.ErrNotExist
	}

	if path == "" || !vfs.IsPathSeparator(path[0]) {
		return nil, "", os.ErrNotExist
	}

	// Look up the parent
	dir, file := vfs.Split(path)

	fs, fsdir, err := r.followSymlinks(dir, false, budget)
	if err != nil {
		return nil, "", err
	}

	fspath := vfs.Join(fsdir, file)

	symlinkFS, ok := fs.FS.(vfs.SymlinkFS)
	if !ok {
		return fs, fspath, nil
	}

	fi, err := symlinkFS.Lstat(fspath)
	if errors.Is(err, os.ErrNotExist) && acceptDangling {
		return fs, fspath, nil
	}

	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return fs, fspath, err
	}

	if budget <= 0 {
		return nil, "", fmt.Errorf("%w: too many symlinks in path %s", os.ErrInvalid, path)
	}

	target, err := symlinkFS.Readlink(fspath)
	if err != nil {
		return nil, "", err
	}

	if vfs.IsAbs(target) {
		return r.followSymlinks(vfs.Join(fs.Mountpoint, target[1:]), acceptDangling, budget-1)
	}

	return r.followSymlinks(vfs.Clean(vfs.Join(dir, target)), acceptDangling, budget-1)
}

func (r *Root) Mountpoints() []string {
	result := []string{}

	for _, mount := range r.mounts {
		result = append(result, mount.Mountpoint)
	}

	return result
}

func (r *Root) FileRead(path string) (vfs.ReaderAt, error) {
	r.Logger().Debugf("FileRead(%q)", path)

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return nil, err
	}

	return fs.FileRead(path)
}

func (r *Root) FileWrite(path string, flag int) (vfs.WriterAt, error) {
	r.Logger().Debugf("FileWrite(%q, %d)", path, flag)

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return nil, err
	}

	return fs.FileWrite(path, flag)
}

func (r *Root) Open(path string) (vfs.File, error) {
	r.Logger().Debugf("Open(%q)", path)

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return nil, err
	}

	return vfs.Open(fs, path)
}

func (r *Root) OpenFile(path string, flag int, perm os.FileMode) (vfs.File, error) {
	r.Logger().Debugf("OpenFile(%q, %d)", path, flag)

	logicalPath := path

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return nil, err
	}

	if ofs, ok := fs.FS.(vfs.OpenFileFS); ok {
		f, err := ofs.OpenFile(path, flag, perm)
		if err != nil {
			return nil, err
		}

		return &wrapFileName{f, logicalPath}, nil
	}

	if flag&os.O_WRONLY == 0 && flag&os.O_RDWR == 0 {
		return vfs.Open(fs, path)
	}

	return nil, vfs.ErrNotSupported
}

type wrapFileName struct {
	vfs.File
	logicalPath string
}

func (w *wrapFileName) Name() string {
	return w.logicalPath
}

func (r *Root) Chmod(path string, mode os.FileMode) error {
	r.Logger().Debugf("Chmod(%q, %v)", path, mode)

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return err
	}

	return fs.Chmod(path, mode)
}

func (r *Root) Chown(path string, uid, gid int) error {
	r.Logger().Debugf("Chown(%q, %d, %d)", path, uid, gid)

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return err
	}

	return fs.Chown(path, uid, gid)
}

func (r *Root) Chtimes(path string, atime, mtime time.Time) error {
	r.Logger().Debugf("Chtimes(%q, %v, %v)", path, atime, mtime)

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return err
	}

	return fs.Chtimes(path, atime, mtime)
}

func (r *Root) Truncate(path string, size int64) error {
	r.Logger().Debugf("Truncate(%q, %d)", path, size)

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return err
	}

	return fs.Truncate(path, size)
}

func (r *Root) SetExtendedAttr(path, name string, value []byte) error {
	r.Logger().Debugf("SetAttr(%q, %s, %v)", path, name, value)

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return err
	}

	return fs.SetExtendedAttr(path, name, value)
}

func (r *Root) UnsetExtendedAttr(path, name string) error {
	r.Logger().Debugf("UnsetAttr(%q, %s)", path, name)

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return err
	}

	return fs.UnsetExtendedAttr(path, name)
}

func (r *Root) SetExtendedAttrs(path string, attrs vfs.Attributes) error {
	r.Logger().Debugf("SetExtendedAttrs(%q, %v)", path, attrs)

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return err
	}

	return vfs.SetExtendedAttrs(fs, path, attrs)
}

func (r *Root) Rename(oldpath, newpath string) error {
	r.Logger().Debugf("Rename(%q, %q)", oldpath, newpath)

	fs, path, err := r.ResolvePath(oldpath)
	if err != nil {
		return err
	}

	newfs, target, err2 := r.ResolvePath(newpath)
	if err2 != nil {
		return err2
	}

	if fs.Mountpoint != newfs.Mountpoint {
		return vfs.ErrNotSupported
	}

	return fs.Rename(path, target)
}

func (r *Root) Rmdir(path string) error {
	r.Logger().Debugf("Rmdir(%q)", path)

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return err
	}

	return fs.Rmdir(path)
}

func (r *Root) Remove(path string) error {
	r.Logger().Debugf("Remove(%q)", path)

	fs, path, err := r.ResolvePath(path)
	if err != nil {
		return err
	}

	return fs.Remove(path)
}

func (r *Root) Mkdir(path string, perm os.FileMode) error {
	r.Logger().Debugf("Mkdir(%q, %v)", path, perm)

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return err
	}

	return fs.Mkdir(path, perm)
}

func (r *Root) Link(target, path string) error {
	r.Logger().Debugf("Link(%q, %q)", target, path)

	if !vfs.IsAbs(target) {
		target = vfs.Clean(vfs.Join(vfs.Dir(path), target))
	}

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return err
	}

	fs2, target, err := r.ResolvePath(target)
	if err != nil {
		return err
	}

	if fs != fs2 {
		return vfs.ErrNotSupported
	}

	if linkFS, ok := fs.FS.(vfs.LinkFS); ok {
		return linkFS.Link(target, path)
	}

	return vfs.ErrNotSupported
}

func (r *Root) Symlink(target, path string) error {
	r.Logger().Debugf("Symlink(%q, %q)", target, path)

	abstarget := target

	if !vfs.IsAbs(target) {
		abstarget = vfs.Clean(vfs.Join(vfs.Dir(path), target))
	}

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return err
	}

	fs2, abstarget, err := r.ResolvePath(abstarget)
	if err != nil {
		return err
	}

	if fs != fs2 {
		return vfs.ErrNotSupported
	}

	symlinkFS, ok := fs.FS.(vfs.SymlinkFS)
	if !ok {
		return vfs.ErrNotSupported
	}

	if !vfs.IsAbs(target) {
		return symlinkFS.Symlink(target, path)
	}

	return symlinkFS.Symlink(abstarget, path)
}

func (r *Root) List(path string) (vfs.ListerAt, error) {
	r.Logger().Debugf("List(%q)", path)

	logicalPath := path

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		logrus.Warnf("List(%q) error: %v", path, err)
		return nil, err
	}

	lister, err := fs.List(path)
	if err != nil {
		return nil, err
	}

	return &virtualLister{
		lister:      lister,
		Root:        r,
		logicalPath: logicalPath,
	}, nil
}

type virtualLister struct {
	lister vfs.ListerAt
	*Root
	logicalPath string // the logical path of the virtual directory
}

func (v *virtualLister) Virtual() ([]vfs.FileInfo, error) {
	virtual := []vfs.FileInfo{}

	for _, mount := range v.mounts {
		if len(mount.Mountpoint) <= len(v.logicalPath) {
			continue
		}

		base, dir := vfs.Split(mount.Mountpoint)

		if base == v.logicalPath {
			fi, err := mount.Stat("/")
			if err != nil {
				return nil, err
			}

			virtual = append(virtual, &virtualdir{
				name:     dir,
				FileInfo: fi,
			})
		}
	}

	return virtual, nil
}

func (v *virtualLister) ListAt(ls []vfs.FileInfo, offset int64) (int, error) {
	virtual, err := v.Virtual()
	if err != nil {
		return 0, err
	}

	if offset >= int64(len(virtual)) {
		return v.lister.ListAt(ls, offset-int64(len(virtual)))
	}

	copied := copy(ls, virtual[offset:])

	if copied == len(ls) {
		return copied, nil
	}

	rest, err := v.lister.ListAt(ls[copied:], offset+int64(copied-len(virtual)))

	return rest + copied, err
}

func (v *virtualLister) Close() error {
	if closer, ok := v.lister.(io.Closer); ok {
		return closer.Close()
	}

	return nil
}

type virtualdir struct {
	name string
	vfs.FileInfo
}

func (f *virtualdir) Name() string { return f.name }

func (r *Root) Stat(path string) (vfs.FileInfo, error) {
	r.Logger().Debugf("Stat(%q)", path)

	logicalPath := path

	fs, path, err := r.FollowSymlinksNoDangling(path)
	if err != nil {
		return nil, err
	}

	fi, err := fs.Stat(path)
	if err != nil {
		return nil, err
	}

	if path == "/" {
		return &virtualdir{
			name:     vfs.Base(logicalPath),
			FileInfo: fi,
		}, nil
	}

	return fi, nil
}

func (r *Root) Lstat(path string) (vfs.FileInfo, error) {
	r.Logger().Debugf("Lstat(%q)", path)

	logicalPath := path

	fs, path, err := r.ResolvePath(path)
	if err != nil {
		return nil, err
	}

	var fi vfs.FileInfo

	if lFS, ok := fs.FS.(vfs.SymlinkFS); ok {
		fi, err = lFS.Lstat(path)
	} else {
		fi, err = fs.Stat(path)
	}

	if err != nil {
		return nil, err
	}

	if path == "/" {
		return &virtualdir{
			name:     vfs.Base(logicalPath),
			FileInfo: fi,
		}, nil
	}

	return fi, nil
}

func (r *Root) Readlink(path string) (string, error) {
	r.Logger().Debugf("Readlink(%q)", path)

	fs, path, err := r.ResolvePath(path)
	if err != nil {
		return "", err
	}

	symlinkFS, ok := fs.FS.(vfs.SymlinkFS)
	if !ok {
		return "", vfs.ErrNotSupported
	}

	target, err := symlinkFS.Readlink(path)
	if err != nil {
		return "", err
	}

	if vfs.IsAbs(target) {
		target = vfs.Join(fs.Mountpoint, target[1:])
	}

	return target, nil
}

func (r *Root) RealPath(path string) (string, error) {
	r.Logger().Debugf("RealPath(%q)", path)

	if path == "." || path == "" || path == "/" {
		return "/", nil
	}

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return "", err
	}

	return vfs.Join(fs.Mountpoint, path[1:]), nil
}

func (r *Root) Handle(path string) ([]byte, error) {
	r.Logger().Debugf("Handle(%q)", path)

	if path == "/" {
		return []byte{0}, nil
	}

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return nil, err
	}

	if inodeFS, ok := fs.FS.(vfs.HandleResolveFS); ok {
		handle, err := inodeFS.Handle(path)

		handle = append([]byte{fs.Index}, handle...)

		return handle, err
	}

	if inodeFS, ok := fs.FS.(vfs.HandleFS); ok && fs.HandleDB != nil {
		handle, err := inodeFS.Handle(path)
		if err == nil {
			err = fs.HandleDB.Put(handle, path)
		}

		handle = append([]byte{fs.Index}, handle...)

		return handle, err
	}

	if fs.HandleDB != nil {
		handle, err := fs.HandleDB.Generate(path)

		handle = append([]byte{fs.Index}, handle...)

		return handle, err
	}

	return []byte{UnsupportedHandle}, nil
}

var UnsupportedHandle = byte(254)

func (r *Root) Path(handle []byte) (string, error) {
	r.Logger().Debugf("Path(%s)", hex.EncodeToString(handle))

	if len(handle) == 0 || handle[0] == UnsupportedHandle {
		return "", vfs.ErrNotSupported
	}

	if len(handle) == 1 && handle[0] == 0 {
		return "/", nil
	}

	for _, mp := range r.mounts {
		if mp.Index != handle[0] {
			continue
		}

		resolveFS, ok := mp.FS.(vfs.HandleResolveFS)
		if !ok && mp.HandleDB == nil {
			return "", vfs.ErrNotSupported
		}

		var (
			path string
			err  error
		)

		if ok {
			path, err = resolveFS.Path(handle[1:])
		} else {
			path, err = mp.HandleDB.Get(handle[1:])
		}

		return vfs.Join(mp.Mountpoint, path[1:]), err
	}

	return "", os.ErrNotExist
}

func (r *Root) MkdirAll(path string, mode os.FileMode) error {
	r.Logger().Debugf("MkdirAll(%q, %v)", path, mode)

	return vfs.MkdirAll(r, path, mode)
}

func (r *Root) Walk(path string, fn vfs.WalkFunc) error {
	r.Logger().Debugf("Walk(%q)", path)

	fs, path, err := r.FollowSymlinks(path)
	if err != nil {
		return err
	}

	return vfs.Walk(fs.FS, path, r.walkFS(fs.Mountpoint, fn))
}

func (r *Root) walkFS(mountpoint string, fn vfs.WalkFunc) vfs.WalkFunc {
	return func(path string, info vfs.FileInfo, err error) error {
		if err1 := fn(vfs.Clean(mountpoint+path), info, err); err1 != nil || err != nil {
			return err1
		}

		if !info.IsDir() {
			return nil
		}

		for _, m := range r.mounts {
			if m.Mountpoint == "/" || vfs.Dir(m.Mountpoint) != vfs.Clean(mountpoint+path) {
				continue
			}

			if err1 := vfs.Walk(m.FS, "/", r.walkFS(m.Mountpoint, fn)); err1 != nil {
				return err1
			}
		}

		return nil
	}
}

func (r *Root) RemoveAll(path string) error {
	r.Logger().Debugf("RemoveAll(%q)", path)

	return vfs.RemoveAll(r, path)
}
