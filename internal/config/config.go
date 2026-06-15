package config

import (
	"strings"

	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
)

func Load(defaults map[string]any) *koanf.Koanf {
	k := koanf.New(".")

	_ = k.Load(confmap.Provider(defaults, "."), nil)

	_ = k.Load(env.Provider("FORGE_", ".", func(s string) string {
		return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(s, "FORGE_")), "_", ".")
	}), nil)

	return k
}
