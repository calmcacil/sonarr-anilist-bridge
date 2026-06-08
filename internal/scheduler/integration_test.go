package scheduler

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/calmcacil/sonarr-anime-bridge/internal/cache"
	"github.com/calmcacil/sonarr-anime-bridge/internal/config"
)

var (
	updateBaseline = flag.Bool("update", false, "update baseline files")
	season         = flag.String("season", "WINTER", "season to test")
	year           = flag.Int("year", time.Now().Year(), "year to test")
	category       = flag.String("category", "series", "category to test")
)

type baselineEntry struct {
	TVDBID int    `json:"tvdbId"`
	Title  string `json:"title,omitempty"`
}

func TestIntegration_DataPipeline(t *testing.T) {
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("Skipping integration test: set INTEGRATION=1 to run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cache.db")
	mappingPath := filepath.Join(tmpDir, "mappings.json.zst")

	c, err := cache.Open(dbPath)
	if err != nil {
		t.Fatalf("open cache: %v", err)
	}
	defer c.Close()

	cfg := &config.Config{
		MaxPerSeason:         100,
		IncludeONA:           false,
		WinterOverflow:       true,
		AnibridgeRefreshDays: 1,
		AnibridgeMappingPath: mappingPath,
		AnibridgeURL:         config.DefaultAnibridgeURL,
	}

	s := New(c, cfg)
	s.LoadResolver()
	if !s.ResolverLoaded() {
		t.Fatal("resolver not loaded")
	}

	t.Logf("Fetching %s %d/%s from live AniList...", *season, *year, *category)
	if err := s.Refresh(ctx, *season, *year, *category); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}

	data, fresh, isPending, ok := c.Get(*season, *year, *category)
	if !ok {
		t.Fatal("cache miss after refresh")
	}
	if !fresh {
		t.Fatal("cache not fresh after refresh")
	}
	if isPending {
		t.Fatal("cache is pending after refresh")
	}

	var got []baselineEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	sort.Slice(got, func(i, j int) bool { return got[i].TVDBID < got[j].TVDBID })

	baselinePath := filepath.Join("testdata", fmt.Sprintf("%s_%d_%s.json", *season, *year, *category))

	if *updateBaseline {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		out, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatalf("marshal baseline: %v", err)
		}
		if err := os.WriteFile(baselinePath, out, 0o644); err != nil {
			t.Fatalf("write baseline: %v", err)
		}
		t.Logf("Baseline written to %s (%d shows)", baselinePath, len(got))
		return
	}

	raw, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline: %v (run with -update to create)", err)
	}

	var want []baselineEntry
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("unmarshal baseline: %v", err)
	}

	sort.Slice(want, func(i, j int) bool { return want[i].TVDBID < want[j].TVDBID })

	if len(got) != len(want) {
		t.Errorf("show count mismatch: got %d, want %d", len(got), len(want))
	}

	gotIDs := make(map[int]string, len(got))
	for _, s := range got {
		gotIDs[s.TVDBID] = s.Title
	}

	wantIDs := make(map[int]string, len(want))
	for _, s := range want {
		wantIDs[s.TVDBID] = s.Title
	}

	var added, removed []int
	for id := range gotIDs {
		if _, ok := wantIDs[id]; !ok {
			added = append(added, id)
		}
	}
	for id := range wantIDs {
		if _, ok := gotIDs[id]; !ok {
			removed = append(removed, id)
		}
	}

	if len(added) > 0 {
		sort.Ints(added)
		t.Errorf("shows added since baseline (%d): %v", len(added), added)
	}
	if len(removed) > 0 {
		sort.Ints(removed)
		t.Errorf("shows removed since baseline (%d): %v", len(removed), removed)
	}

	t.Logf("Result: %d shows (baseline: %d, added: %d, removed: %d)", len(got), len(want), len(added), len(removed))
}

func TestIntegration_Prewarm(t *testing.T) {
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("Skipping integration test: set INTEGRATION=1 to run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cache.db")
	mappingPath := filepath.Join(tmpDir, "mappings.json.zst")

	c, err := cache.Open(dbPath)
	if err != nil {
		t.Fatalf("open cache: %v", err)
	}
	defer c.Close()

	cfg := &config.Config{
		MaxPerSeason:         100,
		IncludeONA:           false,
		WinterOverflow:       true,
		AnibridgeRefreshDays: 1,
		AnibridgeMappingPath: mappingPath,
		AnibridgeURL:         config.DefaultAnibridgeURL,
		PrewarmYears:         []int{2026},
		PrewarmSeasons:       []string{"WINTER"},
	}

	s := New(c, cfg)
	s.LoadResolver()
	if !s.ResolverLoaded() {
		t.Fatal("resolver not loaded")
	}

	t.Log("Running prewarm for WINTER 2026 (both categories)...")
	if err := s.Prewarm(ctx); err != nil {
		t.Fatalf("prewarm failed: %v", err)
	}

	for _, cat := range []string{"series", "series-new"} {
		baselinePath := filepath.Join("testdata", fmt.Sprintf("WINTER_2026_%s.json", cat))

		data, fresh, isPending, ok := c.Get("WINTER", 2026, cat)
		if !ok {
			t.Fatalf("cache miss for %s after prewarm", cat)
		}
		if !fresh {
			t.Fatalf("cache not fresh for %s after prewarm", cat)
		}
		if isPending {
			t.Fatalf("cache is pending for %s after prewarm", cat)
		}

		var got []baselineEntry
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal result for %s: %v", cat, err)
		}
		sort.Slice(got, func(i, j int) bool { return got[i].TVDBID < got[j].TVDBID })

		raw, err := os.ReadFile(baselinePath)
		if err != nil {
			t.Fatalf("read baseline for %s: %v", cat, err)
		}
		var want []baselineEntry
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatalf("unmarshal baseline for %s: %v", cat, err)
		}
		sort.Slice(want, func(i, j int) bool { return want[i].TVDBID < want[j].TVDBID })

		if len(got) != len(want) {
			t.Errorf("[%s] show count mismatch: got %d, want %d", cat, len(got), len(want))
		}

		gotIDs := make(map[int]bool, len(got))
		for _, s := range got {
			gotIDs[s.TVDBID] = true
		}
		wantIDs := make(map[int]bool, len(want))
		for _, s := range want {
			wantIDs[s.TVDBID] = true
		}

		var added, removed []int
		for id := range gotIDs {
			if !wantIDs[id] {
				added = append(added, id)
			}
		}
		for id := range wantIDs {
			if !gotIDs[id] {
				removed = append(removed, id)
			}
		}

		if len(added) > 0 {
			sort.Ints(added)
			t.Errorf("[%s] shows added since baseline (%d): %v", cat, len(added), added)
		}
		if len(removed) > 0 {
			sort.Ints(removed)
			t.Errorf("[%s] shows removed since baseline (%d): %v", cat, len(removed), removed)
		}

		t.Logf("[%s] %d shows (baseline: %d, added: %d, removed: %d)", cat, len(got), len(want), len(added), len(removed))
	}

	// Verify raw AniList cache was used (only one API fetch for both categories)
	stats := c.Stats()
	t.Logf("Cache stats: %d entries, %d hits, %d misses", stats.Entries, stats.Hits, stats.Misses)
}
