package protocol

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
)

type Version byte
type MessageType byte
type MessageFlags byte
type Serialization byte
type Compression byte

const (
	Version1 Version = 0x1
)

const (
	MessageTypeFullClient      MessageType = 0x1
	MessageTypeAudioOnlyClient MessageType = 0x2
	MessageTypeFullServer      MessageType = 0x9
	MessageTypeAudioOnlyServer MessageType = 0xB
	MessageTypeFrontEndResult  MessageType = 0xC
	MessageTypeError           MessageType = 0xF
)

const (
	FlagNoSequence       MessageFlags = 0x0
	FlagPositiveSequence MessageFlags = 0x1
	FlagNegativeSequence MessageFlags = 0x2
	FlagNegativeWithSeq  MessageFlags = 0x3
	FlagWithEvent        MessageFlags = 0x4
)

const (
	SerializationNone   Serialization = 0x0
	SerializationJSON   Serialization = 0x1
	SerializationThrift Serialization = 0x3
)

const (
	CompressionNone Compression = 0x0
	CompressionGzip Compression = 0x1
)

// ParsedFrame 是解析后的协议帧。
type ParsedFrame struct {
	Version       Version
	HeaderSize    int // 以字节计
	MessageType   MessageType
	Flags         MessageFlags
	Serialization Serialization
	Compression   Compression

	HasSequence bool
	Sequence    int32
	ErrorCode   uint32

	Payload []byte
}

// BuildFullClientJSON 构造 Full Client(JSON) 消息。
func BuildFullClientJSON(payload []byte) ([]byte, error) {
	return buildClientFrame(MessageTypeFullClient, FlagNoSequence, SerializationJSON, CompressionNone, payload)
}

// BuildAudioOnly 构造 Audio-Only 消息。
// isLast=true 时 flags=2（SAUC 约定）。
func BuildAudioOnly(payload []byte, isLast bool) ([]byte, error) {
	flags := FlagNoSequence
	if isLast {
		flags = FlagNegativeSequence
	}
	return buildClientFrame(MessageTypeAudioOnlyClient, flags, SerializationNone, CompressionNone, payload)
}

func buildClientFrame(msgType MessageType, flags MessageFlags, ser Serialization, comp Compression, payload []byte) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 12+len(payload)))

	// Header
	buf.WriteByte(byte(Version1<<4) | 0x1) // header size = 1 * 4 bytes
	buf.WriteByte(byte(msgType<<4) | byte(flags))
	buf.WriteByte(byte(ser<<4) | byte(comp))
	buf.WriteByte(0x00)

	if (flags == FlagPositiveSequence || flags == FlagNegativeWithSeq) && msgType != MessageTypeAudioOnlyClient {
		if err := binary.Write(buf, binary.BigEndian, int32(1)); err != nil {
			return nil, fmt.Errorf("write sequence: %w", err)
		}
	}

	if err := binary.Write(buf, binary.BigEndian, uint32(len(payload))); err != nil {
		return nil, fmt.Errorf("write payload size: %w", err)
	}
	if _, err := buf.Write(payload); err != nil {
		return nil, fmt.Errorf("write payload: %w", err)
	}

	return buf.Bytes(), nil
}

// ParseServerFrame 解析服务端二进制帧。
func ParseServerFrame(data []byte) (*ParsedFrame, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("frame too short: %d", len(data))
	}

	frame := &ParsedFrame{}
	frame.Version = Version((data[0] >> 4) & 0x0F)
	headerUnits := int(data[0] & 0x0F)
	if headerUnits <= 0 {
		return nil, fmt.Errorf("invalid header size units: %d", headerUnits)
	}
	frame.HeaderSize = headerUnits * 4
	if len(data) < frame.HeaderSize {
		return nil, fmt.Errorf("incomplete header: need %d, got %d", frame.HeaderSize, len(data))
	}

	frame.MessageType = MessageType((data[1] >> 4) & 0x0F)
	frame.Flags = MessageFlags(data[1] & 0x0F)
	frame.Serialization = Serialization((data[2] >> 4) & 0x0F)
	frame.Compression = Compression(data[2] & 0x0F)

	offset := frame.HeaderSize

	hasSequence := frame.Flags == FlagPositiveSequence || frame.Flags == FlagNegativeSequence || frame.Flags == FlagNegativeWithSeq
	if frame.MessageType == MessageTypeAudioOnlyClient {
		// SAUC 音频帧是特例：flags=2 仅表示最后一帧，不携带 sequence。
		hasSequence = false
	}

	if hasSequence {
		if len(data) < offset+4 {
			return nil, fmt.Errorf("missing sequence")
		}
		frame.HasSequence = true
		frame.Sequence = int32(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
	}

	if frame.MessageType == MessageTypeError {
		if len(data) < offset+4 {
			return nil, fmt.Errorf("missing error code")
		}
		frame.ErrorCode = binary.BigEndian.Uint32(data[offset : offset+4])
		offset += 4
	}

	if len(data) < offset+4 {
		return nil, fmt.Errorf("missing payload size")
	}
	payloadSize := int(binary.BigEndian.Uint32(data[offset : offset+4]))
	offset += 4

	if payloadSize < 0 || len(data) < offset+payloadSize {
		return nil, fmt.Errorf("invalid payload size: %d", payloadSize)
	}

	payload := data[offset : offset+payloadSize]
	if frame.Compression == CompressionGzip {
		decoded, err := gzipDecompress(payload, 10*1024*1024)
		if err != nil {
			return nil, fmt.Errorf("gzip decompress: %w", err)
		}
		payload = decoded
	}

	frame.Payload = payload
	return frame, nil
}

func gzipDecompress(data []byte, maxSize int64) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	if maxSize <= 0 {
		return io.ReadAll(r)
	}

	b, err := io.ReadAll(io.LimitReader(r, maxSize))
	if err != nil {
		return nil, err
	}
	return b, nil
}
