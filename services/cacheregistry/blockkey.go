package cacheregistry

import (
	"encoding/binary"
	"hash/fnv"
)

const DefaultBlockSize = 256

func ChunkTokens(tokenIDs []uint32, blockSize int) [][]uint32 {
	if blockSize <= 0 {
		blockSize = DefaultBlockSize
	}
	if len(tokenIDs) == 0 {
		return nil
	}
	out := make([][]uint32, 0, (len(tokenIDs)+blockSize-1)/blockSize)
	for i := 0; i < len(tokenIDs); i += blockSize {
		j := i + blockSize
		if j > len(tokenIDs) {
			j = len(tokenIDs)
		}
		out = append(out, tokenIDs[i:j])
	}
	return out
}

func HashBlocks(tokenIDs []uint32, blockSize int, adapter string) []uint64 {
	chunks := ChunkTokens(tokenIDs, blockSize)
	out := make([]uint64, 0, len(chunks))
	var parent uint64
	for i, ch := range chunks {
		h := fnv.New64a()
		_, _ = h.Write([]byte(adapter))
		_ = binary.Write(h, binary.LittleEndian, parent)
		_ = binary.Write(h, binary.LittleEndian, uint32(len(ch)))
		for _, t := range ch {
			_ = binary.Write(h, binary.LittleEndian, t)
		}
		key := h.Sum64()
		out = append(out, key)
		parent = key
		_ = i
	}
	return out
}

func HashPromptApprox(prompt string, blockSize int, adapter string) []uint64 {
	if prompt == "" {
		return nil
	}
	ids := make([]uint32, 0, len(prompt))
	for _, r := range prompt {
		ids = append(ids, uint32(r))
	}
	return HashBlocks(ids, blockSize, adapter)
}

func LongestConsecutivePrefix(want []uint64, have map[uint64]float64) (blocks int, score float64) {
	for i, h := range want {
		w, ok := have[h]
		if !ok {
			break
		}
		blocks++
		if w <= 0 {
			w = 1.0
		}
		score += float64(DefaultBlockSize) * w
		_ = i
	}
	return blocks, score
}
