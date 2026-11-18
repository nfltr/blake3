//go:build arm64 && !purego

package guts

import "unsafe"

//go:generate go run gen_arm64.go

// compressChunksNEON compresses 4 whole chunks of buf, one per NEON lane,
// with counters ctr (low words, then high words) and per-block flags,
// into cvs; msg is its scratch for a block's transposed message.
//
//go:noescape
func compressChunksNEON(cvs *[4][8]uint32, buf *[4 * ChunkSize]byte, key *[8]uint32, ctr *[8]uint32, flags *[16]uint32, msg *[64]uint32)

// CompressBuffer compresses up to MaxSIMD chunks in parallel and returns their
// root node.
func CompressBuffer(buf *[MaxSIMD * ChunkSize]byte, buflen int, key *[8]uint32, counter uint64, flags uint32) Node {
	if buflen < 4*ChunkSize {
		return compressBufferGeneric(buf, buflen, key, counter, flags)
	}
	var cvs [MaxSIMD][8]uint32
	var fl [16]uint32
	for i := range fl {
		fl[i] = flags
	}
	fl[0] |= FlagChunkStart
	fl[15] |= FlagChunkEnd
	var msg [64]uint32
	numCVs := uint64(buflen / ChunkSize)
	i := uint64(0)
	for ; i+4 <= numCVs; i += 4 {
		var ctr [8]uint32
		for j := range uint64(4) {
			ctr[j], ctr[4+j] = uint32(counter+i+j), uint32((counter+i+j)>>32)
		}
		compressChunksNEON((*[4][8]uint32)(cvs[i:]), (*[4 * ChunkSize]byte)(buf[i*ChunkSize:]), key, &ctr, &fl, &msg)
	}
	for ; i < numCVs; i++ {
		cvs[i] = ChainingValue(CompressChunk(buf[i*ChunkSize:(i+1)*ChunkSize], key, counter+i, flags))
	}
	if buflen%ChunkSize != 0 {
		cvs[numCVs] = ChainingValue(CompressChunk(buf[numCVs*ChunkSize:buflen], key, counter+numCVs, flags))
		numCVs++
	}
	return mergeSubtrees(&cvs, numCVs, key, flags)
}

// compressBlocksNEON compresses 4 single blocks of buf, one per lane, with
// counters ctr and flags[0], into cvs.
//
//go:noescape
func compressBlocksNEON(cvs *[4][8]uint32, buf *[4 * BlockSize]byte, key *[8]uint32, ctr *[8]uint32, flags *[16]uint32, msg *[64]uint32)

// mergeSubtrees merges numCVs chaining values into their root node, 4
// parents at a time while a level has 4 or more.
func mergeSubtrees(cvs *[MaxSIMD][8]uint32, numCVs uint64, key *[8]uint32, flags uint32) Node {
	fl := [16]uint32{flags | FlagParent}
	var ctr [8]uint32
	var msg [64]uint32
	for numCVs >= 8 {
		rem := numCVs / 2
		i := uint64(0)
		for ; i+4 <= rem; i += 4 {
			// parents i..i+3 are the pairs at cvs[2i:2i+8]; they are read before cvs[i:i+4] is written
			compressBlocksNEON((*[4][8]uint32)(cvs[i:]), (*[4 * BlockSize]byte)(unsafe.Pointer(&cvs[2*i])), key, &ctr, &fl, &msg)
		}
		for ; i < rem; i++ {
			cvs[i] = ChainingValue(ParentNode(cvs[2*i], cvs[2*i+1], key, flags))
		}
		if numCVs%2 != 0 {
			cvs[rem] = cvs[rem*2]
			rem++
		}
		numCVs = rem
	}
	return mergeSubtreesGeneric(cvs, numCVs, key, flags)
}
