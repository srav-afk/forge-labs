package routing

import "testing"

func TestParseCapabilitiesCost(t *testing.T) {
	raw := []byte(`{"runtime":"vllm","cost_per_hour":"1.19","cost_class":"paid","vram_gb":"80","gpu":"A100-80GB","max_model_len":"131072"}`)
	c := ParseCapabilities(raw, "RUNTIME_KIND_VLLM")
	if c.Runtime != "vllm" || c.Cost.PerHourUSD != 1.19 || c.Cost.Class != "paid" {
		t.Fatalf("%+v", c)
	}
	if c.VRAMGB != 80 || c.GPU != "A100-80GB" || c.MaxContext != 131072 {
		t.Fatalf("%+v", c)
	}
}

func TestParseCapabilitiesFreeDefault(t *testing.T) {
	c := ParseCapabilities([]byte(`{"runtime":"ollama"}`), "RUNTIME_KIND_OLLAMA")
	if c.Cost.Class != "free" || c.Cost.PerHourUSD != 0 {
		t.Fatalf("%+v", c)
	}
}
