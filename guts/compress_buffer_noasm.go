//go:build !amd64 && (!arm64 || purego)

package guts

// CompressBuffer compresses up to MaxSIMD chunks in parallel and returns their
// root node.
func CompressBuffer(buf *[MaxSIMD * ChunkSize]byte, buflen int, key *[8]uint32, counter uint64, flags uint32) Node {
	return compressBufferGeneric(buf, buflen, key, counter, flags)
}

func mergeSubtrees(cvs *[MaxSIMD][8]uint32, numCVs uint64, key *[8]uint32, flags uint32) Node {
	return mergeSubtreesGeneric(cvs, numCVs, key, flags)
}

func chunkCVs(cvs [][8]uint32, buf []byte, key *[8]uint32, counter uint64, flags uint32) {
	chunkCVsGeneric(cvs, buf, key, counter, flags)
}

func parentCVs(cvs [][8]uint32, key *[8]uint32, flags uint32) {
	parentCVsGeneric(cvs, key, flags)
}
