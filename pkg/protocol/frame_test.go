package protocol

import (
	"bytes"
	"strings"
	"testing"
)

func TestFrameEncodeDecode_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		frame *Frame
	}{
		{"TunnelOpen", &Frame{Type: FrameTunnelOpen, ConnID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890", Payload: []byte(`{"type":"tunnel_open","subdomain":"myapp"}`)}},
		{"TunnelData", &Frame{Type: FrameTunnelData, ConnID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890", Payload: []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")}},
		{"TunnelClose", &Frame{Type: FrameTunnelClose, ConnID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890", Payload: nil}},
		{"Ping", &Frame{Type: FramePing, ConnID: ZeroConnID, Payload: nil}},
		{"Pong", &Frame{Type: FramePong, ConnID: ZeroConnID, Payload: nil}},
		{"LargePayload_1MB", &Frame{Type: FrameTunnelData, ConnID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890", Payload: bytes.Repeat([]byte("A"), 1024*1024)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := tt.frame.Encode()
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			decoded, err := DecodeFrame(encoded)
			if err != nil {
				t.Fatalf("DecodeFrame() error = %v", err)
			}

			if decoded.Type != tt.frame.Type {
				t.Errorf("Type = %v, want %v", decoded.Type, tt.frame.Type)
			}

			// ConnID should be zero-padded to 36 bytes
			if len(decoded.ConnID) != 36 {
				t.Errorf("ConnID length = %d, want 36", len(decoded.ConnID))
			}

			if !bytes.Equal(decoded.Payload, tt.frame.Payload) {
				t.Errorf("Payload mismatch: got %d bytes, want %d bytes", len(decoded.Payload), len(tt.frame.Payload))
			}
		})
	}
}

func TestFrameEncodeDecode_ZeroLengthPayload(t *testing.T) {
	// TUNNEL_CLOSE with zero payload
	frame := &Frame{Type: FrameTunnelClose, ConnID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"}
	encoded, err := frame.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	decoded, err := DecodeFrame(encoded)
	if err != nil {
		t.Fatalf("DecodeFrame() error = %v", err)
	}

	if decoded.Type != FrameTunnelClose {
		t.Errorf("Type = %v, want FrameTunnelClose", decoded.Type)
	}
	if len(decoded.Payload) != 0 {
		t.Errorf("Payload length = %d, want 0", len(decoded.Payload))
	}
}

func TestDecodeFrame_InvalidType(t *testing.T) {
	raw := make([]byte, FrameHeaderSize)
	raw[0] = 0xFF // 非法类型码
	copy(raw[1:], ZeroConnID)
	// payloadLen = 0

	_, err := DecodeFrame(raw)
	if err == nil {
		t.Fatal("expected error for unknown frame type")
	}
	if !strings.Contains(err.Error(), "unknown frame type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecodeFrame_ShortData(t *testing.T) {
	raw := make([]byte, 10) // 小于 FrameHeaderSize
	_, err := DecodeFrame(raw)
	if err != ErrShortFrame {
		t.Errorf("expected ErrShortFrame, got %v", err)
	}
}

func TestDecodeFrame_ExceedsMaxPayload(t *testing.T) {
	raw := make([]byte, FrameHeaderSize+4)
	raw[0] = byte(FrameTunnelData)
	copy(raw[1:], "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	// payloadLen = MaxPayloadSize + 1
	raw[1+UUIDSize] = 0x00
	raw[1+UUIDSize+1] = 0x40 // 4MB + 1
	raw[1+UUIDSize+2] = 0x00
	raw[1+UUIDSize+3] = 0x01

	_, err := DecodeFrame(raw)
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
}

func TestEncode_ExceedsMaxPayload(t *testing.T) {
	frame := &Frame{
		Type:    FrameTunnelData,
		ConnID:  "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Payload: make([]byte, MaxPayloadSize+1),
	}
	_, err := frame.Encode()
	if err != ErrPayloadTooLarge {
		t.Errorf("expected ErrPayloadTooLarge, got %v", err)
	}
}

func TestDecodeFrame_InvalidConnID(t *testing.T) {
	raw := make([]byte, FrameHeaderSize)
	raw[0] = byte(FrameTunnelData)
	// 非 UUID 格式的 ConnID
	copy(raw[1:], "not-a-uuid-at-all!!!!!-!!!!-!!!!!!!!!!!")
	_, err := DecodeFrame(raw)
	if err == nil {
		t.Fatal("expected error for invalid ConnID")
	}
}

func TestEncode_ConnIDExactLength(t *testing.T) {
	// 36-char UUID should encode/decode without transformation
	connID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	frame := &Frame{Type: FramePing, ConnID: connID}
	encoded, err := frame.Encode()
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	if len(encoded) != FrameHeaderSize {
		t.Errorf("encoded length = %d, want %d", len(encoded), FrameHeaderSize)
	}

	decoded, err := DecodeFrame(encoded)
	if err != nil {
		t.Fatalf("DecodeFrame() error = %v", err)
	}

	if decoded.ConnID != connID {
		t.Errorf("ConnID = %q, want %q", decoded.ConnID, connID)
	}
}

func TestEncodeDecode_AllFrameTypes(t *testing.T) {
	types := []FrameType{FrameTunnelOpen, FrameTunnelData, FrameTunnelClose, FramePing, FramePong}
	connID := "b2c3d4e5-f6a7-8901-bcde-f12345678901"

	for _, ft := range types {
		t.Run("", func(t *testing.T) {
			frame := &Frame{Type: ft, ConnID: connID, Payload: []byte("test")}
			encoded, err := frame.Encode()
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			decoded, err := DecodeFrame(encoded)
			if err != nil {
				t.Fatalf("DecodeFrame() error = %v", err)
			}

			if decoded.Type != ft {
				t.Errorf("Type = %v, want %v", decoded.Type, ft)
			}
		})
	}
}
