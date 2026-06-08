package cache

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"

	"github.com/calmcacil/sonarr-anime-bridge/internal/mapping"
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
		db.Close() //nolint:errcheck // cleanup on error path
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
		db.Close() //nolint:errcheck // cleanup on error path
		return nil, fmt.Errorf("create season_cache table: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS anilist_cache (
			season    TEXT NOT NULL,
			year      INTEGER NOT NULL,
			data      BLOB NOT NULL,
			fetched_at INTEGER NOT NULL,
			PRIMARY KEY (season, year)
		)
	`); err != nil {
		db.Close() //nolint:errcheck // cleanup on error path
		return nil, fmt.Errorf("create anilist_cache table: %w", err)
	}

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_season_cache_refresh
		ON season_cache(fetched_at, is_empty)
	`); err != nil {
		db.Close() //nolint:errcheck // cleanup on error path
		return nil, fmt.Errorf("create refresh index: %w", err)
	}

	if err := migrateMappingVersion(db); err != nil {
		db.Close() //nolint:errcheck // cleanup on error path
		return nil, fmt.Errorf("migrate season_cache: %w", err)
	}

	if err := migrateFilterFutureEnabled(db); err != nil {
		db.Close() //nolint:errcheck // cleanup on error path
		return nil, fmt.Errorf("migrate filter_future_enabled: %w", err)
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



func (c *Cache) Get(season string, year int, category string, mappingVersion string, filterFutureEnabled bool) (data []byte, fresh bool, isPending bool, ok bool) {
	var raw []byte
	var isEmpty int
	var fetchedAt int64
	var storedVersion string
	var storedFFE int

	err := c.db.QueryRow(
		`SELECT data, is_empty, fetched_at, mapping_version, filter_future_enabled FROM season_cache WHERE season=? AND year=? AND category=?`,
		season, year, category,
	).Scan(&raw, &isEmpty, &fetchedAt, &storedVersion, &storedFFE)

	if err != nil {
		c.misses.Add(1)
		return nil, false, false, false
	}

	c.hits.Add(1)

	if _, err := c.db.Exec(
		`UPDATE season_cache SET last_hit=? WHERE season=? AND year=? AND category=?`,
		time.Now().Unix(), season, year, category,
	); err != nil {
		slog.Warn("failed to update last_hit", "error", err, "season", season, "year", year, "category", category)
	}

	if isEmpty == 1 && string(raw) != "[]" {
		slog.Warn("empty entry missing placeholder data", "season", season, "year", year, "category", category)
		return nil, false, false, false
	}
	if isEmpty == 0 && string(raw) == "[]" {
		slog.Warn("non-empty entry has empty placeholder data", "season", season, "year", year, "category", category)
		return nil, false, false, false
	}

	if isEmpty == 1 {
		return nil, false, true, true
	}

	if !json.Valid(raw) {
		slog.Warn("invalid JSON in cache entry", "season", season, "year", year, "category", category)
		return nil, false, false, false
	}

	freshnessThreshold := c.pastYearFreshness
	if year == time.Now().Year() {
		freshnessThreshold = c.currentYearFreshness
	}
	fresh = time.Since(time.Unix(fetchedAt, 0)) < freshnessThreshold && storedVersion == mappingVersion && (storedFFE == 1) == filterFutureEnabled
	return raw, fresh, false, true
}

func (c *Cache) Set(season string, year int, category string, data []byte, mappingVersion string, filterFutureEnabled bool) error {
	now := time.Now().Unix()
	ffe := 0
	if filterFutureEnabled {
		ffe = 1
	}
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO season_cache (season, year, category, data, is_empty, fetched_at, last_hit, mapping_version, filter_future_enabled)
		 VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?)`,
		season, year, category, data, now, now, mappingVersion, ffe,
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

func (c *Cache) Clear() error {
	_, err := c.db.Exec(`DELETE FROM season_cache`)
	if err != nil {
		return err
	}
	_, err = c.db.Exec(`DELETE FROM anilist_cache`)
	if err != nil {
		return err
	}
	c.hits.Store(0)
	c.misses.Store(0)
	return nil
}

func (c *Cache) Vacuum() error {
	_, err := c.db.Exec("VACUUM")
	return err
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

func (c *Cache) NeedsRefresh(currentYear int, currentRefreshDays, pastRefreshDays int, mappingVersion string, filterFutureEnabled bool) ([]CacheKey, error) {
	stalePendingCutoff := time.Now().Add(-1 * time.Hour).Unix()
	rows, err := c.db.Query(`SELECT season, year, category, fetched_at, is_empty, mapping_version, filter_future_enabled FROM season_cache WHERE is_empty = 0 OR (is_empty = 1 AND fetched_at < ?)`, stalePendingCutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // rows.Err() captures iteration errors

	var keys []CacheKey
	now := time.Now()

	for rows.Next() {
		var key CacheKey
		var fetchedAt int64
		var isEmpty int
		var storedVersion string
		var storedFilterFuture int
		if err := rows.Scan(&key.Season, &key.Year, &key.Category, &fetchedAt, &isEmpty, &storedVersion, &storedFilterFuture); err != nil {
			return nil, err
		}

		if isEmpty == 1 {
			keys = append(keys, key)
			continue
		}

		if storedVersion != mappingVersion {
			keys = append(keys, key)
			continue
		}

		if (storedFilterFuture == 1) != filterFutureEnabled {
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
	_ = c.db.QueryRow(
		`SELECT COUNT(*) FROM season_cache WHERE season=? AND year=? AND category=?`,
		season, year, category,
	).Scan(&count)
	return count > 0
}

func (c *Cache) FilterFutureEnabledMatches(season string, year int, category string, enabled bool) (bool, error) {
	var stored int
	err := c.db.QueryRow(
		`SELECT filter_future_enabled FROM season_cache WHERE season=? AND year=? AND category=?`,
		season, year, category,
	).Scan(&stored)
	if err != nil {
		return false, err
	}
	return (stored == 1) == enabled, nil
}

func (c *Cache) Stats() CacheStats {
	stats := CacheStats{Hits: c.hits.Load(), Misses: c.misses.Load()}
	_ = c.db.QueryRow(`SELECT COUNT(*) FROM season_cache`).Scan(&stats.Entries)
	return stats
}

func migrateMappingVersion(db *sql.DB) error {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('season_cache') WHERE name='mapping_version'`).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err := db.Exec(`ALTER TABLE season_cache ADD COLUMN mapping_version TEXT NOT NULL DEFAULT ''`)
		return err
	}
	return nil
}

func migrateFilterFutureEnabled(db *sql.DB) error {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('season_cache') WHERE name='filter_future_enabled'`).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err := db.Exec(`ALTER TABLE season_cache ADD COLUMN filter_future_enabled INTEGER NOT NULL DEFAULT 1`)
		return err
	}
	return nil
}

func MappingVersion(m *mapping.AnibridgeMapping) string {
	if m == nil {
		return ""
	}
	malKeys, aniKeys := m.Keys()
	sort.Ints(malKeys)
	sort.Ints(aniKeys)

	h := sha256.New()
	for _, k := range malKeys {
		fmt.Fprintf(h, "mal:%d\n", k) //nolint:errcheck // sha256.Write never errors
	}
	for _, k := range aniKeys {
		fmt.Fprintf(h, "anilist:%d\n", k) //nolint:errcheck // sha256.Write never errors
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (c *Cache) Ping() error {
	return c.db.Ping()
}

func (c *Cache) GetAniList(season string, year int) (data []byte, fresh bool, ok bool) {
	var raw []byte
	var fetchedAt int64

	err := c.db.QueryRow(
		`SELECT data, fetched_at FROM anilist_cache WHERE season=? AND year=?`,
		season, year,
	).Scan(&raw, &fetchedAt)

	if err != nil {
		c.misses.Add(1)
		return nil, false, false
	}

	c.hits.Add(1)

	freshnessThreshold := c.pastYearFreshness
	if year == time.Now().Year() {
		freshnessThreshold = c.currentYearFreshness
	}
	fresh = time.Since(time.Unix(fetchedAt, 0)) < freshnessThreshold
	return raw, fresh, true
}

func (c *Cache) SetAniList(season string, year int, data []byte) error {
	now := time.Now().Unix()
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO anilist_cache (season, year, data, fetched_at) VALUES (?, ?, ?, ?)`,
		season, year, data, now,
	)
	return err
}
