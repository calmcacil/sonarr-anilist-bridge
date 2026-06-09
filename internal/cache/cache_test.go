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

func TestGetYear_Miss(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	data, fresh, ok := c.GetYear(2026)
	if ok {
		t.Error("expected miss")
	}
	if data != nil {
		t.Error("expected nil data on miss")
	}
	if fresh {
		t.Error("expected not fresh on miss")
	}
}

func TestSetAndGetYear(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	yearData := []byte(`[{"id":1,"title":{"romaji":"Test"},"format":"TV"}]`)
	if err := c.SetYear(2026, yearData); err != nil {
		t.Fatalf("SetYear: %v", err)
	}

	data, fresh, ok := c.GetYear(2026)
	if !ok {
		t.Fatal("expected hit after SetYear")
	}
	if string(data) != string(yearData) {
		t.Errorf("data = %s, want %s", data, yearData)
	}
	if !fresh {
		t.Error("expected fresh")
	}
}

func TestSetYear_OverwritesExisting(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.SetYear(2026, []byte(`"old"`))
	c.SetYear(2026, []byte(`"new"`))

	data, _, ok := c.GetYear(2026)
	if !ok {
		t.Fatal("expected hit")
	}
	if string(data) != `"new"` {
		t.Errorf("data = %s, want \"new\"", data)
	}
}

func TestNeedsRefreshYears(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.SetYear(2025, []byte(`[]`))
	c.SetYear(2026, []byte(`[]`))

	// Entries just created should NOT need refresh
	years, err := c.NeedsRefreshYears(2026, 1, 7)
	if err != nil {
		t.Fatalf("NeedsRefreshYears: %v", err)
	}
	if len(years) != 0 {
		t.Errorf("expected 0 stale years, got %d", len(years))
	}
}

func TestPruneStaleYears(t *testing.T) {
	t.Parallel()

	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.SetYear(2020, []byte(`[]`))
	c.SetYear(2021, []byte(`[]`))

	// Recently set, should not be pruned with short duration
	n, err := c.PruneStaleYears(30)
	if err != nil {
		t.Fatalf("PruneStaleYears: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 pruned from fresh entries, got %d", n)
	}

	// Manually push last_hit and fetched_at far in the past so both
	// prune fallbacks trigger correctly.
	c.db.Exec(`UPDATE year_cache SET last_hit = 0, fetched_at = 0`)
	n, err = c.PruneStaleYears(1)
	if err != nil {
		t.Fatalf("PruneStaleYears: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 pruned with stale entries, got %d", n)
	}
}

func TestClear(t *testing.T) {
	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.SetYear(2026, []byte(`[]`))
	if err := c.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	stats := c.Stats()
	if stats.Entries != 0 {
		t.Errorf("expected 0 entries after clear, got %d", stats.Entries)
	}
}

func TestStats(t *testing.T) {
	c, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c.SetYear(2025, []byte(`[]`))
	c.SetYear(2026, []byte(`[]`))
	c.GetYear(2026)

	stats := c.Stats()
	if stats.Entries != 2 {
		t.Errorf("entries = %d, want 2", stats.Entries)
	}
	if stats.Hits != 1 {
		t.Errorf("hits = %d, want 1", stats.Hits)
	}
	if stats.Misses != 0 {
		t.Errorf("misses = %d, want 0", stats.Misses)
	}
}

func TestConcurrentAccess(t *testing.T) {
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
			c.SetYear(2026, []byte(`[{"tvdbId":1}]`))
			c.GetYear(2026)
			c.SetYear(2025, []byte(`[]`))
			c.GetYear(2025)
			c.Stats()
		}()
	}
	wg.Wait()
}
