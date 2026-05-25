package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

	"github.com/edmondlafaydavid/scoreplay/internal/apispec"
	"github.com/edmondlafaydavid/scoreplay/internal/config"
	"github.com/edmondlafaydavid/scoreplay/internal/database"
	"github.com/edmondlafaydavid/scoreplay/internal/handler"
	"github.com/edmondlafaydavid/scoreplay/internal/observability"
	"github.com/edmondlafaydavid/scoreplay/internal/repository"
	"github.com/edmondlafaydavid/scoreplay/internal/service"
)


func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "err", err)
		os.Exit(1)
	}

	logger.Info("starting",
		"port", cfg.Port,
		"upload_dir", cfg.UploadDir,
		"base_url", cfg.BaseURL,
		"db_max_open_conns", cfg.DBMaxOpenConns,
		"trace_sample_rate", cfg.TraceSampleRate,
	)

	// Tracing — noop when OTEL_EXPORTER_OTLP_ENDPOINT is unset.
	shutdownTracer, err := observability.InitTracer(ctx, "scoreplay", cfg.TraceSampleRate)
	if err != nil {
		logger.Error("init tracer", "err", err)
		os.Exit(1)
	}
	defer shutdownTracer(ctx) //nolint:errcheck

	// Migrations use a dedicated connection: golang-migrate's m.Close() closes
	// the underlying *sql.DB, which would invalidate the server's connection pool.
	migDB, err := openDB(cfg.DatabaseURL)
	if err != nil {
		logger.Error("open migration db", "err", err)
		os.Exit(1)
	}
	if err := database.RunMigrations(migDB); err != nil {
		logger.Error("run migrations", "err", err)
		os.Exit(1)
	}
	migDB.Close()

	db, err := openDB(cfg.DatabaseURL)
	if err != nil {
		logger.Error("open database", "err", err)
		os.Exit(1)
	}
	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	defer db.Close()

	if err := os.MkdirAll(cfg.UploadDir, 0755); err != nil {
		logger.Error("create upload dir", "err", err)
		os.Exit(1)
	}

	// Repositories
	tagRepo := repository.NewTagRepository(db)
	mediaRepo := repository.NewMediaRepository(db)
	clientRepo := repository.NewClientRepository(db)

	// Services
	tagSvc := service.NewTagService(tagRepo)
	mediaSvc := service.NewMediaService(mediaRepo, tagRepo, cfg.UploadDir, cfg.BaseURL)
	clientSvc := service.NewClientService(clientRepo)

	// Handlers
	tagHandler := handler.NewTagHandler(tagSvc)
	mediaHandler := handler.NewMediaHandler(mediaSvc)

	authMiddleware := handler.APIKeyMiddleware(clientSvc)

	// OTel HTTP middleware — creates a span per request using the matched chi
	// route pattern as the span name (e.g. "GET /media/{id}").
	otelMiddleware := otelhttp.NewMiddleware("scoreplay-api",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			if chiCtx := chi.RouteContext(r.Context()); chiCtx != nil {
				if p := chiCtx.RoutePattern(); p != "" {
					return r.Method + " " + p
				}
			}
			return r.Method + " " + r.URL.Path
		}),
	)

	r := chi.NewRouter()

	// Base middleware — applies to every route including /health, /metrics, /openapi.yaml.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(handler.SecurityHeaders)
	r.Use(observability.MetricsMiddleware)
	r.Use(otelMiddleware)
	r.Use(requestLogger(logger))

	// Unprotected endpoints (no API key required).

	// Liveness probe — process is alive. Always returns 200; used by process supervisors.
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck
	})

	// Readiness probe — service is ready to accept traffic. Pings the DB.
	// Returns 503 if the DB is unreachable; used by container orchestrators before
	// routing traffic and as the Docker Compose healthcheck.
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			logger.Warn("readiness check failed", "err", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unavailable"}`)) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`)) //nolint:errcheck
	})
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		w.Write(apispec.Spec) //nolint:errcheck
	})

	// Protected routes — all require a valid API key.
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)

		r.Route("/tags", func(r chi.Router) {
			r.Post("/", tagHandler.CreateTag)
			r.Get("/", tagHandler.ListTags)
		})

		r.Route("/media", func(r chi.Router) {
			r.Get("/", mediaHandler.ListMedia)
			r.Post("/", mediaHandler.CreateMedia)
			r.Get("/{id}", mediaHandler.GetMedia)
		})

		r.Handle("/uploads/*", http.StripPrefix("/uploads/",
			http.FileServer(http.Dir(cfg.UploadDir))))
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("shutting down")
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "err", err)
	}
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	db.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)

			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			}

			// Attach trace/span IDs when an active OTel span is present.
			if sc := trace.SpanFromContext(r.Context()).SpanContext(); sc.IsValid() {
				attrs = append(attrs,
					"trace_id", sc.TraceID().String(),
					"span_id", sc.SpanID().String(),
				)
			}

			logger.Info("request", attrs...)
		})
	}
}
