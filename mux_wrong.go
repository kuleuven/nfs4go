package nfs4go

import (
	"github.com/kuleuven/nfs4go/msg"
	"github.com/kuleuven/nfs4go/xdr"
)

type MuxMismatch struct{}

func (x *MuxMismatch) Handle(request Request, response chan<- Response) {
	reply, data, err := x.HandleProc(request.Header, request.Data)

	response <- Response{
		Reply: reply,
		Data:  data,
		Error: err,
	}
}

func (x *MuxMismatch) HandleProc(header *msg.RPCMsgCall, data Bytes) (*msg.RPCMsgReply, Bytes, error) {
	seq := []interface{}{
		msg.Auth{},
		msg.ACCEPT_PROG_MISMATCH,
		uint32(4), // low:  v4
		uint32(4), // high: v4
	}

	data.Reset()

	err := xdr.NewEncoder(data).EncodeAll(seq...)

	return &msg.RPCMsgReply{
		Xid:       header.Xid,
		MsgType:   msg.RPC_REPLY,
		ReplyStat: msg.MSG_ACCEPTED,
	}, data, err
}
