package nfs4go

func FileOther(fileID uint64, clientSeqID uint32) [3]uint32 {
	return [3]uint32{uint32(fileID >> 32), uint32(fileID), clientSeqID}
}

func FileID(fileOther [3]uint32) uint64 {
	return uint64(fileOther[0])<<32 + uint64(fileOther[1])
}
