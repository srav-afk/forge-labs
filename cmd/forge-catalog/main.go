package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/srav-afk/forge-labs/internal/config"
	"github.com/srav-afk/forge-labs/internal/db"
	"github.com/srav-afk/forge-labs/services/catalog"
	"github.com/srav-afk/forge-labs/services/fleet"
	"github.com/srav-afk/forge-labs/services/provider"
	"gorm.io/gorm"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	k := config.Load(config.ControlPlaneDefaults())
	gdb, err := db.NewGorm(k.String("db.url"))
	if err != nil {
		fatal(err)
	}
	ctx := context.Background()
	switch os.Args[1] {
	case "models":
		listModels(ctx, gdb)
	case "assign":
		if len(os.Args) < 4 {
			fatal(fmt.Errorf("usage: forge-catalog assign <model-name> <worker-id>"))
		}
		assign(ctx, gdb, os.Args[2], os.Args[3])
	case "seed":
		repo := catalog.NewRepository(gdb)
		n, err := repo.SeedFromWorkers(ctx)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("seeded %d models from workers\n", n)
	case "provider-upsert":
		if len(os.Args) < 5 {
			fatal(fmt.Errorf("usage: forge-catalog provider-upsert <id> <base-url> <api-key-ref> [model=provider_model]..."))
		}
		upsertProvider(ctx, gdb, os.Args[2], os.Args[3], os.Args[4], os.Args[5:]...)
	case "fleet-policy":
		if len(os.Args) < 3 {
			fatal(fmt.Errorf("usage: forge-catalog fleet-policy <base-model> [min] [max] [target-concurrency]"))
		}
		upsertFleet(ctx, gdb, os.Args[2:])
	case "workers":
		listWorkers(ctx, gdb)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `forge-catalog — catalog / provider / fleet helpers

  forge-catalog models
  forge-catalog workers
  forge-catalog seed
  forge-catalog assign <model-name> <worker-id>
  forge-catalog provider-upsert <id> <base-url> <api-key-ref> [base=provider]...
  forge-catalog fleet-policy <base-model> [min max target]

Uses FORGE_ENV_FILE / FORGE_DB_URL like the control plane.
`)
}

func listModels(ctx context.Context, gdb *gorm.DB) {
	repo := catalog.NewRepository(gdb)
	models, err := repo.ListModels(ctx)
	if err != nil {
		fatal(err)
	}
	assigns, err := repo.ListAssignments(ctx)
	if err != nil {
		fatal(err)
	}
	byModel := map[string][]string{}
	for _, a := range assigns {
		byModel[a.ModelID] = append(byModel[a.ModelID], a.WorkerID)
	}
	for _, m := range models {
		fmt.Printf("%s  base=%s adapter=%q workers=%v\n", m.Name, m.BaseModel, m.Adapter, byModel[m.ID])
	}
}

func listWorkers(ctx context.Context, gdb *gorm.DB) {
	type row struct {
		ID       string
		Endpoint string
		Runtime  string `gorm:"column:runtime_kind"`
		State    string
	}
	var rows []row
	if err := gdb.WithContext(ctx).Table("workers").Select("id, endpoint, runtime_kind, state").Find(&rows).Error; err != nil {
		fatal(err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(rows)
}

func assign(ctx context.Context, gdb *gorm.DB, name, workerID string) {
	repo := catalog.NewRepository(gdb)
	models, err := repo.ListModels(ctx)
	if err != nil {
		fatal(err)
	}
	var modelID string
	for _, m := range models {
		if m.Name == name {
			modelID = m.ID
			break
		}
	}
	if modelID == "" {
		m := &catalog.Model{
			Name:      name,
			BaseModel: name,
			CreatedAt: time.Now().UTC(),
		}
		if err := repo.UpsertModel(ctx, m); err != nil {
			fatal(err)
		}
		modelID = m.ID
	}
	if err := repo.UpsertAssignment(ctx, &catalog.ModelAssignment{
		ModelID:   modelID,
		WorkerID:  workerID,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		fatal(err)
	}
	fmt.Printf("assigned %s -> %s\n", name, workerID)
}

func upsertProvider(ctx context.Context, gdb *gorm.DB, id, baseURL, keyRef string, maps ...string) {
	p := provider.Provider{
		ID:        id,
		Kind:      "openai_compat",
		BaseURL:   baseURL,
		AuthMode:  "bearer",
		APIKeyRef: keyRef,
		Enabled:   true,
		UpdatedAt: time.Now().UTC(),
	}
	if err := gdb.WithContext(ctx).Save(&p).Error; err != nil {
		fatal(err)
	}
	for _, m := range maps {
		base, prov := m, m
		if i := indexByte(m, '='); i >= 0 {
			base, prov = m[:i], m[i+1:]
		}
		mm := provider.ModelMap{
			ProviderID:    id,
			BaseModel:     base,
			Adapter:       "",
			ProviderModel: prov,
			MaxContext:    128000,
		}
		if err := gdb.WithContext(ctx).Save(&mm).Error; err != nil {
			fatal(err)
		}
		assign(ctx, gdb, base, "provider:"+id+":"+base)
	}
	fmt.Printf("provider %s upserted (%d models)\n", id, len(maps))
}

func upsertFleet(ctx context.Context, gdb *gorm.DB, args []string) {
	base := args[0]
	minR, maxR, target := 0, 3, 8
	if len(args) > 1 {
		fmt.Sscanf(args[1], "%d", &minR)
	}
	if len(args) > 2 {
		fmt.Sscanf(args[2], "%d", &maxR)
	}
	if len(args) > 3 {
		fmt.Sscanf(args[3], "%d", &target)
	}
	cache := fleet.NewPolicyCache(gdb)
	p := fleet.DefaultPolicy(fleet.ModelIdentity{BaseModel: base})
	p.MinReplicas = minR
	p.MaxReplicas = maxR
	p.TargetConcurrency = target
	if err := cache.Upsert(ctx, p); err != nil {
		fatal(err)
	}
	fmt.Printf("fleet policy %s min=%d max=%d target=%d\n", base, minR, maxR, target)
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
