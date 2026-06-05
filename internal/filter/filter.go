package filter

import (
	"log/slog"

	"github.com/calmcacil/sonarr-anime-bridge/internal/anilist"
)

type Config struct {
	ExcludeTags []string
}

// Filter removes shows with short duration or excluded content tags.
// Returns the filtered slice.
func Filter(shows []anilist.Show, cfg Config) []anilist.Show {
	var filtered []anilist.Show
	for _, show := range shows {
		title := show.DisplayTitle()

		if show.SkipByDuration() {
			slog.Debug("skipped show (duration <= 10 min)",
				"title", title,
				"duration", show.Duration)
			continue
		}

		if hasExcludedTag(show, cfg.ExcludeTags) {
			slog.Debug("skipped show (excluded tag)",
				"title", title,
				"tags", show.Tags)
			continue
		}

		filtered = append(filtered, show)
	}

	skipped := len(shows) - len(filtered)
	if skipped > 0 {
		slog.Info("filtered shows",
			"total", len(shows),
			"skipped", skipped,
			"remaining", len(filtered))
	}

	return filtered
}

func hasExcludedTag(show anilist.Show, tags []string) bool {
	for _, exclude := range tags {
		if exclude == "" {
			continue
		}
		if show.HasTag(exclude) {
			return true
		}
	}
	return false
}

// FilterFuture removes shows whose start date is more than aheadMonths
// months in the future. Returns the original slice if aheadMonths is <= 0.
func FilterFuture(shows []anilist.Show, aheadMonths int) []anilist.Show {
	if aheadMonths <= 0 {
		return shows
	}
	var filtered []anilist.Show
	for _, show := range shows {
		title := show.DisplayTitle()
		if !show.IsWithinMonths(aheadMonths) {
			slog.Debug("skipped show (too far in the future)",
				"title", title,
				"ahead_months", aheadMonths)
			continue
		}
		filtered = append(filtered, show)
	}
	return filtered
}
