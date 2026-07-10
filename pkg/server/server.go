package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	// 允许所有来源（内网穿透场景）
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Start 启动服务端
func Start(cfg *Config) {
	manager := NewTunnelManager(cfg.Auth.GlobalToken)
	heartbeatInterval := time.Duration(cfg.Heartbeat.Interval) * time.Second
	heartbeatTimeout := time.Duration(cfg.Heartbeat.Interval*cfg.Heartbeat.TimeoutMultiplier) * time.Second

	tunnelPath := cfg.Server.TunnelPath
	// 确保路径以 / 结尾，方便 ServeMux 前缀匹配
	if !strings.HasSuffix(tunnelPath, "/") {
		tunnelPath += "/"
	}

	mux := http.NewServeMux()
	mux.HandleFunc(tunnelPath, func(w http.ResponseWriter, r *http.Request) {
		handleTunnelUpgrade(w, r, manager, cfg, heartbeatInterval, heartbeatTimeout)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleBrowserRequest(w, r, manager, cfg)
	})

	server := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // 流式传输不设写超时
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	slog.Info("server starting",
		"addr", cfg.Server.Addr,
		"domain", cfg.Server.Domain,
		"max_tunnels", cfg.Server.MaxTunnels,
	)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
	}
}

// handleTunnelUpgrade 处理客户端 WebSocket 升级请求
func handleTunnelUpgrade(
	w http.ResponseWriter,
	r *http.Request,
	manager *TunnelManager,
	cfg *Config,
	heartbeatInterval time.Duration,
	heartbeatTimeout time.Duration,
) {
	// 检查是否为 WebSocket 升级请求
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
		return
	}

	// 认证
	if err := Authenticate(r, cfg.Auth.GlobalToken); err != nil {
		slog.Warn("auth failed",
			"remote_addr", r.RemoteAddr,
			"error", err,
		)
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// 解析子域名列表
	subdomainsHeader := r.Header.Get("X-Tunnel-Subdomains")
	var subdomains []string
	if subdomainsHeader != "" {
		for _, s := range strings.Split(subdomainsHeader, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				subdomains = append(subdomains, s)
			}
		}
	}

	// 检查子域名冲突
	if conflict := manager.CheckConflict(subdomains); conflict != "" {
		slog.Warn("subdomain conflict",
			"subdomain", conflict,
			"remote_addr", r.RemoteAddr,
		)
		http.Error(w, "subdomain "+conflict+" taken", http.StatusConflict)
		return
	}

	// 检查最大隧道数
	if manager.Count() >= cfg.Server.MaxTunnels {
		http.Error(w, "max tunnels reached", http.StatusServiceUnavailable)
		return
	}

	// 执行 WebSocket 升级
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed",
			"remote_addr", r.RemoteAddr,
			"error", err,
		)
		return
	}

	// 创建隧道会话
	session := NewTunnelSession(conn, subdomains, cfg.Auth.GlobalToken, manager, heartbeatInterval, heartbeatTimeout)

	// 注册路由表
	if err := manager.Register(session); err != nil {
		slog.Error("register session failed",
			"error", err,
		)
		conn.Close()
		return
	}

	slog.Info("tunnel session created",
		"session_id", session.ID,
		"subdomains", subdomains,
		"remote_addr", r.RemoteAddr,
	)

	// 启动会话
	ctx := context.Background()
	session.Start(ctx)
}

// handleBrowserRequest 处理浏览器（外部）HTTP 请求
func handleBrowserRequest(
	w http.ResponseWriter,
	r *http.Request,
	manager *TunnelManager,
	cfg *Config,
) {
	// 提取子域名
	subdomain := extractSubdomain(r.Host, cfg.Server.Domain)
	if subdomain == "" {
		http.NotFound(w, r)
		return
	}

	// 查找隧道
	session := manager.Lookup(subdomain)
	if session == nil {
		slog.Debug("subdomain not found",
			"subdomain", subdomain,
			"host", r.Host,
		)
		http.NotFound(w, r)
		return
	}

	// 检查会话是否存活
	if !session.IsActive() {
		slog.Warn("session not active",
			"subdomain", subdomain,
		)
		http.Error(w, "tunnel unavailable", http.StatusBadGateway)
		return
	}

	// 重建 HTTP 请求原始字节（已被 Go HTTP server 消耗）
	// 格式: METHOD PATH HTTP/1.1\r\nHeaders...\r\nBody
	var reqBuf bytes.Buffer
	reqBuf.WriteString(fmt.Sprintf("%s %s HTTP/1.1\r\n", r.Method, r.URL.RequestURI()))
	// 写 Host 头
	reqBuf.WriteString(fmt.Sprintf("Host: %s\r\n", r.Host))
	// 写其他头
	for key, values := range r.Header {
		for _, v := range values {
			reqBuf.WriteString(fmt.Sprintf("%s: %s\r\n", key, v))
		}
	}
	reqBuf.WriteString("\r\n")
	// 写 Body
	if r.Body != nil {
		body, _ := io.ReadAll(r.Body)
		reqBuf.Write(body)
	}
	initialData := reqBuf.Bytes()

	// Hijack TCP 连接
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		slog.Error("server does not support hijacking")
		// 此时无法返回 HTTP 错误（已开始写入）
		return
	}

	browserConn, _, err := hijacker.Hijack()
	if err != nil {
		slog.Error("hijack failed",
			"error", err,
		)
		return
	}

	// 创建并启动中继，传入重建的请求字节
	relay := NewRelay(browserConn, session, initialData)
	ctx := context.Background()
	relay.Start(ctx)
}

// extractSubdomain 从 Host 头中提取子域名
// 例如: "myapp.abc.com:8080" → "myapp"
func extractSubdomain(host, domain string) string {
	host = strings.ToLower(host)
	domain = strings.ToLower(domain)

	// 移除端口
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		// 检查是否为 IPv6 地址
		if strings.Count(host, ":") > 1 {
			// IPv6 地址，端口在最后的 "]:" 之后
			if idx2 := strings.LastIndex(host, "]:"); idx2 != -1 {
				host = host[:idx2+1]
			}
		} else {
			host = host[:idx]
		}
	}

	// 去掉方括号（IPv6）
	host = strings.Trim(host, "[]")

	// 等于主域名 → 无子域名
	if host == domain {
		return ""
	}

	// 检查是否为 *.domain 格式
	if strings.HasSuffix(host, "."+domain) {
		return host[:len(host)-len(domain)-1]
	}

	return ""
}
