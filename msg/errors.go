//nolint:dupl,staticcheck
package msg

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/sirupsen/logrus"
)

const (
	NFS4_OK                     = uint32(0)     /* everything is okay       */
	NFS4ERR_PERM                = uint32(1)     /* caller not privileged    */
	NFS4ERR_NOENT               = uint32(2)     /* no such file/directory   */
	NFS4ERR_IO                  = uint32(5)     /* hard I/O error           */
	NFS4ERR_NXIO                = uint32(6)     /* no such device           */
	NFS4ERR_ACCESS              = uint32(13)    /* access denied            */
	NFS4ERR_EXIST               = uint32(17)    /* file already exists      */
	NFS4ERR_XDEV                = uint32(18)    /* different file systems   */
	NFS4ERR_NOTDIR              = uint32(20)    /* should be a directory    */
	NFS4ERR_ISDIR               = uint32(21)    /* should not be directory  */
	NFS4ERR_INVAL               = uint32(22)    /* invalid argument         */
	NFS4ERR_FBIG                = uint32(27)    /* file exceeds server max  */
	NFS4ERR_NOSPC               = uint32(28)    /* no space on file system  */
	NFS4ERR_ROFS                = uint32(30)    /* read-only file system    */
	NFS4ERR_MLINK               = uint32(31)    /* too many hard links      */
	NFS4ERR_NAMETOOLONG         = uint32(63)    /* name exceeds server max  */
	NFS4ERR_NOTEMPTY            = uint32(66)    /* directory not empty      */
	NFS4ERR_DQUOT               = uint32(69)    /* hard quota limit reached */
	NFS4ERR_STALE               = uint32(70)    /* file no longer exists    */
	NFS4ERR_BADHANDLE           = uint32(10001) /* Illegal filehandle       */
	NFS4ERR_BAD_COOKIE          = uint32(10003) /* READDIR cookie is stale  */
	NFS4ERR_NOTSUPP             = uint32(10004) /* operation not supported  */
	NFS4ERR_TOOSMALL            = uint32(10005) /* response limit exceeded  */
	NFS4ERR_SERVERFAULT         = uint32(10006) /* undefined server error   */
	NFS4ERR_BADTYPE             = uint32(10007) /* type invalid for CREATE  */
	NFS4ERR_DELAY               = uint32(10008) /* file "busy" - retry      */
	NFS4ERR_SAME                = uint32(10009) /* nverify says attrs same  */
	NFS4ERR_DENIED              = uint32(10010) /* lock unavailable         */
	NFS4ERR_EXPIRED             = uint32(10011) /* lock lease expired       */
	NFS4ERR_LOCKED              = uint32(10012) /* I/O failed due to lock   */
	NFS4ERR_GRACE               = uint32(10013) /* in grace period          */
	NFS4ERR_FHEXPIRED           = uint32(10014) /* filehandle expired       */
	NFS4ERR_SHARE_DENIED        = uint32(10015) /* share reserve denied     */
	NFS4ERR_WRONGSEC            = uint32(10016) /* wrong security flavor    */
	NFS4ERR_CLID_INUSE          = uint32(10017) /* clientid in use          */
	NFS4ERR_RESOURCE            = uint32(10018) /* resource exhaustion      */
	NFS4ERR_MOVED               = uint32(10019) /* file system relocated    */
	NFS4ERR_NOFILEHANDLE        = uint32(10020) /* current FH is not set    */
	NFS4ERR_MINOR_VERS_MISMATCH = uint32(10021) /* minor vers not supp */
	NFS4ERR_STALE_CLIENTID      = uint32(10022) /* server has rebooted      */
	NFS4ERR_STALE_STATEID       = uint32(10023) /* server has rebooted      */
	NFS4ERR_OLD_STATEID         = uint32(10024) /* state is out of sync     */
	NFS4ERR_BAD_STATEID         = uint32(10025) /* incorrect stateid        */
	NFS4ERR_BAD_SEQID           = uint32(10026) /* request is out of seq.   */
	NFS4ERR_NOT_SAME            = uint32(10027) /* verify - attrs not same  */
	NFS4ERR_LOCK_RANGE          = uint32(10028) /* lock range not supported */
	NFS4ERR_SYMLINK             = uint32(10029) /* should be file/directory */
	NFS4ERR_RESTOREFH           = uint32(10030) /* no saved filehandle      */
	NFS4ERR_LEASE_MOVED         = uint32(10031) /* some file system moved   */
	NFS4ERR_ATTRNOTSUPP         = uint32(10032) /* recommended attr not sup */
	NFS4ERR_NO_GRACE            = uint32(10033) /* reclaim outside of grace */
	NFS4ERR_RECLAIM_BAD         = uint32(10034) /* reclaim error at server  */
	NFS4ERR_RECLAIM_CONFLICT    = uint32(10035) /* conflict on reclaim    */
	NFS4ERR_BADXDR              = uint32(10036) /* XDR decode failed        */
	NFS4ERR_LOCKS_HELD          = uint32(10037) /* file locks held at CLOSE */
	NFS4ERR_OPENMODE            = uint32(10038) /* conflict in OPEN and I/O */
	NFS4ERR_BADOWNER            = uint32(10039) /* owner translation bad    */
	NFS4ERR_BADCHAR             = uint32(10040) /* UTF-8 char not supported */
	NFS4ERR_BADNAME             = uint32(10041) /* name not supported       */
	NFS4ERR_BAD_RANGE           = uint32(10042) /* lock range not supported */
	NFS4ERR_LOCK_NOTSUPP        = uint32(10043) /* no atomic up/downgrade   */
	NFS4ERR_OP_ILLEGAL          = uint32(10044) /* undefined operation      */
	NFS4ERR_DEADLOCK            = uint32(10045) /* file locking deadlock    */
	NFS4ERR_FILE_OPEN           = uint32(10046) /* open file blocks op.     */
	NFS4ERR_ADMIN_REVOKED       = uint32(10047) /* lock-owner state revoked */
	NFS4ERR_CB_PATH_DOWN        = uint32(10048) /* callback path down       */
	NFS4ERR_NOXATTR             = uint32(10095) /* no extended attributes   */
	NFS4ERR_XATTR2BIG           = uint32(10096) /* extended attributes too big */
	NFS4ERR_NOT_ONLY_OP         = uint32(10081) /* not only operation       */
	NFS4ERR_DEADSESSION         = uint32(10078) /* dead session             */
	NFS4ERR_SEQ_MISORDERED      = uint32(10063) /* sequence misordered      */
	NFS4ERR_OP_NOT_IN_SESSION   = uint32(10071) /* operation not in session */
	NFS4ERR_RETRY_UNCACHED_REP  = uint32(10068) /* retry uncached rep       */
	NFS4ERR_CLIENTID_BUSY       = uint32(10074) /* clientid in use          */
)

type Error uint32

func (e Error) Error() string {
	switch uint32(e) {
	case NFS4ERR_SERVERFAULT:
		return "server fault"
	default:
		return fmt.Sprintf("NFS4ERR %d", uint32(e))
	}
}

func Err2Status(err error) uint32 {
	switch {
	case err == nil:
		return NFS4_OK
	case errors.Is(err, os.ErrNotExist):
		return NFS4ERR_NOENT
	case errors.Is(err, os.ErrExist):
		return NFS4ERR_EXIST
	case errors.Is(err, os.ErrPermission):
		return NFS4ERR_PERM
	case errors.Is(err, os.ErrInvalid):
		return NFS4ERR_INVAL
	case errors.Is(err, syscall.EBADFD):
		return NFS4ERR_BADHANDLE
	case errors.Is(err, syscall.EISDIR):
		return NFS4ERR_ISDIR
	case errors.Is(err, syscall.ENOTDIR):
		return NFS4ERR_NOTDIR
	case errors.Is(err, syscall.ENOTSUP), errors.Is(err, syscall.EOPNOTSUPP):
		return NFS4ERR_NOTSUPP
	case errors.Is(err, io.EOF):
		return NFS4ERR_IO
	default:
		if nfsErr, ok := err.(Error); ok {
			return uint32(nfsErr)
		}

		logrus.Errorf("unknown error: %v", err)

		return NFS4ERR_SERVERFAULT
	}
}
