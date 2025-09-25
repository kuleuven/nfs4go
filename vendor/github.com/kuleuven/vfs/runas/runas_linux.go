//go:build linux
// +build linux

package runas

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/sirupsen/logrus"
	"go.uber.org/multierr"
)

func RunAs(u *User) (Context, error) {
	current, err := CurrentUser()
	if err != nil {
		return nil, err
	}

	if current.Equal(u) {
		return RunAsCurrentUser(), nil
	}

	executor := &executor{
		User:    u,
		Tasks:   make(chan func() error),
		Results: make(chan error),
	}

	return executor, executor.Start()
}

type executor struct {
	User    *User
	Tasks   chan func() error
	Results chan error
	Closed  bool
	sync.Mutex
}

func (e *executor) Run(f func() error) error {
	e.Lock()
	defer e.Unlock()

	e.Tasks <- f

	return <-e.Results
}

func (e *executor) Start() error {
	go e.Loop()

	return <-e.Results
}

func (e *executor) Loop() {
	runtime.LockOSThread()

	var err error

	n := uintptr(len(e.User.Groups))
	if n == 0 {
		_, _, er := syscall.Syscall(syscall.SYS_SETGROUPS, 0, 0, 0)
		if er != 0 {
			err = multierr.Append(err, er)
		}
	} else {
		a := e.User.Groups

		_, _, er := syscall.Syscall(syscall.SYS_SETGROUPS, n, uintptr(unsafe.Pointer(&a[0])), 0)
		if er != 0 {
			err = multierr.Append(err, er)
		}
	}

	_, _, er := syscall.Syscall(syscall.SYS_SETGID, uintptr(e.User.GID), 0, 0)
	if er != 0 {
		err = multierr.Append(err, er)
	}

	_, _, er = syscall.Syscall(syscall.SYS_SETUID, uintptr(e.User.UID), 0, 0)
	if er != 0 {
		err = multierr.Append(err, er)
	}

	e.Results <- err

	if err != nil {
		logrus.Error(err)

		return
	}

	for f := range e.Tasks {
		e.Results <- f()
	}
}

func (e *executor) Close() error {
	e.Lock()
	defer e.Unlock()

	if e.Closed {
		return nil
	}

	close(e.Tasks)

	e.Closed = true

	return nil
}
