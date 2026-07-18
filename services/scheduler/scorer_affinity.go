package scheduler

import "context"

type AffinityScorer struct {
	Window     int
	BlockBytes int
	preferred  string
}

func NewAffinityScorer(window, blockBytes int) *AffinityScorer {
	if window <= 0 {
		window = 1024
	}
	if blockBytes <= 0 {
		blockBytes = 64
	}
	return &AffinityScorer{Window: window, BlockBytes: blockBytes}
}

func (s *AffinityScorer) Name() string { return "affinity" }

func (s *AffinityScorer) Prepare(_ context.Context, req *Request, candidates []Candidate) {
	s.preferred = ""
	if req == nil || len(candidates) == 0 {
		return
	}
	prompt := req.Prompt
	if req.AffinityKey != "" {
		prompt = req.AffinityKey
	}
	if prompt == "" {
		return
	}
	ids := make([]string, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, c.WorkerID)
	}
	key := PrefixKey(prompt, s.Window, s.BlockBytes)
	s.preferred = HRWPick(key, ids)
	req.PreferredWorker = s.preferred
}

func (s *AffinityScorer) Score(_ context.Context, _ *Request, c Candidate) float64 {
	if s.preferred == "" {
		return 0
	}
	if c.WorkerID == s.preferred {
		return 1.0
	}
	return 0.0
}

func (s *AffinityScorer) Preferred() string { return s.preferred }
