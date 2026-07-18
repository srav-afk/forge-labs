package cacheregistry

import "testing"

func TestLongestConsecutivePrefix(t *testing.T) {
	want := []uint64{1, 2, 3, 4}
	have := map[uint64]float64{1: 1, 2: 1, 3: 1}
	n, score := LongestConsecutivePrefix(want, have)
	if n != 3 || score != float64(3*DefaultBlockSize) {
		t.Fatalf("n=%d score=%v", n, score)
	}
	// gap at h1: only first block counts
	haveGap := map[uint64]float64{1: 1, 3: 1}
	n, _ = LongestConsecutivePrefix(want, haveGap)
	if n != 1 {
		t.Fatalf("gap should break chain, n=%d", n)
	}
}

func TestHashBlocksChained(t *testing.T) {
	ids := make([]uint32, 300)
	for i := range ids {
		ids[i] = uint32(i)
	}
	a := HashBlocks(ids, 256, "")
	b := HashBlocks(ids, 256, "lora")
	if len(a) != 2 {
		t.Fatalf("len=%d", len(a))
	}
	if a[0] == b[0] {
		t.Fatal("adapter must namespace hashes")
	}
}

func TestHashPromptStable(t *testing.T) {
	h1 := HashPromptApprox("system: shared\nuser: hi", 64, "")
	h2 := HashPromptApprox("system: shared\nuser: hi", 64, "")
	if len(h1) == 0 || h1[0] != h2[0] {
		t.Fatalf("%v %v", h1, h2)
	}
}
