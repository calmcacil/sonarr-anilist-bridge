package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/calmcacil/sonarr-anime-bridge/internal/cache"
	"github.com/calmcacil/sonarr-anime-bridge/internal/config"
	"github.com/calmcacil/sonarr-anime-bridge/internal/scheduler"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()

	setupLogging(cfg.LogLevel)

	slog.Info("starting sonarr-seasonal",
		"port", cfg.Port,
		"prewarm_years", cfg.PrewarmYears,
		"prewarm_seasons", cfg.PrewarmSeasons,
	)

	db, err := cache.Open(cfg.CacheDBPath)
	if err != nil {
		return fmt.Errorf("open cache: %w", err)
	}
	defer db.Close() //nolint:errcheck // cleanup on exit

	sched := scheduler.New(db, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Warm the cache before accepting requests so the first client hit
	// gets real data instead of triggering an async backfill.
	sched.LoadResolver()
	slog.Info("prewarming cache")
	if err := sched.Prewarm(ctx); err != nil {
		slog.Error("prewarm failed", "error", err)
	}
	slog.Info("prewarm complete")

	mux := http.NewServeMux()
	mux.HandleFunc("/list", handleList(db, sched, cfg))
	mux.HandleFunc("/health", handleHealth(db, sched))
	mux.HandleFunc("/cache/stats", handleCacheStats(db))

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      loggingMiddleware(recoveryMiddleware(mux)),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	sched.StartBackground(ctx)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in HTTP server goroutine", "recover", r)
			}
		}()
		slog.Info("listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	slog.Info("shutting down", "signal", sig)
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	serverErr := server.Shutdown(shutdownCtx)

	// Wait for background goroutines to finish, with a short grace period.
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer waitCancel()
	if err := sched.Wait(waitCtx); err != nil {
		slog.Warn("some background goroutines did not finish in time", "error", err)
	}

	return serverErr
}

func handleList(db *cache.Cache, sched *scheduler.Scheduler, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		season := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("season")))
		if season == "" {
			season = "ALL"
		}

		validSeasons := map[string]bool{"WINTER": true, "SPRING": true, "SUMMER": true, "FALL": true, "ALL": true}
		if !validSeasons[season] {
			http.Error(w, "invalid season parameter", http.StatusBadRequest)
			return
		}

		yearStr := r.URL.Query().Get("year")
		year := time.Now().Year()
		if yearStr != "" {
			if y, err := strconv.Atoi(yearStr); err == nil && y > 0 {
				// Clamp to ±10 years to prevent excessive queries
				switch {
				case y < year-10:
					year -= 10
				case y > year+10:
					year += 10
				default:
					year = y
				}
			}
		}

		category := strings.TrimSpace(r.URL.Query().Get("category"))
		if category == "" {
			category = "series"
		}
		if category != "series" && category != "series-new" {
			category = "series"
		}

		data, fresh, isPending, ok := db.Get(season, year, category)
		if !ok {
			slog.Info("cache miss, triggering backfill",
				"season", season,
				"year", year,
				"category", category,
			)

			if err := sched.FetchAndStore(r.Context(), season, year, category); err != nil {
				slog.Error("trigger backfill failed", "error", err)
			}

			writeJSON(w, []byte("[]"))
			return
		}

		if isPending {
			writeJSON(w, []byte("[]"))
			return
		}

		// Stale data triggers proactive refresh; caller still gets cached response.
		if !fresh {
			slog.Debug("serving stale data, triggering refresh",
				"season", season,
				"year", year,
				"category", category,
			)
			sched.StartRefresh(context.WithoutCancel(r.Context()), season, year, category)
		}

		writeJSON(w, data)
	}
}

func handleHealth(db *cache.Cache, sched *scheduler.Scheduler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		healthy := true
		if err := db.Ping(); err != nil {
			slog.Error("health check failed", "error", err)
			healthy = false
		}
		resolverOK := sched.ResolverLoaded()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case healthy && resolverOK:
			w.Write([]byte(`{"status":"ok"}`))
		case healthy:
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"degraded","reason":"resolver not loaded"}`))
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unhealthy"}`))
		}
	}
}

func handleCacheStats(db *cache.Cache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats := db.Stats()
		data, err := json.Marshal(stats)
		if err != nil {
			slog.Error("marshal cache stats", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeJSON(w, data)
	}
}

func writeJSON(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// loggingMiddleware logs the method, path, status, and duration of each request.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		srw := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(srw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", srw.status,
			"duration", time.Since(start),
		)
	})
}

// recoveryMiddleware catches panics in downstream handlers and returns 500.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rc := recover(); rc != nil {
				slog.Error("panic recovered",
					"path", r.URL.Path,
					"error", rc,
				)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusResponseWriter wraps http.ResponseWriter to capture the status code.
type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (srw *statusResponseWriter) WriteHeader(code int) {
	srw.status = code
	srw.ResponseWriter.WriteHeader(code)
}

func setupLogging(level string) {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn", "warning":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})
	slog.SetDefault(slog.New(handler))
}
