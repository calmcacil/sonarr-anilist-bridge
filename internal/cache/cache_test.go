package cache

import (
	"sync"
	"testing"
)

func TestOpenAndClose(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestGetMiss(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	data, fresh, isPending, ok := c.Get("WINTER", 2026, "series", "", true)
	if ok {
		t.Error("expected miss")
	}
	if data != nil {
		t.Error("expected nil data on miss")
	}
	if fresh {
		t.Error("expected not fresh on miss")
	}
	if isPending {
		t.Error("expected not pending on miss")
	}
}

func TestSetEmptyAndGet(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if _, err := c.SetEmptyIfNotExists("WINTER", 2026, "series"); err != nil {
		t.Fatalf("SetEmptyIfNotExists: %v", err)
	}

	data, fresh, isPending, ok := c.Get("WINTER", 2026, "series", "", true)
	if !ok {
		t.Fatal("expected hit after SetEmptyIfNotExists")
	}
	if data != nil {
		t.Error("expected nil data for pending entry")
	}
	if fresh {
		t.Error("expected not fresh for pending entry")
	}
	if !isPending {
		t.Error("expected isPending true")
	}
}

func TestSetAndGet(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	showData := []byte(`[{"tvdbId":12345,"title":"Test Show"}]`)
	if err := c.Set("SPRING", 2026, "series", showData, "", true); err != nil {
		t.Fatalf("Set: %v", err)
	}

	data, fresh, isPending, ok := c.Get("SPRING", 2026, "series", "", true)
	if !ok {
		t.Fatal("expected hit after Set")
	}
	if string(data) != string(showData) {
		t.Errorf("data = %s, want %s", data, showData)
	}
	if !fresh {
		t.Error("expected fresh")
	}
	if isPending {
		t.Error("expected not pending")
	}
}

func TestSetOverwritesEmpty(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.SetEmptyIfNotExists("WINTER", 2026, "series")

	showData := []byte(`[{"tvdbId":99999,"title":"Real Show"}]`)
	c.Set("WINTER", 2026, "series", showData, "", true)

	data, fresh, isPending, ok := c.Get("WINTER", 2026, "series", "", true)
	if !ok {
		t.Fatal("expected hit")
	}
	if string(data) != string(showData) {
		t.Errorf("data = %s, want %s", data, showData)
	}
	if !fresh {
		t.Error("expected fresh after overwrite")
	}
	if isPending {
		t.Error("expected not pending after overwrite")
	}
}

func TestPruneStale(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.Set("WINTER", 2020, "series", []byte(`[]`), "", true)
	c.Set("SPRING", 2020, "series", []byte(`[]`), "", true)

	n, err := c.PruneStale(365)
	if err != nil {
		t.Fatalf("PruneStale: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 pruned from fresh entries, got %d", n)
	}

	// Manually set last_hit far in the past to test pruning
	c.db.Exec(`UPDATE season_cache SET last_hit = 0`)
	n, err = c.PruneStale(1)
	if err != nil {
		t.Fatalf("PruneStale: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 pruned with stale entries, got %d", n)
	}
}

func TestNeedsRefresh(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.Set("WINTER", 2020, "series", []byte(`[]`), "", true)
	c.Set("SPRING", 2026, "series", []byte(`[]`), "", true)

	// Entries just created should NOT need refresh
	keys, err := c.NeedsRefresh(2026, 7, 30, "", true)
	if err != nil {
		t.Fatalf("NeedsRefresh: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 stale entries, got %d", len(keys))
	}
}

func TestNeedsRefresh_MappingVersionMismatch(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.Set("WINTER", 2026, "series", []byte(`[{"tvdbId":1}]`), "v1", true)

	// Same version → no refresh needed
	keys, err := c.NeedsRefresh(2026, 7, 30, "v1", true)
	if err != nil {
		t.Fatalf("NeedsRefresh: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 stale entries for matching version, got %d", len(keys))
	}

	// Different version → needs refresh
	keys, err = c.NeedsRefresh(2026, 7, 30, "v2", true)
	if err != nil {
		t.Fatalf("NeedsRefresh: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 stale entry for mismatched version, got %d", len(keys))
	}
	if keys[0].Season != "WINTER" || keys[0].Year != 2026 || keys[0].Category != "series" {
		t.Errorf("unexpected key: %+v", keys[0])
	}
}

func TestExists(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if c.Exists("WINTER", 2026, "series") {
		t.Error("expected false before Set")
	}

	c.SetEmptyIfNotExists("WINTER", 2026, "series")

	if !c.Exists("WINTER", 2026, "series") {
		t.Error("expected true after SetEmptyIfNotExists")
	}
}

func TestStats(t *testing.T) {
	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.Set("WINTER", 2026, "series", []byte(`[{"tvdbId":1}]`), "", true)
	c.Set("SPRING", 2026, "series", []byte(`[{"tvdbId":2}]`), "", true)
	c.Get("WINTER", 2026, "series", "", true)

	stats := c.Stats()
	if stats.Entries != 2 {
		t.Errorf("entries = %d, want 2", stats.Entries)
	}
}

func TestSetEmptyIfNotExists_InsertsOnlyOnce(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	inserted, err := c.SetEmptyIfNotExists("WINTER", 2026, "series")
	if err != nil {
		t.Fatalf("first SetEmptyIfNotExists: %v", err)
	}
	if !inserted {
		t.Error("expected first call to return inserted=true")
	}

	inserted, err = c.SetEmptyIfNotExists("WINTER", 2026, "series")
	if err != nil {
		t.Fatalf("second SetEmptyIfNotExists: %v", err)
	}
	if inserted {
		t.Error("expected second call to return inserted=false")
	}
}

func TestSetEmptyIfNotExists_DoesNotReplaceRealData(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	realData := []byte(`[{"tvdbId":123}]`)
	if err := c.Set("SPRING", 2026, "series", realData, "", true); err != nil {
		t.Fatalf("Set: %v", err)
	}

	inserted, err := c.SetEmptyIfNotExists("SPRING", 2026, "series")
	if err != nil {
		t.Fatalf("SetEmptyIfNotExists: %v", err)
	}
	if inserted {
		t.Error("expected false when entry already has real data")
	}

	data, _, _, ok := c.Get("SPRING", 2026, "series", "", true)
	if !ok {
		t.Fatal("expected hit")
	}
	if string(data) != string(realData) {
		t.Errorf("expected real data to be preserved, got %q", data)
	}
}

func TestConcurrentCacheAccess(t *testing.T) {
	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Set("WINTER", 2026, "series", []byte(`[{"tvdbId":1}]`), "", true)
			c.Get("WINTER", 2026, "series", "", true)
			c.SetEmptyIfNotExists("SPRING", 2026, "series")
			c.Get("SPRING", 2026, "series", "", true)
			c.Stats()
		}()
	}
	wg.Wait()
}
