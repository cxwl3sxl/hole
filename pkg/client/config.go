package client

import (
	"flag"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 客户端配置
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Proxies   map[string]string `yaml:"proxy"`
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`
}

// ServerConfig 服务器连接配置
type ServerConfig struct {
	Address string `yaml:"address"`
	TLS     bool   `yaml:"tls"`
	Token   string `yaml:"token"`
}

// HeartbeatConfig 心跳配置
type HeartbeatConfig struct {
	Interval int `yaml:"interval"`
}

// LoadClientConfig 从 YAML 文件加载客户端配置
func LoadClientConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// 设置默认值
	if cfg.Heartbeat.Interval == 0 {
		cfg.Heartbeat.Interval = 30
	}
	if cfg.Proxies == nil {
		cfg.Proxies = make(map[string]string)
	}

	return &cfg, nil
}

// ParseCLI 解析命令行参数
func ParseCLI() (configPath, subdomain, target string) {
	flag.StringVar(&configPath, "config", "", "配置文件路径")
	flag.StringVar(&subdomain, "subdomain", "", "子域名（覆盖 proxy 配置）")
	flag.StringVar(&target, "target", "", "目标服务地址（覆盖 proxy 配置）")
	flag.Parse()
	return
}

// MergeCLIOverrides 用命令行参数覆盖配置
// 同时提供 subdomain 和 target 时，覆盖整个 proxy 映射
func MergeCLIOverrides(cfg *Config, subdomain, target string) {
	if subdomain != "" && target != "" {
		cfg.Proxies = map[string]string{subdomain: target}
	}
}
