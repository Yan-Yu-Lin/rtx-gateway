package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Yan-Yu-Lin/rtx-gateway/internal/admin"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/auth"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/config"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/db"
	"github.com/Yan-Yu-Lin/rtx-gateway/internal/proxy"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()
	database, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	if err := db.Migrate(ctx, database, cfg.DefaultEndpoints); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	if len(os.Args) > 1 && os.Args[1] == "seed-key" {
		return seedKeyCommand(ctx, database, cfg)
	}

	if cfg.SeedTestKey {
		if err := maybeSeedTestKey(ctx, database, cfg, logger); err != nil {
			return err
		}
	}

	if cfg.KeyPepper == "dev-insecure-change-me" {
		logger.Warn("using default key pepper; set RTX_GATEWAY_KEY_PEPPER before production")
	}

	publicServer := &http.Server{
		Addr:              cfg.PublicAddr,
		Handler:           proxy.NewRouter(database, cfg, logger),
		ReadHeaderTimeout: 10 * time.Second,
	}
	adminServer := &http.Server{
		Addr:              cfg.AdminAddr,
		Handler:           admin.NewRouter(database, cfg),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errs := make(chan error, 2)
	go serve("public", publicServer, logger, errs)
	go serve("admin", adminServer, logger, errs)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case signal := <-stop:
		logger.Info("received shutdown signal", "signal", signal.String())
	case err := <-errs:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := publicServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to stop public server", "error", err)
	}
	if err := adminServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to stop admin server", "error", err)
	}
	return nil
}

func serve(name string, server *http.Server, logger *slog.Logger, errs chan<- error) {
	logger.Info("starting server", "name", name, "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errs <- fmt.Errorf("%s server: %w", name, err)
	}
}

func seedKeyCommand(ctx context.Context, database *sql.DB, cfg config.Config) error {
	flags := flag.NewFlagSet("seed-key", flag.ExitOnError)
	name := flags.String("name", "test key", "API key name")
	scopes := flags.String("scopes", "llm,ocr", "comma-separated scopes")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}

	key, err := auth.CreateAPIKey(ctx, database, cfg.KeyPepper, *name, splitScopes(*scopes))
	if err != nil {
		return err
	}

	fmt.Printf("id: %s\n", key.ID)
	fmt.Printf("name: %s\n", key.Name)
	fmt.Printf("prefix: %s\n", key.Prefix)
	fmt.Printf("scopes: %s\n", strings.Join(key.Scopes, ","))
	fmt.Printf("key: %s\n", key.RawKey)
	return nil
}

func maybeSeedTestKey(ctx context.Context, database *sql.DB, cfg config.Config, logger *slog.Logger) error {
	count, err := auth.CountAPIKeys(ctx, database)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	key, err := auth.CreateAPIKey(ctx, database, cfg.KeyPepper, "first-run test key", []string{"llm", "ocr"})
	if err != nil {
		return err
	}

	logger.Warn("created first-run test API key; save it now because it will not be shown again",
		"id", key.ID,
		"prefix", key.Prefix,
		"key", key.RawKey,
	)
	return nil
}

func splitScopes(raw string) []string {
	parts := strings.Split(raw, ",")
	scopes := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			scopes = append(scopes, value)
		}
	}
	return scopes
}
