package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/calmcacil/sonarr-anime-bridge/internal/anilist"
	"github.com/calmcacil/sonarr-anime-bridge/internal/cache"
	"github.com/calmcacil/sonarr-anime-bridge/internal/config"
	"github.com/calmcacil/sonarr-anime-bridge/internal/filter"
	"github.com/calmcacil/sonarr-anime-bridge/internal/mapping"
)


type Scheduler struct {
	cache    *cache.Cache
	cfg      *config.Config
	client   *anilist.Client
	resolver *mapping.Resolver

	wg         sync.WaitGroup
	inflightMu sync.Mutex
	inflight   map[string]bool
}

type Show struct {
	TVDBID int    `json:"tvdbId"`
	Title  string `json:"title,omitempty"`
}

func New(c *cache.Cache, cfg *config.Config) *Scheduler {
	return &Scheduler{
		cache:  c,
		cfg:    cfg,
		client:   anilist.NewWithTimeout(time.Duration(cfg.AniListTimeoutMin) * time.Minute),
		resolver: mapping.NewResolver(),
		inflight: make(map[string]bool),
	}
}

// ResolverLoaded reports whether the anibridge mapping has been loaded.
func (s *Scheduler) ResolverLoaded() bool {
	return s.resolver != nil && s.resolver.Mapping() != nil
}

// MappingVersion returns a hash of the current mapping key set. Used by the
// cache to detect when resolved entries need re-resolution.
func (s *Scheduler) MappingVersion() string {
	return s.mappingVersion()
}

func (s *Scheduler) mappingVersion() string {
	m := s.resolver.Mapping()
	if m == nil {
		return ""
	}
	malKeys, aniKeys := m.Keys()
	return cache.MappingVersion(malKeys, aniKeys)
}

// LoadResolver loads the anibridge mapping synchronously. Must be called
// before any Resolve / ResolveBatch call.
func (s *Scheduler) LoadResolver() {
	path := s.cfg.AnibridgeMappingPath
	upstream := s.cfg.AnibridgeURL
	m, _, err := mapping.LoadOrFetch(context.Background(), path, upstream)
	if err != nil {
		slog.Error("failed to load anibridge mapping", "error", err)
		return
	}
	s.resolver.SetMapping(m)
}

// StartBackground launches background refresh goroutines: stale entry
// refresh (every 10 min), mapping refresh (every 1 h), and cache stats
// logging (every 10 min). Does not block; the caller should Prewarm
// synchronously before calling this.
func (s *Scheduler) StartBackground(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in stale refresh background worker", "recover", r)
			}
		}()
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refreshStale(ctx)
				s.prune(ctx)
				s.logCacheStats()
			}
		}
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in mapping refresh background worker", "recover", r)
			}
		}()
		mapTicker := time.NewTicker(time.Duration(s.cfg.AnibridgeRefreshDays) * 24 * time.Hour)
		defer mapTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-mapTicker.C:
				s.refreshMapping(ctx)
			}
		}
	}()
}

func (s *Scheduler) refreshMapping(ctx context.Context) {
	m, _, err := mapping.LoadOrFetch(ctx, s.cfg.AnibridgeMappingPath, s.cfg.AnibridgeURL)
	if err != nil {
		slog.Warn("anibridge mapping refresh failed, keeping current mapping", "error", err)
		return
	}
	s.resolver.SetMapping(m)
}

func (s *Scheduler) Prewarm(ctx context.Context) error {
	var firstErr error
	formats := []string{"TV"}
	if s.cfg.IncludeONA {
		formats = append(formats, "ONA")
	}

	for _, year := range s.cfg.PrewarmYears {
		for _, season := range s.cfg.PrewarmSeasons {
			rawShows, err := s.fetchOrGetCachedAniList(ctx, season, year, formats)
			if err != nil {
				slog.Error("prewarm fetch failed", "season", season, "year", year, "error", err)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}

			for _, category := range []string{"series", "series-new"} {
				if s.cache.Exists(season, year, category) {
					continue
				}
				slog.Info("prewarming", "season", season, "year", year, "category", category)
				shows, err := s.processSeason(ctx, season, year, formats, category, rawShows)
				if err != nil {
					slog.Error("prewarm process failed", "season", season, "year", year, "category", category, "error", err)
					if firstErr == nil {
						firstErr = err
					}
					continue
				}

				data, err := json.Marshal(shows)
				if err != nil {
					slog.Error("prewarm marshal failed", "season", season, "year", year, "category", category, "error", err)
					if firstErr == nil {
						firstErr = err
					}
					continue
				}

				if err := s.cache.SetWithVersion(season, year, category, data, s.mappingVersion()); err != nil {
					slog.Error("prewarm cache set failed", "season", season, "year", year, "category", category, "error", err)
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
			}
		}
	}
	return firstErr
}

func (s *Scheduler) Refresh(ctx context.Context, season string, year int, category string) error {
	return s.refresh(ctx, season, year, category)
}

// StartRefresh spawns a refresh goroutine with deduplication. If a refresh
// for the same (season, year, category) is already in flight, this is a no-op.
func (s *Scheduler) StartRefresh(ctx context.Context, season string, year int, category string) {
	key := refreshKey(season, year, category)

	s.inflightMu.Lock()
	if s.inflight[key] {
		s.inflightMu.Unlock()
		slog.Debug("refresh already in-flight, skipping",
			"season", season, "year", year, "category", category)
		return
	}
	s.inflight[key] = true
	s.wg.Add(1)
	s.inflightMu.Unlock()
	go func() {
		defer s.wg.Done()
		defer func() {
			s.inflightMu.Lock()
			delete(s.inflight, key)
			s.inflightMu.Unlock()
		}()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in refresh goroutine", "season", season, "year", year, "category", category, "recover", r)
			}
		}()
		if err := s.refresh(ctx, season, year, category); err != nil {
			slog.Error("refresh failed", "season", season, "year", year, "category", category, "error", err)
		}
	}()
}

// Wait blocks until all background goroutines complete, or until the context
// is cancelled. Call after server.Shutdown to ensure in-flight operations finish.
func (s *Scheduler) Wait(ctx context.Context) error {
	ch := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(ch)
	}()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func refreshKey(season string, year int, category string) string {
	return fmt.Sprintf("%s/%d/%s", season, year, category)
}

func (s *Scheduler) FetchAndStore(ctx context.Context, season string, year int, category string) error {
	inserted, err := s.cache.SetEmptyIfNotExists(season, year, category)
	if err != nil {
		return fmt.Errorf("set pending marker: %w", err)
	}
	if !inserted {
		// Entry already exists (pending or cached). The background
		// refresh loop will retry stale pending entries, so no
		// need to fire off a duplicate fetch.
		return nil
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in backfill refresh", "season", season, "year", year, "category", category, "recover", r)
			}
		}()
		if err := s.refresh(context.WithoutCancel(ctx), season, year, category); err != nil {
			slog.Error("backfill refresh failed", "season", season, "year", year, "category", category, "error", err)
		}
	}()
	return nil
}

func (s *Scheduler) fetchOrGetCachedAniList(ctx context.Context, season string, year int, formats []string) ([]anilist.Show, error) {
	if raw, fresh, ok := s.cache.GetAniList(season, year); ok && fresh {
		slog.Debug("using cached anilist data", "season", season, "year", year)
		var shows []anilist.Show
		if err := json.Unmarshal(raw, &shows); err != nil {
			return nil, fmt.Errorf("unmarshal cached anilist: %w", err)
		}
		return shows, nil
	}

	slog.Info("fetching season from anilist", "season", season, "year", year)
	shows, err := s.client.FetchSeason(ctx, season, year, s.cfg.MaxPerSeason, formats)
	if err != nil {
		return nil, fmt.Errorf("fetch season %s %d: %w", season, year, err)
	}

	data, err := json.Marshal(shows)
	if err != nil {
		return nil, fmt.Errorf("marshal anilist shows: %w", err)
	}
	if err := s.cache.SetAniList(season, year, data); err != nil {
		slog.Warn("failed to cache anilist data", "error", err)
	}

	return shows, nil
}

func (s *Scheduler) refresh(ctx context.Context, season string, year int, category string) error {
	seasons := []string{season}
	if season == "ALL" {
		seasons = config.AllSeasons()
	}

	allShows := make([]Show, 0)
	formats := []string{"TV"}
	if s.cfg.IncludeONA {
		formats = append(formats, "ONA")
	}

	for _, ssn := range seasons {
		rawShows, err := s.fetchOrGetCachedAniList(ctx, ssn, year, formats)
		if err != nil {
			slog.Error("season fetch failed", "season", ssn, "year", year, "error", err)
			continue
		}
		shows, err := s.processSeason(ctx, ssn, year, formats, category, rawShows)
		if err != nil {
			slog.Error("season process failed", "season", ssn, "year", year, "error", err)
			continue
		}
		allShows = append(allShows, shows...)
	}

	data, err := json.Marshal(allShows)
	if err != nil {
		return fmt.Errorf("marshal shows: %w", err)
	}

	if err := s.cache.SetWithVersion(season, year, category, data, s.mappingVersion()); err != nil {
		return fmt.Errorf("cache set: %w", err)
	}

	slog.Info("cached", "season", season, "year", year, "category", category, "shows", len(allShows))
	return nil
}

func (s *Scheduler) processSeason(ctx context.Context, season string, year int, formats []string, category string, rawShows []anilist.Show) ([]Show, error) {
	slog.Info("processing season", "season", season, "year", year, "category", category)

	shows := rawShows

	if s.cfg.WinterOverflow && season == "WINTER" {
		shows = s.fetchWinterOverflow(ctx, year, formats, shows)
	}

	if season == "WINTER" {
		shows = filter.FilterWinterMonth(shows)
	}

	shows = filter.FilterSeries(shows)

	shows = filter.Filter(shows, filter.Config{
		ExcludeTags: s.cfg.ExcludeTags,
	})
	shows = filter.FilterFuture(shows, s.cfg.AheadMonthsOrDefault())

	if category == "series-new" {
		shows = filter.FilterFirstSeason(shows)
	}

	return s.resolveShows(shows), nil
}

func (s *Scheduler) fetchWinterOverflow(ctx context.Context, year int, formats []string, shows []anilist.Show) []anilist.Show {
	overflowYear := year - 1
	overflow, err := s.client.FetchSeason(ctx, "WINTER", overflowYear, s.cfg.MaxPerSeason, formats)
	if err != nil {
		slog.Warn("winter overflow fetch failed", "year", overflowYear, "error", err)
		return shows
	}

	seen := make(map[int]bool, len(shows))
	for _, sh := range shows {
		seen[sh.ID] = true
	}

	var added int
	for _, sh := range overflow {
		if sh.StartDate.Month != nil && *sh.StartDate.Month == 12 && !seen[sh.ID] {
			shows = append(shows, sh)
			seen[sh.ID] = true
			added++
		}
	}

	if added > 0 {
		slog.Info("winter overflow merged", "year", year, "overflow_year", overflowYear, "added", added, "total", len(shows))
	}

	return shows
}

func (s *Scheduler) resolveShows(shows []anilist.Show) []Show {
	if s.resolver == nil {
		slog.Warn("resolver not yet loaded, skipping resolution")
		return make([]Show, 0)
	}
	resolved := s.resolver.ResolveBatch(shows)
	out := make([]Show, 0)
	for _, show := range shows {
		if r, ok := resolved[show.ID]; ok && r.Resolved {
			out = append(out, Show{TVDBID: r.TVDBID, Title: r.Title})
		}
	}
	return out
}

func (s *Scheduler) refreshStale(ctx context.Context) {
	currentYear := time.Now().Year()
	keys, err := s.cache.NeedsRefresh(currentYear, s.cfg.RefreshCurrentDays, s.cfg.RefreshPastDays)
	if err != nil {
		slog.Error("needs refresh query failed", "error", err)
		return
	}
	for _, key := range keys {
		slog.Info("refreshing stale", "season", key.Season, "year", key.Year, "category", key.Category)
		if err := s.refresh(ctx, key.Season, key.Year, key.Category); err != nil {
			slog.Error("stale refresh failed", "season", key.Season, "year", key.Year, "category", key.Category, "error", err)
		}
	}
}

func (s *Scheduler) prune(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		return
	}
	n, err := s.cache.PruneStale(s.cfg.CacheStaleDays)
	if err != nil {
		slog.Error("prune failed", "error", err)
		return
	}
	if n > 0 {
		slog.Info("pruned cache entries", "count", n)
	}
}

func (s *Scheduler) logCacheStats() {
	stats := s.cache.Stats()
	total := stats.Hits + stats.Misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(stats.Hits) / float64(total) * 100
	}
	slog.Info("cache stats",
		"entries", stats.Entries,
		"hits", stats.Hits,
		"misses", stats.Misses,
		"hit_rate", fmt.Sprintf("%.1f%%", hitRate),
	)
}

