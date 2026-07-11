package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"net"
	"sync"
	"time"

	"hole/pkg/protocol"

	"github.com/gorilla/websocket"
)

// TunnelSession 客户端隧道会话
type TunnelSession struct {
	Conn     *websocket.Conn
	Token    string
	Proxies  map[string]ProxyTarget // subdomain → proxy target
	forwards map[string]*LocalForward
	mu       sync.Mutex
	wsMu     sync.Mutex // 串行化 WebSocket 写操作
	done     chan struct{}
}

// WriteFrame 线程安全地写入帧
func (s *TunnelSession) WriteFrame(frame *protocol.Frame) error {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	return protocol.WriteFrame(s.Conn, frame)
}

// NewTunnelSession 创建客户端隧道会话
func NewTunnelSession(conn *websocket.Conn, token string, proxies map[string]ProxyTarget) *TunnelSession {
	return &TunnelSession{
		Conn:     conn,
		Token:    token,
		Proxies:  proxies,
		forwards: make(map[string]*LocalForward),
		done:     make(chan struct{}),
	}
}

// Start 启动帧循环和心跳发送
func (s *TunnelSession) Start(ctx context.Context) {
	// 启动心跳发送
	go s.sendHeartbeat(ctx)

	// 帧循环
	s.readLoop(ctx)
}

// Stop 停止会话
func (s *TunnelSession) Stop() {
	select {
	case <-s.done:
		return
	default:
		close(s.done)
	}

	s.mu.Lock()
	for _, fwd := range s.forwards {
		fwd.Close()
	}
	s.forwards = make(map[string]*LocalForward)
	s.mu.Unlock()

	s.Conn.Close()

	slog.Info("tunnel session stopped")
}

// Wait 阻塞直到会话断开
func (s *TunnelSession) Wait() {
	<-s.done
}

// AddForward 注册本地转发
func (s *TunnelSession) AddForward(fwd *LocalForward) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.forwards[fwd.TunnelConnID] = fwd
}

// RemoveForward 移除本地转发
func (s *TunnelSession) RemoveForward(connID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.forwards, connID)
}

// GetForward 获取本地转发
func (s *TunnelSession) GetForward(connID string) *LocalForward {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.forwards[connID]
}

func (s *TunnelSession) readLoop(ctx context.Context) {
	defer s.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ctx.Done():
			return
		default:
		}

		frame, err := protocol.ReadFrame(s.Conn)
		if err != nil {
			slog.Warn("read frame error",
				"error", err,
			)
			return
		}

		if err := s.handleFrame(frame); err != nil {
			slog.Warn("handle frame error",
				"error", err,
			)
		}
	}
}

type tunnelOpenPayload struct {
	Type       string `json:"type"`
	Subdomain  string `json:"subdomain"`
	RemoteAddr string `json:"remote_addr"`
}

func (s *TunnelSession) handleFrame(frame *protocol.Frame) error {
	switch frame.Type {
	case protocol.FrameTunnelOpen:
		return s.handleTunnelOpen(frame)

	case protocol.FrameTunnelData:
		fwd := s.GetForward(frame.ConnID)
		if fwd != nil {
			if err := fwd.WriteData(frame.Payload); err != nil {
				slog.Warn("write to local failed",
					"conn_id", frame.ConnID,
					"error", err,
				)
				fwd.Close()
				s.RemoveForward(frame.ConnID)
			}
		}
		return nil

	case protocol.FrameTunnelClose:
		fwd := s.GetForward(frame.ConnID)
		if fwd != nil {
			fwd.Close()
			s.RemoveForward(frame.ConnID)
		}
		slog.Debug("tunnel close received",
			"conn_id", frame.ConnID,
		)
		return nil

	case protocol.FramePong:
		return nil

	default:
		slog.Warn("unknown frame type",
			"type", frame.Type,
		)
		return nil
	}
}

func (s *TunnelSession) handleTunnelOpen(frame *protocol.Frame) error {
	var payload tunnelOpenPayload
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		slog.Error("failed to parse tunnel_open payload",
			"error", err,
		)
		_ = s.WriteFrame(&protocol.Frame{
			Type:   protocol.FrameTunnelClose,
			ConnID: frame.ConnID,
		})
		return nil
	}

	slog.Info("tunnel_open received",
		"conn_id", frame.ConnID,
		"subdomain", payload.Subdomain,
	)

	// 查找代理目标
	proxy, ok := s.Proxies[payload.Subdomain]
	if !ok {
		slog.Warn("unknown subdomain in tunnel_open",
			"subdomain", payload.Subdomain,
		)
		_ = s.WriteFrame(&protocol.Frame{
			Type:   protocol.FrameTunnelClose,
			ConnID: frame.ConnID,
		})
		return nil
	}

	// 建立到本地服务的连接
	var localConn net.Conn
	var err error
	if proxy.TLS {
		localConn, err = tls.Dial("tcp", proxy.Target, &tls.Config{
			InsecureSkipVerify: true,
		})
	} else {
		localConn, err = net.Dial("tcp", proxy.Target)
	}
	if err != nil {
		slog.Warn("dial local failed",
			"target", proxy.Target,
			"tls", proxy.TLS,
			"error", err,
		)
		_ = s.WriteFrame(&protocol.Frame{
			Type:   protocol.FrameTunnelClose,
			ConnID: frame.ConnID,
		})
		return nil
	}

	slog.Info("tunnel_open connected to target",
		"conn_id", frame.ConnID,
		"target", proxy.Target,
	)

	// 创建并启动本地转发
	fwd := NewLocalForward(frame.ConnID, localConn, s)
	s.AddForward(fwd)
	go fwd.Start(context.Background())

	slog.Debug("tunnel opened",
		"conn_id", frame.ConnID,
		"subdomain", payload.Subdomain,
		"target", proxy.Target,
		"tls", proxy.TLS,
	)

	return nil
}

func (s *TunnelSession) sendHeartbeat(ctx context.Context) {
	interval := 30 // default
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			frame := &protocol.Frame{
				Type:   protocol.FramePing,
				ConnID: protocol.ZeroConnID,
			}
			if err := s.WriteFrame(frame); err != nil {
				slog.Warn("heartbeat write failed",
					"error", err,
				)
				return
			}
		case <-s.done:
			return
		case <-ctx.Done():
			return
		}
	}
}
