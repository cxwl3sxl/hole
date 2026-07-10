package main

import (
	"flag"
	"log"
	"log/slog"
	"os"

	"hole/pkg/server"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "配置文件路径")
	flag.Parse()

	if configPath == "" {
		log.Fatal("请指定 -config 参数")
	}

	cfg, err := server.LoadServerConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化日志
	initSlog(cfg.Logging.Level)

	slog.Info("starting hole-server",
		"config", configPath,
	)

	server.Start(cfg)
}

func initSlog(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: lvl,
	}
	handler := slog.NewTextHandler(os.Stderr, opts)
	slog.SetDefault(slog.New(handler))
}
