package guts

// MaxSubtree is the most bytes CompressSubtree takes: 256 chunks, a Bao
// chunk group of 2^8 chunks.
const MaxSubtree = 256 * ChunkSize

// CompressSubtree returns the root node of the BLAKE3 subtree over buf
// (0 < len(buf) <= MaxSubtree) whose first chunk is chunk counter: every
// chunk's chaining value first, a SIMD width at a time, then the parents
// a level at a time, so every SIMD lane of a parent compression has a
// parent to hash (CompressBuffer merges each MaxSIMD chunks on their own,
// half or more of its lanes idle in the levels above 8 parents). Pairing
// a level's neighbours and carrying an odd last one up is the same tree
// as BLAKE3's (the left subtree the largest power of two of the chunks).
func CompressSubtree(buf []byte, key *[8]uint32, counter uint64, flags uint32) Node {
	if len(buf) <= ChunkSize {
		return CompressChunk(buf, key, counter, flags)
	}
	if len(buf) > MaxSubtree {
		panic("guts: CompressSubtree of more than MaxSubtree bytes")
	}
	var cvs [MaxSubtree / ChunkSize][8]uint32
	n := len(buf) / ChunkSize
	chunkCVs(cvs[:n], buf[:n*ChunkSize], key, counter, flags)
	if len(buf)%ChunkSize != 0 {
		cvs[n] = ChainingValue(CompressChunk(buf[n*ChunkSize:], key, counter+uint64(n), flags))
		n++
	}
	for n > 2 {
		half := n / 2
		parentCVs(cvs[:2*half], key, flags)
		if n%2 != 0 {
			cvs[half] = cvs[n-1]
			half++
		}
		n = half
	}
	return ParentNode(cvs[0], cvs[1], key, flags)
}

// chunkCVsGeneric writes the chaining value of each whole chunk of buf
// (len(cvs) of them, the first chunk counter) into cvs.
func chunkCVsGeneric(cvs [][8]uint32, buf []byte, key *[8]uint32, counter uint64, flags uint32) {
	for i := range cvs {
		cvs[i] = ChainingValue(CompressChunk(buf[i*ChunkSize:(i+1)*ChunkSize], key, counter+uint64(i), flags))
	}
}

// parentCVsGeneric replaces cvs[i] with the parent of cvs[2i] and
// cvs[2i+1], for each of the len(cvs)/2 pairs.
func parentCVsGeneric(cvs [][8]uint32, key *[8]uint32, flags uint32) {
	for i := 0; i < len(cvs)/2; i++ {
		cvs[i] = ChainingValue(ParentNode(cvs[2*i], cvs[2*i+1], key, flags))
	}
}
