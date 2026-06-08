package filter

import (
	"testing"

	"github.com/calmcacil/sonarr-anime-bridge/internal/anilist"
)

func makePtr[T any](v T) *T {
	return &v
}

func TestFilter_SkipsShortDuration(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{Duration: makePtr(24), Episodes: makePtr(12)},
		{Duration: makePtr(6), Episodes: makePtr(1)},
		{Duration: makePtr(10), Episodes: makePtr(1)},
	}

	result := Filter(shows, Config{})
	if len(result) != 1 {
		t.Fatalf("expected 1 show after filter, got %d", len(result))
	}
	if *result[0].Duration != 24 {
		t.Errorf("expected remaining show to have duration 24, got %d", *result[0].Duration)
	}
}

func TestFilter_ExcludeTags(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{Tags: []anilist.Tag{{Name: "Action"}}},
		{Tags: []anilist.Tag{{Name: "Hentai"}}},
		{Tags: []anilist.Tag{{Name: "Comedy"}}},
	}

	result := Filter(shows, Config{ExcludeTags: []string{"Hentai"}})
	if len(result) != 2 {
		t.Fatalf("expected 2 shows, got %d", len(result))
	}
}

func TestFilter_ExcludeTagsCaseInsensitive(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{Tags: []anilist.Tag{{Name: "HENTAI"}}},
	}

	result := Filter(shows, Config{ExcludeTags: []string{"hentai"}})
	if len(result) != 0 {
		t.Error("expected show to be excluded case-insensitively")
	}
}

func TestFilterFuture_RemovesFutureShows(t *testing.T) {
	t.Parallel()

	year := 2099
	shows := []anilist.Show{
		{StartDate: anilist.FuzzyDate{Year: &year, Month: makePtr(12)}},
		{StartDate: anilist.FuzzyDate{Year: makePtr(2020), Month: makePtr(1)}},
	}

	result := FilterFuture(shows, 3)
	if len(result) != 1 {
		t.Fatalf("expected 1 show within range, got %d", len(result))
	}
}

func TestFilterFuture_NoLimit(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{StartDate: anilist.FuzzyDate{Year: makePtr(2099), Month: makePtr(12)}},
	}

	result := FilterFuture(shows, 0)
	if len(result) != 1 {
		t.Errorf("expected 1 show when months=0, got %d", len(result))
	}
}

func TestHasExcludedTag(t *testing.T) {
	t.Parallel()

	show := anilist.Show{Tags: []anilist.Tag{{Name: "Action"}, {Name: "Hentai"}}}
	if !hasExcludedTag(show, []string{"Hentai"}) {
		t.Error("expected hentai tag to match")
	}
	if hasExcludedTag(show, []string{"Guro"}) {
		t.Error("expected guro tag not to match")
	}
	if !hasExcludedTag(show, []string{"", "Hentai"}) {
		t.Error("empty entry should not prevent matching valid entries")
	}
}

func TestFilterFirstSeason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		show anilist.Show
		want bool
	}{
		{
			name: "no relations",
			show: anilist.Show{ID: 1, Title: anilist.Title{English: makePtr("Original Show")}},
			want: true,
		},
		{
			name: "empty relations",
			show: anilist.Show{
				ID:        2,
				Relations: &anilist.RelationBlock{Edges: nil},
			},
			want: true,
		},
		{
			name: "has prequel",
			show: anilist.Show{
				ID: 3,
				Relations: &anilist.RelationBlock{
					Edges: []anilist.RelationEdge{{RelationType: "PREQUEL"}},
				},
			},
			want: false,
		},
		{
			name: "has parent",
			show: anilist.Show{
				ID: 4,
				Relations: &anilist.RelationBlock{
					Edges: []anilist.RelationEdge{{RelationType: "PARENT"}},
				},
			},
			want: false,
		},
		{
			name: "has sequel only",
			show: anilist.Show{
				ID: 5,
				Relations: &anilist.RelationBlock{
					Edges: []anilist.RelationEdge{{RelationType: "SEQUEL"}},
				},
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			shows := []anilist.Show{tc.show}
			result := FilterFirstSeason(shows)
			got := len(result) == 1
			if got != tc.want {
				if tc.want {
					t.Errorf("expected show %d to be kept, but it was filtered out", tc.show.ID)
				} else {
					t.Errorf("expected show %d to be filtered out, but it was kept", tc.show.ID)
				}
			}
		})
	}
}

func TestFilterFirstSeason_Mixed(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{ID: 1, Title: anilist.Title{English: makePtr("New Show")}},
		{ID: 2, Relations: &anilist.RelationBlock{
			Edges: []anilist.RelationEdge{{RelationType: "PREQUEL"}},
		}},
		{ID: 3, Relations: &anilist.RelationBlock{
			Edges: []anilist.RelationEdge{{RelationType: "PARENT"}},
		}},
		{ID: 4, Relations: &anilist.RelationBlock{
			Edges: []anilist.RelationEdge{{RelationType: "SEQUEL"}},
		}},
	}

	result := FilterFirstSeason(shows)
	if len(result) != 2 {
		t.Fatalf("expected 2 shows (IDs 1, 4), got %d", len(result))
	}
	seen := make(map[int]bool)
	for _, s := range result {
		seen[s.ID] = true
	}
	if !seen[1] {
		t.Error("expected ID 1 (no relations) to be kept")
	}
	if seen[2] {
		t.Error("expected ID 2 (prequel) to be filtered out")
	}
	if seen[3] {
		t.Error("expected ID 3 (parent) to be filtered out")
	}
	if !seen[4] {
		t.Error("expected ID 4 (sequel only) to be kept")
	}
}

func TestFilterSeries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format string
		want   bool
	}{
		{"TV", true},
		{"ONA", true},
		{"MOVIE", false},
		{"OVA", false},
		{"SPECIAL", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.format, func(t *testing.T) {
			shows := []anilist.Show{{Format: tc.format}}
			result := FilterSeries(shows)
			got := len(result) == 1
			if got != tc.want {
				t.Errorf("FilterSeries(%q) kept=%v, want=%v", tc.format, got, tc.want)
			}
		})
	}
}

func TestFilterWinterMonth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		month int
		want  bool
	}{
		{12, true},
		{1, true},
		{2, true},
		{3, true},
		{4, false},
		{7, false},
		{11, false},
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			shows := []anilist.Show{
				{StartDate: anilist.FuzzyDate{Month: &tc.month}},
			}
			result := FilterWinterMonth(shows)
			got := len(result) == 1
			if got != tc.want {
				t.Errorf("FilterWinterMonth(month=%d) kept=%v, want=%v", tc.month, got, tc.want)
			}
		})
	}
}

func TestFilterWinterMonth_NilMonth(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{StartDate: anilist.FuzzyDate{Month: nil}},
	}
	result := FilterWinterMonth(shows)
	if len(result) != 1 {
		t.Error("expected show with nil month to be kept (cannot rule out)")
	}
}
