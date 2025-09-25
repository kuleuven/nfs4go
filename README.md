# NFS v4 server in go

This package entails a server implementation for NFS v4 in pure go. It is heavily based on the works of <https://github.com/smallfz/libnfs-go> and allows to expose a virtual file system <https://github.com/kuleuven/vfs> over NFS v4.

Protocols v4.0, v4.1 and v4.2 are supported. RFC 7530, RFC 5661 and RFC 8276 are largely implemented. The current implementation has minimal server state, only a list of active clients is kept. No file locking is supported. The implemented authentication mechanism is `AUTH_FLAVOR_UNIX`, so that the client sends uid/gid/groups information to the server. It is possible to provide each user a different virtual file system.

The following operations are required by the RFCs but we didn't implement them:

* `OP4_BACKCHANNEL_CTL`
* `OP4_BIND_CONN_TO_SESSION`
* `OP4_FREE_STATEID`
* `OP4_ILLEGAL`
* `OP4_LOCK`
* `OP4_LOCKT`
* `OP4_LOCKU`
* `OP4_SET_SSV`
* `OP4_TEST_STATEID`

The following operations are optional by the RFCs and we didn't implement them:

* `OP4_ALLOCATE`
* `OP4_CLONE`
* `OP4_COPY`
* `OP4_COPY_NOTIFY`
* `OP4_DEALLOCATE`
* `OP4_DELEGPURGE`
* `OP4_DELEGRETURN`
* `OP4_GETDEVICEINFO`
* `OP4_GET_DIR_DELEGATION`
* `OP4_IO_ADVISE`
* `OP4_LAYOUTCOMMIT`
* `OP4_LAYOUTERROR`
* `OP4_LAYOUTGET`
* `OP4_LAYOUTRETURN`
* `OP4_LAYOUTSTATS`
* `OP4_OFFLOAD_CANCEL`
* `OP4_OFFLOAD_STATUS`
* `OP4_OPENATTR`
* `OP4_READ_PLUS`
* `OP4_SEEK`
* `OP4_WANT_DELEGATION`
* `OP4_WRITE_SAME`

## Implementation details

* `clock` provides a global clock used whenever the current time is needed. It caches `time.Now()` since it is expensive to call frequently.
* `logger` provides a global logger that can be adapted for different logging needs.
* `xdr` handles decoding and encoding of the NFS protocol messages.
* `msg` contains the definitions of the NFS protocol messages.
* `bufpool` manages a pool of buffers for efficient memory allocation.
* `clients` manages the state of all NFS clients. A client can have one or multiple sessions. In case of NFS v4.0, we map a client ip to a single session.
* `worker` manages the combination of a session and user credentials, and maps it to a single virtual file system and state (open files). If a worker is idle for 5 minutes, it will be discarded and the virtual file system will be closed.

## Usage

See `cmd/nfs4go/example.go` for an example exposing /srv over NFS v4.