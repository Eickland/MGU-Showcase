// Команда server — бэкенд витрины МГУ.
//
//	server [serve]                     — запустить HTTP API (по умолчанию)
//	server migrate                     — применить миграции БД и выйти
//	server seed [-file F] [-overwrite] — загрузить начальные записи в реестр
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
	"syscall"
	"time"

	"mgu-showcase/backend/internal/auth"
	"mgu-showcase/backend/internal/config"
	"mgu-showcase/backend/internal/db"
	"mgu-showcase/backend/internal/httpapi"
	"mgu-showcase/backend/internal/registry"
	"mgu-showcase/backend/internal/registry/memstore"
	"mgu-showcase/backend/internal/registry/pgstore"
	"mgu-showcase/backend/internal/seed"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd, args := "serve", os.Args[1:]
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}

	switch cmd {
	case "serve":
		return serve(ctx, cfg, log)
	case "migrate":
		conn, err := openPostgres(ctx, cfg)
		if err != nil {
			return err
		}
		defer conn.Close()
		return db.Migrate(ctx, conn, log)
	case "seed":
		fs := flag.NewFlagSet("seed", flag.ExitOnError)
		file := fs.String("file", cfg.SeedFile, "путь к JSON-файлу с записями")
		overwrite := fs.Bool("overwrite", false, "перезаписать существующие записи (по slug)")
		_ = fs.Parse(args)

		reg, closeFn, err := openRegistry(ctx, cfg, log)
		if err != nil {
			return err
		}
		defer closeFn()
		f, err := seed.Load(*file)
		if err != nil {
			return err
		}
		return seed.Apply(ctx, reg, f, *overwrite, log)
	default:
		return fmt.Errorf("неизвестная команда %q (serve | migrate | seed)", cmd)
	}
}

func serve(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	reg, closeFn, err := openRegistry(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer closeFn()

	if cfg.Storage == "memory" && cfg.SeedFile != "" {
		f, err := seed.Load(cfg.SeedFile)
		if err != nil {
			return err
		}
		if err := seed.Apply(ctx, reg, f, false, log); err != nil {
			return err
		}
	}

	authn := auth.Chain{auth.AdminToken{Token: cfg.AdminToken}}
	if cfg.AuthDevHeaders {
		log.Warn("AUTH_DEV_HEADERS включён: сервер доверяет X-Dev-User/X-Dev-Roles. Не используйте в продакшене!")
		authn = append(authn, auth.DevHeaders{})
	}
	if cfg.AdminToken == "" {
		log.Warn("ADMIN_TOKEN не задан: POST/PATCH/DELETE недоступны")
	}

	api := httpapi.NewServer(httpapi.Config{CORSAllowedOrigins: cfg.CORSAllowedOrigins}, reg, authn, log)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", cfg.HTTPAddr, "storage", cfg.Storage)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func openPostgres(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	if cfg.Storage != "postgres" {
		return nil, errors.New("команда требует STORAGE=postgres")
	}
	return db.Open(ctx, cfg.DatabaseURL)
}

func openRegistry(ctx context.Context, cfg config.Config, log *slog.Logger) (*registry.Registry, func(), error) {
	if cfg.Storage == "memory" {
		log.Warn("STORAGE=memory: данные хранятся в памяти и пропадут после перезапуска")
		return registry.New(memstore.New()), func() {}, nil
	}
	conn, err := openPostgres(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	if cfg.MigrateOnStart {
		if err := db.Migrate(ctx, conn, log); err != nil {
			conn.Close()
			return nil, nil, err
		}
	}
	return registry.New(pgstore.New(conn)), func() { conn.Close() }, nil
}

func newLogger(cfg config.Config) *slog.Logger {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	if cfg.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
