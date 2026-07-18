package routing

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
)

const PolicyChannel = "forge:policy:updates"
const PolicyKey = "forge:policy:routing:v1"

type RoutingPolicy struct {
	Version        uint64
	WeightLoad     float64
	WeightLatency  float64
	WeightCost     float64
	WeightAffinity float64
	WorkerWeights  map[string]map[string]float64
}

type PolicyHolder struct {
	ptr atomic.Pointer[RoutingPolicy]
}

func NewPolicyHolder() *PolicyHolder {
	return &PolicyHolder{}
}

func (h *PolicyHolder) Load() *RoutingPolicy {
	return h.ptr.Load()
}

func (h *PolicyHolder) StoreIfNewer(p *RoutingPolicy) bool {
	if p == nil {
		return false
	}
	for {
		cur := h.ptr.Load()
		if cur != nil && p.Version <= cur.Version {
			return false
		}
		if h.ptr.CompareAndSwap(cur, p) {
			return true
		}
	}
}

func RunPolicySubscriber(ctx context.Context, rdb *redis.Client, holder *PolicyHolder) {
	if rdb == nil || holder == nil {
		return
	}
	if raw, err := rdb.Get(ctx, PolicyKey).Bytes(); err == nil {
		if p, err := parsePolicyJSON(raw); err == nil {
			holder.StoreIfNewer(p)
		}
	}
	sub := rdb.Subscribe(ctx, PolicyChannel)
	defer sub.Close()
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_ = msg
			raw, err := rdb.Get(ctx, PolicyKey).Bytes()
			if err != nil {
				log.Printf("policy: reload: %v", err)
				continue
			}
			p, err := parsePolicyJSON(raw)
			if err != nil {
				log.Printf("policy: parse: %v", err)
				continue
			}
			if holder.StoreIfNewer(p) {
				log.Printf("policy: swapped to v%d", p.Version)
			}
		}
	}
}

func parsePolicyJSON(raw []byte) (*RoutingPolicy, error) {
	var wire struct {
		Version        uint64  `json:"version"`
		WeightLoad     float64 `json:"weight_load"`
		WeightLatency  float64 `json:"weight_latency"`
		WeightCost     float64 `json:"weight_cost"`
		WeightAffinity float64 `json:"weight_affinity"`
		Models         map[string]struct {
			Weights map[string]float64 `json:"weights"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, err
	}
	p := &RoutingPolicy{
		Version:        wire.Version,
		WeightLoad:     wire.WeightLoad,
		WeightLatency:  wire.WeightLatency,
		WeightCost:     wire.WeightCost,
		WeightAffinity: wire.WeightAffinity,
		WorkerWeights:  map[string]map[string]float64{},
	}
	for m, mp := range wire.Models {
		p.WorkerWeights[m] = mp.Weights
	}
	return p, nil
}

func PolicyVersion(raw string) (uint64, error) {
	return strconv.ParseUint(raw, 10, 64)
}
