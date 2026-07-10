package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"hole/pkg/protocol"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// TunnelSession 表示一个客户端隧道连接
type TunnelSession struct {
	ID           string
	Subdomains   []string
	Token        string
	Conn         *websocket.Conn
	manager      *TunnelManager
	activeRelays map[string]*Relay // tunnel_conn_id → Relay
	lastPong     time.Time
	mu           sync.Mutex
	done         chan struct{}

	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration
}

// NewTunnelSession 创建隧道会话
func NewTunnelSession(conn *websocket.Conn, subdomains []string, token string, manager *TunnelManager, heartbeatInterval, heartbeatTimeout time.Duration) *TunnelSession {
	return &TunnelSession{
		ID:                uuid.New().String(),
		Subdomains:        subdomains,
		Token:             token,
		Conn:              conn,
		manager:           manager,
		activeRelays:      make(map[string]*Relay),
		lastPong:          time.Now(),
		done:              make(chan struct{}),
		heartbeatInterval: heartbeatInterval,
		heartbeatTimeout:  heartbeatTimeout,
	}
}

// Start 启动帧读取循环和心跳监控
func (s *TunnelSession) Start(ctx context.Context) {
	// 启动心跳监控
	go s.heartbeatMonitor(ctx)

	// 帧读取循环
	go s.readLoop(ctx)
}

// Stop 停止会话并清理资源
func (s *TunnelSession) Stop() {
	select {
	case <-s.done:
		// 已停止
		return
	default:
		close(s.done)
	}

	s.mu.Lock()
	// 关闭所有活跃连接
	for _, relay := range s.activeRelays {
		relay.Close()
	}
	s.activeRelays = make(map[string]*Relay)
	s.mu.Unlock()

	// 从路由表注销
	s.manager.Unregister(s)

	// 关闭 WebSocket 连接
	s.Conn.Close()

	slog.Info("tunnel session stopped",
		"session_id", s.ID,
		"subdomains", s.Subdomains,
	)
}

// IsActive 检查会话是否活跃
func (s *TunnelSession) IsActive() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

// AddRelay 注册中继
func (s *TunnelSession) AddRelay(relay *Relay) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeRelays[relay.TunnelConnID] = relay
}

// RemoveRelay 移除中继
func (s *TunnelSession) RemoveRelay(connID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.activeRelays, connID)
}

// GetRelay 获取中继
func (s *TunnelSession) GetRelay(connID string) *Relay {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeRelays[connID]
}

// heartbeatMonitor 心跳超时监控
func (s *TunnelSession) heartbeatMonitor(ctx context.Context) {
	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			elapsed := time.Since(s.lastPong)
			s.mu.Unlock()

			if elapsed > s.heartbeatTimeout {
				slog.Warn("heartbeat timeout, closing session",
					"session_id", s.ID,
					"elapsed", elapsed,
				)
				s.Stop()
				return
			}
		case <-s.done:
			return
		case <-ctx.Done():
			return
		}
	}
}

// readLoop 帧读取循环
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
				"session_id", s.ID,
				"error", err,
			)
			return
		}

		if err := s.HandleFrame(frame); err != nil {
			slog.Warn("handle frame error",
				"session_id", s.ID,
				"error", err,
			)
		}
	}
}

// HandleFrame 处理接收到的帧
func (s *TunnelSession) HandleFrame(frame *protocol.Frame) error {
	switch frame.Type {
	case protocol.FramePing:
		// 回复 Pong
		pong := &protocol.Frame{
			Type:   protocol.FramePong,
			ConnID: protocol.ZeroConnID,
		}
		return protocol.WriteFrame(s.Conn, pong)

	case protocol.FramePong:
		s.mu.Lock()
		s.lastPong = time.Now()
		s.mu.Unlock()

	case protocol.FrameTunnelData:
		// 查找对应的 relay 并写入数据到浏览器连接
		relay := s.GetRelay(frame.ConnID)
		if relay != nil {
			if err := relay.WriteData(frame.Payload); err != nil {
				slog.Warn("write to browser failed",
					"conn_id", frame.ConnID,
					"error", err,
				)
				relay.Close()
				s.RemoveRelay(frame.ConnID)
			}
		} else {
			slog.Warn("received data for unknown conn",
				"conn_id", frame.ConnID,
			)
		}

	case protocol.FrameTunnelClose:
		relay := s.GetRelay(frame.ConnID)
		if relay != nil {
			relay.Close()
			s.RemoveRelay(frame.ConnID)
		}

		slog.Debug("tunnel close received",
			"conn_id", frame.ConnID,
		)

	default:
		return fmt.Errorf("%w: 0x%02x", protocol.ErrUnknownFrameType, frame.Type)
	}

	return nil
}
