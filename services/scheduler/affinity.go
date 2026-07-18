package scheduler

import (
	"encoding/binary"

	"github.com/cespare/xxhash/v2"
)

func PrefixKey(prompt string, window, blockBytes int) uint64 {
	if blockBytes <= 0 {
		blockBytes = 64
	}
	if window <= 0 {
		window = 1024
	}
	n := window
	if len(prompt) < n {
		n = len(prompt)
	}
	n -= n % blockBytes
	if n <= 0 {
		if prompt == "" {
			return 0
		}
		return xxhash.Sum64String(prompt)
	}
	return xxhash.Sum64String(prompt[:n])
}

func HRWPick(key uint64, workerIDs []string) string {
	if len(workerIDs) == 0 {
		return ""
	}
	var best string
	var bestScore uint64
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], key)
	for i, w := range workerIDs {
		h := xxhash.New()
		_, _ = h.Write(buf[:])
		_, _ = h.WriteString(w)
		s := h.Sum64()
		if i == 0 || s > bestScore || (s == bestScore && w < best) {
			bestScore, best = s, w
		}
	}
	return best
}
