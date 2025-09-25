package handledb

import (
	"crypto"
	"encoding/binary"
	"errors"
	"os"
	"sync"

	"github.com/kuleuven/vfs/bytetree"

	_ "crypto/md5" //nolint:gosec
)

type DB struct {
	tree *bytetree.ByteTree
	sync.Mutex
}

// 64 exabyte is not a good idea :)
// <path>/inodes.db [inode:number]
// <path>/files.db  [number:path]
// <path>/lock

// byte -> 0 <not known>
// byte -> negative int64 <position of record>
// byte -> int64 <position of next byte range>

// byte -> [byte]byte

func New(path string) (*DB, error) {
	if err := os.MkdirAll(path, 0o750); err != nil {
		return nil, err
	}

	tree, err := bytetree.New(path)

	// tree.Print()

	return &DB{
		tree: tree,
	}, err
}

func (db *DB) Put(handle []byte, path string) error {
	db.Lock()
	defer db.Unlock()

	for range 10 {
		err := db.tree.Put(bytetree.Value{Handle: handle, Path: path})
		if errors.Is(err, bytetree.ErrHasValue) {
			continue
		}

		return err
	}

	return bytetree.ErrHasValue
}

func (db *DB) Get(handle []byte) (string, error) {
	db.Lock()
	defer db.Unlock()

	return db.tree.Get(handle)
}

func (db *DB) Generate(path string) ([]byte, error) {
	hasher := crypto.MD5.New()

	_, err := hasher.Write([]byte(path))
	if err != nil {
		return nil, err
	}

	handle := hasher.Sum(nil)[:8]

	for {
		stored, err := db.Get(handle)
		if err != nil {
			return handle, db.Put(handle, path)
		}

		if stored == path {
			return handle, nil
		}

		binary.BigEndian.PutUint64(handle, binary.BigEndian.Uint64(handle)+1)
	}
}

func (db *DB) Close() error {
	return db.tree.Close()
}
