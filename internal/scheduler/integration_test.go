package scheduler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/calmcacil/sonarr-anime-bridge/internal/cache"
	"github.com/calmcacil/sonarr-anime-bridge/internal/config"
)

func skipUnlessIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run integration tests")
	}
}

func TestIntegration_DataPipeline(t *testing.T) {
	skipUnlessIntegration(t)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cache.db")

	cfg := &config.Config{
		PrewarmYears:   []int{time.Now().Year()},
		PrewarmSeasons: []string{"WINTER"},
		MaxPerSeason:   50,
		IncludeTypes:   []string{"TV", "ONA"},
		CacheDBPath:    dbPath,
	}

	c, err := cache.Open(dbPath)
	if err != nil {
		t.Fatalf("cache.Open: %v", err)
	}
	defer c.Close()

	sched := New(c, cfg)
	sched.LoadResolver()

	ctx := context.Background()
	if err := sched.refresh(ctx, "WINTER", time.Now().Year(), "series"); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	data, fresh, isPending, ok := c.Get("WINTER", time.Now().Year(), "series")
	if !ok {
		t.Fatal("expected cache hit after refresh")
	}
	if isPending {
		t.Fatal("expected not pending after refresh")
	}
	if !fresh {
		t.Log("data is not fresh — acceptable if fetch was slow")
	}

	var shows []Show
	if err := json.Unmarshal(data, &shows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(shows) == 0 {
		t.Fatal("expected at least one resolved show")
	}

	baselinePath := filepath.Join("testdata", "baseline-series.json")
	compareOrSaveBaseline(t, baselinePath, shows)
}

func TestIntegration_Prewarm(t *testing.T) {
	skipUnlessIntegration(t)

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cache.db")

	cfg := &config.Config{
		PrewarmYears:   []int{time.Now().Year()},
		PrewarmSeasons: []string{"WINTER"},
		MaxPerSeason:   50,
		IncludeTypes:   []string{"TV", "ONA"},
		CacheDBPath:    dbPath,
	}

	c, err := cache.Open(dbPath)
	if err != nil {
		t.Fatalf("cache.Open: %v", err)
	}
	defer c.Close()

	sched := New(c, cfg)
	sched.LoadResolver()

	ctx := context.Background()
	if err := sched.Prewarm(ctx); err != nil {
		t.Fatalf("Prewarm: %v", err)
	}

	for _, category := range []string{"series", "series-new"} {
		data, fresh, isPending, ok := c.GetWithVersion("WINTER", time.Now().Year(), category, sched.MappingVersion())
		if !ok {
			t.Fatalf("expected cache hit for %s", category)
		}
		if isPending {
			t.Fatalf("expected not pending for %s", category)
		}
		if !fresh {
			t.Logf("%s data is not fresh — acceptable", category)
		}

		var shows []Show
		if err := json.Unmarshal(data, &shows); err != nil {
			t.Fatalf("unmarshal %s: %v", category, err)
		}
		if len(shows) == 0 {
			t.Fatalf("expected at least one resolved show in %s", category)
		}
	}

	stats := c.Stats()
	if stats.Entries == 0 {
		t.Fatal("expected cache entries after prewarm")
	}
	t.Logf("cache entries: %d", stats.Entries)
}

func compareOrSaveBaseline(t *testing.T, path string, shows []Show) {
	t.Helper()

	tvdbIDs := make([]int, len(shows))
	for i, s := range shows {
		tvdbIDs[i] = s.TVDBID
	}
	sort.Ints(tvdbIDs)

	data, err := json.MarshalIndent(tvdbIDs, "", "  ")
	if err != nil {
		t.Fatalf("marshal baseline: %v", err)
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			t.Fatalf("read baseline: %v", err)
		}
		t.Logf("no baseline at %s, saving current output", path)
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("write baseline: %v", err)
		}
		return
	}

	var expected []int
	if err := json.Unmarshal(existing, &expected); err != nil {
		t.Fatalf("unmarshal existing baseline: %v", err)
	}

	if len(tvdbIDs) != len(expected) {
		t.Errorf("show count: got %d, want %d", len(tvdbIDs), len(expected))
	}
	for i := range tvdbIDs {
		if i >= len(expected) {
			break
		}
		if tvdbIDs[i] != expected[i] {
			t.Errorf("tvdbId mismatch at position %d: got %d, want %d", i, tvdbIDs[i], expected[i])
		}
	}
}
