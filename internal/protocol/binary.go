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

// Realtime lifecycle events that affect protocol envelope fields.
const (
	EventStartConnection   int32 = 1
	EventFinishConnection  int32 = 2
	EventConnectionStarted int32 = 50
	EventConnectionFailed  int32 = 51
	EventConnectionEnded   int32 = 52
)

// ParsedFrame is a parsed protocol frame.
type ParsedFrame struct {
	Version       Version
	HeaderSize    int // In bytes.
	MessageType   MessageType
	Flags         MessageFlags
	Serialization Serialization
	Compression   Compression

	HasSequence bool
	Sequence    int32
	ErrorCode   uint32

	HasEvent  bool
	Event     int32
	SessionID string
	ConnectID string

	Payload []byte
}

// EventFrame describes one event-carrying frame in the websocket protocol.
type EventFrame struct {
	MessageType MessageType
	Flags       MessageFlags

	Event     int32
	SessionID string
	ConnectID string

	Sequence      int32
	ErrorCode     uint32
	Serialization Serialization
	Compression   Compression
	Payload       []byte
}

// BuildFullClientJSON builds a Full Client(JSON) message.
func BuildFullClientJSON(payload []byte) ([]byte, error) {
	return buildClientFrame(MessageTypeFullClient, FlagNoSequence, SerializationJSON, CompressionNone, payload)
}

// BuildAudioOnly builds an Audio-Only message.
// isLast=true sets flags=2 (SAUC convention).
func BuildAudioOnly(payload []byte, isLast bool) ([]byte, error) {
	flags := FlagNoSequence
	if isLast {
		flags = FlagNegativeSequence
	}
	return buildClientFrame(MessageTypeAudioOnlyClient, flags, SerializationNone, CompressionNone, payload)
}

// BuildFullClientJSONWithEvent builds a Full Client(JSON) message with event envelope.
func BuildFullClientJSONWithEvent(event int32, sessionID string, payload []byte) ([]byte, error) {
	return BuildEventFrame(EventFrame{
		MessageType:   MessageTypeFullClient,
		Flags:         FlagWithEvent,
		Event:         event,
		SessionID:     sessionID,
		Serialization: SerializationJSON,
		Compression:   CompressionNone,
		Payload:       payload,
	})
}

// BuildAudioOnlyWithEvent builds an Audio-Only Client message with event envelope.
func BuildAudioOnlyWithEvent(event int32, sessionID string, payload []byte) ([]byte, error) {
	return BuildEventFrame(EventFrame{
		MessageType:   MessageTypeAudioOnlyClient,
		Flags:         FlagWithEvent,
		Event:         event,
		SessionID:     sessionID,
		Serialization: SerializationNone,
		Compression:   CompressionNone,
		Payload:       payload,
	})
}

// BuildEventFrame builds a generic event-carrying frame.
func BuildEventFrame(frame EventFrame) ([]byte, error) {
	flags := frame.Flags
	if flags == 0 {
		flags = FlagWithEvent
	}
	if flags&FlagWithEvent == 0 {
		return nil, fmt.Errorf("event frame must include FlagWithEvent")
	}

	serialization := frame.Serialization
	if serialization == SerializationNone && frame.MessageType != MessageTypeAudioOnlyClient && frame.MessageType != MessageTypeAudioOnlyServer {
		serialization = SerializationJSON
	}

	buf := bytes.NewBuffer(make([]byte, 0, 24+len(frame.Payload)+len(frame.SessionID)+len(frame.ConnectID)))

	// Header
	buf.WriteByte(byte(Version1<<4) | 0x1) // header size = 1 * 4 bytes
	buf.WriteByte(byte(frame.MessageType<<4) | byte(flags))
	buf.WriteByte(byte(serialization<<4) | byte(frame.Compression))
	buf.WriteByte(0x00)

	if hasSequenceField(frame.MessageType, flags) {
		seq := frame.Sequence
		if seq == 0 {
			seq = 1
		}
		if err := binary.Write(buf, binary.BigEndian, seq); err != nil {
			return nil, fmt.Errorf("write sequence: %w", err)
		}
	}

	if err := binary.Write(buf, binary.BigEndian, frame.Event); err != nil {
		return nil, fmt.Errorf("write event: %w", err)
	}

	if hasSessionIDField(frame.Event) {
		if err := binary.Write(buf, binary.BigEndian, uint32(len(frame.SessionID))); err != nil {
			return nil, fmt.Errorf("write session id length: %w", err)
		}
		if _, err := buf.WriteString(frame.SessionID); err != nil {
			return nil, fmt.Errorf("write session id: %w", err)
		}
	}

	if hasConnectIDField(frame.Event) {
		if err := binary.Write(buf, binary.BigEndian, uint32(len(frame.ConnectID))); err != nil {
			return nil, fmt.Errorf("write connect id length: %w", err)
		}
		if _, err := buf.WriteString(frame.ConnectID); err != nil {
			return nil, fmt.Errorf("write connect id: %w", err)
		}
	}

	if frame.MessageType == MessageTypeError {
		if err := binary.Write(buf, binary.BigEndian, frame.ErrorCode); err != nil {
			return nil, fmt.Errorf("write error code: %w", err)
		}
	}

	payload := frame.Payload
	if frame.Compression == CompressionGzip && len(payload) > 0 {
		compressed, err := gzipCompress(payload)
		if err != nil {
			return nil, fmt.Errorf("gzip compress: %w", err)
		}
		payload = compressed
	}

	if err := binary.Write(buf, binary.BigEndian, uint32(len(payload))); err != nil {
		return nil, fmt.Errorf("write payload size: %w", err)
	}
	if _, err := buf.Write(payload); err != nil {
		return nil, fmt.Errorf("write payload: %w", err)
	}

	return buf.Bytes(), nil
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

// ParseServerFrame parses a server binary frame.
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

	hasSequence := hasSequenceField(frame.MessageType, frame.Flags)

	if hasSequence {
		if len(data) < offset+4 {
			return nil, fmt.Errorf("missing sequence")
		}
		frame.HasSequence = true
		frame.Sequence = int32(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
	}

	if hasEventField(frame.Flags) {
		if len(data) < offset+4 {
			return nil, fmt.Errorf("missing event")
		}
		frame.HasEvent = true
		frame.Event = int32(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4

		if hasSessionIDField(frame.Event) {
			if len(data) < offset+4 {
				return nil, fmt.Errorf("missing session id length")
			}
			sessionIDLen := int(binary.BigEndian.Uint32(data[offset : offset+4]))
			offset += 4
			if sessionIDLen < 0 || len(data) < offset+sessionIDLen {
				return nil, fmt.Errorf("invalid session id length: %d", sessionIDLen)
			}
			if sessionIDLen > 0 {
				frame.SessionID = string(data[offset : offset+sessionIDLen])
				offset += sessionIDLen
			}
		}

		if hasConnectIDField(frame.Event) {
			if len(data) < offset+4 {
				return nil, fmt.Errorf("missing connect id length")
			}
			connectIDLen := int(binary.BigEndian.Uint32(data[offset : offset+4]))
			offset += 4
			if connectIDLen < 0 || len(data) < offset+connectIDLen {
				return nil, fmt.Errorf("invalid connect id length: %d", connectIDLen)
			}
			if connectIDLen > 0 {
				frame.ConnectID = string(data[offset : offset+connectIDLen])
				offset += connectIDLen
			}
		}
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

func hasSequenceField(msgType MessageType, flags MessageFlags) bool {
	hasSequence := flags == FlagPositiveSequence || flags == FlagNegativeSequence || flags == FlagNegativeWithSeq
	if msgType == MessageTypeAudioOnlyClient {
		// SAUC audio frame is a special case: flags=2 means last frame, no sequence field.
		hasSequence = false
	}
	return hasSequence
}

func hasEventField(flags MessageFlags) bool {
	return flags&FlagWithEvent == FlagWithEvent
}

func hasSessionIDField(event int32) bool {
	return event != EventStartConnection &&
		event != EventFinishConnection &&
		event != EventConnectionStarted &&
		event != EventConnectionFailed &&
		event != EventConnectionEnded
}

func hasConnectIDField(event int32) bool {
	return event == EventConnectionStarted || event == EventConnectionFailed || event == EventConnectionEnded
}

func gzipCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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
