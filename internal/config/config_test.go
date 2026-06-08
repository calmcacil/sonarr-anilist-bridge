package config

import (
	"os"
	"testing"
	"time"
)

func TestResolveSeasons_All(t *testing.T) {
	t.Parallel()

	got := ResolveSeasons([]string{"all"})
	want := []string{"WINTER", "SPRING", "SUMMER", "FALL"}
	if len(got) != len(want) {
		t.Fatalf("expected %d seasons, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("season[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveSeasons_AllCaseInsensitive(t *testing.T) {
	t.Parallel()

	got := ResolveSeasons([]string{"ALL"})
	if len(got) != 4 {
		t.Errorf("expected 4 seasons for ALL, got %d", len(got))
	}
}

func TestResolveSeasons_Specific(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"winter", "WINTER"},
		{"WINTER", "WINTER"},
		{"Spring", "SPRING"},
		{"summer", "SUMMER"},
		{"FALL", "FALL"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := ResolveSeasons([]string{tc.input})
			if len(got) != 1 || got[0] != tc.want {
				t.Errorf("ResolveSeasons(%q) = %v, want [%q]", tc.input, got, tc.want)
			}
		})
	}
}

func TestResolveSeasons_Empty(t *testing.T) {
	t.Parallel()

	got := ResolveSeasons(nil)
	if len(got) != 4 {
		t.Errorf("expected 4 seasons for nil, got %d: %v", len(got), got)
	}

	got2 := ResolveSeasons([]string{})
	if len(got2) != 4 {
		t.Errorf("expected 4 seasons for empty slice, got %d: %v", len(got2), got2)
	}
}

func TestAllSeasons(t *testing.T) {
	t.Parallel()

	got := AllSeasons()
	want := []string{"WINTER", "SPRING", "SUMMER", "FALL"}
	if len(got) != len(want) {
		t.Fatalf("expected %d seasons, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllSeasons[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoad_Defaults(t *testing.T) {
	for _, key := range []string{
		"PORT", "MAX_PER_SEASON", "CACHE_DB_PATH", "LOG_LEVEL",
		"PREWARM_YEARS", "PREWARM_SEASONS", "INCLUDE_TYPES", "EXCLUDE_TAGS",
		"MAPPING_PATH", "MAPPING_URL",
	} {
		os.Unsetenv(key)
	}

	cfg := Load()

	if cfg.Port != DefaultPort {
		t.Errorf("Port = %d, want %d", cfg.Port, DefaultPort)
	}
	if cfg.MaxPerSeason != DefaultMaxPerSeason {
		t.Errorf("MaxPerSeason = %d, want %d", cfg.MaxPerSeason, DefaultMaxPerSeason)
	}
	if cfg.CacheDBPath != DefaultCacheDBPath {
		t.Errorf("CacheDBPath = %q, want %q", cfg.CacheDBPath, DefaultCacheDBPath)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if len(cfg.PrewarmYears) != 1 || cfg.PrewarmYears[0] != time.Now().Year() {
		t.Errorf("PrewarmYears = %v, want [%d]", cfg.PrewarmYears, time.Now().Year())
	}
	if len(cfg.IncludeTypes) != 2 || cfg.IncludeTypes[0] != "TV" || cfg.IncludeTypes[1] != "ONA" {
		t.Errorf("IncludeTypes = %v, want [TV ONA]", cfg.IncludeTypes)
	}
	if cfg.ExcludeTags != nil {
		t.Errorf("ExcludeTags = %v, want nil", cfg.ExcludeTags)
	}
	if cfg.AnibridgeMappingPath != DefaultAnibridgeMappingPath {
		t.Errorf("AnibridgeMappingPath = %q, want %q", cfg.AnibridgeMappingPath, DefaultAnibridgeMappingPath)
	}
	if cfg.AnibridgeURL != DefaultAnibridgeURL {
		t.Errorf("AnibridgeURL = %q, want %q", cfg.AnibridgeURL, DefaultAnibridgeURL)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	keys := []string{
		"PORT", "MAX_PER_SEASON", "LOG_LEVEL", "PREWARM_YEARS", "PREWARM_SEASONS",
		"INCLUDE_TYPES", "EXCLUDE_TAGS", "MAPPING_PATH", "MAPPING_URL",
	}
	for _, key := range keys {
		os.Unsetenv(key)
	}

	os.Setenv("PORT", "9090")
	os.Setenv("MAX_PER_SEASON", "50")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("PREWARM_YEARS", "2025,2026")
	os.Setenv("PREWARM_SEASONS", "winter,spring")
	os.Setenv("INCLUDE_TYPES", "TV")
	os.Setenv("EXCLUDE_TAGS", "hentai,guro")
	os.Setenv("MAPPING_PATH", "/custom/mapping.json.zst")
	os.Setenv("MAPPING_URL", "https://example.com/mappings.json.zst")
	t.Cleanup(func() {
		for _, key := range keys {
			os.Unsetenv(key)
		}
	})

	cfg := Load()

	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if cfg.MaxPerSeason != 50 {
		t.Errorf("MaxPerSeason = %d, want 50", cfg.MaxPerSeason)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if len(cfg.PrewarmYears) != 2 || cfg.PrewarmYears[0] != 2025 || cfg.PrewarmYears[1] != 2026 {
		t.Errorf("PrewarmYears = %v, want [2025 2026]", cfg.PrewarmYears)
	}
	if len(cfg.PrewarmSeasons) != 2 || cfg.PrewarmSeasons[0] != "WINTER" || cfg.PrewarmSeasons[1] != "SPRING" {
		t.Errorf("PrewarmSeasons = %v, want [WINTER SPRING]", cfg.PrewarmSeasons)
	}
	if len(cfg.IncludeTypes) != 1 || cfg.IncludeTypes[0] != "TV" {
		t.Errorf("IncludeTypes = %v, want [TV]", cfg.IncludeTypes)
	}
	if len(cfg.ExcludeTags) != 2 || cfg.ExcludeTags[0] != "HENTAI" || cfg.ExcludeTags[1] != "GURO" {
		t.Errorf("ExcludeTags = %v, want [HENTAI GURO]", cfg.ExcludeTags)
	}
	if cfg.AnibridgeMappingPath != "/custom/mapping.json.zst" {
		t.Errorf("AnibridgeMappingPath = %q, want /custom/mapping.json.zst", cfg.AnibridgeMappingPath)
	}
	if cfg.AnibridgeURL != "https://example.com/mappings.json.zst" {
		t.Errorf("AnibridgeURL = %q, want https://example.com/mappings.json.zst", cfg.AnibridgeURL)
	}
}

func TestLoad_IncludeTypesDefault(t *testing.T) {
	os.Unsetenv("INCLUDE_TYPES")
	cfg := Load()
	if len(cfg.IncludeTypes) != 2 || cfg.IncludeTypes[0] != "TV" || cfg.IncludeTypes[1] != "ONA" {
		t.Errorf("IncludeTypes default = %v, want [TV ONA]", cfg.IncludeTypes)
	}
}

func TestLoad_IncludeTypesCustom(t *testing.T) {
	os.Setenv("INCLUDE_TYPES", "tv,ona,special")
	t.Cleanup(func() { os.Unsetenv("INCLUDE_TYPES") })

	cfg := Load()
	if len(cfg.IncludeTypes) != 3 {
		t.Fatalf("IncludeTypes = %v, want 3 entries", cfg.IncludeTypes)
	}
	for _, want := range []string{"TV", "ONA", "SPECIAL"} {
		found := false
		for _, got := range cfg.IncludeTypes {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("IncludeTypes missing %q", want)
		}
	}
}
