package cache

import (
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

	data, fresh, isPending, ok := c.Get("WINTER", 2026, "series")
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

	if err := c.SetEmpty("WINTER", 2026, "series"); err != nil {
		t.Fatalf("SetEmpty: %v", err)
	}

	data, fresh, isPending, ok := c.Get("WINTER", 2026, "series")
	if !ok {
		t.Fatal("expected hit after SetEmpty")
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
	if err := c.Set("SPRING", 2026, "series", showData); err != nil {
		t.Fatalf("Set: %v", err)
	}

	data, fresh, isPending, ok := c.Get("SPRING", 2026, "series")
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

	c.SetEmpty("WINTER", 2026, "series")

	showData := []byte(`[{"tvdbId":99999,"title":"Real Show"}]`)
	c.Set("WINTER", 2026, "series", showData)

	data, fresh, isPending, ok := c.Get("WINTER", 2026, "series")
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

	c.Set("WINTER", 2020, "series", []byte(`[]`))
	c.Set("SPRING", 2020, "series", []byte(`[]`))

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

	c.Set("WINTER", 2020, "series", []byte(`[]`))
	c.Set("SPRING", 2026, "series", []byte(`[]`))

	// Entries just created should NOT need refresh
	keys, err := c.NeedsRefresh(2026, 7, 30)
	if err != nil {
		t.Fatalf("NeedsRefresh: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 stale entries, got %d", len(keys))
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

	c.SetEmpty("WINTER", 2026, "series")

	if !c.Exists("WINTER", 2026, "series") {
		t.Error("expected true after SetEmpty")
	}
}

func TestStats(t *testing.T) {
	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.Set("WINTER", 2026, "series", []byte(`[{"tvdbId":1}]`))
	c.Set("SPRING", 2026, "series", []byte(`[{"tvdbId":2}]`))
	c.Get("WINTER", 2026, "series")

	stats := c.Stats()
	if stats.Entries != 2 {
		t.Errorf("entries = %d, want 2", stats.Entries)
	}
}
