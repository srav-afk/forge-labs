package cacheregistry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type BlockLocation struct {
	WorkerID string
	Tier     string
	Weight   float64
}

type Index struct {
	mu       sync.RWMutex
	byBlock  map[string]map[uint64]map[string]BlockLocation
	byWorker map[string]map[uint64]float64
	rdb      *redis.Client
	ttl      time.Duration
}

func NewIndex(rdb *redis.Client) *Index {
	return &Index{
		byBlock:  map[string]map[uint64]map[string]BlockLocation{},
		byWorker: map[string]map[uint64]float64{},
		rdb:      rdb,
		ttl:      60 * time.Second,
	}
}

func ns(base, adapter string) string {
	if adapter == "" {
		return base
	}
	return base + "#" + adapter
}

func (idx *Index) Store(base, adapter, workerID, tier string, hashes []uint64) {
	if workerID == "" || len(hashes) == 0 {
		return
	}
	weight := 1.0
	if tier == "cpu" {
		weight = 0.8
	}
	key := ns(base, adapter)
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.byBlock[key] == nil {
		idx.byBlock[key] = map[uint64]map[string]BlockLocation{}
	}
	if idx.byWorker[workerID] == nil {
		idx.byWorker[workerID] = map[uint64]float64{}
	}
	for _, h := range hashes {
		if idx.byBlock[key][h] == nil {
			idx.byBlock[key][h] = map[string]BlockLocation{}
		}
		idx.byBlock[key][h][workerID] = BlockLocation{WorkerID: workerID, Tier: tier, Weight: weight}
		idx.byWorker[workerID][h] = weight
	}
}

func (idx *Index) Remove(workerID string, hashes []uint64) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	for _, h := range hashes {
		delete(idx.byWorker[workerID], h)
		for _, m := range idx.byBlock {
			if locs, ok := m[h]; ok {
				delete(locs, workerID)
			}
		}
	}
}

func (idx *Index) ClearWorker(workerID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	held := idx.byWorker[workerID]
	delete(idx.byWorker, workerID)
	for h := range held {
		for _, m := range idx.byBlock {
			if locs, ok := m[h]; ok {
				delete(locs, workerID)
			}
		}
	}
}

func (idx *Index) WorkerBlocks(workerID string) map[uint64]float64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	src := idx.byWorker[workerID]
	out := make(map[uint64]float64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func (idx *Index) SnapshotForScoring() map[string]map[uint64]float64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make(map[string]map[uint64]float64, len(idx.byWorker))
	for w, blocks := range idx.byWorker {
		cp := make(map[uint64]float64, len(blocks))
		for k, v := range blocks {
			cp[k] = v
		}
		out[w] = cp
	}
	return out
}

func (idx *Index) Persist(ctx context.Context, base, adapter, workerID string, hashes []uint64) {
	if idx.rdb == nil {
		return
	}
	pipe := idx.rdb.Pipeline()
	for _, h := range hashes {
		key := fmt.Sprintf("forge:cache:%s:block:%x", ns(base, adapter), h)
		pipe.SAdd(ctx, key, workerID)
		pipe.Expire(ctx, key, idx.ttl)
	}
	_, _ = pipe.Exec(ctx)
}
