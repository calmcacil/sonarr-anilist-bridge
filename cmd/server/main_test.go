package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/calmcacil/sonarr-anime-bridge/internal/cache"
	"github.com/calmcacil/sonarr-anime-bridge/internal/config"
	"github.com/calmcacil/sonarr-anime-bridge/internal/scheduler"
)

func newTestCache(t *testing.T) *cache.Cache {
	t.Helper()
	f, err := os.CreateTemp("", "cache-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	c, err := cache.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func newTestScheduler(t *testing.T, c *cache.Cache) *scheduler.Scheduler {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		MaxPerSeason:         100,
		IncludeTypes:         []string{"TV"},
		AnibridgeMappingPath: dir + "/mappings.json.zst",
	}
	return scheduler.New(c, cfg)
}

func TestHandleHealth_OK(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)
	s := newTestScheduler(t, c)

	// Load resolver so health check reports ok
	s.LoadResolver()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handleHealth(c, s)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %q", resp["status"])
	}
}

func TestHandleHealth_Degraded(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)
	s := newTestScheduler(t, c)
	// Don't load resolver — should report degraded

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handleHealth(c, s)(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["status"] != "degraded" {
		t.Errorf("expected status degraded, got %q", resp["status"])
	}
}

func TestHandleCacheStats(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)

	req := httptest.NewRequest(http.MethodGet, "/cache/stats", nil)
	w := httptest.NewRecorder()

	handleCacheStats(c)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var stats cache.CacheStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if stats.Entries != 0 {
		t.Errorf("expected 0 entries, got %d", stats.Entries)
	}
}

func TestHandleList_InvalidSeason(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)
	s := newTestScheduler(t, c)
	cfg := &config.Config{}

	req := httptest.NewRequest(http.MethodGet, "/list?season=INVALID&year=2026", nil)
	w := httptest.NewRecorder()

	handleList(c, s, cfg)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleList_CacheMiss(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)
	s := newTestScheduler(t, c)
	cfg := &config.Config{}

	req := httptest.NewRequest(http.MethodGet, "/list?season=WINTER&year=2026", nil)
	w := httptest.NewRecorder()

	handleList(c, s, cfg)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var shows []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &shows); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(shows) != 0 {
		t.Errorf("expected empty list on cache miss, got %d shows", len(shows))
	}
}

func TestHandleList_CacheHit(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)
	s := newTestScheduler(t, c)
	cfg := &config.Config{}

	// Pre-populate cache
	data := []byte(`[{"tvdbId":12345,"title":"Test Show"}]`)
	if err := c.Set("WINTER", 2026, "series", data); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/list?season=WINTER&year=2026", nil)
	w := httptest.NewRecorder()

	handleList(c, s, cfg)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var shows []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &shows); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(shows) != 1 {
		t.Fatalf("expected 1 show, got %d", len(shows))
	}
	if shows[0]["tvdbId"].(float64) != 12345 {
		t.Errorf("expected tvdbId 12345, got %v", shows[0]["tvdbId"])
	}
}

func TestHandleList_DefaultParams(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)
	s := newTestScheduler(t, c)
	cfg := &config.Config{}

	req := httptest.NewRequest(http.MethodGet, "/list", nil)
	w := httptest.NewRecorder()

	handleList(c, s, cfg)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleList_InvalidCategory(t *testing.T) {
	t.Parallel()
	c := newTestCache(t)
	s := newTestScheduler(t, c)
	cfg := &config.Config{}

	// Invalid category should default to "series"
	data := []byte(`[{"tvdbId":99999,"title":"Category Test"}]`)
	if err := c.Set("WINTER", 2026, "series", data); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/list?season=WINTER&year=2026&category=invalid", nil)
	w := httptest.NewRecorder()

	handleList(c, s, cfg)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var shows []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &shows); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(shows) != 1 {
		t.Fatalf("expected 1 show (category defaulted to series), got %d", len(shows))
	}
}
