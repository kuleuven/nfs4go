package nfs4go

import (
	"errors"
	"fmt"
	"io"

	"github.com/kuleuven/nfs4go/bufpool"
	"github.com/kuleuven/nfs4go/msg"
	"github.com/kuleuven/nfs4go/xdr"
)

func ReceiveCall(r io.Reader) (*msg.RPCMsgCall, Bytes, error) {
	decoder := xdr.NewDecoder(r)

	frag, err := decoder.Uint32()
	if err != nil {
		return nil, nil, err
	}

	if frag&(1<<31) == 0 {
		return nil, nil, errors.New("(!)ignored: fragmented request")
	}

	headerSize := (frag << 1) >> 1
	restSize := int(headerSize)

	header := &msg.RPCMsgCall{}

	size, err := decoder.Read(header)
	if err != nil {
		return nil, nil, fmt.Errorf("ReadAs(%T): %v", header, err)
	}

	restSize -= size

	if header.MsgType != msg.RPC_CALL {
		return nil, nil, errors.New("expecting a rpc call message")
	}

	buf := bufpool.Get()

	data := buf.Allocate(restSize)
	n, err := io.ReadFull(r, data)

	buf.Commit(n)

	return header, buf, err
}

func SendReply(w io.Writer, reply *msg.RPCMsgReply, data Bytes) error {
	defer data.Discard()

	payload := data.Bytes()
	encoder := xdr.NewEncoder(w)
	length := 12 + len(payload)
	frag := uint32(length) | uint32(1<<31)

	if err := encoder.Uint32(frag); err != nil {
		return err
	}

	if err := encoder.Encode(reply); err != nil {
		return err
	}

	if _, err := w.Write(payload); err != nil {
		return err
	}

	return nil
}
