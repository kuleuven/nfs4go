//go:generate go run ../xdr/generate/generate.go -- structs.go
//nolint:staticcheck
package msg

type Fsid4 struct {
	Major uint64
	Minor uint64
}

type Specdata4 struct {
	D1 uint32
	D2 uint32
}

type FAttr4 struct {
	Mask []uint32 // bitmap4
	Vals []byte
}

type GETATTR4args struct {
	AttrRequest []uint32 // bitmap4
}

type GETATTR4resok struct {
	Attr FAttr4
}

type NfsClientId4 struct {
	Verifier uint64 // type: verifier4
	Id       []byte
}

type ClientAddr4 struct {
	NetId string
	Addr  string
}

type CbClient4 struct {
	CbProgram  uint32
	CbLocation ClientAddr4
}

type SETCLIENTID4args struct {
	Client        NfsClientId4
	Callback      CbClient4
	CallbackIdent uint32
}

type SETCLIENTID4resok struct {
	ClientId           uint64 // type: clientid4
	SetClientIdConfirm uint64 // type: verifier4
}

type SETCLIENTID_CONFIRM4args struct {
	ClientId uint64
	Verifier uint64
}

type PUTFH4args struct {
	Fh []byte // nfs_fh4
}

type LOOKUP4args struct {
	ObjName string
}

type GETFH4args struct{}

type GETFH4resok struct {
	Fh []byte // nfs_fh4
}

type ACCESS4args struct {
	Access uint32
}

type ACCESS4resok struct {
	Supported uint32
	Access    uint32
}

type READDIR4args struct {
	Cookie      uint64
	CookieVerf  uint64   // opaque [8]
	DirCount    uint32   // max size of bytes of directory info.
	MaxCount    uint32   // max size of entire response(xdr header + READDIR4resok).
	AttrRequest []uint32 // bitmap4
}

type Entry4 struct {
	Cookie uint64
	Name   string
	Attrs  FAttr4
	Next   *Entry4
}

type DirList4 struct {
	Entries *Entry4
	Eof     bool
}

type READDIR4resok struct {
	CookieVerf uint64
	Reply      DirList4
}

type SECINFO4args struct {
	Name string
}

const (
	// RPCSEC_GSS: https://datatracker.ietf.org/doc/html/rfc2203
	RPCSEC_GSS = uint32(6)
)

const (
	RPC_GSS_SVC_NONE      = uint32(1)
	RPC_GSS_SVC_INTEGRITY = uint32(2)
	RPC_GSS_SVC_PRIVACY   = uint32(3)
)

type RPCSecGssInfo struct {
	Oid     string
	Qop     uint32
	Service uint32 // RPC_GSS_SVC_*
}

type Secinfo4 struct {
	Flavor uint32
	// FlavorInfo *RPCSecGssInfo // non-nil if flavor == RPCSEC_GSS
}

type SECINFO4resok struct {
	Items []Secinfo4
}

type RENEW4args struct {
	ClientId uint64
}

type ClientOwner4 struct {
	Verifier uint64 // opaque [8]
	OwnerId  []byte
}

const (
	SP4_NONE      = uint32(0)
	SP4_MACH_CRED = uint32(1)
	SP4_SSV       = uint32(2)
)

type StateProtectOps4 struct {
	MustEnforce []uint32 // bitmap4
	MustAllow   []uint32 // bitmap4
}

/*
type StateProtect4R struct {
	How     uint32
	MachOps *StateProtectOps4 // non-nil if how == SP4_MACH_CRED
	SsvInfo *SsvProtInfo4     // non-nil if how == SP4_SSV
}*/

type ServerOwner4 struct {
	MinorId uint64
	MajorId string
}

type NfsTime4 struct {
	Seconds  uint64
	NSeconds uint32
}

type NfsImplId4 struct {
	Domain string // case-insensitive
	Name   string // case-sensitive
	Date   NfsTime4
}

type ChangeInfo4 struct {
	Atomic bool
	Before uint64
	After  uint64
}

type CREATE4args struct {
	Type        CREATE4type
	ObjName     string
	CreateAttrs FAttr4
}

type CREATE4type struct {
	ObjType  uint32 `xdr:"union"`
	Void     Void
	VoidReg  Void
	VoidDir  Void
	BlkData  Specdata4
	ChrData  Specdata4
	LinkData string
	VoidSock Void
	VoidFifo Void
}

type CREATE4resok struct {
	CInfo   ChangeInfo4
	AttrSet []uint32 // bitmap4
}

type OpenOwner4 struct {
	ClientId uint64
	Owner    string
}

const (
	OPEN4_NOCREATE = uint32(0)
	OPEN4_CREATE   = uint32(1)
)

type OpenHow4 struct {
	How   uint32     `xdr:"union"` // OPEN4_NOCREATE | OPEN4_CREATE
	Void  Void       // if OpenHow == OPEN4_NOCREATE
	Claim CreateHow4 // if OpenHow == OPEN4_CREATE
}

const (
	UNCHECKED4   = uint32(0)
	GUARDED4     = uint32(1)
	EXCLUSIVE4   = uint32(2)
	EXCLUSIVE4_1 = uint32(3)
)

type CreateHow4 struct {
	CreateMode           uint32         `xdr:"union"` // UNCHECKED4 | GUARDED4 | EXCLUSIVE4
	CreateAttrsUnchecked FAttr4         // if CreateMode == UNCHECKED4
	CreateAttrsGuarded   FAttr4         // if CreateMode == GUARDED4
	CreateVerf           uint64         // if CreateMode == EXCLUSIVE4
	CreateVerf41         CreateVerfAttr // if CreateMode == EXCLUSIVE4_1
}

type CreateVerfAttr struct {
	Verf  uint64
	Attrs FAttr4
}

const (
	OPEN_DELEGATE_NONE     = uint32(0)
	OPEN_DELEGATE_READ     = uint32(1)
	OPEN_DELEGATE_WRITE    = uint32(2)
	OPEN_DELEGATE_NONE_EXT = uint32(3)
)

type StateId4 struct {
	SeqId uint32
	Other [3]uint32 // 64 bit?
}

type OpenClaimDelegateCur4 struct {
	DelegateStateId StateId4
	File            string
}

const (
	CLAIM_NULL          = uint32(0)
	CLAIM_PREVIOUS      = uint32(1)
	CLAIM_DELEGATE_CUR  = uint32(2)
	CLAIM_DELEGATE_PREV = uint32(3)

	CLAIM_FH            = uint32(4) /* new to v4.1 */
	CLAIM_DELEG_CUR_FH  = uint32(5) /* new to v4.1 */
	CLAIM_DELEG_PREV_FH = uint32(6) /* new to v4.1 */
)

type OpenClaim4 struct {
	Claim uint32 `xdr:"union"` // CLAIM_*

	File              string                // if Claim == CLAIM_NULL
	DelegateType      uint32                // OPEN_DELEGATE_*, if Claim == CLAIM_PREVIOUS
	DelegateCurInfo   OpenClaimDelegateCur4 // if Claim == CLAIM_DELEGATE_CUR
	FileDelegatePrev  string                // if Claim == CLAIM_DELEGATE_PREV
	VoidClaimFH       Void                  // if Claim == CLAIM_FH
	DelegCurFHStateID StateId4              // if Claim == CLAIM_DELEG_CUR_FH
	VoidDelegPrevFH   Void                  // if Claim == CLAIM_DELEG_PREV_FH
}

type OPEN4args struct {
	SeqID       uint32
	ShareAccess uint32
	ShareDeny   uint32
	Owner       OpenOwner4
	OpenHow     OpenHow4
	OpenClaim   OpenClaim4
}

type OPENDG4args struct {
	OpenStateId StateId4
	SeqId       uint32
	ShareAccess uint32
	ShareDeny   uint32
}

type NfsAce4 struct {
	Type       uint32
	Flag       uint32
	AccessMask uint32
	Who        string
}

type NfsPosixAce4 struct {
	Tag  uint32
	Perm uint32
	Who  string
}

type NfsModifiedLimit4 struct {
	NumBlocks     uint32
	BytesPerBlock uint32
}

type NfsSpaceLimit4 struct {
	LimitBy uint32

	FileSize  uint64             // if LimitBy == NFS_LIMIT_SIZE
	ModBlocks *NfsModifiedLimit4 // if LimitBy == NFS_LIMIT_BLOCKS
}

type OpenReadDelegation4 struct {
	StateId     StateId4
	Recall      bool
	Permissions NfsAce4
}

type OpenWriteDelegation4 struct {
	StateId     StateId4
	Recall      bool
	SpaceLimit  NfsSpaceLimit4
	Permissions NfsAce4
}

type OpenDelegation4 struct {
	Type            uint32 `xdr:"union"`
	DelegateNone    Void
	DelegateRead    OpenReadDelegation4
	DelegateWrite   OpenWriteDelegation4
	DelegateNoneWhy OpenDelegationNoneWhy4
}

type OpenDelegationNoneWhy4 struct {
	Why uint32
}

const (
	OPEN4_RESULT_CONFIRM        = uint32(0x00000002)
	OPEN4_RESULT_LOCKTYPE_POSIX = uint32(0x00000004)
)

type OPEN4resok struct {
	StateId    StateId4
	CInfo      ChangeInfo4
	Rflags     uint32 // OPEN4_RESULT_CONFIRM | OPEN4_RESULT_LOCKTYPE_POSIX
	AttrSet    []uint32
	Delegation OpenDelegation4
}

type OpenDelegationNone4 struct {
	Mode uint32
}

type CLOSE4args struct {
	SeqId       uint32
	OpenStateId StateId4
}

type SETATTR4args struct {
	StateId StateId4
	Attrs   FAttr4
}

type REMOVE4args struct {
	Target string
}

type REMOVE4resok struct {
	CInfo ChangeInfo4
}

type COMMIT4args struct {
	Offset uint64
	Count  uint32
}

type COMMIT4resok struct {
	Verifier uint64
}

/*

enum stable_how4 {
    UNSTABLE4       = 0,
    DATA_SYNC4      = 1,
    FILE_SYNC4      = 2
};

struct WRITE4args {
    stateid4        stateid;
    offset4         offset;
    stable_how4     stable;
    opaque          data<>;
};

*/

const (
	UNSTABLE4  = uint32(0)
	DATA_SYNC4 = uint32(1)
	FILE_SYNC4 = uint32(2)
)

type WRITE4args struct {
	StateId StateId4
	Offset  uint64
	Stable  uint32 // USTABLE4 | DATA_SYNC4 | FILE_SYNC4
	Data    []byte
}

type WRITE4resok struct {
	Count     uint32
	Committed uint32 // USTABLE4 | DATA_SYNC4 | FILE_SYNC4
	WriteVerf uint64
}

type READ4args struct {
	StateId StateId4
	Offset  uint64
	Count   uint32
}

type READ4resok struct {
	Eof  bool
	Data []byte
}

type RENAME4args struct {
	OldName string
	NewName string
}

type RENAME4resok struct {
	SourceCInfo ChangeInfo4
	TargetCInfo ChangeInfo4
}

type LINK4args struct {
	NewName string
}

type LINK4resok struct {
	CInfo ChangeInfo4
}

type READLINK4resok struct {
	Link string
}

type GETXATTR4args struct {
	Name string
}

type GETXATTR4resok struct {
	Value []byte
}

const (
	SETXATTR4_EITHER  = uint32(0)
	SETXATTR4_CREATE  = uint32(1)
	SETXATTR4_REPLACE = uint32(2)
)

type SETXATTR4args struct {
	Option uint32
	Name   string
	Value  []byte
}

type SETXATTR4resok struct {
	CInfo ChangeInfo4
}

type LISTXATTRS4args struct {
	Cookie   uint64
	MaxCount uint32
}

type LISTXATTRS4resok struct {
	Cookie uint64
	Names  []string
	EOF    bool
}

type REMOVEXATTR4args struct {
	Name string
}

type REMOVEXATTR4resok struct {
	CInfo ChangeInfo4
}

type EXCHANGE_ID4args struct {
	ClientOwner  ClientOwner4
	Flags        uint32
	StateProtect StateProtect4
	ClientImplId *NfsImplId4
}

type EXCHANGE_ID4resok struct {
	ClientID     uint64
	SequenceID   uint32
	Flags        uint32
	StateProtect StateProtect4
	ServerOwner  ServerOwner4
	ServerScope  []byte
	ServerImplId *NfsImplId4
}

type Void struct{}

type StateProtect4 struct {
	How     uint32 `xdr:"union"`
	Void    Void
	MachOps StateProtectOps4 // non-nil if how == SP4_MACH_CRED
	SsvInfo SsvSpParams4     // non-nil if how == SP4_SSV
}

type SsvSpParams4 struct {
	Ops           StateProtectOps4
	HashAlgs      []string
	EncrAlgs      []string
	Window        uint32
	NumGssHandles uint32
}

type SsvProtInfo4 struct {
	Ops     StateProtectOps4
	HashAlg uint32
	EncrAlg uint32
	SsvLen  uint32
	Window  uint32
	Handles []string
}

type CREATE_SESSION4args struct {
	ClientID      uint64
	SequenceID    uint32
	Flags         uint32
	ForeChanAttrs ChannelAttrs4
	BackChanAttrs ChannelAttrs4
	CbProgram     uint32
	SecParms      []byte
}

type Creds struct {
	ExpirationValue  uint32
	Hostname         string
	UID              uint32
	GID              uint32
	AdditionalGroups []uint32
}

type ChannelAttrs4 struct {
	HeaderPadSize         uint32
	MaxRequestSize        uint32
	MaxResponseSize       uint32
	MaxResponseSizeCached uint32
	MaxOperations         uint32
	MaxRequests           uint32
	RdmaIrd               []byte
}

type CREATE_SESSION4resok struct {
	SessionID     [16]byte
	SequenceID    uint32
	Flags         uint32
	ForeChanAttrs ChannelAttrs4
	BackChanAttrs ChannelAttrs4
}

type SEQUENCE4args struct {
	SessionID     [16]byte
	SequenceID    uint32
	SlotID        uint32
	SlotIDHighest uint32
	CacheThis     bool
}

type SEQUENCE4resok struct {
	SessionID           [16]byte
	SequenceID          uint32
	SlotID              uint32
	SlotIDHighest       uint32
	SlotIDHighestTarget uint32
	Flags               uint32
}
