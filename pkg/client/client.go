package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var (
	ErrAuthFailed        = errors.New("authentication failed")
	ErrSubdomainConflict = errors.New("subdomain conflict")
)

// Client 客户端实例
type Client struct {
	config  *Config
	session *TunnelSession
	cancel  context.CancelFunc
}

// NewClient 创建客户端
func NewClient(cfg *Config) *Client {
	return &Client{
		config: cfg,
	}
}

// Run 运行客户端（阻塞直到上下文取消或致命错误）
func (c *Client) Run(ctx context.Context) error {
	ctx, c.cancel = context.WithCancel(ctx)
	defer c.cancel()

	c.reconnectLoop(ctx)
	return nil
}

// Restart 断开当前隧道连接，触发自动重连（使用最新配置）
func (c *Client) Restart() {
	if c.session != nil {
		slog.Info("restarting tunnel session...")
		c.session.Stop()
	}
}

func (c *Client) reconnectLoop(ctx context.Context) {
	attempt := 0
	maxBackoff := 60 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := c.connect(ctx)
		if err != nil {
			if errors.Is(err, ErrAuthFailed) || errors.Is(err, ErrSubdomainConflict) {
				slog.Error("fatal connection error", "error", err)
				return
			}

			// 指数退避重连
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			if backoff > maxBackoff {
				backoff = maxBackoff
			}

			slog.Warn("connection failed, reconnecting",
				"attempt", attempt+1,
				"backoff", backoff,
				"error", err,
			)

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}

			attempt++
			continue
		}

		// 连接成功，重置尝试计数
		attempt = 0
		slog.Info("connected to server")

		// 等待会话断开
		c.session.Wait()
		slog.Info("session disconnected, reconnecting...")
	}
}

func (c *Client) connect(ctx context.Context) error {
	// 构建 WebSocket URL
	scheme := "ws"
	if c.config.Server.TLS {
		scheme = "wss"
	}
	tunnelPath := c.config.Server.TunnelPath
	if !strings.HasPrefix(tunnelPath, "/") {
		tunnelPath = "/" + tunnelPath
	}
	u := fmt.Sprintf("%s://%s%s", scheme, c.config.Server.Address, tunnelPath)

	// 构建请求头
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.config.Server.Token)

	// 构建子域名列表
	var subdomains []string
	for subdomain := range c.config.Proxies {
		subdomains = append(subdomains, subdomain)
	}
	if len(subdomains) > 0 {
		header.Set("X-Tunnel-Subdomains", strings.Join(subdomains, ","))
	}

	// 建立 WebSocket 连接
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, resp, err := dialer.DialContext(ctx, u, header)
	if err != nil {
		if resp != nil {
			switch resp.StatusCode {
			case http.StatusUnauthorized:
				return fmt.Errorf("%w: invalid token", ErrAuthFailed)
			case http.StatusConflict:
				return fmt.Errorf("%w: subdomain taken", ErrSubdomainConflict)
			}
		}
		return fmt.Errorf("dial error: %w", err)
	}

	if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// 创建隧道会话
	c.session = NewTunnelSession(conn, c.config.Server.Token, c.config.Proxies)

	// 启动会话
	go c.session.Start(ctx)

	return nil
}
