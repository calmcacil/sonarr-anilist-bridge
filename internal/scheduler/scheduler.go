package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
}

type Show struct {
	TVDBID int    `json:"tvdbId"`
	Title  string `json:"title,omitempty"`
}

func New(c *cache.Cache, cfg *config.Config) *Scheduler {
	return &Scheduler{
		cache:  c,
		cfg:    cfg,
		client: anilist.NewWithTimeout(time.Duration(cfg.AniListTimeoutMin) * time.Minute),
		resolver: mapping.NewResolver(),
	}
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
// refresh (every 10 min) and mapping refresh (every 1 h). Does not
// block; the caller should Prewarm synchronously before calling this.
func (s *Scheduler) StartBackground(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refreshStale(ctx)
				s.prune(ctx)
			}
		}
	}()

	go func() {
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
	for _, year := range s.cfg.PrewarmYears {
		for _, season := range s.cfg.PrewarmSeasons {
			for _, category := range []string{"series", "series-new"} {
				if s.cache.Exists(season, year, category) {
					continue
				}
				slog.Info("prewarming", "season", season, "year", year, "category", category)
				if err := s.refresh(ctx, season, year, category); err != nil {
					slog.Error("prewarm failed", "season", season, "year", year, "category", category, "error", err)
					if firstErr == nil {
						firstErr = err
					}
				}
			}
		}
	}
	return firstErr
}

func (s *Scheduler) Refresh(ctx context.Context, season string, year int, category string) error {
	return s.refresh(ctx, season, year, category)
}

func (s *Scheduler) FetchAndStore(ctx context.Context, season string, year int, category string) error {
	inserted, err := s.cache.SetEmptyIfNotExists(season, year, category)
	if err != nil {
		return fmt.Errorf("set pending marker: %w", err)
	}
	if !inserted {
		return nil
	}
	go func() {
		if err := s.refresh(context.WithoutCancel(ctx), season, year, category); err != nil {
			slog.Error("backfill refresh failed", "season", season, "year", year, "category", category, "error", err)
		}
	}()
	return nil
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
		shows := s.processSeason(ctx, ssn, year, formats, category)
		allShows = append(allShows, shows...)
	}

	data, err := json.Marshal(allShows)
	if err != nil {
		return fmt.Errorf("marshal shows: %w", err)
	}

	if err := s.cache.Set(season, year, category, data); err != nil {
		return fmt.Errorf("cache set: %w", err)
	}

	slog.Info("cached", "season", season, "year", year, "category", category, "shows", len(allShows))
	return nil
}

func (s *Scheduler) processSeason(ctx context.Context, season string, year int, formats []string, category string) []Show {
	slog.Info("fetching season", "season", season, "year", year)

	shows, err := s.client.FetchSeason(ctx, season, year, s.cfg.MaxPerSeason, formats)
	if err != nil {
		slog.Error("fetch failed", "season", season, "year", year, "error", err)
		return nil
	}

	if s.cfg.WinterOverflow && season == "WINTER" {
		shows = s.fetchWinterOverflow(ctx, year, formats, shows)
	}

	if season == "WINTER" {
		shows = filterWinterMonth(shows)
	}

	shows = filterSeries(shows)

	shows = filter.Filter(shows, filter.Config{
		Blacklist:   nil,
		ExcludeTags: s.cfg.ExcludeTags,
	})
	shows = filter.FilterFuture(shows, s.cfg.AheadMonthsOrDefault())

	if category == "series-new" {
		shows = filterFirstSeason(shows)
	}

	return s.resolveShows(shows)
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
		return nil
	}
	resolved := s.resolver.ResolveBatch(shows)
	var out []Show
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

func filterSeries(shows []anilist.Show) []anilist.Show {
	var out []anilist.Show
	for _, sh := range shows {
		if sh.IsSeries() {
			out = append(out, sh)
		}
	}
	return out
}

// filterFirstSeason keeps only shows that are first-season entries
// (no PREQUEL or PARENT relations).
func filterFirstSeason(shows []anilist.Show) []anilist.Show {
	var out []anilist.Show
	for _, sh := range shows {
		if sh.IsNew() {
			out = append(out, sh)
		}
	}
	return out
}

func filterWinterMonth(shows []anilist.Show) []anilist.Show {
	var filtered []anilist.Show
	for _, sh := range shows {
		if sh.IsWinterStart() {
			filtered = append(filtered, sh)
		}
	}
	return filtered
}
