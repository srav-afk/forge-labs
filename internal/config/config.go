package config

import (
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

func Load(defaults map[string]any) *koanf.Koanf {
	k := koanf.New(".")

	_ = k.Load(confmap.Provider(defaults, "."), nil)

	envFile := os.Getenv("FORGE_ENV_FILE")
	if envFile == "" {
		envFile = "development.env"
	}
	if _, err := os.Stat(envFile); err == nil {
		_ = k.Load(file.Provider(envFile), dotenv.ParserEnv("FORGE_", ".", normalizeKey))
	}

	_ = k.Load(env.Provider("FORGE_", ".", normalizeKey), nil)

	return k
}

func normalizeKey(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, "FORGE_")), "_", ".")
}

func Duration(k *koanf.Koanf, key string, fallback time.Duration) time.Duration {
	raw := k.String(key)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}

func RuntimeLabel(kind string) string {
	const prefix = "RUNTIME_KIND_"
	if strings.HasPrefix(kind, prefix) {
		return strings.ToLower(strings.TrimPrefix(kind, prefix))
	}
	return strings.ToLower(kind)
}
