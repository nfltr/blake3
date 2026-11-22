package guts

import (
	"math/bits"
	"math/rand"
	"testing"
)

var testFlags = []uint32{0, FlagKeyedHash, FlagDeriveKeyContext, FlagDeriveKeyMaterial}

// compressBufferReference is CompressBuffer with no assembly at all:
// compressBufferGeneric calls mergeSubtrees, which is assembly on arm64 and
// amd64, so it cannot be the reference for either.
func compressBufferReference(buf *[MaxSIMD * ChunkSize]byte, buflen int, key *[8]uint32, counter uint64, flags uint32) Node {
	if buflen <= ChunkSize {
		return CompressChunk(buf[:buflen], key, counter, flags)
	}
	var cvs [MaxSIMD][8]uint32
	numCVs := uint64(0)
	for off := 0; off < buflen; off += ChunkSize {
		cvs[numCVs] = ChainingValue(CompressChunk(buf[off:min(off+ChunkSize, buflen)], key, counter+numCVs, flags))
		numCVs++
	}
	return mergeSubtreesGeneric(&cvs, numCVs, key, flags)
}

// checkBuffer compares CompressBuffer (assembly where the platform has it)
// with the fully generic code for one input.
func checkBuffer(t *testing.T, buf *[MaxSIMD * ChunkSize]byte, buflen int, key *[8]uint32, counter uint64, flags uint32) {
	t.Helper()
	if got, want := CompressBuffer(buf, buflen, key, counter, flags), compressBufferReference(buf, buflen, key, counter, flags); got != want {
		t.Fatalf("CompressBuffer(buflen %d, counter %#x, flags %#x) = %v, want %v", buflen, counter, flags, got, want)
	}
}

func TestCompressBufferDifferential(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var buf [MaxSIMD * ChunkSize]byte
	for i := 0; i < 3000; i++ {
		rng.Read(buf[:])
		key := IV
		if rng.Intn(2) == 0 {
			for j := range key {
				key[j] = rng.Uint32()
			}
		}
		buflen := rng.Intn(len(buf) + 1)
		if i < len(buf)/ChunkSize*4 {
			buflen = (i/4 + 1) * ChunkSize // every whole-chunk count
		}
		counter := rng.Uint64() >> uint(rng.Intn(64))
		if i%7 == 0 {
			counter = 1<<32 - uint64(rng.Intn(MaxSIMD)) // lanes straddle the high word
		}
		checkBuffer(t, &buf, buflen, &key, counter, testFlags[rng.Intn(len(testFlags))])
	}
}

// TestMergeSubtreesDifferential checks mergeSubtrees (assembly where the
// platform has it) against mergeSubtreesGeneric for every CV count.
func TestMergeSubtreesDifferential(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for numCVs := uint64(1); numCVs <= MaxSIMD; numCVs++ {
		for _, flags := range testFlags {
			var cvs [MaxSIMD][8]uint32
			for i := range cvs {
				for j := range cvs[i] {
					cvs[i][j] = rng.Uint32()
				}
			}
			key := IV
			key[0] = rng.Uint32()
			ref := cvs
			if got, want := mergeSubtrees(&cvs, numCVs, &key, flags), mergeSubtreesGeneric(&ref, numCVs, &key, flags); got != want {
				t.Fatalf("mergeSubtrees(numCVs %d, flags %#x) = %v, want %v", numCVs, flags, got, want)
			}
		}
	}
}

func FuzzCompressBuffer(f *testing.F) {
	f.Add([]byte("abc"), uint64(0), uint8(0), uint32(0))
	f.Add(make([]byte, MaxSIMD*ChunkSize), uint64(1<<32-2), uint8(1), uint32(7))
	f.Fuzz(func(t *testing.T, in []byte, counter uint64, fl uint8, keySeed uint32) {
		var buf [MaxSIMD * ChunkSize]byte
		buflen := copy(buf[:], in)
		if len(in) > 0 && len(in) < 4*ChunkSize {
			// stretch short inputs so the vector path runs on them too
			for buflen < 4*ChunkSize+int(in[0])%(len(buf)-4*ChunkSize) {
				buflen += copy(buf[buflen:], in)
			}
		}
		key := IV
		if keySeed != 0 {
			rng := rand.New(rand.NewSource(int64(keySeed)))
			for j := range key {
				key[j] = rng.Uint32()
			}
		}
		checkBuffer(t, &buf, buflen, &key, counter, testFlags[int(fl)%len(testFlags)])
	})
}

func BenchmarkCompressBuffer(b *testing.B) {
	var buf [MaxSIMD * ChunkSize]byte
	b.SetBytes(int64(len(buf)))
	for i := 0; i < b.N; i++ {
		CompressBuffer(&buf, len(buf), &IV, 0, 0)
	}
}

// subtreeReference is CompressSubtree with no assembly and BLAKE3's own
// tree shape: each chunk on its own, then the left subtree taking the
// largest power of two of the chunks short of all.
func subtreeReference(buf []byte, key *[8]uint32, counter uint64, flags uint32) Node {
	if len(buf) <= ChunkSize {
		return CompressChunk(buf, key, counter, flags)
	}
	var cvs [][8]uint32
	for off := 0; off < len(buf); off += ChunkSize {
		cvs = append(cvs, ChainingValue(CompressChunk(buf[off:min(off+ChunkSize, len(buf))], key, counter+uint64(len(cvs)), flags)))
	}
	var rec func(cvs [][8]uint32) Node
	rec = func(cvs [][8]uint32) Node {
		mid := 1 << (bits.Len(uint(len(cvs)-1)) - 1)
		l, r := cvs[:mid], cvs[mid:]
		lcv, rcv := l[0], r[0]
		if len(l) > 1 {
			lcv = ChainingValue(rec(l))
		}
		if len(r) > 1 {
			rcv = ChainingValue(rec(r))
		}
		return ParentNode(lcv, rcv, key, flags)
	}
	return rec(cvs)
}

func checkSubtree(t *testing.T, buf []byte, key *[8]uint32, counter uint64, flags uint32) {
	t.Helper()
	if got, want := CompressSubtree(buf, key, counter, flags), subtreeReference(buf, key, counter, flags); got != want {
		t.Fatalf("CompressSubtree(len %d, counter %#x, flags %#x) = %v, want %v", len(buf), counter, flags, got, want)
	}
}

// TestCompressSubtreeDifferential compares CompressSubtree (assembly where
// the platform has it) with subtreeReference: every whole-chunk count up to
// MaxSubtree, random lengths, keys, flags and counters across 2^32.
func TestCompressSubtreeDifferential(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	buf := make([]byte, MaxSubtree)
	for i := 0; i < 1500; i++ {
		rng.Read(buf)
		key := IV
		if rng.Intn(2) == 0 {
			for j := range key {
				key[j] = rng.Uint32()
			}
		}
		n := 1 + rng.Intn(len(buf))
		if i < MaxSubtree/ChunkSize {
			n = (i + 1) * ChunkSize // every whole-chunk count
		}
		counter := rng.Uint64() >> uint(rng.Intn(64))
		if i%7 == 0 {
			counter = 1<<32 - uint64(rng.Intn(2*MaxSIMD)) // lanes straddle the high word
		}
		checkSubtree(t, buf[:n], &key, counter, testFlags[rng.Intn(len(testFlags))])
	}
}

func FuzzCompressSubtree(f *testing.F) {
	f.Add([]byte("abc"), uint16(0), uint64(0), uint8(0))
	f.Add(make([]byte, ChunkSize), uint16(17*ChunkSize+5), uint64(1<<32-3), uint8(1))
	f.Fuzz(func(t *testing.T, in []byte, n uint16, counter uint64, fl uint8) {
		if len(in) == 0 {
			return
		}
		// stretch in to n bytes (at most 64 KiB) so whole SIMD windows and
		// their remainders both run
		buf := make([]byte, max(len(in), int(n)))
		for off := 0; off < len(buf); off += copy(buf[off:], in) {
		}
		checkSubtree(t, buf, &IV, counter, testFlags[int(fl)%len(testFlags)])
	})
}

func BenchmarkCompressSubtree(b *testing.B) {
	buf := make([]byte, MaxSubtree)
	b.SetBytes(int64(len(buf)))
	for i := 0; i < b.N; i++ {
		CompressSubtree(buf, &IV, 0, 0)
	}
}
