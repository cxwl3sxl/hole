package server

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config 服务端配置
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Auth      AuthConfig      `yaml:"auth"`
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`
	Logging   LoggingConfig   `yaml:"logging"`
}

// ServerConfig HTTP 服务器配置
type ServerConfig struct {
	Addr        string `yaml:"addr"`
	Domain      string `yaml:"domain"`
	IdleTimeout int    `yaml:"idle_timeout"`
	MaxTunnels  int    `yaml:"max_tunnels"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	GlobalToken string `yaml:"global_token"`
}

// HeartbeatConfig 心跳配置
type HeartbeatConfig struct {
	Interval          int `yaml:"interval"`
	TimeoutMultiplier int `yaml:"timeout_multiplier"`
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// LoadServerConfig 从 YAML 文件加载服务端配置
func LoadServerConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// 设置默认值
	setDefaults(&cfg)

	return &cfg, nil
}

func setDefaults(cfg *Config) {
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = ":8080"
	}
	if cfg.Server.Domain == "" {
		cfg.Server.Domain = "abc.com"
	}
	if cfg.Server.IdleTimeout == 0 {
		cfg.Server.IdleTimeout = 300
	}
	if cfg.Server.MaxTunnels == 0 {
		cfg.Server.MaxTunnels = 1000
	}
	if cfg.Heartbeat.Interval == 0 {
		cfg.Heartbeat.Interval = 30
	}
	if cfg.Heartbeat.TimeoutMultiplier == 0 {
		cfg.Heartbeat.TimeoutMultiplier = 3
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
}
