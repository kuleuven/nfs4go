//nolint:staticcheck
package nfs4go

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"

	"github.com/kuleuven/nfs4go/auth"
	"github.com/kuleuven/nfs4go/clients"
	"github.com/kuleuven/nfs4go/clock"
	"github.com/kuleuven/nfs4go/logger"
	"github.com/kuleuven/nfs4go/msg"
	"github.com/kuleuven/nfs4go/xdr"
	"github.com/kuleuven/vfs"
)

const (
	A_supported_attrs    = 0  // bitmap4
	A_type               = 1  // nfs_ftype4, enum int
	A_fh_expire_type     = 2  // uint32
	A_change             = 3  // changeid4, uint64
	A_size               = 4  // uint64
	A_link_support       = 5  // bool
	A_symlink_support    = 6  // bool
	A_named_attr         = 7  // bool
	A_fsid               = 8  // fsid4, struct{uint64, uint64}
	A_unique_handles     = 9  // bool
	A_lease_time         = 10 // nfs_lease4, uint32
	A_rdattr_error       = 11 // nfsstat4, enum int
	A_acl                = 12 // nfsace4, struct{uint32, uint32, uint32, string}
	A_aclsupport         = 13 // uint32
	A_case_insensitive   = 16
	A_case_preserving    = 17
	A_chown_restricted   = 18 // bool
	A_filehandle         = 19 // nfs_fh4, opaque<>
	A_fileid             = 20 // uint64
	A_maxname            = 29 // uint32
	A_maxread            = 30 // uint64
	A_maxwrite           = 31 // uint64
	A_mode               = 33 // (v4.1) uint32
	A_no_trunc           = 34 // bool
	A_numlinks           = 35 // uint32
	A_owner              = 36 // string
	A_owner_group        = 37 // string
	A_rawdev             = 41 // struct{uint32, uint32}
	A_space_used         = 45 // uint64
	A_time_access        = 47 // nfstime4, struct{uint64, uint32}
	A_time_metadata      = 52 // nfstime4, struct{uint64, uint32}
	A_time_modify        = 53 // nfstime4, struct{uint64, uint32}
	A_time_modify_set    = 54
	A_mounted_on_fileid  = 55 // uint64
	A_suppattr_exclcreat = 75 // (v4.1) bitmap4
	A_xattr_support      = 82
	A_acl_trueform       = 88
	A_acl_trueform_scope = 89
	A_posix_default_acl  = 90
	A_posix_access_acl   = 91
)

var AttrsDefaultSet = []int{
	// A_supported_attrs,
	A_type,
	A_fh_expire_type,
	A_change,
	A_size,
	// A_link_support,
	// A_symlink_support,
	// A_named_attr,
	A_fsid,
	// A_unique_handles,
	// A_lease_time,
	A_rdattr_error,
	A_filehandle,
	A_fileid,
	A_mode,
	A_numlinks,
	A_owner,
	A_owner_group,
	A_rawdev,
	A_space_used,
	A_time_access,
	A_time_metadata,
	A_time_modify,
	A_mounted_on_fileid,
	// A_suppattr_exclcreat,
}

var AttrsSupported = []int{
	A_supported_attrs,
	A_type,
	A_fh_expire_type,
	A_change,
	A_size,
	A_link_support,
	A_symlink_support,
	A_named_attr,
	A_fsid,
	A_unique_handles,
	A_lease_time,
	A_aclsupport,
	A_case_insensitive,
	A_case_preserving,
	A_rdattr_error,
	A_chown_restricted,
	A_filehandle,
	A_fileid,
	A_maxname,
	A_maxread,
	A_maxwrite,
	A_mode,
	A_no_trunc,
	A_numlinks,
	A_owner,
	A_owner_group,
	A_rawdev,
	A_space_used,
	A_time_access,
	A_time_metadata,
	A_time_modify,
	A_mounted_on_fileid,
	A_suppattr_exclcreat,
	A_xattr_support,
	A_acl,
	A_aclsupport,
	/*A_acl_trueform,
	A_acl_trueform_scope,
	A_posix_default_acl,
	A_posix_access_acl,*/
}

func GetAttrNameByID(id int) (string, bool) { //nolint:funlen,gocyclo
	switch id {
	case A_supported_attrs:
		return "supported_attrs", true
	case A_type:
		return "type", true
	case A_fh_expire_type:
		return "fh_expire_type", true
	case A_change:
		return "change", true
	case A_size:
		return "size", true
	case A_link_support:
		return "link_support", true
	case A_symlink_support:
		return "symlink_support", true
	case A_named_attr:
		return "named_attr", true
	case A_fsid:
		return "fsid", true
	case A_unique_handles:
		return "unique_handles", true
	case A_lease_time:
		return "lease_time", true
	case A_rdattr_error:
		return "rdattr_error", true
	case A_case_insensitive:
		return "case_insensitive", true
	case A_case_preserving:
		return "case_preserving", true
	case A_filehandle:
		return "filehandle", true
	case A_fileid:
		return "fileid", true
	case A_maxname:
		return "maxname", true
	case A_maxread:
		return "maxread", true
	case A_maxwrite:
		return "maxwrite", true
	case A_mode:
		return "mode", true
	case A_no_trunc:
		return "no_trunc", true
	case A_numlinks:
		return "numlinks", true
	case A_owner:
		return "owner", true
	case A_owner_group:
		return "owner_group", true
	case A_rawdev:
		return "rawdev", true
	case A_space_used:
		return "space_used", true
	case A_time_access:
		return "time_access", true
	case A_time_metadata:
		return "time_metadata", true
	case A_time_modify:
		return "time_modify", true
	case A_mounted_on_fileid:
		return "mounted_on_fileid", true
	case A_suppattr_exclcreat:
		return "suppattr_exclcreat", true
	case A_xattr_support:
		return "xattr_support", true
	case A_acl:
		return "acl", true
	case A_aclsupport:
		return "aclsupport", true
	case A_acl_trueform:
		return "acl_trueform", true
	case A_acl_trueform_scope:
		return "acl_trueform_scope", true
	case A_posix_default_acl:
		return "posix_default_acl", true
	case A_posix_access_acl:
		return "posix_access_acl", true
	}

	return fmt.Sprintf("%d", id), false
}

func fileInfoToAttrs(fh []byte, fi vfs.FileInfo, err error, attrsRequest map[int]bool, creds *auth.Creds, sessionID uint64) msg.FAttr4 { //nolint:funlen,gocognit,gocyclo
	idxSupport := map[int]bool{}

	for _, a := range AttrsSupported {
		idxSupport[a] = true
	}

	// attrsFS := vfs.Attributes()

	idxDefault := map[int]bool{}

	for _, a := range AttrsDefaultSet {
		idxDefault[a] = true
	}

	if attrsRequest == nil {
		attrsRequest = idxDefault
	}

	maxID := 0
	for a := range attrsRequest {
		if a > maxID {
			maxID = a
		}
	}

	idxReturn := map[int]bool{}
	for a := range attrsRequest {
		idxReturn[a] = false
	}

	buff := bytes.NewBuffer([]byte{})
	w := xdr.NewEncoder(buff)

	// debugf := func(a int, v interface{}) {
	// 	attrName, _ := GetAttrNameById(a)
	// 	switch a {
	// 	case A_type, A_mode, A_fileid, A_numlinks, A_mounted_on_fileid:
	// 		fallthrough
	// 	default:
	// 		log.Printf("     attr.%s = %v", attrName, v)
	// 	}
	// }

	writeAny := func(a int, target interface{}, sizeExpected int) {
		attrName, _ := GetAttrNameByID(a)
		if size, werr := w.Write(target); werr != nil {
			logger.Logger.Errorf("attr.%s: w.WriteAny: %v", attrName, werr)
		} else if size != sizeExpected {
			logger.Logger.Errorf("attr.%s: w.WriteAny: %d bytes wrote(but expects %d).",
				attrName, size, sizeExpected,
			)
		}
	}

	for a := 0; a <= maxID; a++ {
		requested, found := attrsRequest[a]
		if !found || !requested {
			continue
		}

		supported, found := idxSupport[a]
		if !found || !supported {
			logger.Logger.Warnf("attr requested but not supported: %d", a)
			continue
		}

		attrName, _ := GetAttrNameByID(a)
		// log.Debugf(" - preparing attr: %s", attrName)

		idxReturn[a] = true

		switch a {
		case A_supported_attrs:
			v := bitmap4Encode(idxSupport)
			writeAny(a, v, 4+4*len(v))

		case A_type:
			v := msg.NF4REG

			if fi.IsDir() {
				v = msg.NF4DIR
			} else {
				switch fi.Mode().Type() { //nolint:exhaustive
				case os.ModeDir:
					v = msg.NF4DIR
				case os.ModeSymlink:
					v = msg.NF4LNK
				case os.ModeSocket:
					v = msg.NF4SOCK
				default:
				}
			}

			writeAny(a, v, 4)

		case A_fh_expire_type:
			v := msg.FH4_VOLATILE_ANY | msg.FH4_NOEXPIRE_WITH_OPEN
			writeAny(a, v, 4)

		case A_change:
			// This indicates the client whether the file handle has been modified (e.g. files in the folder have been added/removed)
			// However, the client assumes a global view across all uids, and we serve different views for different uids. So we must
			// enforce a value per uid. We also include the sessionID to force clients to revalidate their file handles.
			changeid := uint64(fi.ModTime().Unix())*uint64(math.MaxUint32) + uint64(creds.UID) + sessionID
			writeAny(a, changeid, 8)

		case A_size:
			size := uint64(fi.Size())
			writeAny(a, size, 8)

		case A_link_support, A_case_preserving:
			writeAny(a, true, 4)

		case A_symlink_support:
			writeAny(a, true, 4)

		case A_named_attr, A_case_insensitive:
			writeAny(a, false, 4)

		case A_fsid:
			fsid := msg.Fsid4{Major: 1, Minor: 1}
			writeAny(a, fsid, 8+8)

		case A_unique_handles:
			writeAny(a, false, 4) // was true

		case A_lease_time:
			ttl := uint32(clients.ClientExpiration.Seconds()) // seconds, rfc7530:5.8.1.11
			writeAny(a, ttl, 4)

		case A_rdattr_error:
			status := msg.Err2Status(err)
			writeAny(a, status, 4)

		case A_aclsupport:
			writeAny(a, uint32(1), 4)

		case A_acl:
			acl := []msg.NfsAce4{{
				Type:       msg.ACE4_ACCESS_ALLOWED_ACE_TYPE,
				Flag:       0,
				AccessMask: msg.ACE4_WRITE_OWNER | msg.ACE4_WRITE_ACL | msg.ACE4_WRITE_DATA | msg.ACE4_READ_ACL | msg.ACE4_READ_DATA,
				Who:        "OWNER@",
			}, {
				Type:       msg.ACE4_ACCESS_ALLOWED_ACE_TYPE,
				Flag:       msg.ACE4_IDENTIFIER_GROUP,
				AccessMask: msg.ACE4_READ_ACL | msg.ACE4_READ_DATA,
				Who:        "GROUP@",
			}, {
				Type:       msg.ACE4_ACCESS_ALLOWED_ACE_TYPE,
				Flag:       0,
				AccessMask: 0,
				Who:        "EVERYONE@",
			}}

			w.Write(acl) //nolint:errcheck

		case A_chown_restricted:
			writeAny(a, true, 4) // TODO: check

		case A_filehandle:
			writeAny(a, fh, 4+len(fh)+xdr.Pad(len(fh)))

		case A_fileid, A_mounted_on_fileid:
			window := fh

			if len(window) < 8 {
				window = append(window, make([]byte, 8-len(window))...)
			}

			var fileid uint64

			for fileid == 0 && len(window) >= 8 {
				fileid = binary.LittleEndian.Uint64(window[len(window)-8:])

				window = window[:len(window)-1]
			}

			if fileid == 0 {
				fileid--
			}

			writeAny(a, fileid, 8)

		case A_maxname:
			writeAny(a, 255, 4) // TODO: check

		case A_maxread, A_maxwrite:
			writeAny(a, uint64(32*1024), 8)

		case A_mode:
			mask := (uint32(1) << 9) - 1
			mode := uint32(fi.Mode()) & mask

			writeAny(a, mode, 4)

		case A_no_trunc:
			writeAny(a, true, 4) // TODO: check

		case A_numlinks:
			n := uint32(fi.NumLinks())

			if n == 0 {
				// Ensure we don't return 0s, must be > 0 for root (otherwise "Stale handle")
				n = 1
			}

			writeAny(a, n, 4)

		case A_owner:
			owner := fmt.Sprintf("%d", fi.Uid())

			writeAny(a, owner, 4+len(owner)+xdr.Pad(len(owner)))

		case A_owner_group:
			group := fmt.Sprintf("%d", fi.Gid())

			writeAny(a, group, 4+len(group)+xdr.Pad(len(group)))

		case A_rawdev:
			v := msg.Specdata4{ /* uint32, uint32 */ }
			writeAny(a, v, 4+4)

		case A_space_used:
			v := uint64(1024*4 + fi.Size())
			writeAny(a, v, 8)

		case A_time_access, A_time_metadata, A_time_modify:
			v := msg.NfsTime4{
				Seconds:  uint64(fi.ModTime().Unix()),
				NSeconds: uint32(fi.ModTime().Nanosecond()),
			}
			writeAny(a, v, 8+4)

		case A_suppattr_exclcreat:
			v := bitmap4Encode(idxSupport)
			writeAny(a, v, 4+4*len(v))

		case A_xattr_support:
			writeAny(a, uint32(1), 4)

		case A_acl_trueform:
			writeAny(a, uint32(2), 4) // ACL_MODEL_POSIX_DRAFT

		case A_acl_trueform_scope:
			writeAny(a, uint32(1), 4) // ACL_SCOPE_FILE_OBJECT

		case A_posix_default_acl:
			acl := []msg.NfsPosixAce4{}

			w.Write(acl) //nolint:errcheck

		case A_posix_access_acl:
			acl := []msg.NfsPosixAce4{
				{
					Tag:  1,
					Perm: 7,
				},
				{
					Tag:  3,
					Perm: 7,
				},
				{
					Tag:  6,
					Perm: 0,
				},
			}

			w.Write(acl) //nolint:errcheck

		default:
			logger.Logger.Warnf("(!)requested attr %s not handled!", attrName)
		}
	}

	attrMask := bitmap4Encode(idxReturn)
	dat := buff.Bytes()

	return msg.FAttr4{
		Mask: attrMask,
		Vals: dat,
	}
}

type Attr struct {
	SupportedAttrs    []uint32 // bitmap4
	Type              uint32
	FhExpireType      uint32
	Change            uint64
	Size              *uint64
	LinkSupport       bool
	SymlinkSupport    bool
	NamedAttr         bool
	Fsid              *msg.Fsid4
	UniqueHandles     bool
	LeaseTime         uint32
	RdattrError       uint32
	FileHandle        []byte
	FileID            uint64
	Mode              *uint32
	NumLinks          uint32
	Owner             string
	OwnerGroup        string
	Rawdev            *msg.Specdata4
	SpaceUsed         uint64
	TimeAccess        *msg.NfsTime4
	TimeMetadata      *msg.NfsTime4
	TimeModify        *msg.NfsTime4
	MountedOnFileID   uint64
	SuppAttrExclCreat []uint32
	XAttrSupport      bool
	ACLSupport        bool
	ACL               []msg.NfsAce4
	PosixACL          []msg.NfsPosixAce4
	PosixDefaultACL   []msg.NfsPosixAce4
}

func decodeFAttrs4(attr msg.FAttr4) (*Attr, error) { //nolint:funlen,gocognit,gocyclo
	decAttr := &Attr{}

	idx := bitmap4Decode(attr.Mask)

	maxID := 0

	for aid, on := range idx {
		if on {
			if aid > maxID {
				maxID = aid
			}
		}
	}

	ar := xdr.NewDecoder(bytes.NewBuffer(attr.Vals))

	for i := 0; i <= maxID; i++ {
		if on, found := idx[i]; found && on { //nolint:nestif
			switch i {
			case A_supported_attrs:
				bm4 := []uint32{}

				if _, err := ar.Read(&bm4); err != nil {
					return nil, err
				}

				decAttr.SupportedAttrs = bm4

			case A_type:
				v := uint32(0)

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.Type = v

			case A_fh_expire_type:
				v := uint32(0)

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.FhExpireType = v

			case A_change:
				v := uint64(0)

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.Change = v

			case A_size:
				v := uint64(0)

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.Size = &v

			case A_link_support:
				v := false

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.LinkSupport = v

			case A_symlink_support:
				v := false

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.SymlinkSupport = v

			case A_named_attr:
				v := false

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.NamedAttr = v

			case A_fsid:
				v := msg.Fsid4{}

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.Fsid = &v

			case A_unique_handles:
				v := false

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.UniqueHandles = v

			case A_lease_time:
				v := uint32(0)

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.LeaseTime = v

			case A_rdattr_error:
				v := uint32(0)

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.RdattrError = v

			case A_filehandle:
				v := []byte{}

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.FileHandle = v

			case A_fileid:
				v := uint64(0)

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.FileID = v

			case A_mode: // v4.1
				v := uint32(0)

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.Mode = &v

			case A_numlinks:
				v := uint32(0)

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.NumLinks = v

			case A_owner:
				v := ""

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.Owner = v

			case A_owner_group:
				v := ""

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.OwnerGroup = v

			case A_rawdev:
				v := msg.Specdata4{}

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.Rawdev = &v

			case A_space_used:
				v := uint64(0)

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.SpaceUsed = v

			case A_time_access, A_time_metadata, A_time_modify:
				v := msg.NfsTime4{}

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				switch i {
				case A_time_access:
					decAttr.TimeAccess = &v
				case A_time_metadata:
					decAttr.TimeMetadata = &v
				case A_time_modify:
					decAttr.TimeModify = &v
				}

			case A_time_modify_set:
				ok, err := ar.Bool()
				if err != nil {
					return nil, err
				}

				if ok {
					decAttr.TimeModify = new(msg.NfsTime4)

					if _, err := ar.Read(&decAttr.TimeModify); err != nil {
						return nil, err
					}
				} else {
					now := clock.Now()

					decAttr.TimeModify = &msg.NfsTime4{
						Seconds:  uint64(now.Unix()),
						NSeconds: uint32(now.Nanosecond()),
					}
				}

			case A_mounted_on_fileid:
				v := uint64(0)

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.MountedOnFileID = v

			case A_suppattr_exclcreat: // v4.1
				v := []uint32{}

				if _, err := ar.Read(&v); err != nil {
					return nil, err
				}

				decAttr.SuppAttrExclCreat = v

			case A_xattr_support:
				supp, err := ar.Bool()
				if err != nil {
					return nil, err
				}

				decAttr.XAttrSupport = supp

			case A_aclsupport:
				supp, err := ar.Bool()
				if err != nil {
					return nil, err
				}

				decAttr.ACLSupport = supp

			case A_acl:
				if _, err := ar.Read(&decAttr.ACL); err != nil {
					return nil, err
				}

			case A_posix_default_acl:
				if _, err := ar.Read(&decAttr.PosixDefaultACL); err != nil {
					return nil, err
				}

			case A_posix_access_acl:
				if _, err := ar.Read(&decAttr.PosixACL); err != nil {
					return nil, err
				}
			}
		}
	}

	return decAttr, nil
}
