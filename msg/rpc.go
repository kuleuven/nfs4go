//go:generate go run ../xdr/generate/generate.go -- rpc.go
//nolint:staticcheck
package msg

const (
	RPC_CALL = uint32(iota)
	RPC_REPLY
)

const (
	MSG_ACCEPTED = uint32(iota)
	MSG_DENIED
)

const (
	ACCEPT_SUCCESS       = uint32(iota) /* RPC executed successfully       */
	ACCEPT_PROG_UNAVAIL                 /* remote hasn't exported program  */
	ACCEPT_PROG_MISMATCH                /* remote can't support version #  */
	ACCEPT_PROC_UNAVAIL                 /* program can't support procedure */
	ACCEPT_GARBAGE_ARGS                 /* procedure can't decode params   */
)

const (
	REJECT_RPC_MISMATCH = uint32(iota) /* RPC version number != 2          */
	REJECT_AUTH_ERROR                  /* remote can't authenticate caller */
)

const (
	AUTH_FLAVOR_NULL = iota
	AUTH_FLAVOR_UNIX
	AUTH_FLAVOR_SHORT
	AUTH_FLAVOR_DES
)

const (
	AUTH_BADCRED      = uint32(iota + 1) /* bad credentials (seal broken) */
	AUTH_REJECTEDCRED                    /* client must begin new session */
	AUTH_BADVERF                         /* bad verifier (seal broken)    */
	AUTH_REJECTEDVERF                    /* verifier expired or replayed  */
	AUTH_TOOWEAK                         /* rejected for security reasons */
)

type Auth struct {
	Flavor uint32
	Body   []byte
}

type RPCMsgCall struct {
	Xid     uint32
	MsgType uint32 /* RPC_CALL */
	RPCVer  uint32 /* rfc1057, const: 2 */

	Prog uint32 /* nfs: 100003 */
	Vers uint32 /* 3 */
	Proc uint32 /* see proc.go */

	Cred Auth
	Verf Auth
}

type RPCMsgReply struct {
	Xid       uint32 /* exact as the corresponding call. */
	MsgType   uint32 /* RPC_REPLY */
	ReplyStat uint32 /* MSG_ACCEPT | MSG_DENIED */
}

type RejectReply struct {
	RejectStat uint32 /* RPC_MISMATCH | AUTH_ERROR */
	Lo         uint32
	Hi         uint32
}
