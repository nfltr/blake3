package guts

import (
	"math/rand"
	"testing"
)

var testFlags = []uint32{0, FlagKeyedHash, FlagDeriveKeyContext, FlagDeriveKeyMaterial}

// checkBuffer compares CompressBuffer (assembly where the platform has it)
// with the generic code for one input.
func checkBuffer(t *testing.T, buf *[MaxSIMD * ChunkSize]byte, buflen int, key *[8]uint32, counter uint64, flags uint32) {
	t.Helper()
	if got, want := CompressBuffer(buf, buflen, key, counter, flags), compressBufferGeneric(buf, buflen, key, counter, flags); got != want {
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
