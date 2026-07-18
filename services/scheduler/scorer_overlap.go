package scheduler

import (
	"context"

	"github.com/srav-afk/forge-labs/services/cacheregistry"
)

type OverlapScorer struct {
	holder *cacheregistry.SnapshotHolder
	weight float64
}

func NewOverlapScorer(holder *cacheregistry.SnapshotHolder) *OverlapScorer {
	return &OverlapScorer{holder: holder, weight: 1.0}
}

func (s *OverlapScorer) Name() string { return "cache-overlap" }

func (s *OverlapScorer) Score(_ context.Context, req *Request, c Candidate) float64 {
	if s == nil || s.holder == nil || req == nil {
		return 0.5
	}
	if c.Capabilities != nil && c.Capabilities["provider"] == "true" {
		return 0.5
	}
	snap := s.holder.Load()
	if snap == nil || len(snap.WorkerBlocks) == 0 {
		return 0.5
	}
	have, ok := snap.WorkerBlocks[c.WorkerID]
	if !ok || len(have) == 0 {
		return 0.5
	}
	adapter := req.Adapter
	want := cacheregistry.HashPromptApprox(req.Prompt, cacheregistry.DefaultBlockSize, adapter)
	if len(want) == 0 {
		return 0.5
	}
	blocks, score := cacheregistry.LongestConsecutivePrefix(want, have)
	if blocks == 0 {
		return 0.0
	}
	norm := score / float64(len(want)*cacheregistry.DefaultBlockSize)
	if norm > 1 {
		norm = 1
	}
	return norm
}
