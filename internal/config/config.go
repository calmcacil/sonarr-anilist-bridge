package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAnibridgeMappingPath = "/data/anibridge_mappings.json.zst"
	DefaultAnibridgeURL         = "https://github.com/anibridge/anibridge-mappings/releases/download/v3/mappings.json.zst"
)

type Config struct {
	Port                int
	PrewarmYears        []int
	PrewarmSeasons      []string
	MaxPerSeason        int
	IncludeTypes        []string
	ExcludeTags         []string
	CacheDBPath         string
	LogLevel            string
	FilterFutureEnabled bool

	AnibridgeMappingPath string
	AnibridgeURL         string
}

const (
	DefaultPort         = 8080
	DefaultMaxPerSeason = 100
	DefaultCacheDBPath  = "/data/cache.db"
)

func AllSeasons() []string {
	return []string{"WINTER", "SPRING", "SUMMER", "FALL"}
}

func ResolveSeasons(raw []string) []string {
	if len(raw) == 0 {
		return AllSeasons()
	}
	seen := make(map[string]bool, len(raw))
	var out []string
	for _, s := range raw {
		if strings.EqualFold(s, "all") {
			return AllSeasons()
		}
		var season string
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "winter":
			season = "WINTER"
		case "spring":
			season = "SPRING"
		case "summer":
			season = "SUMMER"
		case "fall":
			season = "FALL"
		}
		if season != "" && !seen[season] {
			seen[season] = true
			out = append(out, season)
		}
	}
	return out
}

func Load() *Config {
	cfg := &Config{
		Port:     getEnvInt("PORT", DefaultPort),
		MaxPerSeason: getEnvInt("MAX_PER_SEASON", DefaultMaxPerSeason),
		CacheDBPath:  getEnvStr("CACHE_DB_PATH", DefaultCacheDBPath),
		LogLevel:     getEnvStr("LOG_LEVEL", "info"),

		AnibridgeMappingPath: getEnvStr("MAPPING_PATH", DefaultAnibridgeMappingPath),
		AnibridgeURL:         getEnvStr("MAPPING_URL", DefaultAnibridgeURL),
	}

	// Validate and clamp Port
	if cfg.Port < 1 || cfg.Port > 65535 {
		slog.Warn("PORT invalid, using default", "value", cfg.Port, "default", DefaultPort)
		cfg.Port = DefaultPort
	}

	// Clamp MaxPerSeason to reasonable bounds
	if cfg.MaxPerSeason < 1 {
		slog.Warn("MAX_PER_SEASON clamped to 1", "value", cfg.MaxPerSeason)
		cfg.MaxPerSeason = 1
	}
	if cfg.MaxPerSeason > 500 {
		slog.Warn("MAX_PER_SEASON clamped to 500", "value", cfg.MaxPerSeason)
		cfg.MaxPerSeason = 500
	}

	cfg.PrewarmYears = parseYearList("PREWARM_YEARS", []int{time.Now().Year()})

	// If PREWARM_YEARS was set but parsing fell back to the default, warn.
	if v := os.Getenv("PREWARM_YEARS"); v != "" {
		currentYear := time.Now().Year()
		if len(cfg.PrewarmYears) == 1 && cfg.PrewarmYears[0] == currentYear {
			slog.Warn("PREWARM_YEARS contained no valid years, falling back to default",
				"raw_value", v, "default_year", currentYear)
		}
	}

	cfg.PrewarmSeasons = ResolveSeasons(parseStringList("PREWARM_SEASONS", []string{"all"}))
	cfg.IncludeTypes = parseStringList("INCLUDE_TYPES", []string{"TV", "ONA"})
	cfg.ExcludeTags = parseStringList("EXCLUDE_TAGS", nil)
	cfg.FilterFutureEnabled = getEnvBool("FILTER_FUTURE_ENABLED", true)

	slog.Info("config loaded",
		"port", cfg.Port,
		"max_per_season", cfg.MaxPerSeason,
		"include_types", cfg.IncludeTypes,
		"exclude_tags", cfg.ExcludeTags,
		"filter_future_enabled", cfg.FilterFutureEnabled,
		"prewarm_years", cfg.PrewarmYears,
		"prewarm_seasons", cfg.PrewarmSeasons,
		"cache_db_path", cfg.CacheDBPath,
		"mapping_path", cfg.AnibridgeMappingPath,
		"mapping_url", cfg.AnibridgeURL,
		"log_level", cfg.LogLevel,
	)

	return cfg
}

func getEnvStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func parseStringList(key string, def []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	parts := strings.Split(v, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, strings.ToUpper(p))
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func parseYearList(key string, def []int) []int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	parts := strings.Split(v, ",")
	var out []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if y, err := strconv.Atoi(p); err == nil && y > 0 {
			out = append(out, y)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}
