package client

import (
	"flag"
	"os"

	"gopkg.in/yaml.v3"
)

// ProxyTarget 表示一个代理目标，支持普通 TCP 和 TLS
type ProxyTarget struct {
	Target string `yaml:"target"`
	TLS    bool   `yaml:"tls"`
}

// UnmarshalYAML 支持两种配置格式：
//
//	简写（无 TLS）： myapp: "127.0.0.1:3000"
//	完整（有 TLS）： myapp: {target: "127.0.0.1:3000", tls: true}
func (pt *ProxyTarget) UnmarshalYAML(value *yaml.Node) error {
	// 先尝试解析为字符串（兼容旧格式）
	var str string
	if err := value.Decode(&str); err == nil {
		pt.Target = str
		pt.TLS = false
		return nil
	}

	// 再尝试解析为结构体
	type rawProxy struct {
		Target string `yaml:"target"`
		TLS    bool   `yaml:"tls"`
	}
	var raw rawProxy
	if err := value.Decode(&raw); err != nil {
		return err
	}
	pt.Target = raw.Target
	pt.TLS = raw.TLS
	return nil
}

// Config 客户端配置
type Config struct {
	Server    ServerConfig          `yaml:"server"`
	Proxies   map[string]ProxyTarget `yaml:"proxy"`
	Heartbeat HeartbeatConfig       `yaml:"heartbeat"`
}

// ServerConfig 服务器连接配置
type ServerConfig struct {
	Address    string `yaml:"address"`
	TLS        bool   `yaml:"tls"`
	Token      string `yaml:"token"`
	TunnelPath string `yaml:"tunnel_path"` // WebSocket 升级路径
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
		cfg.Proxies = make(map[string]ProxyTarget)
	}
	if cfg.Server.TunnelPath == "" {
		cfg.Server.TunnelPath = "/_tunnel/"
	}

	return &cfg, nil
}

// ParseCLI 解析命令行参数
func ParseCLI() (configPath, subdomain, target string, tls bool) {
	flag.StringVar(&configPath, "config", "", "配置文件路径")
	flag.StringVar(&subdomain, "subdomain", "", "子域名（覆盖 proxy 配置）")
	flag.StringVar(&target, "target", "", "目标服务地址（覆盖 proxy 配置）")
	flag.BoolVar(&tls, "tls", false, "目标服务是否走 TLS（与 -subdomain/-target 配合）")
	flag.Parse()
	return
}

// MergeCLIOverrides 用命令行参数覆盖配置
// 同时提供 subdomain 和 target 时，覆盖整个 proxy 映射
func MergeCLIOverrides(cfg *Config, subdomain, target string, tls bool) {
	if subdomain != "" && target != "" {
		cfg.Proxies = map[string]ProxyTarget{
			subdomain: {Target: target, TLS: tls},
		}
	}
}
