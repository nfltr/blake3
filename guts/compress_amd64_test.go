package guts

import "testing"

// TestCompressSubtreeAVX2 runs the differential test on the AVX2 path too
// on a machine with AVX-512 (CompressSubtree's chunk and parent paths
// differ per width).
func TestCompressSubtreeAVX2(t *testing.T) {
	if !haveAVX512 || !haveAVX2 {
		t.Skip("needs AVX-512 (to switch it off) and AVX2")
	}
	haveAVX512 = false
	defer func() { haveAVX512 = true }()
	TestCompressSubtreeDifferential(t)
	TestCompressBufferDifferential(t)
}
