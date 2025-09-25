//go:build !linux
// +build !linux

package runas

func RunAs(u *User) (Context, error) {
	return RunAsCurrentUser(), nil
}
