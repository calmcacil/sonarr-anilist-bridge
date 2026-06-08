package cache

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

type Cache struct {
	db                    *sql.DB
	currentYearFreshness  time.Duration
	pastYearFreshness     time.Duration
	hits                  atomic.Int64
	misses                atomic.Int64
}

type CacheKey struct {
	Season   string
	Year     int
	Category string
}

type CacheStats struct {
	Entries int
	Hits    int64
	Misses  int64
}



func Open(path string) (*Cache, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(5)

	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS season_cache (
			season    TEXT NOT NULL,
			year      INTEGER NOT NULL,
			category  TEXT NOT NULL,
			data      BLOB NOT NULL,
			is_empty  INTEGER NOT NULL DEFAULT 0,
			fetched_at INTEGER NOT NULL,
			last_hit  INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (season, year, category)
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}

	return &Cache{
		db:                   db,
		currentYearFreshness: 24 * time.Hour,
		pastYearFreshness:    7 * 24 * time.Hour,
	}, nil
}

func (c *Cache) Close() error {
	return c.db.Close()
}

func (c *Cache) Get(season string, year int, category string) (data []byte, fresh bool, isPending bool, ok bool) {
	var raw []byte
	var isEmpty int
	var fetchedAt int64

	err := c.db.QueryRow(
		`SELECT data, is_empty, fetched_at FROM season_cache WHERE season=? AND year=? AND category=?`,
		season, year, category,
	).Scan(&raw, &isEmpty, &fetchedAt)

	if err != nil {
		c.misses.Add(1)
		return nil, false, false, false
	}

	c.hits.Add(1)

	// Update last_hit
	if _, err := c.db.Exec(
		`UPDATE season_cache SET last_hit=? WHERE season=? AND year=? AND category=?`,
		time.Now().Unix(), season, year, category,
	); err != nil {
		slog.Warn("failed to update last_hit", "error", err, "season", season, "year", year, "category", category)
	}

	if isEmpty == 1 {
		return nil, false, true, true
	}

	freshnessThreshold := c.pastYearFreshness
	if year == time.Now().Year() {
		freshnessThreshold = c.currentYearFreshness
	}
	fresh = time.Since(time.Unix(fetchedAt, 0)) < freshnessThreshold
	return raw, fresh, false, true
}

func (c *Cache) Set(season string, year int, category string, data []byte) error {
	now := time.Now().Unix()
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO season_cache (season, year, category, data, is_empty, fetched_at, last_hit)
		 VALUES (?, ?, ?, ?, 0, ?, ?)`,
		season, year, category, data, now, now,
	)
	return err
}

// Deprecated: Use SetEmptyIfNotExists for concurrent-safe placeholder
// insertion. SetEmpty uses INSERT OR REPLACE and may overwrite data that
// another goroutine just finished writing. Kept for test compatibility.
func (c *Cache) SetEmpty(season string, year int, category string) error {
	now := time.Now().Unix()
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO season_cache (season, year, category, data, is_empty, fetched_at, last_hit)
		 VALUES (?, ?, ?, X'5B5D', 1, ?, ?)`,
		season, year, category, now, now,
	)
	return err
}

// SetEmptyIfNotExists inserts a pending placeholder only if no entry
// exists for this key yet. Returns (true, nil) if inserted, (false, nil)
// if a prior call already created the entry.
func (c *Cache) SetEmptyIfNotExists(season string, year int, category string) (bool, error) {
	now := time.Now().Unix()
	result, err := c.db.Exec(
		`INSERT OR IGNORE INTO season_cache (season, year, category, data, is_empty, fetched_at, last_hit)
		 VALUES (?, ?, ?, X'5B5D', 1, ?, ?)`,
		season, year, category, now, now,
	)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

func (c *Cache) PruneStale(staleDays int) (int, error) {
	cutoff := time.Now().Add(-time.Duration(staleDays) * 24 * time.Hour).Unix()
	result, err := c.db.Exec(
		`DELETE FROM season_cache WHERE last_hit < ?`,
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (c *Cache) NeedsRefresh(currentYear int, currentRefreshDays, pastRefreshDays int) ([]CacheKey, error) {
	stalePendingCutoff := time.Now().Add(-1 * time.Hour).Unix()
	rows, err := c.db.Query(`SELECT season, year, category, fetched_at, is_empty FROM season_cache WHERE is_empty = 0 OR (is_empty = 1 AND fetched_at < ?)`, stalePendingCutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []CacheKey
	now := time.Now()

	for rows.Next() {
		var key CacheKey
		var fetchedAt int64
		var isEmpty int
		if err := rows.Scan(&key.Season, &key.Year, &key.Category, &fetchedAt, &isEmpty); err != nil {
			return nil, err
		}

		// Always retry stale pending entries (failed backfills) older than 1 hour
		if isEmpty == 1 {
			keys = append(keys, key)
			continue
		}

		ttl := time.Duration(pastRefreshDays) * 24 * time.Hour
		if key.Year == currentYear {
			ttl = time.Duration(currentRefreshDays) * 24 * time.Hour
		}

		if now.Sub(time.Unix(fetchedAt, 0)) > ttl {
			keys = append(keys, key)
		}
	}

	return keys, rows.Err()
}

func (c *Cache) Exists(season string, year int, category string) bool {
	var count int
	c.db.QueryRow(
		`SELECT COUNT(*) FROM season_cache WHERE season=? AND year=? AND category=?`,
		season, year, category,
	).Scan(&count)
	return count > 0
}

func (c *Cache) Stats() CacheStats {
	stats := CacheStats{Hits: c.hits.Load(), Misses: c.misses.Load()}
	c.db.QueryRow(`SELECT COUNT(*) FROM season_cache`).Scan(&stats.Entries)
	return stats
}

func (c *Cache) Ping() error {
	return c.db.Ping()
}
