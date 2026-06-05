package scheduler

import (
	"testing"

	"github.com/calmcacil/sonarr-anime-bridge/internal/anilist"
)

func makePtr[T any](v T) *T {
	return &v
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
			result := filterFirstSeason(shows)
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

	result := filterFirstSeason(shows)
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
			result := filterSeries(shows)
			got := len(result) == 1
			if got != tc.want {
				t.Errorf("filterSeries(%q) kept=%v, want=%v", tc.format, got, tc.want)
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
			result := filterWinterMonth(shows)
			got := len(result) == 1
			if got != tc.want {
				t.Errorf("filterWinterMonth(month=%d) kept=%v, want=%v", tc.month, got, tc.want)
			}
		})
	}
}

func TestFilterWinterMonth_NilMonth(t *testing.T) {
	t.Parallel()

	shows := []anilist.Show{
		{StartDate: anilist.FuzzyDate{Month: nil}},
	}
	result := filterWinterMonth(shows)
	if len(result) != 1 {
		t.Error("expected show with nil month to be kept (cannot rule out)")
	}
}
