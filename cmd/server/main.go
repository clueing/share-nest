package main

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"file-service/internal/config"
	"file-service/internal/repo"
	"file-service/internal/server"
	"file-service/internal/storage"
)

func main() {
	cfg := config.Load()

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		slog.Error("create data dir", "err", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		slog.Error("create db dir", "err", err)
		os.Exit(1)
	}

	fileStorage, err := storage.NewLocal(filepath.Join(cfg.DataDir, "files"))
	if err != nil {
		slog.Error("init storage", "err", err)
		os.Exit(1)
	}

	dbRepo, err := repo.NewSQLite(cfg.DBPath)
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}
	defer dbRepo.Close()

	if err := dbRepo.Init(); err != nil {
		slog.Error("init database", "err", err)
		os.Exit(1)
	}

	srv, err := server.New(cfg, dbRepo, fileStorage)
	if err != nil {
		slog.Error("init server", "err", err)
		os.Exit(1)
	}

	slog.Info("file service started", "addr", cfg.Addr, "data_dir", cfg.DataDir)
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

