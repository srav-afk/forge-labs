package config

func ControlPlaneDefaults() map[string]any {
	return map[string]any{
		"http.addr":    ":8080",
		"metrics.addr": ":9090",
		"grpc.addr":    ":8081",
		"db.url":       "postgres://forge:forge@localhost:5432/forge?sslmode=disable",
	}
}

func WorkerDefaults() map[string]any {
	return map[string]any{
		"metrics.addr":         ":9091",
		"controlplane.grpc":    "localhost:8081",
		"worker.id":            "mac-studio-01",
		"worker.endpoint":      "127.0.0.1:50051",
		"worker.runtime":       "RUNTIME_KIND_OLLAMA",
		"worker.model.base":    "qwen3:8b",
		"worker.model.context": 32768,
		"worker.capabilities": map[string]string{
			"accelerator":    "apple-m3-pro",
			"quantization":   "Q4_K_M",
			"family":         "qwen",
			"parameter_size": "8.2B",
		},
	}
}
