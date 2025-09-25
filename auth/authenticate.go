package auth

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/kuleuven/nfs4go/msg"
	"github.com/kuleuven/nfs4go/xdr"
)

type AuthError struct {
	Code uint32
}

func (err *AuthError) Error() string {
	return fmt.Sprintf("auth error: %d", err.Code)
}

var (
	ErrBadCredentials = &AuthError{Code: msg.AUTH_BADCRED}
	ErrTooWeak        = &AuthError{Code: msg.AUTH_TOOWEAK}
)

// Authenticate connecting clients.
// Returns *Auth to reply to the client.
func Authenticate(cred, verf msg.Auth) (msg.Auth, *Creds, error) {
	if cred.Flavor < msg.AUTH_FLAVOR_UNIX {
		return msg.Auth{}, nil, ErrTooWeak
	}

	var credentials Creds

	if err := xdr.NewDecoder(bytes.NewBuffer(cred.Body)).Decode(&credentials); err != nil {
		return msg.Auth{}, nil, err
	}

	return msg.Auth{Flavor: msg.AUTH_FLAVOR_UNIX, Body: []byte{}}, &credentials, nil
}

type Creds struct {
	ExpirationValue  uint32
	Hostname         string
	UID              uint32
	GID              uint32
	AdditionalGroups []uint32
}

// Decode is a specialized Decode function to avoid the use of the reflect package.
func (c *Creds) Decode(decoder *xdr.Decoder) error {
	var err error

	c.ExpirationValue, err = decoder.Uint32()
	if err != nil {
		return err
	}

	c.Hostname, err = decoder.String()
	if err != nil {
		return err
	}

	c.UID, err = decoder.Uint32()
	if err != nil {
		return err
	}

	c.GID, err = decoder.Uint32()
	if err != nil {
		return err
	}

	length, err := decoder.Uint32()
	if err != nil {
		return err
	}

	c.AdditionalGroups = nil

	for range length {
		gid, err := decoder.Uint32()
		if err != nil {
			return err
		}

		c.AdditionalGroups = append(c.AdditionalGroups, gid)
	}

	return nil
}

func (c *Creds) Equal(other *Creds) bool {
	if slices.Equal(c.AdditionalGroups, other.AdditionalGroups) {
		return c.UID == other.UID
	}

	if d := len(c.AdditionalGroups) - len(other.AdditionalGroups); d > 1 || d < -1 {
		return false
	}

	myGroups := append([]uint32{c.GID}, c.AdditionalGroups...)
	otherGroups := append([]uint32{other.GID}, other.AdditionalGroups...)

	slices.Sort(myGroups)
	slices.Sort(otherGroups)

	myGroups = slices.Compact(myGroups)
	otherGroups = slices.Compact(otherGroups)

	return c.UID == other.UID && slices.Equal(myGroups, otherGroups)
}

func (c *Creds) String() string {
	return fmt.Sprintf("expiration: %d, hostname: %s, uid: %d, gid: %d, groups: %v", c.ExpirationValue, c.Hostname, c.UID, c.GID, c.AdditionalGroups)
}
