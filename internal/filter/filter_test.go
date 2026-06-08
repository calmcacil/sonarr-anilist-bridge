package filter

import (
	"testing"

	"github.com/calmcacil/sonarr-anime-bridge/internal/anilist"
	"github.com/calmcacil/sonarr-anime-bridge/internal/testutil"
)

func TestFilter_SkipsShortDuration(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{Duration: testutil.Ptr(24), Episodes: testutil.Ptr(12)},
		{Duration: testutil.Ptr(6), Episodes: testutil.Ptr(1)},
		{Duration: testutil.Ptr(10), Episodes: testutil.Ptr(1)},
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
		{StartDate: anilist.FuzzyDate{Year: &year, Month: testutil.Ptr(12)}},
		{StartDate: anilist.FuzzyDate{Year: testutil.Ptr(2020), Month: testutil.Ptr(1)}},
	}

	result := FilterFuture(shows, 3)
	if len(result) != 1 {
		t.Fatalf("expected 1 show within range, got %d", len(result))
	}
}

func TestFilterFuture_NoLimit(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{StartDate: anilist.FuzzyDate{Year: testutil.Ptr(2099), Month: testutil.Ptr(12)}},
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
			show: anilist.Show{ID: 1, Title: anilist.Title{English: testutil.Ptr("Original Show")}},
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
		{ID: 1, Title: anilist.Title{English: testutil.Ptr("New Show")}},
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

func TestFilterByFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format string
		want   bool
	}{
		{"TV", true},
		{"ONA", true},
		{"TV_SHORT", true},
		{"MOVIE", false},
		{"OVA", false},
		{"SPECIAL", false},
	}

	formats := []string{"TV", "ONA", "TV_SHORT"}
	for _, tc := range tests {
		t.Run(tc.format, func(t *testing.T) {
			shows := []anilist.Show{{Format: tc.format}}
			result := FilterByFormats(shows, formats)
			got := len(result) == 1
			if got != tc.want {
				t.Errorf("FilterByFormats(%q) kept=%v, want=%v", tc.format, got, tc.want)
			}
		})
	}
}

func TestFilterByFormats_EmptyFormats(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{Format: "TV"},
		{Format: "MOVIE"},
	}

	result := FilterByFormats(shows, nil)
	if len(result) != 0 {
		t.Error("expected no shows when formats list is empty")
	}

	result = FilterByFormats(shows, []string{})
	if len(result) != 0 {
		t.Error("expected no shows when formats list is empty")
	}
}

func TestFilterBySeason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		season string
		want   bool
	}{
		{"WINTER", true},
		{"SPRING", true},
		{"SUMMER", true},
		{"FALL", true},
	}

	for _, tc := range tests {
		t.Run(tc.season, func(t *testing.T) {
			shows := []anilist.Show{{Season: tc.season}}
			result := FilterBySeason(shows, tc.season)
			if len(result) != 1 {
				t.Errorf("FilterBySeason(%q) should keep matching show", tc.season)
			}
		})
	}
}

func TestFilterBySeason_ExcludesOtherSeasons(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{Season: "SPRING"},
		{Season: "SUMMER"},
		{Season: "FALL"},
		{Season: "WINTER"},
	}

	result := FilterBySeason(shows, "WINTER")
	if len(result) != 1 || result[0].Season != "WINTER" {
		t.Errorf("expected only WINTER show, got %d shows", len(result))
	}

	result = FilterBySeason(shows, "SPRING")
	if len(result) != 1 || result[0].Season != "SPRING" {
		t.Errorf("expected only SPRING show, got %d shows", len(result))
	}

	result = FilterBySeason(shows, "SUMMER")
	if len(result) != 1 || result[0].Season != "SUMMER" {
		t.Errorf("expected only SUMMER show, got %d shows", len(result))
	}

	result = FilterBySeason(shows, "FALL")
	if len(result) != 1 || result[0].Season != "FALL" {
		t.Errorf("expected only FALL show, got %d shows", len(result))
	}
}

func TestFilterBySeason_EmptySeasonFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		season string
		month  int
		want   bool
	}{
		{"WINTER/Dec", "WINTER", 12, true},
		{"WINTER/Jan", "WINTER", 1, true},
		{"WINTER/Mar", "WINTER", 3, true},
		{"WINTER/Apr", "WINTER", 4, false},
		{"SPRING/Apr", "SPRING", 4, true},
		{"SPRING/Mar", "SPRING", 3, false},
		{"SUMMER/Jul", "SUMMER", 7, true},
		{"SUMMER/Jun", "SUMMER", 6, false},
		{"FALL/Oct", "FALL", 10, true},
		{"FALL/Dec", "FALL", 12, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			shows := []anilist.Show{
				{Season: "", StartDate: anilist.FuzzyDate{Month: &tc.month}},
			}
			result := FilterBySeason(shows, tc.season)
			got := len(result) == 1
			if got != tc.want {
				t.Errorf("FilterBySeason(%q) with month=%d: kept=%v, want=%v", tc.season, tc.month, got, tc.want)
			}
		})
	}
}

func TestFilterBySeason_EmptySeasonUnknownMonth(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{Season: "", StartDate: anilist.FuzzyDate{Month: nil}},
	}

	result := FilterBySeason(shows, "WINTER")
	if len(result) != 0 {
		t.Error("expected show with empty season and nil month to be excluded (cannot determine season)")
	}
}

func TestFilterBySeason_All(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{Season: "WINTER"},
		{Season: "SUMMER"},
		{Season: ""},
	}

	result := FilterBySeason(shows, "ALL")
	if len(result) != 3 {
		t.Errorf("expected all 3 shows for ALL, got %d", len(result))
	}
}

func TestFilterBySeason_UnknownSeason(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{Season: "WINTER"},
	}

	result := FilterBySeason(shows, "INVALID")
	if len(result) != 0 {
		t.Error("expected no shows for unknown season")
	}
}
