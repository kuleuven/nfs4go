package nfs4go

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kuleuven/nfs4go/auth"
	"github.com/kuleuven/nfs4go/bufpool"
	"github.com/kuleuven/nfs4go/clients"
	"github.com/kuleuven/nfs4go/clock"
	"github.com/kuleuven/nfs4go/logger"
	"github.com/kuleuven/nfs4go/msg"
	"github.com/kuleuven/nfs4go/worker"
	"github.com/kuleuven/nfs4go/xdr"
	"github.com/kuleuven/vfs"
	"github.com/sirupsen/logrus"
)

type Muxv4 struct {
	Clients *clients.Clients
	Logger  *logrus.Entry

	// Retrieve a FS for the specified creds and sessionID.
	// In case of a fatal error, Discard() is called to avoid to keep the FS in the pool.
	// The passed sessionID is set only when using nfs v4.1 or higher.
	FS func(creds *auth.Creds, sessionID [16]byte) *worker.Worker
}

type FileHandle struct {
	Handle []byte
	Path   string
}

func (x *Muxv4) Handle(request Request, response chan<- Response) {
	reply, data, err := x.HandleProc(request.Header, request.Data)
	if err != nil {
		x.Logger.Error(err)
	}
	/*
		if rate := float64(len(data.Bytes())) / 1000000 / time.Since(now).Seconds(); rate < 100 {
			fmt.Printf("%0.2f MB/s\n", rate)
		}
	*/
	response <- Response{
		Reply: reply,
		Data:  data,
		Error: err,
	}
}

func (x *Muxv4) HandleProc(header *msg.RPCMsgCall, data Bytes) (*msg.RPCMsgReply, Bytes, error) {
	switch header.Proc {
	case msg.PROC4_VOID:
		return x.Void(header, data)
	case msg.PROC4_COMPOUND:
		return x.Compound(header, data)
	default:
		return nil, nil, fmt.Errorf("not implemented: %s", msg.Proc4Name(header.Proc))
	}
}

func (x *Muxv4) Void(header *msg.RPCMsgCall, data Bytes) (*msg.RPCMsgReply, Bytes, error) {
	data.Reset()

	err := xdr.NewEncoder(data).EncodeAll(
		msg.Auth{
			Flavor: msg.AUTH_FLAVOR_NULL,
			Body:   []byte{},
		},
		msg.ACCEPT_SUCCESS,
		[0]byte{},
	)
	if err != nil {
		return nil, nil, err
	}

	return &msg.RPCMsgReply{
		Xid:       header.Xid,
		MsgType:   msg.RPC_REPLY,
		ReplyStat: msg.MSG_ACCEPTED,
	}, data, nil
}

func (x *Muxv4) Compound(header *msg.RPCMsgCall, data Bytes) (*msg.RPCMsgReply, Bytes, error) { //nolint:funlen
	resp, creds, err := auth.Authenticate(header.Cred, header.Verf)
	if authErr, ok := err.(*auth.AuthError); ok {
		data.Reset()

		return &msg.RPCMsgReply{
			Xid:       header.Xid,
			MsgType:   msg.RPC_REPLY,
			ReplyStat: msg.MSG_DENIED,
		}, data, xdr.NewEncoder(data).EncodeAll(msg.REJECT_AUTH_ERROR, authErr.Code)
	} else if err != nil {
		return nil, nil, err
	}

	defer data.Discard()

	var (
		tag      string
		minorVer uint32
		opsCnt   uint32
	)

	err = xdr.NewDecoder(data).DecodeAll(&tag, &minorVer, &opsCnt)
	if err != nil {
		return nil, nil, err
	}

	if minorVer > 2 {
		reply := &msg.RPCMsgReply{
			Xid:       header.Xid,
			MsgType:   msg.RPC_REPLY,
			ReplyStat: msg.MSG_ACCEPTED,
		}

		seq := []interface{}{
			resp,
			msg.ACCEPT_SUCCESS,
			msg.NFS4ERR_MINOR_VERS_MISMATCH,
			tag,
			uint32(0),
		}

		data = bufpool.Get()

		return reply, data, xdr.NewEncoder(data).EncodeAll(seq...)
	}

	compound := &Compound{
		Muxv4:    x,
		AuthResp: resp,
		MinorVer: minorVer,
		Tag:      tag,
		OpsCount: int(opsCnt),
		Creds:    creds,
	}

	x.Logger.Tracef("[COMPOUND WITH %d OPS] (v4.%d)", opsCnt, minorVer)

	dataOut := bufpool.Get()

	err = compound.Run(data, dataOut)
	if err != nil {
		return nil, nil, err
	}

	return &msg.RPCMsgReply{
		Xid:       header.Xid,
		MsgType:   msg.RPC_REPLY,
		ReplyStat: msg.MSG_ACCEPTED,
	}, dataOut, err
}

type Compound struct {
	*Muxv4
	AuthResp msg.Auth
	MinorVer uint32 // 0, 1 or 2 to indicate v4.0, v4.1 or v4.2
	Tag      string
	OpsCount int         // Number of ops in compound
	Creds    *auth.Creds // Credentials used for authentication

	// Fields only valid within Compound
	CurrentHandle *FileHandle
	SavedHandle   *FileHandle
	SessionID     [16]byte      // v4.1
	Slot          *clients.Slot // v4.1
}

var ErrNotImplemented = errors.New("not implemented")

var AllowedFirstOps41 = []uint32{
	msg.OP4_CREATE_SESSION,
	msg.OP4_DESTROY_SESSION,
	msg.OP4_SEQUENCE,
	msg.OP4_BIND_CONN_TO_SESSION,
	msg.OP4_EXCHANGE_ID,
}

var FatalStatuses = []uint32{
	msg.NFS4ERR_OP_ILLEGAL,
	msg.NFS4ERR_OP_NOT_IN_SESSION,
	msg.NFS4ERR_SERVERFAULT,
	msg.NFS4ERR_NOTSUPP,
	msg.NFS4ERR_FHEXPIRED,
	msg.NFS4ERR_STALE,
}

func (x *Compound) Run(in, out Bytes) error {
	// Read first operation
	var op uint32

	if err := xdr.NewDecoder(in).Decode(&op); err != nil {
		return err
	}

	if x.MinorVer > 0 {
		if !slices.Contains(AllowedFirstOps41, op) {
			return x.WriteHeaderAndSingleOperation(out, op, msg.NFS4ERR_OP_NOT_IN_SESSION)
		}

		if op != msg.OP4_SEQUENCE && x.OpsCount > 1 {
			return x.WriteHeaderAndSingleOperation(out, op, msg.NFS4ERR_NOT_ONLY_OP)
		}

		if op == msg.OP4_SEQUENCE {
			return x.RunSequence(in, out)
		}
	}

	if err := x.WriteHeader(out, x.OpsCount, msg.NFS4_OK); err != nil {
		return err
	}

	lastStatus, err := x.doOperation(in, out, op)
	if err != nil {
		return err
	}

	opsExecuted := 1

	for i := 1; i < x.OpsCount && !slices.Contains(FatalStatuses, lastStatus); i++ {
		lastStatus, err = x.Operation(in, out)
		if err != nil {
			return err
		}

		opsExecuted++
	}

	return x.RewriteHeaderIfNeeded(out, opsExecuted, lastStatus)
}

func (x *Compound) RunSequence(in, out Bytes) error { //nolint:funlen
	var args msg.SEQUENCE4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return err
	}

	x.Logger.Tracef("SEQUENCE %s %d %d", hex.EncodeToString(args.SessionID[:]), args.SequenceID, args.SlotID)

	clientID := clients.ClientIDFromSessionID(args.SessionID)

	client, ok := x.Clients.Get(clientID)
	if !ok {
		x.Logger.Warnf("client %d not found", clientID)

		return x.WriteHeaderAndSingleOperation(out, msg.OP4_SEQUENCE, msg.NFS4ERR_DEADSESSION)
	}

	slot := client.GetSlot(args.SessionID, args.SlotID)

	if slot == nil {
		x.Logger.Warnf("slot %x %d not found", args.SessionID, args.SlotID)

		return x.WriteHeaderAndSingleOperation(out, msg.OP4_SEQUENCE, msg.NFS4ERR_DEADSESSION)
	}

	if slot.SequenceID == args.SequenceID && slot.ContainsData {
		// Copy data from cache
		slot.Buf.Copy(out)

		return nil
	}

	if slot.SequenceID == args.SequenceID && slot.Buf == nil { // Client requests cached information without anything available
		x.Logger.Warnf("client %d requested cached information without anything available", clientID)

		return x.WriteHeaderAndSingleOperation(out, msg.OP4_SEQUENCE, msg.NFS4ERR_RETRY_UNCACHED_REP)
	}

	if slot.SequenceID != args.SequenceID && slot.SequenceID+1 != args.SequenceID {
		x.Logger.Warnf("client %d requested out of order sequence %d %d", clientID, slot.SequenceID, args.SequenceID)

		return x.WriteHeaderAndSingleOperation(out, msg.OP4_SEQUENCE, msg.NFS4ERR_SEQ_MISORDERED)
	}

	slot.SequenceID = args.SequenceID
	slot.ContainsData = args.CacheThis && slot.Buf != nil

	x.SessionID = args.SessionID
	x.Slot = slot

	if err := x.WriteHeader(out, x.OpsCount, msg.NFS4_OK); err != nil {
		return err
	}

	lastStatus, err := OperationResponse(out, msg.OP4_SEQUENCE, msg.NFS4_OK, msg.SEQUENCE4resok{
		SessionID:           args.SessionID,
		SequenceID:          args.SequenceID,
		SlotID:              args.SlotID,
		SlotIDHighest:       clients.MaxSlotID,
		SlotIDHighestTarget: clients.MaxSlotID,
	})
	if err != nil {
		return err
	}

	opsExecuted := 1

	for i := 1; i < x.OpsCount && !slices.Contains(FatalStatuses, lastStatus); i++ {
		lastStatus, err = x.Operation(in, out)
		if err != nil {
			return err
		}

		opsExecuted++
	}

	if err = x.RewriteHeaderIfNeeded(out, opsExecuted, lastStatus); err != nil {
		return err
	}

	if slot.ContainsData {
		out.Copy(slot.Buf)
	}

	return nil
}

func (x *Compound) WriteHeader(out Bytes, opsCount int, lastStatus uint32) error {
	seq := []interface{}{
		x.AuthResp,
		msg.ACCEPT_SUCCESS,
		lastStatus,
		x.Tag,
		opsCount,
	}

	return xdr.NewEncoder(out).EncodeAll(seq...)
}

func (x *Compound) WriteHeaderAndSingleOperation(out Bytes, op, status uint32) error {
	if err := x.WriteHeader(out, 1, status); err != nil {
		return err
	}

	return xdr.NewEncoder(out).EncodeAll(op, status)
}

func (x *Compound) RewriteHeaderIfNeeded(out Bytes, opsCount int, lastStatus uint32) error {
	if lastStatus == msg.NFS4_OK && opsCount == x.OpsCount {
		return nil
	}

	offset := out.SeekWrite(0)

	if err := x.WriteHeader(out, opsCount, lastStatus); err != nil {
		return err
	}

	out.SeekWrite(offset)

	return nil
}

func OperationResponse(out Bytes, op, status uint32, data ...interface{}) (uint32, error) {
	if status != msg.NFS4_OK {
		logger.Logger.Warnf("Operation [%s] failed with status %d", msg.Proc4Name(op), status)
	}

	encoder := xdr.NewEncoder(out)

	if err := encoder.EncodeAll(op, status); err != nil {
		return status, err
	}

	return status, encoder.EncodeAll(data...)
}

var NotImplementedRequiredOps = []uint32{
	msg.OP4_BACKCHANNEL_CTL,
	msg.OP4_BIND_CONN_TO_SESSION,
	msg.OP4_FREE_STATEID,
	msg.OP4_ILLEGAL,
	msg.OP4_LOCK,
	msg.OP4_LOCKT,
	msg.OP4_LOCKU,
	msg.OP4_SET_SSV,
	msg.OP4_TEST_STATEID,
}

var NotImplementedOptionalOps = []uint32{
	msg.OP4_ALLOCATE,
	msg.OP4_CLONE,
	msg.OP4_COPY,
	msg.OP4_COPY_NOTIFY,
	msg.OP4_DEALLOCATE,
	msg.OP4_DELEGPURGE,
	msg.OP4_DELEGRETURN,
	msg.OP4_GETDEVICEINFO,
	msg.OP4_GET_DIR_DELEGATION,
	msg.OP4_IO_ADVISE,
	msg.OP4_LAYOUTCOMMIT,
	msg.OP4_LAYOUTERROR,
	msg.OP4_LAYOUTGET,
	msg.OP4_LAYOUTRETURN,
	msg.OP4_LAYOUTSTATS,
	msg.OP4_OFFLOAD_CANCEL,
	msg.OP4_OFFLOAD_STATUS,
	msg.OP4_OPENATTR,
	msg.OP4_READ_PLUS,
	msg.OP4_SEEK,
	msg.OP4_WANT_DELEGATION,
	msg.OP4_WRITE_SAME,
}

func (x *Compound) Operation(in, out Bytes) (uint32, error) {
	var op uint32

	err := xdr.NewDecoder(in).Decode(&op)
	if err != nil {
		return 0, err
	}

	return x.doOperation(in, out, op)
}

func (x *Compound) doOperation(in, out Bytes, op uint32) (uint32, error) { //nolint:funlen,gocyclo
	switch op {
	case msg.OP4_SETCLIENTID:
		return x.SetClientID(in, out)
	case msg.OP4_SETCLIENTID_CONFIRM:
		return x.SetClientIDConfirm(in, out)
	case msg.OP4_EXCHANGE_ID:
		return x.ExchangeID(in, out)
	case msg.OP4_CREATE_SESSION:
		return x.CreateSession(in, out)
	case msg.OP4_RECLAIM_COMPLETE:
		return x.ReclaimComplete(in, out)
	case msg.OP4_DESTROY_SESSION:
		return x.DestroySession(in, out)
	case msg.OP4_DESTROY_CLIENTID:
		return x.DestroyClientID(in, out)
	case msg.OP4_PUTROOTFH:
		return x.PutRootFH(in, out)
	case msg.OP4_PUTPUBFH:
		return x.PutPubFH(in, out)
	case msg.OP4_PUTFH:
		return x.PutFH(in, out)
	case msg.OP4_GETFH:
		return x.GetFH(in, out)
	case msg.OP4_SAVEFH:
		return x.SaveFH(in, out)
	case msg.OP4_RESTOREFH:
		return x.RestoreFH(in, out)
	case msg.OP4_GETATTR:
		return x.GetAttr(in, out)
	case msg.OP4_LOOKUP:
		return x.Lookup(in, out)
	case msg.OP4_LOOKUPP:
		return x.LookupParent(in, out)
	case msg.OP4_ACCESS:
		return x.Access(in, out)
	case msg.OP4_READDIR:
		return x.ReadDir(in, out)
	case msg.OP4_RENEW:
		return x.Renew(in, out)
	case msg.OP4_SECINFO:
		return x.Secinfo(in, out)
	case msg.OP4_SECINFO_NO_NAME:
		return x.SecinfoNoName(in, out)
	case msg.OP4_CREATE:
		return x.Create(in, out)
	case msg.OP4_RENAME:
		return x.Rename(in, out)
	case msg.OP4_REMOVE:
		return x.Remove(in, out)
	case msg.OP4_LINK:
		return x.Link(in, out)
	case msg.OP4_READLINK:
		return x.Readlink(in, out)
	case msg.OP4_SETATTR:
		return x.SetAttr(in, out)
	case msg.OP4_OPEN:
		return x.Open(in, out)
	case msg.OP4_OPEN_DOWNGRADE:
		return x.OpenDowngrade(in, out)
	case msg.OP4_CLOSE:
		return x.Close(in, out)
	case msg.OP4_READ:
		return x.Read(in, out)
	case msg.OP4_WRITE:
		return x.Write(in, out)
	case msg.OP4_COMMIT:
		return x.Commit(in, out)
	case msg.OP4_VERIFY:
		return x.Verify(in, out)
	case msg.OP4_NVERIFY:
		return x.NVerify(in, out)
	case msg.OP4_GETXATTR:
		return x.GetXAttr(in, out)
	case msg.OP4_SETXATTR:
		return x.SetXAttr(in, out)
	case msg.OP4_LISTXATTRS:
		return x.ListXAttrs(in, out)
	case msg.OP4_REMOVEXATTR:
		return x.RemoveXAttr(in, out)
	default: // Note: only return statuses listed in FatalStatuses
		if slices.Contains(NotImplementedOptionalOps, op) {
			x.Logger.Infof("we did not implement optional operation: %v", op)

			return OperationResponse(out, op, msg.NFS4ERR_NOTSUPP)
		}

		if slices.Contains(NotImplementedRequiredOps, op) {
			x.Logger.Errorf("we did not implement required operation: %v", op)

			return OperationResponse(out, op, msg.NFS4ERR_SERVERFAULT)
		}

		return OperationResponse(out, op, msg.NFS4ERR_OP_ILLEGAL)
	}
}

func (x *Compound) SetClientID(in, out Bytes) (uint32, error) {
	if x.MinorVer > 0 {
		return OperationResponse(out, msg.OP4_SETCLIENTID, msg.NFS4ERR_OP_ILLEGAL)
	}

	args := struct {
		Client        msg.NfsClientId4
		Callback      msg.CbClient4
		CallbackIdent uint32
	}{}

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("SETCLIENTID %s", args.Client.Id)

	client := clients.Client{
		Name:     args.Client.Id,
		Verifier: args.Client.Verifier,
		Creds:    x.Creds,
	}

	clientID, confirmValue, _, err := x.Clients.Add(client)
	if err != nil {
		return OperationResponse(out, msg.OP4_SETCLIENTID, msg.Err2Status(err))
	}

	resp := struct {
		ClientID           uint64 // type: clientid4
		SetClientIDConfirm uint64 // type: verifier4
	}{
		ClientID:           clientID,
		SetClientIDConfirm: confirmValue,
	}

	return OperationResponse(out, msg.OP4_SETCLIENTID, msg.NFS4_OK, resp)
}

func (x *Compound) SetClientIDConfirm(in, out Bytes) (uint32, error) {
	if x.MinorVer > 0 {
		return OperationResponse(out, msg.OP4_SETCLIENTID_CONFIRM, msg.NFS4ERR_OP_ILLEGAL)
	}

	args := struct {
		ClientID uint64
		Verifier uint64
	}{}

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("SETCLIENTID_CONFIRM %d %d", args.ClientID, args.Verifier)

	_, err := x.Clients.Confirm(args.ClientID, args.Verifier, x.Creds)

	return OperationResponse(out, msg.OP4_SETCLIENTID_CONFIRM, msg.Err2Status(err))
}

func (x *Compound) ExchangeID(in, out Bytes) (uint32, error) {
	if x.MinorVer < 1 {
		return OperationResponse(out, msg.OP4_EXCHANGE_ID, msg.NFS4ERR_OP_ILLEGAL)
	}

	var args msg.EXCHANGE_ID4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("EXCHANGE_ID %s %d", args.ClientOwner.OwnerId, args.Flags)

	if clientID, ok := x.Clients.GetByName(args.ClientOwner.OwnerId, args.ClientOwner.Verifier, x.Creds); ok {
		return OperationResponse(out, msg.OP4_EXCHANGE_ID, msg.NFS4_OK, msg.EXCHANGE_ID4resok{
			ClientID: clientID,
			Flags:    msg.EXCHGID4_FLAG_USE_NON_PNFS | msg.EXCHGID4_FLAG_BIND_PRINC_STATEID | msg.EXCHGID4_FLAG_CONFIRMED_R,
			ServerOwner: msg.ServerOwner4{
				MajorId: "sftp-nfs-4.1",
			},
		})
	}

	if args.Flags&msg.EXCHGID4_FLAG_UPD_CONFIRMED_REC_A != 0 {
		return OperationResponse(out, msg.OP4_EXCHANGE_ID, msg.NFS4ERR_STALE_CLIENTID)
	}

	client := clients.Client{
		Name:     args.ClientOwner.OwnerId,
		Verifier: args.ClientOwner.Verifier,
		Creds:    x.Creds,
	}

	clientID, _, seqID, err := x.Clients.Add(client)
	if err != nil {
		return OperationResponse(out, msg.OP4_EXCHANGE_ID, msg.Err2Status(err))
	}

	return OperationResponse(out, msg.OP4_EXCHANGE_ID, msg.NFS4_OK, msg.EXCHANGE_ID4resok{
		ClientID:   clientID,
		SequenceID: seqID,
		Flags:      msg.EXCHGID4_FLAG_USE_NON_PNFS | msg.EXCHGID4_FLAG_BIND_PRINC_STATEID,
		ServerOwner: msg.ServerOwner4{
			MajorId: "sftp-nfs-4.1",
		},
	})
}

func (x *Compound) CreateSession(in, out Bytes) (uint32, error) {
	if x.MinorVer < 1 {
		return OperationResponse(out, msg.OP4_CREATE_SESSION, msg.NFS4ERR_OP_ILLEGAL)
	}

	var args msg.CREATE_SESSION4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("CREATE_SESSION %d %d %v", args.ClientID, args.SequenceID, args)

	client, err := x.Clients.Confirm41(args.ClientID, args.SequenceID, x.Creds)
	if err != nil {
		return OperationResponse(out, msg.OP4_CREATE_SESSION, msg.Err2Status(err))
	}

	sessionID := client.BuildSession(args.Flags&msg.CREATE_SESSION4_FLAG_PERSIST != 0)

	args.ForeChanAttrs.HeaderPadSize = 0
	args.ForeChanAttrs.RdmaIrd = nil
	args.BackChanAttrs.HeaderPadSize = 0
	args.BackChanAttrs.RdmaIrd = nil

	x.Logger.Infof("ForeChanAttrs: %v", args.ForeChanAttrs)
	x.Logger.Infof("BackChanAttrs: %v", args.BackChanAttrs)

	return OperationResponse(out, msg.OP4_CREATE_SESSION, msg.NFS4_OK, msg.CREATE_SESSION4resok{
		SessionID:     sessionID,
		SequenceID:    args.SequenceID,
		Flags:         args.Flags & (msg.CREATE_SESSION4_FLAG_CONN_BACK_CHAN | msg.CREATE_SESSION4_FLAG_PERSIST),
		ForeChanAttrs: args.ForeChanAttrs,
		BackChanAttrs: args.BackChanAttrs,
	})
}

func (x *Compound) ReclaimComplete(in, out Bytes) (uint32, error) {
	if x.MinorVer < 1 {
		return OperationResponse(out, msg.OP4_RECLAIM_COMPLETE, msg.NFS4ERR_OP_ILLEGAL)
	}

	var arg bool

	if err := xdr.NewDecoder(in).Decode(&arg); err != nil {
		return 0, err
	}

	x.Logger.Tracef("RECLAIM_COMPLETE %v", arg)

	return OperationResponse(out, msg.OP4_RECLAIM_COMPLETE, msg.NFS4_OK)
}

func (x *Compound) DestroySession(in, out Bytes) (uint32, error) {
	if x.MinorVer < 1 {
		return OperationResponse(out, msg.OP4_DESTROY_SESSION, msg.NFS4ERR_OP_ILLEGAL)
	}

	var sessionID [16]byte

	if err := xdr.NewDecoder(in).Decode(&sessionID); err != nil {
		return 0, err
	}

	x.Logger.Tracef("DESTROY_SESSION %s", hex.EncodeToString(sessionID[:]))

	client, ok := x.Clients.Get(clients.ClientIDFromSessionID(sessionID))
	if !ok {
		return OperationResponse(out, msg.OP4_DESTROY_SESSION, msg.NFS4_OK)
	}

	client.RemoveSession(sessionID)

	return OperationResponse(out, msg.OP4_DESTROY_SESSION, msg.NFS4_OK)
}

func (x *Compound) DestroyClientID(in, out Bytes) (uint32, error) {
	if x.MinorVer < 1 {
		return OperationResponse(out, msg.OP4_DESTROY_CLIENTID, msg.NFS4ERR_OP_ILLEGAL)
	}

	var clientID uint64

	if err := xdr.NewDecoder(in).Decode(&clientID); err != nil {
		return 0, err
	}

	x.Logger.Tracef("DESTROY_CLIENTID %d", clientID)

	return OperationResponse(out, msg.OP4_DESTROY_CLIENTID, msg.Err2Status(x.Clients.RemoveClient(clientID)))
}

func (x *Compound) PutRootFH(in, out Bytes) (uint32, error) {
	x.Logger.Trace("PUTROOTFH")

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	fh, err := fs.Handle("/")
	if err != nil {
		fs.Discard() // This is not normal, root should always exist

		return OperationResponse(out, msg.OP4_PUTROOTFH, msg.Err2Status(err))
	}

	x.CurrentHandle = &FileHandle{
		Handle: fh,
		Path:   "/",
	}

	return OperationResponse(out, msg.OP4_PUTROOTFH, msg.NFS4_OK)
}

func (x *Compound) PutPubFH(in, out Bytes) (uint32, error) {
	x.Logger.Trace("PUTPUBFH")

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	fh, err := fs.Handle("/")
	if err != nil {
		fs.Discard() // This is not normal, root should always exist

		return OperationResponse(out, msg.OP4_PUTPUBFH, msg.Err2Status(err))
	}

	x.CurrentHandle = &FileHandle{
		Handle: fh,
		Path:   "/",
	}

	return OperationResponse(out, msg.OP4_PUTPUBFH, msg.NFS4_OK)
}

func (x *Compound) PutFH(in, out Bytes) (uint32, error) {
	args := msg.PUTFH4args{}

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("PUTFH %s", hex.EncodeToString(args.Fh))

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	var (
		path string
		err  error
	)

	if fi, cached := fs.Cache.Get(args.Fh); cached {
		path = fi.Path
	} else {
		path, err = fs.Path(args.Fh)
	}

	if err != nil {
		DiscardOnServerFault(fs, err)

		if errors.Is(err, syscall.EOPNOTSUPP) || errors.Is(err, os.ErrNotExist) {
			err = msg.Error(msg.NFS4ERR_STALE)
		}

		return OperationResponse(out, msg.OP4_PUTFH, msg.Err2Status(err))
	}

	x.CurrentHandle = &FileHandle{
		Handle: args.Fh,
		Path:   path,
	}

	return OperationResponse(out, msg.OP4_PUTFH, msg.NFS4_OK)
}

func (x *Compound) GetFH(in, out Bytes) (uint32, error) {
	x.Logger.Trace("GETFH")

	if x.CurrentHandle == nil {
		return OperationResponse(out, msg.OP4_GETFH, msg.NFS4ERR_NOFILEHANDLE)
	}

	return OperationResponse(out,
		msg.OP4_GETFH,
		msg.NFS4_OK,
		msg.GETFH4resok{
			Fh: x.CurrentHandle.Handle,
		},
	)
}

func (x *Compound) SaveFH(in, out Bytes) (uint32, error) {
	x.Logger.Trace("SAVEFH")

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_SAVEFH,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	x.SavedHandle = x.CurrentHandle

	return OperationResponse(out,
		msg.OP4_SAVEFH,
		msg.NFS4_OK,
	)
}

func (x *Compound) RestoreFH(in, out Bytes) (uint32, error) {
	x.Logger.Trace("RESTOREFH")

	if x.SavedHandle == nil {
		return OperationResponse(out,
			msg.OP4_RESTOREFH,
			msg.NFS4ERR_RESTOREFH,
		)
	}

	x.CurrentHandle = x.SavedHandle

	return OperationResponse(out,
		msg.OP4_RESTOREFH,
		msg.NFS4_OK,
	)
}

func (x *Compound) GetAttr(in, out Bytes) (uint32, error) {
	args := msg.GETATTR4args{}

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	idxReq := bitmap4Decode(args.AttrRequest)

	x.Logger.Tracef("GETATTR %s", bitmapString(idxReq))

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_GETATTR,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	if fi, cached := fs.Cache.Get(x.CurrentHandle.Handle); cached {
		attrs := fileInfoToAttrs(x.CurrentHandle.Handle, fi, nil, idxReq, x.Creds, fs.SessionID)

		return OperationResponse(out,
			msg.OP4_GETATTR,
			msg.NFS4_OK,
			msg.GETATTR4resok{
				Attr: attrs,
			},
		)
	}

	fi, err := fs.Lstat(x.CurrentHandle.Path)
	if err != nil {
		DiscardOnServerFault(fs, err)

		return OperationResponse(out,
			msg.OP4_GETATTR,
			msg.Err2Status(err),
		)
	}

	fs.Cache.Put(x.CurrentHandle.Handle, worker.Entry{
		Path:     x.CurrentHandle.Path,
		FileInfo: fi,
	})

	attrs := fileInfoToAttrs(x.CurrentHandle.Handle, fi, nil, idxReq, x.Creds, fs.SessionID)

	return OperationResponse(out,
		msg.OP4_GETATTR,
		msg.NFS4_OK,
		msg.GETATTR4resok{
			Attr: attrs,
		},
	)
}

func (x *Compound) Lookup(in, out Bytes) (uint32, error) {
	args := msg.LOOKUP4args{}

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("LOOKUP %s", args.ObjName)

	if args.ObjName == "" {
		return OperationResponse(out,
			msg.OP4_LOOKUP,
			msg.NFS4ERR_INVAL,
		)
	}

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_LOOKUP,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	path := vfs.Join(x.CurrentHandle.Path, args.ObjName)

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	handle, err := fs.Handle(path)
	if err != nil {
		DiscardOnServerFault(fs, err)

		return OperationResponse(out,
			msg.OP4_LOOKUP,
			msg.Err2Status(err),
		)
	}

	x.CurrentHandle = &FileHandle{
		Handle: handle,
		Path:   path,
	}

	return OperationResponse(out,
		msg.OP4_LOOKUP,
		msg.NFS4_OK,
	)
}

func (x *Compound) LookupParent(in, out Bytes) (uint32, error) {
	x.Logger.Trace("LOOKUPP")

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_LOOKUPP,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	if x.CurrentHandle.Path == "/" {
		return OperationResponse(out,
			msg.OP4_LOOKUPP,
			msg.NFS4ERR_INVAL,
		)
	}

	path := vfs.Base(x.CurrentHandle.Path)

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	handle, err := fs.Handle(path)
	if err != nil {
		DiscardOnServerFault(fs, err)

		return OperationResponse(out,
			msg.OP4_LOOKUPP,
			msg.Err2Status(err),
		)
	}

	x.CurrentHandle = &FileHandle{
		Handle: handle,
		Path:   path,
	}

	return OperationResponse(out,
		msg.OP4_LOOKUPP,
		msg.NFS4_OK,
	)
}

func (x *Compound) Access(in, out Bytes) (uint32, error) { //nolint:funlen
	args := msg.ACCESS4args{}

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("ACCESS %o", args.Access)

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_ACCESS,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	var fi vfs.FileInfo

	fi, cached := fs.Cache.Get(x.CurrentHandle.Handle)

	if !cached {
		var err error

		fi, err = fs.Lstat(x.CurrentHandle.Path)
		if err != nil {
			DiscardOnServerFault(fs, err)

			return OperationResponse(out,
				msg.OP4_ACCESS,
				msg.Err2Status(err),
			)
		}

		fs.Cache.Put(x.CurrentHandle.Handle, worker.Entry{
			Path:     x.CurrentHandle.Path,
			FileInfo: fi,
		})
	}

	support := msg.ACCESS4_READ
	support |= msg.ACCESS4_LOOKUP
	support |= msg.ACCESS4_MODIFY
	support |= msg.ACCESS4_EXTEND
	support |= msg.ACCESS4_DELETE
	support |= msg.ACCESS4_EXECUTE
	support |= msg.ACCESS4_XAREAD
	support |= msg.ACCESS4_XAWRITE
	support |= msg.ACCESS4_XALIST

	perm := (uint32(fi.Mode()) >> 6) & uint32(0b0111)

	r := perm & (uint32(1) << 2)
	w := perm & (uint32(1) << 1)
	xe := perm & uint32(1)

	accForFh := uint32(0)

	if r > 0 {
		accForFh |= msg.ACCESS4_READ
		accForFh |= msg.ACCESS4_LOOKUP
		accForFh |= msg.ACCESS4_XAREAD
		accForFh |= msg.ACCESS4_XALIST
	}

	if w > 0 {
		accForFh |= msg.ACCESS4_MODIFY
		accForFh |= msg.ACCESS4_EXTEND
		accForFh |= msg.ACCESS4_DELETE
		accForFh |= msg.ACCESS4_XAWRITE
	}

	if xe > 0 {
		accForFh |= msg.ACCESS4_LOOKUP
		accForFh |= msg.ACCESS4_EXECUTE
	}

	support &= args.Access
	accForFh &= args.Access

	return OperationResponse(out,
		msg.OP4_ACCESS,
		msg.NFS4_OK,
		msg.ACCESS4resok{
			Supported: support,
			Access:    accForFh,
		},
	)
}

type Entry4 struct {
	IsEntry bool
	Cookie  uint64
	Name    string
	Attrs   *msg.FAttr4
}

func (x *Compound) ReadDir(in, out Bytes) (uint32, error) { //nolint:funlen,gocognit
	args := msg.READDIR4args{}

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	idxReq := bitmap4Decode(args.AttrRequest)

	x.Logger.Tracef("READDIR %d %d %s", args.Cookie, args.CookieVerf, bitmapString(idxReq))

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_LOOKUP,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	var lister vfs.ListerAt

	switch {
	case args.Cookie == 0:
		var err error

		lister, err = fs.List(x.CurrentHandle.Path)
		if err != nil {
			DiscardOnServerFault(fs, err)

			return OperationResponse(out,
				msg.OP4_READDIR,
				msg.Err2Status(err),
			)
		}

		args.CookieVerf = fs.AddLister(&worker.Lister{
			Lister: lister,
		})
	case args.CookieVerf == worker.EOFLister:
		return OperationResponse(out,
			msg.OP4_READDIR,
			msg.NFS4_OK,
			msg.READDIR4resok{
				CookieVerf: args.CookieVerf,
				Reply: msg.DirList4{
					Eof: true,
				},
			},
		)
	default:
		stored, ok := fs.GetLister(args.CookieVerf)

		if !ok {
			return OperationResponse(out,
				msg.OP4_READDIR,
				msg.NFS4ERR_NOT_SAME,
			)
		}

		lister = stored.Lister
	}

	batchSize := 128
	lbuf := make([]vfs.FileInfo, batchSize)
	offset := args.Cookie - 1000

	if args.Cookie == 0 {
		offset = 0
	}

	n, err := lister.ListAt(lbuf, int64(offset))
	if err != nil && !errors.Is(err, io.EOF) {
		fs.Discard()

		return OperationResponse(out,
			msg.OP4_READDIR,
			msg.Err2Status(err),
		)
	}

	var first, prev *msg.Entry4

	for i, fi := range lbuf[:n] {
		fh, handleErr := fs.Handle(vfs.Join(x.CurrentHandle.Path, fi.Name()))
		if handleErr != nil {
			x.Logger.Warnf("failed to get handle: %s", handleErr)
		} else {
			fs.Cache.Put(fh, worker.Entry{
				Path:     vfs.Join(x.CurrentHandle.Path, fi.Name()),
				FileInfo: fi,
			})
		}

		entry := &msg.Entry4{
			Cookie: offset + 1000 + uint64(i) + 1, // the offset of the next entry if existing
			Name:   fi.Name(),
			Attrs:  fileInfoToAttrs(fh, fi, handleErr, idxReq, x.Creds, fs.SessionID),
		}

		if first == nil {
			first = entry
		} else {
			prev.Next = entry
		}

		prev = entry

		if size := SizeOf(xdr.Marshal(entry.Cookie, entry.Name)); args.DirCount < size {
			err = nil
			break
		} else {
			args.DirCount -= size
		}

		if size := SizeOf(xdr.Marshal(entry.Cookie, entry.Name, entry.Attrs)) + 4; args.MaxCount < size+128 {
			err = nil
			break
		} else {
			args.MaxCount -= size
		}
	}

	if errors.Is(err, io.EOF) {
		if closeErr := fs.CloseLister(args.CookieVerf); closeErr != nil {
			return OperationResponse(out,
				msg.OP4_READDIR,
				msg.Err2Status(closeErr),
			)
		}

		args.CookieVerf = worker.EOFLister
	}

	return OperationResponse(out,
		msg.OP4_READDIR,
		msg.NFS4_OK,
		msg.READDIR4resok{
			CookieVerf: args.CookieVerf,
			Reply: msg.DirList4{
				Entries: first,
				Eof:     errors.Is(err, io.EOF),
			},
		},
	)
}

func SizeOf(data []byte, _ error) uint32 {
	return uint32(len(data))
}

func (x *Compound) Renew(in, out Bytes) (uint32, error) {
	if x.MinorVer > 0 {
		return OperationResponse(out,
			msg.OP4_SETCLIENTID,
			msg.NFS4ERR_OP_ILLEGAL,
		)
	}

	args := msg.RENEW4args{}

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("RENEW %d", args.ClientId)

	client, ok := x.Clients.Get(args.ClientId)
	if !ok {
		return OperationResponse(out,
			msg.OP4_RENEW,
			msg.NFS4ERR_EXPIRED,
		)
	}

	if !client.Creds.Equal(x.Creds) {
		x.Logger.Warnf("Renew: Creds don't match %v %v", client.Creds, x.Creds)

		return OperationResponse(out,
			msg.OP4_RENEW,
			msg.NFS4ERR_ACCESS,
		)
	}

	return OperationResponse(out,
		msg.OP4_RENEW,
		msg.NFS4_OK,
	)
}

func (x *Compound) Secinfo(in, out Bytes) (uint32, error) {
	x.Logger.Trace("SECINFO")

	return OperationResponse(out,
		msg.OP4_SECINFO,
		msg.NFS4_OK,
		msg.SECINFO4resok{
			Items: []msg.Secinfo4{
				{
					Flavor: msg.AUTH_FLAVOR_UNIX,
				},
			},
		},
	)
}

func (x *Compound) SecinfoNoName(in, out Bytes) (uint32, error) {
	var style uint32

	if err := xdr.NewDecoder(in).Decode(&style); err != nil {
		return 0, err
	}

	x.Logger.Tracef("SECINFO_NO_NAME %d", style)

	return OperationResponse(out,
		msg.OP4_SECINFO_NO_NAME,
		msg.NFS4_OK,
		msg.SECINFO4resok{
			Items: []msg.Secinfo4{
				{
					Flavor: msg.AUTH_FLAVOR_UNIX,
				},
			},
		},
	)
}

func (x *Compound) Create(in, out Bytes) (uint32, error) { //nolint:funlen
	var args msg.CREATE4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("CREATE %s %d", args.ObjName, args.Type.ObjType)

	if args.ObjName == "" {
		return OperationResponse(out,
			msg.OP4_CREATE,
			msg.NFS4ERR_INVAL,
		)
	}

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_CREATE,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	path := vfs.Join(x.CurrentHandle.Path, args.ObjName)

	decAttrs, err := decodeFAttrs4(args.CreateAttrs)
	if err != nil {
		return OperationResponse(out,
			msg.OP4_CREATE,
			msg.Err2Status(err),
		)
	}

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	switch args.Type.ObjType {
	case msg.NF4DIR:
		mode := os.FileMode(0o755)

		if decAttrs.Mode != nil {
			mode = os.FileMode(*decAttrs.Mode)
		}

		mode |= os.ModeDir

		if err = fs.Mkdir(path, mode); err != nil {
			DiscardOnServerFault(fs, err)

			return OperationResponse(out,
				msg.OP4_CREATE,
				msg.Err2Status(err),
			)
		}
	case msg.NF4REG: // Create is not allowed for regular files
		return OperationResponse(out,
			msg.OP4_CREATE,
			msg.NFS4ERR_OP_ILLEGAL,
		)
	case msg.NF4LNK:
		err = fs.Symlink(args.Type.LinkData, path)
		if err != nil {
			DiscardOnServerFault(fs, err)

			return OperationResponse(out,
				msg.OP4_CREATE,
				msg.Err2Status(err),
			)
		}
	case msg.NF4BLK, msg.NF4CHR, msg.NF4FIFO, msg.NF4SOCK:
		return OperationResponse(out,
			msg.OP4_CREATE,
			msg.NFS4ERR_NOTSUPP,
		)
	default:
		return OperationResponse(out,
			msg.OP4_CREATE,
			msg.NFS4ERR_BADTYPE,
		)
	}

	fs.Cache.Invalidate(x.CurrentHandle.Handle)

	handle, err := fs.Handle(path)
	if err != nil {
		DiscardOnServerFault(fs, err)

		return OperationResponse(out,
			msg.OP4_CREATE,
			msg.Err2Status(err),
		)
	}

	x.CurrentHandle = &FileHandle{
		Handle: handle,
		Path:   path,
	}

	return OperationResponse(out,
		msg.OP4_CREATE,
		msg.NFS4_OK,
		msg.CREATE4resok{
			CInfo:   msg.ChangeInfo4{}, // non-atomic change
			AttrSet: []uint32{A_mode},  // We only support mode
		},
	)
}

func (x *Compound) Rename(in, out Bytes) (uint32, error) { //nolint:funlen
	var args msg.RENAME4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("RENAME %s %s", args.OldName, args.NewName)

	if args.OldName == "" || args.NewName == "" {
		return OperationResponse(out,
			msg.OP4_RENAME,
			msg.NFS4ERR_INVAL,
		)
	}

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_RENAME,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	if x.SavedHandle == nil {
		return OperationResponse(out,
			msg.OP4_RENAME,
			msg.NFS4ERR_RESTOREFH,
		)
	}

	oldName := vfs.Join(x.SavedHandle.Path, args.OldName)
	newName := vfs.Join(x.CurrentHandle.Path, args.NewName)

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	if err := fs.Rename(oldName, newName); err != nil {
		DiscardOnServerFault(fs, err)

		status := msg.Err2Status(err)

		if status == msg.NFS4ERR_NOTSUPP {
			status = msg.NFS4ERR_XDEV
		}

		return OperationResponse(out,
			msg.OP4_RENAME,
			status,
		)
	}

	fs.Cache.Invalidate(x.SavedHandle.Handle)
	fs.Cache.Invalidate(x.CurrentHandle.Handle)

	return OperationResponse(out,
		msg.OP4_RENAME,
		msg.NFS4_OK,
		msg.RENAME4resok{
			SourceCInfo: msg.ChangeInfo4{},
			TargetCInfo: msg.ChangeInfo4{},
		},
	)
}

func (x *Compound) Remove(in, out Bytes) (uint32, error) {
	var args msg.REMOVE4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("REMOVE %s", args.Target)

	if args.Target == "" {
		return OperationResponse(out,
			msg.OP4_REMOVE,
			msg.NFS4ERR_INVAL,
		)
	}

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_REMOVE,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	path := vfs.Join(x.CurrentHandle.Path, args.Target)

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	err := fs.Remove(path)
	if err != nil && fs.Rmdir(path) == nil {
		err = nil
	}

	if err != nil {
		DiscardOnServerFault(fs, err)

		return OperationResponse(out,
			msg.OP4_REMOVE,
			msg.Err2Status(err),
		)
	}

	fs.Cache.Invalidate(x.CurrentHandle.Handle)

	return OperationResponse(out,
		msg.OP4_REMOVE,
		msg.NFS4_OK,
		msg.REMOVE4resok{
			CInfo: msg.ChangeInfo4{},
		},
	)
}

func (x *Compound) Link(in, out Bytes) (uint32, error) {
	var args msg.LINK4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("LINK %s", args.NewName)

	if args.NewName == "" {
		return OperationResponse(out,
			msg.OP4_LINK,
			msg.NFS4ERR_INVAL,
		)
	}

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_LINK,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	if x.SavedHandle == nil {
		return OperationResponse(out,
			msg.OP4_LINK,
			msg.NFS4ERR_RESTOREFH,
		)
	}

	newName := vfs.Join(x.CurrentHandle.Path, args.NewName)

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	if err := fs.Link(x.SavedHandle.Path, newName); err != nil {
		DiscardOnServerFault(fs, err)

		status := msg.Err2Status(err)

		if status == msg.NFS4ERR_NOTSUPP {
			status = msg.NFS4ERR_XDEV
		}

		return OperationResponse(out,
			msg.OP4_RENAME,
			status,
		)
	}

	fs.Cache.Invalidate(x.CurrentHandle.Handle)

	return OperationResponse(out,
		msg.OP4_RENAME,
		msg.NFS4_OK,
		msg.LINK4resok{
			CInfo: msg.ChangeInfo4{},
		},
	)
}

func (x *Compound) Readlink(in, out Bytes) (uint32, error) {
	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_READLINK,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	x.Logger.Trace("READLINK")

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	target, err := fs.Readlink(x.CurrentHandle.Path)
	if err != nil {
		DiscardOnServerFault(fs, err)

		return OperationResponse(out,
			msg.OP4_READLINK,
			msg.Err2Status(err),
		)
	}

	return OperationResponse(out,
		msg.OP4_READLINK,
		msg.NFS4_OK,
		msg.READLINK4resok{
			Link: target,
		},
	)
}

var SupportedWriteAttrs = []uint32{A_mode, A_size, A_owner, A_owner_group}

func (x *Compound) SetAttr(in, out Bytes) (uint32, error) { //nolint:funlen,gocognit
	var args msg.SETATTR4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("SETATTR %v", bitmapString(bitmap4Decode(args.Attrs.Mask)))

	if x.CurrentHandle == nil {
		return OperationResponse(out, msg.OP4_SETATTR, msg.NFS4ERR_NOFILEHANDLE)
	}

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	fs.Cache.Invalidate(x.CurrentHandle.Handle)

	decAttrs, argErr := decodeFAttrs4(args.Attrs)
	if argErr != nil {
		return OperationResponse(out, msg.OP4_SETATTR, msg.Err2Status(argErr))
	}

	changed := []uint32{}

	if decAttrs.Mode != nil {
		if err := fs.Chmod(x.CurrentHandle.Path, os.FileMode(*decAttrs.Mode)); err != nil {
			DiscardOnServerFault(fs, err)

			x.Logger.Warnf("failed to chmod: %v", err)

			return OperationResponse(out, msg.OP4_SETATTR, msg.Err2Status(err))
		}

		changed = append(changed, A_mode)
	}

	var uid, gid uint32

	if (decAttrs.Owner != "") != (decAttrs.OwnerGroup != "") {
		fi, argErr := fs.Lstat(x.CurrentHandle.Path)
		if argErr != nil {
			DiscardOnServerFault(fs, argErr)

			return OperationResponse(out, msg.OP4_SETATTR, msg.Err2Status(argErr))
		}

		uid = fi.Uid()
		gid = fi.Gid()
	}

	if decAttrs.Owner != "" {
		uid64, err := strconv.ParseUint(decAttrs.Owner, 10, 32)
		if err != nil {
			x.Logger.Warnf("failed to parse uid: %v", err)

			return OperationResponse(out, msg.OP4_SETATTR, msg.Err2Status(err))
		}

		uid = uint32(uid64)

		changed = append(changed, A_owner)
	}

	if decAttrs.OwnerGroup != "" {
		gid64, err := strconv.ParseUint(decAttrs.OwnerGroup, 10, 32)
		if err != nil {
			x.Logger.Warnf("failed to parse uid: %v", err)

			return OperationResponse(out, msg.OP4_SETATTR, msg.Err2Status(err))
		}

		gid = uint32(gid64)

		changed = append(changed, A_owner_group)
	}

	if decAttrs.Owner != "" || decAttrs.OwnerGroup != "" {
		if err := fs.Chown(x.CurrentHandle.Path, int(uid), int(gid)); err != nil {
			DiscardOnServerFault(fs, err)

			x.Logger.Warnf("failed to chown: %v", err)

			return OperationResponse(out, msg.OP4_SETATTR, msg.Err2Status(err))
		}
	}

	if decAttrs.Size != nil {
		if err := fs.Truncate(x.CurrentHandle.Path, int64(*decAttrs.Size)); err != nil {
			DiscardOnServerFault(fs, err)

			x.Logger.Warnf("failed to truncate: %v", err)

			return OperationResponse(out, msg.OP4_SETATTR, msg.Err2Status(err))
		}

		changed = append(changed, A_size)
	}

	mtime := clock.MustIncrement(clock.Now())

	if decAttrs.TimeMetadata != nil {
		mtime = time.Unix(int64(decAttrs.TimeMetadata.Seconds), int64(decAttrs.TimeMetadata.NSeconds))
	}

	if err := fs.Chtimes(x.CurrentHandle.Path, mtime, mtime); err != nil {
		DiscardOnServerFault(fs, err)

		x.Logger.Warnf("failed to set time: %v", err)

		return OperationResponse(out, msg.OP4_SETATTR, msg.Err2Status(err))
	}

	changed = append(changed, A_time_modify)

	if len(changed) == 0 {
		return OperationResponse(out, msg.OP4_SETATTR, msg.NFS4ERR_ATTRNOTSUPP)
	}

	return OperationResponse(out, msg.OP4_SETATTR, msg.NFS4_OK, changed)
}

func (x *Compound) Open(in, out Bytes) (uint32, error) { //nolint:funlen,gocognit,gocyclo
	var args msg.OPEN4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	if x.MinorVer > 0 {
		args.Owner.ClientId = clients.ClientIDFromSessionID(x.SessionID)
		args.SeqID = x.Slot.SequenceID*clients.MaxSlotID + x.Slot.SlotID
	}

	x.Logger.Tracef("OPEN %d %+v", args.SeqID, args)

	var (
		flag int
		mode = os.FileMode(0o644)
	)

	switch args.ShareAccess & msg.OPEN4_SHARE_ACCESS_BOTH {
	case msg.OPEN4_SHARE_ACCESS_READ:
		flag |= os.O_RDONLY
	case msg.OPEN4_SHARE_ACCESS_WRITE:
		flag |= os.O_WRONLY
	case msg.OPEN4_SHARE_ACCESS_BOTH:
		flag |= os.O_RDWR
	}

	if args.OpenHow.How == msg.OPEN4_CREATE {
		flag |= os.O_CREATE

		var decAttrs *Attr

		switch args.OpenHow.Claim.CreateMode {
		case msg.EXCLUSIVE4:
			// TODO: If file already exists and verifier matches, continue
			flag |= os.O_EXCL
		case msg.GUARDED4:
			flag |= os.O_EXCL

			var err error

			decAttrs, err = decodeFAttrs4(args.OpenHow.Claim.CreateAttrsGuarded)
			if err != nil {
				return 0, err
			}

			mode = os.FileMode(*decAttrs.Mode) & os.ModePerm

			if decAttrs.Size != nil && *decAttrs.Size == 0 {
				flag |= os.O_TRUNC
			}
		case msg.UNCHECKED4:
			var err error

			decAttrs, err = decodeFAttrs4(args.OpenHow.Claim.CreateAttrsUnchecked)
			if err != nil {
				return 0, err
			}

			mode = os.FileMode(*decAttrs.Mode) & os.ModePerm

		case msg.EXCLUSIVE4_1:
			// TODO: If file already exists and verifier matches, continue
			flag |= os.O_EXCL

			var err error

			decAttrs, err = decodeFAttrs4(args.OpenHow.Claim.CreateVerf41.Attrs)
			if err != nil {
				return 0, err
			}

			mode = os.FileMode(*decAttrs.Mode) & os.ModePerm

		default:
			return 0, fmt.Errorf("unsupported create mode: %d", args.OpenHow.Claim.CreateMode)
		}
	}

	path := x.CurrentHandle.Path

	switch args.OpenClaim.Claim {
	case msg.CLAIM_NULL:
		path = vfs.Join(path, args.OpenClaim.File)
	case msg.CLAIM_FH:
		// OK
	case msg.CLAIM_PREVIOUS, msg.CLAIM_DELEGATE_CUR, msg.CLAIM_DELEGATE_PREV, msg.CLAIM_DELEG_CUR_FH:
		return OperationResponse(out,
			msg.OP4_OPEN,
			msg.NFS4ERR_NOTSUPP,
		)
	default:
		return 0, fmt.Errorf("invalid claim: %v", args.OpenClaim.Claim)
	}

	if args.ShareDeny != 0 {
		return OperationResponse(out,
			msg.OP4_OPEN,
			msg.NFS4ERR_SHARE_DENIED,
		)
	}

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_OPEN,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	client, ok := x.Clients.Get(args.Owner.ClientId)
	if !ok {
		return OperationResponse(out,
			msg.OP4_OPEN,
			msg.NFS4ERR_STALE_CLIENTID,
		)
	}

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	// Check whether seqId is already an open file
	if fileID, ok := fs.GetFileByClientSeqID(client, args.SeqID); ok {
		return OperationResponse(out,
			msg.OP4_OPEN,
			msg.NFS4_OK,
			msg.OPEN4resok{
				StateId: msg.StateId4{
					SeqId: 1,
					Other: FileOther(fileID, args.SeqID),
				},
				CInfo:   msg.ChangeInfo4{},
				Rflags:  msg.OPEN4_RESULT_PRESERVE_UNLINKED,
				AttrSet: []uint32{A_mode},
			},
		)
	}

	fi, statErr := fs.Lstat(path)
	if statErr == nil && fi.IsDir() {
		return OperationResponse(out,
			msg.OP4_OPEN,
			msg.NFS4ERR_ISDIR,
		)
	}

	if statErr == nil && fi.Mode()&os.ModeSymlink == os.ModeSymlink {
		return OperationResponse(out,
			msg.OP4_OPEN,
			msg.NFS4ERR_SYMLINK,
		)
	}

	var f vfs.WriterAtReaderAt

	switch {
	case flag&os.O_WRONLY != 0:
		h, err := fs.FileWrite(path, flag)
		if err != nil {
			DiscardOnServerFault(fs, err)

			return OperationResponse(out,
				msg.OP4_OPEN,
				msg.Err2Status(err),
			)
		}

		if errors.Is(statErr, os.ErrNotExist) {
			fs.Chmod(path, mode) //nolint:errcheck
		}

		f = NopReaderAt(h)
	case flag&os.O_RDWR != 0:
		h, err := fs.OpenFile(path, flag, mode)
		if err != nil {
			DiscardOnServerFault(fs, err)

			return OperationResponse(out,
				msg.OP4_OPEN,
				msg.Err2Status(err),
			)
		}

		f = h
	default:
		h, err := fs.FileRead(path)
		if err != nil {
			DiscardOnServerFault(fs, err)

			return OperationResponse(out,
				msg.OP4_OPEN,
				msg.Err2Status(err),
			)
		}

		f = NopWriterAt(h)
	}

	handle := x.CurrentHandle.Handle

	var err error

	if args.OpenClaim.Claim != msg.CLAIM_FH {
		handle, err = fs.Handle(path)
	}

	if err != nil {
		DiscardOnServerFault(fs, err)

		if errors.Is(err, syscall.EOPNOTSUPP) {
			err = msg.Error(msg.NFS4ERR_FHEXPIRED)
		}

		defer f.Close()

		return OperationResponse(out,
			msg.OP4_OPEN,
			msg.Err2Status(err),
		)
	}

	if args.OpenHow.How == msg.OPEN4_CREATE {
		fs.Cache.Invalidate(x.CurrentHandle.Handle)
	} else if flag&os.O_RDWR != 0 || flag&os.O_WRONLY != 0 {
		fs.Cache.Invalidate(handle)
	}

	fileID := fs.AddFile(&worker.File{
		File:        f,
		Handle:      handle,
		Client:      client,
		ClientSeqID: args.SeqID,
	})

	x.CurrentHandle = &FileHandle{
		Handle: handle,
		Path:   path,
	}

	return OperationResponse(out,
		msg.OP4_OPEN,
		msg.NFS4_OK,
		msg.OPEN4resok{
			StateId: msg.StateId4{
				SeqId: 1,
				Other: FileOther(fileID, args.SeqID),
			},
			CInfo:   msg.ChangeInfo4{},
			Rflags:  0, // msg.OPEN4_RESULT_PRESERVE_UNLINKED (only supported if GetAttr continuous to work with Current Handle)
			AttrSet: []uint32{A_mode},
		},
	)
}

func (x *Compound) OpenDowngrade(in, out Bytes) (uint32, error) {
	var args msg.OPENDG4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("OPEN_DOWNGRADE %d", args.OpenStateId.Other[0])

	if args.OpenStateId.SeqId > 1 {
		return OperationResponse(out,
			msg.OP4_OPEN_DOWNGRADE,
			msg.NFS4ERR_BAD_SEQID,
		)
	}

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	if _, ok := fs.GetFile(FileID(args.OpenStateId.Other)); !ok {
		return OperationResponse(out,
			msg.OP4_OPEN_DOWNGRADE,
			msg.NFS4ERR_BAD_SEQID,
		)
	}

	return OperationResponse(out,
		msg.OP4_OPEN_DOWNGRADE,
		msg.NFS4_OK,
	)
}

func (x *Compound) Close(in, out Bytes) (uint32, error) { //nolint:funlen
	var (
		args msg.CLOSE4args
		err  error
	)
	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("CLOSE %d", args.OpenStateId.Other[0])

	if args.OpenStateId.SeqId > 1 {
		return OperationResponse(out,
			msg.OP4_CLOSE,
			msg.NFS4ERR_BAD_SEQID,
		)
	}

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_CLOSE,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	if fs.IsRemovedFile(FileID(args.OpenStateId.Other)) {
		return OperationResponse(out,
			msg.OP4_CLOSE,
			msg.NFS4_OK,
			msg.StateId4{
				SeqId: 2,
				Other: args.OpenStateId.Other,
			},
		)
	}

	f, ok := fs.RemoveFile(FileID(args.OpenStateId.Other))
	if !ok {
		return OperationResponse(out,
			msg.OP4_CLOSE,
			msg.NFS4ERR_BAD_SEQID,
		)
	}

	fs.Cache.Invalidate(f.Handle)

	err = f.File.Close()
	if err != nil {
		return OperationResponse(out,
			msg.OP4_CLOSE,
			msg.Err2Status(err),
		)
	}

	return OperationResponse(out,
		msg.OP4_CLOSE,
		msg.NFS4_OK,
		msg.StateId4{
			SeqId: 2,
			Other: args.OpenStateId.Other,
		},
	)
}

func (x *Compound) Read(in, out Bytes) (uint32, error) { //nolint:funlen
	var args msg.READ4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_READ,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	x.Logger.Tracef("READ %d %d %d", args.StateId.Other[0], args.Offset, args.Count)

	if args.StateId.SeqId > 1 {
		x.Logger.Warnf("bad seqid: %d", args.StateId.SeqId)

		return OperationResponse(out,
			msg.OP4_READ,
			msg.NFS4ERR_BAD_SEQID,
		)
	}

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	f, ok := fs.GetFile(FileID(args.StateId.Other))

	if ok && !bytes.Equal(f.Handle, x.CurrentHandle.Handle) {
		ok = false
	}

	if !ok {
		x.Logger.Warn("file handle is not equal to current handle")

		return OperationResponse(out,
			msg.OP4_READ,
			msg.NFS4ERR_BAD_SEQID,
		)
	}

	buf := bufpool.Get()

	b := buf.Allocate(int(args.Count))

	n, err := f.File.ReadAt(b, int64(args.Offset))
	if err != nil && !errors.Is(err, io.EOF) {
		x.Logger.Errorf("failed to read: %v", err)

		buf.Discard()

		return OperationResponse(out,
			msg.OP4_READ,
			msg.Err2Status(err),
		)
	}

	defer buf.Discard()

	return OperationResponse(out,
		msg.OP4_READ,
		msg.NFS4_OK,
		msg.READ4resok{
			Eof:  errors.Is(err, io.EOF),
			Data: b[:n],
		},
	)
}

func (x *Compound) Write(in, out Bytes) (uint32, error) { //nolint:funlen
	var args msg.WRITE4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("WRITE %d %d %d", args.StateId.Other[0], args.Offset, len(args.Data))

	if args.StateId.SeqId > 1 {
		return OperationResponse(out,
			msg.OP4_WRITE,
			msg.NFS4ERR_BAD_SEQID,
		)
	}

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_WRITE,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	f, ok := fs.GetFile(FileID(args.StateId.Other))

	if ok && !bytes.Equal(f.Handle, x.CurrentHandle.Handle) {
		ok = false
	}

	if !ok {
		return OperationResponse(out,
			msg.OP4_WRITE,
			msg.NFS4ERR_BAD_SEQID,
		)
	}

	// TODO: args.Stable is USTABLE4 | DATA_SYNC4 | FILE_SYNC4, we expect the underlying filesystem to handle syncing

	n, err := f.File.WriteAt(args.Data, int64(args.Offset))
	if err != nil {
		x.Logger.Errorf("failed to write: %v", err)

		return OperationResponse(out,
			msg.OP4_WRITE,
			msg.Err2Status(err),
		)
	}

	// Don't invalidate cache immediately while writing, unless FILE_SYNC4 is required
	if args.Stable == msg.FILE_SYNC4 {
		fs.Cache.Invalidate(f.Handle)
	}

	return OperationResponse(out,
		msg.OP4_WRITE,
		msg.NFS4_OK,
		msg.WRITE4resok{
			Count:     uint32(n),
			Committed: msg.FILE_SYNC4, // Pretend it's FILE_SYNC4 even if it's DATA_SYNC4 (TODO: why is this a problem?)
			WriteVerf: fs.SessionID,
		},
	)
}

func (x *Compound) Commit(in, out Bytes) (uint32, error) {
	var args msg.COMMIT4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("COMMIT %d %d", args.Offset, args.Count)

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	fs.Cache.Invalidate(x.CurrentHandle.Handle)

	return OperationResponse(out,
		msg.OP4_COMMIT,
		msg.NFS4_OK,
	)
}

func (x *Compound) Verify(in, out Bytes) (uint32, error) { //nolint:funlen
	var args msg.FAttr4

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("VERIFY")

	verifyAttrs := bitmap4Decode(args.Mask)
	for i, on := range verifyAttrs {
		if !on {
			continue
		}

		if !slices.Contains(AttrsSupported, i) {
			return OperationResponse(out,
				msg.OP4_VERIFY,
				msg.NFS4ERR_ATTRNOTSUPP,
			)
		}
	}

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_VERIFY,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	if fi, cached := fs.Cache.Get(x.CurrentHandle.Handle); cached {
		attrs := fileInfoToAttrs(x.CurrentHandle.Handle, fi, nil, verifyAttrs, x.Creds, fs.SessionID)

		if !bytes.Equal(attrs.Vals, args.Vals) {
			return OperationResponse(out,
				msg.OP4_VERIFY,
				msg.NFS4ERR_NOT_SAME,
			)
		}

		return OperationResponse(out,
			msg.OP4_VERIFY,
			msg.NFS4_OK,
		)
	}

	fi, err := fs.Lstat(x.CurrentHandle.Path)
	if err != nil {
		DiscardOnServerFault(fs, err)

		return OperationResponse(out,
			msg.OP4_VERIFY,
			msg.Err2Status(err),
		)
	}

	fs.Cache.Put(x.CurrentHandle.Handle, worker.Entry{
		Path:     x.CurrentHandle.Path,
		FileInfo: fi,
	})

	attrs := fileInfoToAttrs(x.CurrentHandle.Handle, fi, nil, verifyAttrs, x.Creds, fs.SessionID)

	if !bytes.Equal(attrs.Vals, args.Vals) {
		return OperationResponse(out,
			msg.OP4_VERIFY,
			msg.NFS4ERR_NOT_SAME,
		)
	}

	return OperationResponse(out,
		msg.OP4_VERIFY,
		msg.NFS4_OK,
	)
}

func (x *Compound) NVerify(in, out Bytes) (uint32, error) { //nolint:funlen
	var args msg.FAttr4

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("NVERIFY")

	verifyAttrs := bitmap4Decode(args.Mask)
	for i, on := range verifyAttrs {
		if !on {
			continue
		}

		if !slices.Contains(AttrsSupported, i) {
			return OperationResponse(out,
				msg.OP4_NVERIFY,
				msg.NFS4ERR_ATTRNOTSUPP,
			)
		}
	}

	if x.CurrentHandle == nil {
		return OperationResponse(out,
			msg.OP4_NVERIFY,
			msg.NFS4ERR_NOFILEHANDLE,
		)
	}

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	if fi, cached := fs.Cache.Get(x.CurrentHandle.Handle); cached {
		attrs := fileInfoToAttrs(x.CurrentHandle.Handle, fi, nil, verifyAttrs, x.Creds, fs.SessionID)

		if bytes.Equal(attrs.Vals, args.Vals) {
			return OperationResponse(out,
				msg.OP4_NVERIFY,
				msg.NFS4ERR_SAME,
			)
		}

		return OperationResponse(out,
			msg.OP4_NVERIFY,
			msg.NFS4_OK,
		)
	}

	fi, err := fs.Lstat(x.CurrentHandle.Path)
	if err != nil {
		DiscardOnServerFault(fs, err)

		return OperationResponse(out,
			msg.OP4_NVERIFY,
			msg.Err2Status(err),
		)
	}

	fs.Cache.Put(x.CurrentHandle.Handle, worker.Entry{
		Path:     x.CurrentHandle.Path,
		FileInfo: fi,
	})

	attrs := fileInfoToAttrs(x.CurrentHandle.Handle, fi, nil, verifyAttrs, x.Creds, fs.SessionID)

	if bytes.Equal(attrs.Vals, args.Vals) {
		return OperationResponse(out,
			msg.OP4_NVERIFY,
			msg.NFS4ERR_SAME,
		)
	}

	return OperationResponse(out,
		msg.OP4_NVERIFY,
		msg.NFS4_OK,
	)
}

func (x *Compound) GetXAttr(in, out Bytes) (uint32, error) {
	var args msg.GETXATTR4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("GETXATTR %s", args.Name)

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	var fi vfs.FileInfo

	fi, cached := fs.Cache.Get(x.CurrentHandle.Handle)
	if !cached {
		var err error

		fi, err = fs.Lstat(x.CurrentHandle.Path)
		if err != nil {
			DiscardOnServerFault(fs, err)

			return OperationResponse(out,
				msg.OP4_GETXATTR,
				msg.Err2Status(err),
			)
		}

		fs.Cache.Put(x.CurrentHandle.Handle, worker.Entry{
			Path:     x.CurrentHandle.Path,
			FileInfo: fi,
		})
	}

	attrs, err := fi.Extended()
	if err != nil {
		return OperationResponse(out,
			msg.OP4_GETXATTR,
			msg.Err2Status(err),
		)
	}

	if val, ok := attrs.Get(attrPrefix + args.Name); ok {
		return OperationResponse(out,
			msg.OP4_GETXATTR,
			msg.NFS4_OK,
			msg.GETXATTR4resok{
				Value: val,
			},
		)
	}

	return OperationResponse(out,
		msg.OP4_GETXATTR,
		msg.NFS4ERR_NOXATTR,
	)
}

const attrPrefix = "user."

func (x *Compound) SetXAttr(in, out Bytes) (uint32, error) {
	var args msg.SETXATTR4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("SETXATTR %s", args.Name)

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	fi, err := fs.Lstat(x.CurrentHandle.Path)
	if err != nil {
		DiscardOnServerFault(fs, err)

		return OperationResponse(out,
			msg.OP4_SETXATTR,
			msg.Err2Status(err),
		)
	}

	if err = fs.SetExtendedAttr(x.CurrentHandle.Path, attrPrefix+args.Name, args.Value); err != nil {
		return OperationResponse(out,
			msg.OP4_SETXATTR,
			msg.Err2Status(err),
		)
	}

	now := clock.MustIncrement(fi.ModTime())

	// Make sure changeID is updated
	fs.Chtimes(x.CurrentHandle.Path, now, now) //nolint:errcheck

	fs.Cache.Invalidate(x.CurrentHandle.Handle)

	return OperationResponse(out,
		msg.OP4_SETXATTR,
		msg.NFS4_OK,
		msg.SETXATTR4resok{
			CInfo: msg.ChangeInfo4{},
		},
	)
}

func (x *Compound) ListXAttrs(in, out Bytes) (uint32, error) { //nolint:funlen
	var args msg.LISTXATTRS4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("LISTXATTRS")

	if args.Cookie > 0 {
		return OperationResponse(out, msg.OP4_LISTXATTRS, msg.NFS4_OK, msg.LISTXATTRS4resok{
			Cookie: 1,
			EOF:    true,
		})
	}

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	var fi vfs.FileInfo

	fi, cached := fs.Cache.Get(x.CurrentHandle.Handle)
	if !cached {
		var err error

		fi, err = fs.Lstat(x.CurrentHandle.Path)
		if err != nil {
			DiscardOnServerFault(fs, err)

			return OperationResponse(out,
				msg.OP4_LISTXATTRS,
				msg.Err2Status(err),
			)
		}

		fs.Cache.Put(x.CurrentHandle.Handle, worker.Entry{
			Path:     x.CurrentHandle.Path,
			FileInfo: fi,
		})
	}

	var names []string

	attrs, err := fi.Extended()
	if err != nil {
		return OperationResponse(out,
			msg.OP4_LISTXATTRS,
			msg.Err2Status(err),
		)
	}

	for name := range attrs {
		names = append(names, strings.TrimPrefix(name, attrPrefix))
	}

	return OperationResponse(out,
		msg.OP4_LISTXATTRS,
		msg.NFS4_OK,
		msg.LISTXATTRS4resok{
			Cookie: 1,
			Names:  names,
			EOF:    true,
		},
	)
}

func (x *Compound) RemoveXAttr(in, out Bytes) (uint32, error) {
	var args msg.REMOVEXATTR4args

	if err := xdr.NewDecoder(in).Decode(&args); err != nil {
		return 0, err
	}

	x.Logger.Tracef("REMOVEXATTR %s", args.Name)

	fs := x.FS(x.Creds, x.SessionID)

	defer fs.Close()

	fi, err := fs.Lstat(x.CurrentHandle.Path)
	if err != nil {
		DiscardOnServerFault(fs, err)

		return OperationResponse(out,
			msg.OP4_REMOVEXATTR,
			msg.Err2Status(err),
		)
	}

	if err = fs.UnsetExtendedAttr(x.CurrentHandle.Path, attrPrefix+args.Name); err != nil {
		return OperationResponse(out,
			msg.OP4_REMOVEXATTR,
			msg.Err2Status(err),
		)
	}

	now := clock.MustIncrement(fi.ModTime())

	// Make sure changeID is updated
	// TODO: incorporate in other way in changeid
	fs.Chtimes(x.CurrentHandle.Path, now, now) //nolint:errcheck

	fs.Cache.Invalidate(x.CurrentHandle.Handle)

	return OperationResponse(out,
		msg.OP4_REMOVEXATTR,
		msg.NFS4_OK,
		msg.REMOVEXATTR4resok{
			CInfo: msg.ChangeInfo4{},
		},
	)
}

func DiscardOnServerFault(fs interface{ Discard() }, err error) {
	if err == nil {
		return
	}

	for _, e := range NonFatalErrors {
		if errors.Is(err, e) {
			return
		}
	}

	logger.Logger.Warnf("discarding FS because of error: %v", err)
	fs.Discard()
}

// NonFatalErrors are errors that are not marked as system failures
// These are also passed correctly through the SFTP file system.
var NonFatalErrors = []error{
	os.ErrNotExist,
	os.ErrExist,
	os.ErrPermission,
	os.ErrInvalid,
	syscall.EISDIR,
	syscall.ENOTDIR,
	syscall.EOPNOTSUPP,
	io.EOF,
}
