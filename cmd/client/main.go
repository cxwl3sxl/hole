package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	"hole/pkg/client"
)

func main() {
	configPath, subdomain, target := client.ParseCLI()

	if configPath == "" {
		log.Fatal("请指定 -config 参数")
	}

	cfg, err := client.LoadClientConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 命令行覆盖
	client.MergeCLIOverrides(cfg, subdomain, target)

	// 初始化日志
	initSlog()

	slog.Info("starting hole-client",
		"config", configPath,
		"server", cfg.Server.Address,
		"proxies", cfg.Proxies,
	)

	c := client.NewClient(cfg)
	ctx := context.Background()
	if err := c.Run(ctx); err != nil {
		slog.Error("client error", "error", err)
		os.Exit(1)
	}
}

func initSlog() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	handler := slog.NewTextHandler(os.Stderr, opts)
	slog.SetDefault(slog.New(handler))
}
