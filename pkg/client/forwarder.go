package client

import (
	"context"
	"io"
	"log/slog"
	"net"
	"time"

	"hole/pkg/protocol"

	"github.com/gorilla/websocket"
)

// LocalForward 本地转发（对应一个 TUNNEL_OPEN 分配的连接）
type LocalForward struct {
	TunnelConnID string
	LocalConn    net.Conn
	wsConn       *websocket.Conn
	cancel       context.CancelFunc
}

// NewLocalForward 创建本地转发
func NewLocalForward(tunnelConnID string, localConn net.Conn, wsConn *websocket.Conn) *LocalForward {
	return &LocalForward{
		TunnelConnID: tunnelConnID,
		LocalConn:    localConn,
		wsConn:       wsConn,
	}
}

// Start 启动本地转发（local→tunnel 方向）
// tunnel→local 方向由 session 帧循环直接写入
func (f *LocalForward) Start(ctx context.Context) {
	ctx, f.cancel = context.WithCancel(ctx)
	defer f.cancel()
	defer f.LocalConn.Close()

	slog.Debug("forward started",
		"conn_id", f.TunnelConnID,
		"local_addr", f.LocalConn.RemoteAddr(),
	)

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		f.LocalConn.SetReadDeadline(time.Now().Add(30 * time.Second))

		n, err := f.LocalConn.Read(buf)
		if err != nil {
			if err != io.EOF && !isTimeoutError(err) {
				slog.Debug("local read error",
					"conn_id", f.TunnelConnID,
					"error", err,
				)
			}
			// 发送 TUNNEL_CLOSE
			_ = protocol.WriteFrame(f.wsConn, &protocol.Frame{
				Type:   protocol.FrameTunnelClose,
				ConnID: f.TunnelConnID,
			})
			return
		}

		// 发送 TUNNEL_DATA
		frame := &protocol.Frame{
			Type:    protocol.FrameTunnelData,
			ConnID:  f.TunnelConnID,
			Payload: buf[:n],
		}
		if err := protocol.WriteFrame(f.wsConn, frame); err != nil {
			slog.Error("failed to send tunnel_data",
				"conn_id", f.TunnelConnID,
				"error", err,
			)
			return
		}
	}
}

// WriteData 写入数据到本地连接（由 session 帧循环调用）
func (f *LocalForward) WriteData(data []byte) error {
	f.LocalConn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	_, err := f.LocalConn.Write(data)
	return err
}

// Close 关闭转发
func (f *LocalForward) Close() {
	if f.cancel != nil {
		f.cancel()
	}
	f.LocalConn.Close()
}

func isTimeoutError(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}
