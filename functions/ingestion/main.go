// Package main is the Azure Functions custom handler for the ingestion function.
//
// It starts a plain HTTP server that the Azure Functions host proxies requests
// to. The Connect Protocol handlers for IngestionService and CommandService are
// registered on the mux, with a shared Bearer-token auth interceptor.
//
// Required environment variables:
//
//	DATABASE_URL         - Azure SQL connection string
//	                       e.g. sqlserver://user:pass@host?database=db&encrypt=true
//	INGESTION_API_KEY    - Shared secret validated on every inbound request.
//	                       Must match FUNCTION_API_KEY on the local service.
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
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"
	_ "github.com/microsoft/go-mssqldb"
	"go.uber.org/zap"

	"github.com/anicoll/winet-integration/gen/winet/v1/winetv1connect"
)

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	dsn := mustEnv("DATABASE_URL")
	apiKey := mustEnv("INGESTION_API_KEY")
	port := os.Getenv("FUNCTIONS_CUSTOMHANDLER_PORT")
	if port == "" {
		port = "8080"
	}

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		logger.Fatal("failed to open database connection", zap.Error(err))
	}
	defer func() { _ = db.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		logger.Fatal("failed to reach database", zap.Error(err))
	}
	logger.Info("database connection established")

	svc := newService(newStore(db), logger)

	authInterceptor := newAuthInterceptor(apiKey, logger)
	opts := connect.WithInterceptors(authInterceptor)

	mux := http.NewServeMux()
	mux.Handle(winetv1connect.NewIngestionServiceHandler(svc, opts))
	mux.Handle(winetv1connect.NewCommandServiceHandler(svc, opts))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      http.StripPrefix("/api", mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		logger.Info("ingestion function listening", zap.String("port", port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down ingestion function")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown error", zap.Error(err))
	}
}

// newAuthInterceptor returns a Connect interceptor that validates the Bearer
// token in the Authorization header against the configured API key.
func newAuthInterceptor(apiKey string, logger *zap.Logger) connect.UnaryInterceptorFunc {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			auth := req.Header().Get("Authorization")
			token, ok := strings.CutPrefix(auth, "Bearer ")
			if !ok || token != apiKey {
				logger.Warn("unauthorized request",
					zap.String("procedure", req.Spec().Procedure),
				)
				return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("invalid or missing API key"))
			}
			return next(ctx, req)
		}
	})
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "required environment variable %s is not set\n", key)
		os.Exit(1)
	}
	return v
}
