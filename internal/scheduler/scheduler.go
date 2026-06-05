package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/calmcacil/anilistgen/internal/anilist"
	"github.com/calmcacil/anilistgen/internal/cache"
	"github.com/calmcacil/anilistgen/internal/config"
	"github.com/calmcacil/anilistgen/internal/filter"
	"github.com/calmcacil/anilistgen/internal/mapping"
)

const mappingRefreshInterval = 1 * time.Hour

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
		client: anilist.New(),
		resolver: mapping.NewResolver(),
	}
}

// LoadResolver loads the anibridge mapping synchronously. Must be called
// before any Resolve / ResolveBatch call.
func (s *Scheduler) LoadResolver() {
	path := s.cfg.AnibridgeMappingPath
	upstream := s.cfg.AnibridgeURL
	m, _, err := mapping.LoadOrFetch(path, upstream)
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
		mapTicker := time.NewTicker(mappingRefreshInterval)
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
	_ = ctx
	path := s.cfg.AnibridgeMappingPath
	upstream := s.cfg.AnibridgeURL
	m, meta, err := mapping.LoadOrFetch(path, upstream)
	if err != nil {
		slog.Warn("anibridge mapping refresh failed, keeping current mapping", "error", err)
		return
	}
	s.resolver.SetMapping(m)
	_ = meta
}

func (s *Scheduler) Prewarm(ctx context.Context) error {
	for _, year := range s.cfg.PrewarmYears {
		for _, season := range s.cfg.PrewarmSeasons {
			for _, category := range []string{"series", "series-new"} {
				if s.cache.Exists(season, year, category) {
					continue
				}
				slog.Info("prewarming", "season", season, "year", year, "category", category)
				s.refresh(ctx, season, year, category)
			}
		}
	}
	return nil
}

func (s *Scheduler) Refresh(ctx context.Context, season string, year int, category string) {
	s.refresh(ctx, season, year, category)
}

func (s *Scheduler) FetchAndStore(ctx context.Context, season string, year int, category string) error {
	if s.cache.Exists(season, year, category) {
		return nil
	}
	s.cache.SetEmpty(season, year, category)
	go s.refresh(context.WithoutCancel(ctx), season, year, category)
	return nil
}

func (s *Scheduler) refresh(ctx context.Context, season string, year int, category string) {
	seasons := []string{season}
	if season == "ALL" {
		seasons = config.AllSeasons()
	}

	var allShows []Show
	formats := []string{"TV"}
	if s.cfg.IncludeONA {
		formats = append(formats, "ONA")
	}

	for _, ssn := range seasons {
		shows := s.processSeason(ctx, ssn, year, formats)
		if category == "series-new" {
			shows = filterNew(shows)
		}
		allShows = append(allShows, shows...)
	}

	data, err := json.Marshal(allShows)
	if err != nil {
		slog.Error("marshal shows", "season", season, "year", year, "error", err)
		return
	}

	if err := s.cache.Set(season, year, category, data); err != nil {
		slog.Error("cache set", "season", season, "year", year, "error", err)
		return
	}

	slog.Info("cached", "season", season, "year", year, "category", category, "shows", len(allShows))
}

func (s *Scheduler) processSeason(ctx context.Context, season string, year int, formats []string) []Show {
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
		s.refresh(ctx, key.Season, key.Year, key.Category)
	}
}

func (s *Scheduler) prune(ctx context.Context) {
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

func filterNew(shows []Show) []Show {
	return shows
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
