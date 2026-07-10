package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"time"

	"hole/pkg/protocol"

	"github.com/google/uuid"
)

// Relay 管理一次浏览器↔本地服务的双向数据中继
type Relay struct {
	TunnelConnID string
	BrowserConn  net.Conn
	Session      *TunnelSession
	cancel       context.CancelFunc
	initialData  []byte // 浏览器已发出的 HTTP 请求原始字节（由 HTTP server 预解析，需重建）
}

// NewRelay 创建中继
func NewRelay(browserConn net.Conn, session *TunnelSession, initialData []byte) *Relay {
	return &Relay{
		TunnelConnID: uuid.New().String(),
		BrowserConn:  browserConn,
		Session:      session,
		initialData:  initialData,
	}
}

// Start 启动中继
// 1. 发送 TUNNEL_OPEN 通知客户端
// 2. 启动 browser→tunnel 方向 goroutine（同步阻塞）
// tunnel→browser 方向由 session.HandleFrame 通过 WriteData 直接写入
func (r *Relay) Start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)

	// 注册到会话
	r.Session.AddRelay(r)
	defer r.Session.RemoveRelay(r.TunnelConnID)
	defer r.cancel()
	defer r.BrowserConn.Close()

	// 发送 TUNNEL_OPEN 帧
	openPayload := []byte(`{"type":"tunnel_open","subdomain":"` + r.getSubdomain() + `","remote_addr":"` + r.BrowserConn.RemoteAddr().String() + `"}`)
	openFrame := &protocol.Frame{
		Type:    protocol.FrameTunnelOpen,
		ConnID:  r.TunnelConnID,
		Payload: openPayload,
	}
	if err := protocol.WriteFrame(r.Session.Conn, openFrame); err != nil {
		slog.Error("failed to send tunnel_open",
			"conn_id", r.TunnelConnID,
			"error", err,
		)
		return
	}

	slog.Info("relay started",
		"conn_id", r.TunnelConnID,
		"remote_addr", r.BrowserConn.RemoteAddr(),
	)

	// 发送初始数据（已由 HTTP server 解析的请求字节）
	if len(r.initialData) > 0 {
		frame := &protocol.Frame{
			Type:    protocol.FrameTunnelData,
			ConnID:  r.TunnelConnID,
			Payload: r.initialData,
		}
		if err := protocol.WriteFrame(r.Session.Conn, frame); err != nil {
			slog.Error("failed to send initial tunnel_data",
				"conn_id", r.TunnelConnID,
				"error", err,
			)
			return
		}
	}

	// 启动 browser→tunnel 方向数据拷贝（同步阻塞直到连接关闭）
	r.handleBrowserToTunnel(ctx)
}

// handleBrowserToTunnel 从浏览器 TCP 连接读取数据，封装为 TUNNEL_DATA 发送
func (r *Relay) handleBrowserToTunnel(ctx context.Context) {
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		r.BrowserConn.SetReadDeadline(time.Now().Add(30 * time.Second))

		n, err := r.BrowserConn.Read(buf)
		if err != nil {
			if err != io.EOF && !isTimeoutError(err) {
				slog.Debug("browser read error",
					"conn_id", r.TunnelConnID,
					"error", err,
				)
			}
			// 发送 TUNNEL_CLOSE 通知客户端
			_ = protocol.WriteFrame(r.Session.Conn, &protocol.Frame{
				Type:   protocol.FrameTunnelClose,
				ConnID: r.TunnelConnID,
			})
			return
		}

		// 发送 TUNNEL_DATA
		frame := &protocol.Frame{
			Type:    protocol.FrameTunnelData,
			ConnID:  r.TunnelConnID,
			Payload: buf[:n],
		}
		if err := protocol.WriteFrame(r.Session.Conn, frame); err != nil {
			slog.Error("failed to send tunnel_data",
				"conn_id", r.TunnelConnID,
				"error", err,
			)
			return
		}
	}
}

// WriteData 写入数据到浏览器连接（由 session.HandleFrame 调用）
func (r *Relay) WriteData(data []byte) error {
	r.BrowserConn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	_, err := r.BrowserConn.Write(data)
	return err
}

// Close 关闭中继
func (r *Relay) Close() {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *Relay) getSubdomain() string {
	if len(r.Session.Subdomains) > 0 {
		return r.Session.Subdomains[0]
	}
	return "unknown"
}

func isTimeoutError(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}
