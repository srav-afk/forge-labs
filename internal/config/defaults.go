package config

func ControlPlaneDefaults() map[string]any {
	return map[string]any{
		"http.addr":                     ":8080",
		"metrics.addr":                  ":9090",
		"grpc.addr":                     ":8081",
		"db.url":                        "postgres://forge:forge@localhost:5432/forge?sslmode=disable",
		"redis.url":                     "redis://localhost:6379/0",
		"heartbeat.reconcile":           "3s",
		"routing.snapshot.interval":     "500ms",
		"scheduler.weight.load":         0.6,
		"scheduler.weight.latency":      0.2,
		"scheduler.weight.affinity":     0.2,
		"scheduler.latency.ref.ms":      100.0,
		"scheduler.ewma.tau":            "10s",
		"admission.per.worker.limit":    4,
		"admission.retry.after.seconds": 2,
		"affinity.prefix.window":        1024,
		"affinity.block.bytes":          64,
	}
}

func WorkerDefaults() map[string]any {
	return map[string]any{
		"metrics.addr":         ":9091",
		"controlplane.grpc":    "localhost:8081",
		"redis.url":            "redis://localhost:6379/0",
		"heartbeat.interval":   "2s",
		"heartbeat.ttl":        "6s",
		"worker.id":            "mac-studio-01",
		"worker.endpoint":      "127.0.0.1:50051",
		"worker.grpc.addr":     ":50051",
		"worker.runtime":       "RUNTIME_KIND_OLLAMA",
		"worker.model.base":    "qwen3:8b",
		"worker.model.context": 32768,
		"worker.ready":         true,
		"ollama.url":           "http://127.0.0.1:11434",
		"ollama.keep.alive":    "5m",
		"worker.capabilities": map[string]string{
			"accelerator":    "apple-m3-pro",
			"quantization":   "Q4_K_M",
			"family":         "qwen",
			"parameter_size": "8.2B",
		},
	}
}
