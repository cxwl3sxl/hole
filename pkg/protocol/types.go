package protocol

// FrameType 帧类型
type FrameType byte

const (
	FrameTunnelOpen  FrameType = 0x01
	FrameTunnelData  FrameType = 0x02
	FrameTunnelClose FrameType = 0x03
	FramePing        FrameType = 0x04
	FramePong        FrameType = 0x05
)

const (
	FrameHeaderSize = 41       // 1(type) + 36(connID) + 4(payloadLen)
	UUIDSize        = 36       // "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
	MaxPayloadSize  = 4 << 20  // 4MB
)

// 预定义的连接 ID 常量
var (
	ZeroConnID = "00000000-0000-0000-0000-000000000000"
)
