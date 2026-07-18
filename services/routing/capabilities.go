package routing

import (
	"encoding/json"
	"strconv"
	"strings"
)

type CostHint struct {
	PerHourUSD float64
	Class      string
}

type CapabilityView struct {
	Runtime    string
	MaxContext uint32
	VRAMGB     float64
	GPU        string
	Cost       CostHint
	Raw        map[string]string
}

func ParseCapabilities(raw []byte, runtimeKind string) CapabilityView {
	out := CapabilityView{
		Runtime: normalizeRuntime(runtimeKind),
		Raw:     map[string]string{},
	}
	if len(raw) == 0 {
		return out
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return out
	}
	out.Raw = m
	if v := m["runtime"]; v != "" {
		out.Runtime = strings.ToLower(v)
	}
	if v := m["gpu"]; v != "" {
		out.GPU = v
	}
	if v := m["vram_gb"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			out.VRAMGB = f
		}
	}
	if v := m["max_model_len"]; v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			out.MaxContext = uint32(n)
		}
	}
	if v := m["cost_per_hour"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			out.Cost.PerHourUSD = f
		}
	}
	if v := m["cost_class"]; v != "" {
		out.Cost.Class = strings.ToLower(v)
	} else if out.Cost.PerHourUSD <= 0 {
		out.Cost.Class = "free"
	} else {
		out.Cost.Class = "paid"
	}
	return out
}

func normalizeRuntime(kind string) string {
	k := strings.ToUpper(kind)
	switch {
	case strings.Contains(k, "VLLM"):
		return "vllm"
	case strings.Contains(k, "OLLAMA"):
		return "ollama"
	case strings.Contains(k, "MLX"):
		return "mlx"
	case strings.Contains(k, "LLAMACPP"):
		return "llamacpp"
	case strings.Contains(k, "FORGE"):
		return "forge"
	default:
		return strings.ToLower(kind)
	}
}
