package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAnibridgeMappingPath = "/data/anibridge_mappings.json.zst"
	DefaultAnibridgeURL         = "https://github.com/anibridge/anibridge-mappings/releases/download/v3/mappings.json.zst"
	DefaultAnibridgeRefreshDays = 1
)

type Config struct {
	Port               int
	PrewarmYears       []int
	PrewarmSeasons     []string
	MaxPerSeason       int
	IncludeONA         bool
	WinterOverflow     bool
	AheadMonths        *int
	ExcludeTags        []string
	CacheDBPath        string
	CacheStaleDays     int
	RefreshCurrentDays int
	RefreshPastDays    int
	AniListTimeoutMin  int
	LogLevel           string

	AnibridgeMappingPath string
	AnibridgeRefreshDays int
	AnibridgeURL         string
}

const (
	DefaultPort               = 8080
	DefaultMaxPerSeason       = 100
	DefaultCacheDBPath        = "/data/cache.db"
	DefaultCacheStaleDays     = 14
	DefaultRefreshCurrentDays = 7
	DefaultRefreshPastDays    = 30
	DefaultAniListTimeoutMin  = 10
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

func (c *Config) AheadMonthsOrDefault() int {
	if c.AheadMonths != nil {
		return *c.AheadMonths
	}
	return 3
}

func Load() *Config {
	cfg := &Config{
		Port:               getEnvInt("PORT", DefaultPort),
		MaxPerSeason:       getEnvInt("MAX_PER_SEASON", DefaultMaxPerSeason),
		IncludeONA:         getEnvBool("ALG_ANILIST_INCLUDE_ONA", false),
		WinterOverflow:     getEnvBool("ALG_ANILIST_WINTER_OVERFLOW", true),
		CacheDBPath:        getEnvStr("CACHE_DB_PATH", DefaultCacheDBPath),
		CacheStaleDays:     getEnvInt("CACHE_STALE_DAYS", DefaultCacheStaleDays),
		RefreshCurrentDays: getEnvInt("REFRESH_CURRENT_DAYS", DefaultRefreshCurrentDays),
		RefreshPastDays:    getEnvInt("REFRESH_PAST_DAYS", DefaultRefreshPastDays),
		AniListTimeoutMin:  getEnvInt("ALG_ANILIST_TIMEOUT_MINUTES", DefaultAniListTimeoutMin),
		LogLevel:           getEnvStr("LOG_LEVEL", "info"),

		AnibridgeMappingPath: getEnvStr("ALG_ANIBRIDGE_MAPPING_PATH", DefaultAnibridgeMappingPath),
		AnibridgeRefreshDays: getEnvInt("ALG_ANIBRIDGE_REFRESH_DAYS", DefaultAnibridgeRefreshDays),
		AnibridgeURL:         getEnvStr("ALG_ANIBRIDGE_URL", DefaultAnibridgeURL),
	}

	cfg.PrewarmYears = parseYearList("PREWARM_YEARS", []int{time.Now().Year()})
	cfg.PrewarmSeasons = ResolveSeasons(parseStringList("PREWARM_SEASONS", []string{"all"}))

	if aheadStr := os.Getenv("AHEAD_MONTHS"); aheadStr != "" {
		if m, err := strconv.Atoi(aheadStr); err == nil && m >= 0 {
			cfg.AheadMonths = &m
		}
	}
	if aheadStr := os.Getenv("ALG_ANILIST_AHEAD_MONTHS"); aheadStr != "" && cfg.AheadMonths == nil {
		if m, err := strconv.Atoi(aheadStr); err == nil && m >= 0 {
			cfg.AheadMonths = &m
		}
	}

	cfg.ExcludeTags = parseStringList("ALG_ANILIST_EXCLUDE_TAGS", nil)

	return cfg
}

func getEnvStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
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

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v == "1" || strings.EqualFold(v, "true")
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
			out = append(out, p)
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
