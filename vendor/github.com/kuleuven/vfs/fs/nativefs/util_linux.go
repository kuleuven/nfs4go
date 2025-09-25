//go:build linux
// +build linux

package nativefs

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/joshlf/go-acl"
	"github.com/kuleuven/vfs"
)

func Inode(fi os.FileInfo) uint64 {
	if s, ok := fi.Sys().(*syscall.Stat_t); ok {
		return s.Ino
	}

	return 1
}

func FindByInodes(root string, handle []byte) (string, error) {
	// Find by traversing inodes
	curr := root

outer:
	for len(handle) >= 8 {
		inode := binary.LittleEndian.Uint64(handle[:8])

		entries, err := os.ReadDir(curr)
		if err != nil {
			return "", err
		}

		for _, entry := range entries {
			fi, err := entry.Info()
			if err != nil || Inode(fi) != inode {
				continue
			}

			curr = filepath.Join(curr, entry.Name())
			handle = handle[8:]

			continue outer
		}

		return "", os.ErrNotExist
	}

	return curr, nil
}

func SystemPermissions(path string) (*vfs.Permissions, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	a, err := acl.Get(path)
	if err != nil {
		return nil, err
	}

	pfi, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return nil, err
	}

	pa, err := acl.Get(filepath.Dir(path))
	if err != nil {
		return nil, err
	}

	readErr := checkPermission(fi, a, Read)

	if fi.IsDir() && readErr == nil {
		readErr = checkPermission(fi, a, Execute)
	}

	return &vfs.Permissions{
		Read:             readErr == nil,
		Write:            checkPermission(fi, a, Write) == nil,
		Delete:           checkPermission(pfi, pa, Write) == nil,
		Own:              checkOwnership(fi) == nil,
		GetExtendedAttrs: checkPermission(fi, a, Read) == nil,
		SetExtendedAttrs: checkPermission(fi, a, Write) == nil,
	}, nil
}

func checkOwnership(fi os.FileInfo) error {
	if syscall.Getuid() == 0 {
		return nil
	}

	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return os.ErrPermission
	}

	if int(stat.Uid) != syscall.Getuid() {
		return os.ErrPermission
	}

	return nil
}

type Permission uint32

const (
	Read    Permission = 4
	Write   Permission = 2
	Execute Permission = 1
)

func (p Permission) Check(m os.FileMode) error {
	if uint32(p)&uint32(m) > 0 {
		return nil
	}

	return os.ErrPermission
}

var ErrTypeAssertion = fmt.Errorf("type assertion error")

func checkPermission(fi os.FileInfo, a acl.ACL, perm Permission) error {
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return ErrTypeAssertion
	}

	gids, err := syscall.Getgroups()
	if err != nil {
		return err
	}

	groups := append([]int{syscall.Getgid()}, gids...)

	var aclmode os.FileMode

	switch {
	case stat.Uid == uint32(syscall.Getuid()):
		return perm.Check(fi.Mode() >> 6)
	case find(a, acl.TagUser, syscall.Getuid(), &aclmode):
		return perm.Check(aclmode & aclmask(a))
	default:
		// See below
	}

	foundGroup := false
	mask := aclmask(a)

	// Check group permission
	if slices.Contains(groups, int(stat.Gid)) {
		if perm.Check(fi.Mode()>>3&mask) == nil {
			return nil
		}

		foundGroup = true
	}

	// Check acl group permissions
	for _, g := range groups {
		if !find(a, acl.TagGroup, g, &aclmode) {
			continue
		}

		if perm.Check(aclmode&mask) == nil {
			return nil
		}

		foundGroup = true
	}

	if foundGroup {
		return os.ErrPermission
	}

	return perm.Check(fi.Mode())
}

func aclmask(a acl.ACL) os.FileMode {
	for _, entry := range a {
		if entry.Tag == acl.TagMask {
			return entry.Perms
		}
	}

	return 7
}

func find(a acl.ACL, tag acl.Tag, id int, result *os.FileMode) bool {
	q := fmt.Sprintf("%d", id)

	for _, entry := range a {
		if entry.Tag != tag {
			continue
		}

		if entry.Qualifier != q {
			continue
		}

		*result = entry.Perms

		return true
	}

	return false
}
