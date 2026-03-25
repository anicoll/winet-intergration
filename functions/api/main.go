// Package main is the Azure Functions custom handler for the api function.
//
// It starts a plain HTTP server that the Azure Functions host proxies requests
// to. The existing internal/pkg/server REST handlers and internal/pkg/auth
// JWT auth service are wired against a hand-written Azure SQL database layer.
//
// Required environment variables:
//
//	DATABASE_URL     - Azure SQL connection string
//	                   e.g. sqlserver://user:pass@host?database=db&encrypt=true
//	JWT_SECRET       - HMAC secret for signing JWT access tokens
//	ALLOWED_ORIGIN   - Comma-separated list of allowed CORS origins
//	WINET_DEVICE_ID  - Device ID to target when queuing inverter commands
//
// Optional environment variables:
//
//	JWT_ACCESS_TTL   - Access token TTL (default: 15m)
//	JWT_REFRESH_TTL  - Refresh token TTL (default: 720h)
//	SECURE_COOKIES   - Set Secure flag on cookies (default: true)
//
// The Azure Functions host injects FUNCTIONS_CUSTOMHANDLER_PORT at runtime.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/microsoft/go-mssqldb"
	"go.uber.org/zap"

	"github.com/anicoll/winet-integration/internal/pkg/auth"
	"github.com/anicoll/winet-integration/internal/pkg/server"
	api "github.com/anicoll/winet-integration/pkg/server"
)

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	dsn := mustEnv("DATABASE_URL")
	jwtSecret := mustEnv("JWT_SECRET")
	allowedOrigins := strings.Split(mustEnv("ALLOWED_ORIGIN"), ",")
	deviceID := mustEnv("WINET_DEVICE_ID")

	port := os.Getenv("FUNCTIONS_CUSTOMHANDLER_PORT")
	if port == "" {
		port = "8080"
	}

	accessTTL := envDuration("JWT_ACCESS_TTL", 15*time.Minute)
	refreshTTL := envDuration("JWT_REFRESH_TTL", 720*time.Hour)
	secureCookies := envBool("SECURE_COOKIES", true)

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		logger.Fatal("failed to open database connection", zap.Error(err))
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		logger.Fatal("failed to reach database", zap.Error(err))
	}
	logger.Info("database connection established")

	st := newStore(db)
	cs := newCommandStore(db, deviceID)

	authSvc := auth.NewService(jwtSecret, accessTTL, refreshTTL, st, st)
	authSvc.StartCleanup(ctx, time.Hour)

	apiHandler := api.HandlerWithOptions(server.New(cs, st, authSvc, secureCookies), api.StdHTTPServerOptions{
		Middlewares: []api.MiddlewareFunc{
			server.TimeoutMiddleware,
			server.LoggingMiddleware(allowedOrigins),
			server.AuthMiddleware(authSvc),
		},
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error("HTTP handler error", zap.Error(err))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		},
	})

	mux := http.NewServeMux()
	mux.Handle("/", apiHandler)
	mux.HandleFunc("OPTIONS /", func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if slices.Contains(allowedOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		logger.Info("api function listening", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down api function")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown error", zap.Error(err))
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "required environment variable %s is not set\n", key)
		os.Exit(1)
	}
	return v
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
