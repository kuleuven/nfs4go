//nolint:dupl,staticcheck
package msg

import (
	"fmt"
)

const (
	PROC4_VOID     = uint32(0)
	PROC4_COMPOUND = uint32(1)
)

const (
	OP4_ACCESS              = uint32(3)
	OP4_CLOSE               = uint32(4)
	OP4_COMMIT              = uint32(5)
	OP4_CREATE              = uint32(6)
	OP4_DELEGPURGE          = uint32(7)
	OP4_DELEGRETURN         = uint32(8)
	OP4_GETATTR             = uint32(9)
	OP4_GETFH               = uint32(10)
	OP4_LINK                = uint32(11)
	OP4_LOCK                = uint32(12)
	OP4_LOCKT               = uint32(13)
	OP4_LOCKU               = uint32(14)
	OP4_LOOKUP              = uint32(15)
	OP4_LOOKUPP             = uint32(16)
	OP4_NVERIFY             = uint32(17)
	OP4_OPEN                = uint32(18)
	OP4_OPENATTR            = uint32(19)
	OP4_OPEN_CONFIRM        = uint32(20)
	OP4_OPEN_DOWNGRADE      = uint32(21)
	OP4_PUTFH               = uint32(22)
	OP4_PUTPUBFH            = uint32(23)
	OP4_PUTROOTFH           = uint32(24)
	OP4_READ                = uint32(25)
	OP4_READDIR             = uint32(26)
	OP4_READLINK            = uint32(27)
	OP4_REMOVE              = uint32(28)
	OP4_RENAME              = uint32(29)
	OP4_RENEW               = uint32(30)
	OP4_RESTOREFH           = uint32(31)
	OP4_SAVEFH              = uint32(32)
	OP4_SECINFO             = uint32(33)
	OP4_SETATTR             = uint32(34)
	OP4_SETCLIENTID         = uint32(35)
	OP4_SETCLIENTID_CONFIRM = uint32(36)
	OP4_VERIFY              = uint32(37)
	OP4_WRITE               = uint32(38)
	OP4_RELEASE_LOCKOWNER   = uint32(39)

	OP4_BACKCHANNEL_CTL      = uint32(40)
	OP4_BIND_CONN_TO_SESSION = uint32(41)
	OP4_EXCHANGE_ID          = uint32(42)
	OP4_CREATE_SESSION       = uint32(43)
	OP4_DESTROY_SESSION      = uint32(44)
	OP4_FREE_STATEID         = uint32(45)
	OP4_GET_DIR_DELEGATION   = uint32(46)
	OP4_GETDEVICEINFO        = uint32(47)
	OP4_GETDEVICELIST        = uint32(48)
	OP4_LAYOUTCOMMIT         = uint32(49)
	OP4_LAYOUTGET            = uint32(50)
	OP4_LAYOUTRETURN         = uint32(51)
	OP4_SECINFO_NO_NAME      = uint32(52)
	OP4_SEQUENCE             = uint32(53)
	OP4_SET_SSV              = uint32(54)
	OP4_TEST_STATEID         = uint32(55)
	OP4_WANT_DELEGATION      = uint32(56)
	OP4_DESTROY_CLIENTID     = uint32(57)
	OP4_RECLAIM_COMPLETE     = uint32(58)

	OP4_ALLOCATE       = uint32(59)
	OP4_COPY           = uint32(60)
	OP4_COPY_NOTIFY    = uint32(61)
	OP4_DEALLOCATE     = uint32(62)
	OP4_IO_ADVISE      = uint32(63)
	OP4_LAYOUTERROR    = uint32(64)
	OP4_LAYOUTSTATS    = uint32(65)
	OP4_OFFLOAD_CANCEL = uint32(66)
	OP4_OFFLOAD_STATUS = uint32(67)
	OP4_READ_PLUS      = uint32(68)
	OP4_SEEK           = uint32(69)
	OP4_WRITE_SAME     = uint32(70)
	OP4_CLONE          = uint32(71)

	OP4_GETXATTR    = uint32(72)
	OP4_SETXATTR    = uint32(73)
	OP4_LISTXATTRS  = uint32(74)
	OP4_REMOVEXATTR = uint32(75)

	OP4_ILLEGAL = uint32(10044)
)

const (
	PROC4_CB_NULL     = uint32(0)
	PROC4_CB_COMPOUND = uint32(1)
)

const (
	OP4_CB_GETATTR = uint32(3)
	OP4_CB_RECALL  = uint32(4)
	OP4_CB_ILLEGAL = uint32(10044)
)

func Proc4Name(proc uint32) string { //nolint:funlen,gocyclo
	switch proc {
	case PROC4_VOID:
		return "void"
	case PROC4_COMPOUND:
		return "compound"
	case OP4_ACCESS:
		return "access"
	case OP4_CLOSE:
		return "close"
	case OP4_COMMIT:
		return "commit"
	case OP4_CREATE:
		return "create"
	case OP4_DELEGPURGE:
		return "delegpurge"
	case OP4_DELEGRETURN:
		return "delegreturn"
	case OP4_GETATTR:
		return "getattr"
	case OP4_GETFH:
		return "getfh"
	case OP4_LINK:
		return "link"
	case OP4_LOCK:
		return "lock"
	case OP4_LOCKT:
		return "lockt"
	case OP4_LOCKU:
		return "locku"
	case OP4_LOOKUP:
		return "lookup"
	case OP4_LOOKUPP:
		return "lookupp"
	case OP4_NVERIFY:
		return "nverify"
	case OP4_OPEN:
		return "open"
	case OP4_OPENATTR:
		return "openattr"
	case OP4_OPEN_CONFIRM:
		return "open_confirm"
	case OP4_OPEN_DOWNGRADE:
		return "open_downgrade"
	case OP4_PUTFH:
		return "putfh"
	case OP4_PUTPUBFH:
		return "putpubfh"
	case OP4_PUTROOTFH:
		return "putrootfh"
	case OP4_READ:
		return "read"
	case OP4_READDIR:
		return "readdir"
	case OP4_READLINK:
		return "readlink"
	case OP4_REMOVE:
		return "remove"
	case OP4_RENAME:
		return "rename"
	case OP4_RENEW:
		return "renew"
	case OP4_RESTOREFH:
		return "restorefh"
	case OP4_SAVEFH:
		return "savefh"
	case OP4_SECINFO:
		return "secinfo"
	case OP4_SETATTR:
		return "setattr"
	case OP4_SETCLIENTID:
		return "setclientid"
	case OP4_SETCLIENTID_CONFIRM:
		return "setclientid_confirm"
	case OP4_VERIFY:
		return "verify"
	case OP4_WRITE:
		return "write"
	case OP4_RELEASE_LOCKOWNER:
		return "release_lockowner"
	case OP4_BACKCHANNEL_CTL:
		return "backchannel_ctl"
	case OP4_BIND_CONN_TO_SESSION:
		return "bind_conn_to_session"
	case OP4_EXCHANGE_ID:
		return "exchange_id"
	case OP4_CREATE_SESSION:
		return "create_session"
	case OP4_DESTROY_SESSION:
		return "destroy_session"
	case OP4_FREE_STATEID:
		return "free_stateid"
	case OP4_GET_DIR_DELEGATION:
		return "get_dir_delegation"
	case OP4_GETDEVICEINFO:
		return "getdeviceinfo"
	case OP4_GETDEVICELIST:
		return "getdevicelist"
	case OP4_LAYOUTCOMMIT:
		return "layoutcommit"
	case OP4_LAYOUTGET:
		return "layoutget"
	case OP4_LAYOUTRETURN:
		return "layoutreturn"
	case OP4_SECINFO_NO_NAME:
		return "secinfo_no_name"
	case OP4_SEQUENCE:
		return "sequence"
	case OP4_TEST_STATEID:
		return "test_stateid"
	case OP4_WANT_DELEGATION:
		return "want_delegation"
	case OP4_DESTROY_CLIENTID:
		return "destroy_clientid"
	case OP4_RECLAIM_COMPLETE:
		return "reclaim_complete"
	case OP4_ILLEGAL:
		return "illegal"
	}

	return fmt.Sprintf("%d", proc)
}

const (
	NF4REG       = uint32(1)
	NF4DIR       = uint32(2)
	NF4BLK       = uint32(3)
	NF4CHR       = uint32(4)
	NF4LNK       = uint32(5)
	NF4SOCK      = uint32(6)
	NF4FIFO      = uint32(7)
	NF4ATTRDIR   = uint32(8)
	NF4NAMEDATTR = uint32(9)
)

const (
	FH4_PERSISTENT         = uint32(0x00000000)
	FH4_NOEXPIRE_WITH_OPEN = uint32(0x00000001)
	FH4_VOLATILE_ANY       = uint32(0x00000002)
	FH4_VOL_MIGRATION      = uint32(0x00000004)
	FH4_VOL_RENAME         = uint32(0x00000008)
)

const (
	ACCESS4_READ    = uint32(0x00000001)
	ACCESS4_LOOKUP  = uint32(0x00000002)
	ACCESS4_MODIFY  = uint32(0x00000004)
	ACCESS4_EXTEND  = uint32(0x00000008)
	ACCESS4_DELETE  = uint32(0x00000010)
	ACCESS4_EXECUTE = uint32(0x00000020)

	ACCESS4_XAREAD  = uint32(0x00000040)
	ACCESS4_XAWRITE = uint32(0x00000080)
	ACCESS4_XALIST  = uint32(0x00000100)
)

const (
	ACE4_ACCESS_ALLOWED_ACE_TYPE = uint32(0x00000000)
	ACE4_ACCESS_DENIED_ACE_TYPE  = uint32(0x00000001)
	ACE4_SYSTEM_AUDIT_ACE_TYPE   = uint32(0x00000002)
	ACE4_SYSTEM_ALARM_ACE_TYPE   = uint32(0x00000003)
)

const (
	ACE4_FILE_INHERIT_ACE           = uint32(0x00000001) //	ACE applies to files in the directory.
	ACE4_DIRECTORY_INHERIT_ACE      = uint32(0x00000002) // ACE applies to subdirectories.
	ACE4_NO_PROPAGATE_INHERIT_ACE   = uint32(0x00000004) // ACE is inherited by child objects but not further propagated.
	ACE4_INHERIT_ONLY_ACE           = uint32(0x00000008) //	ACE does not apply to the directory itself, only to children.
	ACE4_SUCCESSFUL_ACCESS_ACE_FLAG = uint32(0x00000010) // ACE is used for logging successful access (audit entries).
	ACE4_FAILED_ACCESS_ACE_FLAG     = uint32(0x00000020) //	ACE is used for logging failed access (audit entries).
	ACE4_IDENTIFIER_GROUP           = uint32(0x00000040) //	Who field specifies a group rather than a user.
)

const (
	ACE4_READ_DATA      uint32 = 0x00000001     // Read file data or list directory contents
	ACE4_LIST_DIRECTORY uint32 = ACE4_READ_DATA // Alias for directories

	ACE4_WRITE_DATA uint32 = 0x00000002      // Write file data or create files in a directory
	ACE4_ADD_FILE   uint32 = ACE4_WRITE_DATA // Alias for directories

	ACE4_APPEND_DATA      uint32 = 0x00000004       // Append to a file or create subdirectories
	ACE4_ADD_SUBDIRECTORY uint32 = ACE4_APPEND_DATA // Alias for directories

	ACE4_READ_NAMED_ATTRS  uint32 = 0x00000008 // Read named attributes
	ACE4_WRITE_NAMED_ATTRS uint32 = 0x00000010 // Write named attributes

	ACE4_EXECUTE uint32 = 0x00000020 // Execute a file or search a directory

	ACE4_DELETE_CHILD uint32 = 0x00000040 // Delete a file or directory within a directory

	ACE4_READ_ATTRIBUTES  uint32 = 0x00000080 // Read standard attributes (size, timestamps, etc.)
	ACE4_WRITE_ATTRIBUTES uint32 = 0x00000100 // Modify standard attributes (timestamps, etc.)

	ACE4_DELETE      uint32 = 0x00010000 // Delete the file or directory
	ACE4_READ_ACL    uint32 = 0x00020000 // Read the ACL
	ACE4_WRITE_ACL   uint32 = 0x00040000 // Modify the ACL
	ACE4_WRITE_OWNER uint32 = 0x00080000 // Change file ownership
	ACE4_SYNCHRONIZE uint32 = 0x00100000 // Synchronize file access for concurrent access
)

const (
	OPEN4_SHARE_ACCESS_READ  = 0x00000001
	OPEN4_SHARE_ACCESS_WRITE = 0x00000002
	OPEN4_SHARE_ACCESS_BOTH  = 0x00000003
)

const OPEN4_RESULT_PRESERVE_UNLINKED = 0x00000008

const (
	EXCHGID4_FLAG_BIND_PRINC_STATEID  = 0x00000100
	EXCHGID4_FLAG_UPD_CONFIRMED_REC_A = 0x40000000
	EXCHGID4_FLAG_CONFIRMED_R         = 0x80000000
	EXCHGID4_FLAG_USE_NON_PNFS        = 0x00010000
)

const (
	CREATE_SESSION4_FLAG_PERSIST        = 0x00000001
	CREATE_SESSION4_FLAG_CONN_BACK_CHAN = 0x00000002
	CREATE_SESSION4_FLAG_CONN_RDMA      = 0x00000004
)

const (
	WND4_NOT_SUPP_FTYPE = 3
)
