package bytetree

import (
	"encoding/binary"
	"io"
	"os"
)

type values struct {
	file *os.File
}

type Value struct {
	Handle []byte
	Path   string
}

func (v *values) Get(ptr int64) (Value, error) {
	var buf [8]byte

	_, err := v.file.ReadAt(buf[:], ptr)
	if err != nil {
		return Value{}, err
	}

	handleLen := int(binary.BigEndian.Uint32(buf[:4]))
	pathLen := int(binary.BigEndian.Uint32(buf[4:]))

	payload := make([]byte, handleLen+pathLen)

	_, err = v.file.ReadAt(payload, ptr+8)
	if err != nil {
		return Value{}, err
	}

	return Value{
		Handle: payload[:handleLen],
		Path:   string(payload[handleLen:]),
	}, nil
}

func (v *values) Add(value Value) (int64, error) {
	lock, err := Lock(v.file)
	if err != nil {
		return 0, err
	}

	defer lock.Unlock() //nolint:errcheck

	ptr, err := v.file.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}

	err = binary.Write(v.file, binary.BigEndian, uint32(len(value.Handle)))
	if err != nil {
		return 0, err
	}

	err = binary.Write(v.file, binary.BigEndian, uint32(len(value.Path)))
	if err != nil {
		return 0, err
	}

	_, err = v.file.Write(value.Handle)
	if err != nil {
		return 0, err
	}

	_, err = v.file.Write([]byte(value.Path))
	if err != nil {
		return 0, err
	}

	return ptr, nil
}

func (v *values) Close() error {
	return v.file.Close()
}
