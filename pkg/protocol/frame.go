package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/gorilla/websocket"
)

var (
	ErrUnknownFrameType = errors.New("unknown frame type")
	ErrPayloadTooLarge  = errors.New("payload too large")
	ErrInvalidConnID    = errors.New("invalid connection ID")
	ErrShortFrame       = errors.New("frame too short")
)

// Frame 隧道协议帧
type Frame struct {
	Type    FrameType
	ConnID  string // 36 字节 UUID 字符串
	Payload []byte
}

// Encode 将 Frame 编码为二进制字节流
// 格式: [1字节类型][36字节ConnID][4字节大端PayloadLen][变长Payload]
func (f *Frame) Encode() ([]byte, error) {
	connID := f.ConnID
	if len(connID) > UUIDSize {
		connID = connID[:UUIDSize]
	} else if len(connID) < UUIDSize {
		// 补零到 36 字节
		connID = connID + strings.Repeat("\x00", UUIDSize-len(connID))
	}

	payloadLen := len(f.Payload)
	if payloadLen > MaxPayloadSize {
		return nil, ErrPayloadTooLarge
	}

	buf := make([]byte, FrameHeaderSize+payloadLen)
	buf[0] = byte(f.Type)
	copy(buf[1:], connID)
	binary.BigEndian.PutUint32(buf[1+UUIDSize:], uint32(payloadLen))
	copy(buf[FrameHeaderSize:], f.Payload)

	return buf, nil
}

// DecodeFrame 从二进制数据解码 Frame
func DecodeFrame(raw []byte) (*Frame, error) {
	if len(raw) < FrameHeaderSize {
		return nil, ErrShortFrame
	}

	frameType := FrameType(raw[0])
	if !isValidFrameType(frameType) {
		return nil, fmt.Errorf("%w: 0x%02x", ErrUnknownFrameType, frameType)
	}

	connID := string(raw[1 : 1+UUIDSize])
	// 去除尾部空字节填充
	connID = strings.TrimRight(connID, "\x00")
	// 验证 ConnID 格式（允许全零和标准 UUID）
	if !isValidConnID(connID) {
		return nil, fmt.Errorf("%w: invalid format", ErrInvalidConnID)
	}

	payloadLen := int(binary.BigEndian.Uint32(raw[1+UUIDSize : FrameHeaderSize]))
	if payloadLen > MaxPayloadSize {
		return nil, fmt.Errorf("%w: %d bytes", ErrPayloadTooLarge, payloadLen)
	}

	if len(raw) < FrameHeaderSize+payloadLen {
		return nil, ErrShortFrame
	}

	payload := make([]byte, payloadLen)
	copy(payload, raw[FrameHeaderSize:FrameHeaderSize+payloadLen])

	return &Frame{
		Type:    frameType,
		ConnID:  connID,
		Payload: payload,
	}, nil
}

// ReadFrame 从 WebSocket 连接读取一个 Frame
func ReadFrame(conn *websocket.Conn) (*Frame, error) {
	_, message, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	return DecodeFrame(message)
}

// WriteFrame 向 WebSocket 连接写入一个 Frame
func WriteFrame(conn *websocket.Conn, f *Frame) error {
	data, err := f.Encode()
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, data)
}

func isValidFrameType(t FrameType) bool {
	switch t {
	case FrameTunnelOpen, FrameTunnelData, FrameTunnelClose, FramePing, FramePong:
		return true
	default:
		return false
	}
}

func isValidConnID(connID string) bool {
	if len(connID) != UUIDSize {
		return false
	}
	// 允许全零连接 ID (PING/PONG)
	if connID == ZeroConnID {
		return true
	}
	// 标准 UUID 格式: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	if len(connID) != 36 {
		return false
	}
	for i, c := range connID {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		case c == '-' && (i == 8 || i == 13 || i == 18 || i == 23):
		default:
			return false
		}
	}
	return true
}
