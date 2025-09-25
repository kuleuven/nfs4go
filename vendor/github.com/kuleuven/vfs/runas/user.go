package runas

import (
	"fmt"
	"os"
	"slices"
)

type User struct {
	UID    uint32
	GID    uint32
	Groups []uint32
}

func CurrentUser() (*User, error) {
	gids, err := os.Getgroups()
	if err != nil {
		return nil, err
	}

	groups := []uint32{}

	for _, gid := range gids {
		groups = append(groups, uint32(gid))
	}

	return &User{
		UID:    uint32(os.Getuid()),
		GID:    uint32(os.Getgid()),
		Groups: groups,
	}, nil
}

func (u *User) String() string {
	return fmt.Sprintf("uid=%d gid=%d groups=%v", u.UID, u.GID, u.Groups)
}

func (u *User) Equal(other *User) bool {
	return u.UID == other.UID && u.GID == other.GID && slices.Equal(u.Groups, other.Groups)
}
