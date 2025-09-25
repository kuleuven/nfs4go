package bytetree

import (
	"errors"
	"fmt"
	"os"
	"time"
)

type FileLock struct {
	path string
}

var ErrTypeAssertion = fmt.Errorf("type assertion error")

var ErrLockHeld = fmt.Errorf("lock held")

func Lock(file *os.File) (*FileLock, error) {
	for range 10 {
		lock, err := TryLock(file)
		if errors.Is(err, ErrLockHeld) {
			time.Sleep(time.Second)

			continue
		}

		return lock, err
	}

	return nil, ErrLockHeld
}

func (file *FileLock) Unlock() error {
	return os.Remove(file.path)
}
