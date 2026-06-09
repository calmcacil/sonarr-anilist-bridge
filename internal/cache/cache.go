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
		db.Close() //nolint:errcheck // cleanup on error path
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		db.Close() //nolint:errcheck // cleanup on error path
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS year_cache (
			year       INTEGER NOT NULL PRIMARY KEY,
			data       BLOB NOT NULL,
			fetched_at INTEGER NOT NULL,
			last_hit   INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		db.Close() //nolint:errcheck // cleanup on error path
		return nil, fmt.Errorf("create year_cache table: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close() //nolint:errcheck // cleanup on error path
		return nil, fmt.Errorf("ping sqlite: %w", err)
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

func (c *Cache) GetYear(year int) (data []byte, fresh bool, ok bool) {
	var raw []byte
	var fetchedAt int64

	err := c.db.QueryRow(
		`SELECT data, fetched_at FROM year_cache WHERE year=?`,
		year,
	).Scan(&raw, &fetchedAt)

	if err != nil {
		c.misses.Add(1)
		return nil, false, false
	}

	c.hits.Add(1)

	if _, err := c.db.Exec(
		`UPDATE year_cache SET last_hit=? WHERE year=?`,
		time.Now().Unix(), year,
	); err != nil {
		slog.Warn("failed to update last_hit", "error", err, "year", year)
	}

	freshnessThreshold := c.pastYearFreshness
	if year == time.Now().Year() {
		freshnessThreshold = c.currentYearFreshness
	}
	fresh = time.Since(time.Unix(fetchedAt, 0)) < freshnessThreshold
	return raw, fresh, true
}

func (c *Cache) SetYear(year int, data []byte) error {
	now := time.Now().Unix()
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO year_cache (year, data, fetched_at, last_hit) VALUES (?, ?, ?, ?)`,
		year, data, now, now,
	)
	return err
}

func (c *Cache) Clear() error {
	_, err := c.db.Exec(`DELETE FROM year_cache`)
	if err != nil {
		return err
	}
	c.hits.Store(0)
	c.misses.Store(0)
	return nil
}

func (c *Cache) HasYear(year int) bool {
	var count int
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM year_cache WHERE year=?`, year).Scan(&count)
	return count > 0
}

func (c *Cache) Vacuum() error {
	_, err := c.db.Exec("VACUUM")
	return err
}

func (c *Cache) NeedsRefreshYears(currentYear int, currentRefreshDays, pastRefreshDays int) ([]int, error) {
	rows, err := c.db.Query(`SELECT year, fetched_at FROM year_cache`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // rows.Err() captures iteration errors

	var years []int
	now := time.Now()

	for rows.Next() {
		var year int
		var fetchedAt int64
		if err := rows.Scan(&year, &fetchedAt); err != nil {
			return nil, err
		}

		ttl := time.Duration(pastRefreshDays) * 24 * time.Hour
		if year == currentYear {
			ttl = time.Duration(currentRefreshDays) * 24 * time.Hour
		}

		if now.Sub(time.Unix(fetchedAt, 0)) > ttl {
			years = append(years, year)
		}
	}

	return years, rows.Err()
}

func (c *Cache) PruneStaleYears(days int) (int, error) {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	// Use fetched_at as a fallback when last_hit is 0 (e.g. entries created
	// before the column existed or after a failed last_hit UPDATE).
	result, err := c.db.Exec(
		`DELETE FROM year_cache WHERE CASE WHEN last_hit > 0 THEN last_hit ELSE fetched_at END < ?`,
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (c *Cache) Stats() CacheStats {
	stats := CacheStats{Hits: c.hits.Load(), Misses: c.misses.Load()}
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM year_cache`).Scan(&stats.Entries)
	return stats
}

func (c *Cache) Ping() error {
	return c.db.Ping()
}
