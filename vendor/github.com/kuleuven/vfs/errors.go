package vfs

import (
	"fmt"
	"syscall"
)

var ErrNotSupported = syscall.EOPNOTSUPP

var ErrNotImplemented = fmt.Errorf("%w: not implemented", syscall.EPERM)

var ErrInvalidHandle = fmt.Errorf("%w: invalid handle", syscall.EBADFD)
