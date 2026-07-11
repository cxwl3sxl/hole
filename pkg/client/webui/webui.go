package webui

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"hole/pkg/client"

	"gopkg.in/yaml.v3"
)

//go:embed static
var staticFiles embed.FS

// Server Web 管理界面服务器
type Server struct {
	config     *client.Config
	configPath string
	restartFn  func() // 重启隧道连接的函数
	mu         sync.RWMutex
}

// NewServer 创建 Web 管理界面服务器
func NewServer(cfg *client.Config, configPath string, restartFn func()) *Server {
	return &Server{
		config:     cfg,
		configPath: configPath,
		restartFn:  restartFn,
	}
}

// Start 启动 Web 管理界面 HTTP 服务器
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API 路由
	mux.HandleFunc("/api/proxies", s.handleProxies)
	mux.HandleFunc("/api/proxies/save", s.handleSaveConfig)
	mux.HandleFunc("/api/proxies/restart", s.handleRestart)

	// 静态文件
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("failed to get static fs: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	slog.Info("web management interface started",
		"addr", s.config.WebUI.Addr,
	)

	return http.ListenAndServe(s.config.WebUI.Addr, mux)
}

// handleProxies 获取/更新代理映射
func (s *Server) handleProxies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		defer s.mu.RUnlock()
		writeJSON(w, http.StatusOK, s.config.Proxies)

	case http.MethodPut:
		var proxies map[string]client.ProxyTarget
		if err := json.NewDecoder(r.Body).Decode(&proxies); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}

		s.mu.Lock()
		s.config.Proxies = proxies
		s.mu.Unlock()

		slog.Info("proxies updated via web UI", "count", len(proxies))
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	case http.MethodOptions:
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSaveConfig 保存配置到文件
func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	cfg := *s.config
	s.mu.RUnlock()

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "marshal config failed: " + err.Error()})
		return
	}

	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save config failed: " + err.Error()})
		return
	}

	slog.Info("config saved to file", "path", s.configPath)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "path": s.configPath})
}

// handleRestart 保存配置并重启隧道
func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 先保存配置
	s.mu.RLock()
	cfg := *s.config
	s.mu.RUnlock()

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "marshal config failed: " + err.Error()})
		return
	}

	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save config failed: " + err.Error()})
		return
	}

	// 触发重连
	if s.restartFn != nil {
		s.restartFn()
	}

	slog.Info("config saved and tunnel restarted", "path", s.configPath)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "path": s.configPath})
}

func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}
