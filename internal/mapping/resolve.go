package mapping

import (
	"log/slog"

	"github.com/calmcacil/anilistgen/internal/anilist"
)

type Resolver struct {
	community *CommunityMapping
}

type ResolvedShow struct {
	MALID    int
	TVDBID   int
	Title    string
	Resolved bool
}

// NewResolver creates a Resolver that uses the given community mapping for
// MAL-to-TVDB ID lookups.
func NewResolver(cm *CommunityMapping) *Resolver {
	return &Resolver{community: cm}
}

func (r *Resolver) Resolve(malID int, title string) (int, bool) {
	if malID <= 0 {
		return 0, false
	}
	if t, ok := r.community.Lookup(malID); ok {
		slog.Debug("resolved via community mapping",
			"title", title, "mal", malID, "tvdb", t)
		return t, true
	}
	return 0, false
}

func (r *Resolver) ResolveBatch(shows []anilist.Show) map[int]ResolvedShow {
	result := make(map[int]ResolvedShow, len(shows))
	for _, show := range shows {
		malID := 0
		if show.IDMal != nil {
			malID = *show.IDMal
		}
		rs := ResolvedShow{
			MALID: malID,
			Title: show.DisplayTitle(),
		}
		tvdbID, ok := r.Resolve(malID, rs.Title)
		if ok {
			rs.TVDBID = tvdbID
			rs.Resolved = true
		}
		result[show.ID] = rs
	}
	return result
}
