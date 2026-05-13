package main

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"file-service/internal/config"
	"file-service/internal/repo"
	"file-service/internal/server"
	"file-service/internal/storage"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				if ts, ok := attr.Value.Any().(time.Time); ok {
					return slog.String(slog.TimeKey, ts.Format("2006-01-02 15:04:05"))
				}
			}
			return attr
		},
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("加载配置失败", "错误", err)
		os.Exit(1)
	}
	slog.Info("已加载配置", "站点", cfg.SiteName, "监听", cfg.Addr, "数据目录", cfg.DataDir, "分页", cfg.PageSize)

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		slog.Error("创建数据目录失败", "错误", err)
		os.Exit(1)
	}
	slog.Info("数据目录已就绪", "路径", cfg.DataDir)

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		slog.Error("创建数据库目录失败", "错误", err)
		os.Exit(1)
	}
	slog.Info("数据库目录已就绪", "路径", filepath.Dir(cfg.DBPath))

	fileStorage, err := storage.NewLocal(filepath.Join(cfg.DataDir, "files"))
	if err != nil {
		slog.Error("初始化文件存储失败", "错误", err)
		os.Exit(1)
	}
	slog.Info("文件存储已就绪", "路径", filepath.Join(cfg.DataDir, "files"))

	dbRepo, err := repo.NewSQLite(cfg.DBPath, cfg.AccessLogRetention)
	if err != nil {
		slog.Error("打开数据库失败", "错误", err)
		os.Exit(1)
	}
	defer dbRepo.Close()
	slog.Info("数据库已打开", "路径", cfg.DBPath)

	if err := dbRepo.Init(); err != nil {
		slog.Error("初始化数据库失败", "错误", err)
		os.Exit(1)
	}
	slog.Info("数据库结构已就绪")

	srv, err := server.New(cfg, dbRepo, fileStorage)
	if err != nil {
		slog.Error("初始化服务失败", "错误", err)
		os.Exit(1)
	}
	slog.Info("HTTP 路由已就绪")

	slog.Info("文件服务已启动", "监听", cfg.Addr, "数据目录", cfg.DataDir)
	if err := http.ListenAndServe(cfg.Addr, srv.Handler()); err != nil {
		slog.Error("服务已停止", "错误", err)
		os.Exit(1)
	}
}
