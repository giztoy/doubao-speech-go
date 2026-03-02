package protocol

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"strings"
	"testing"
)

func TestBuildAudioOnlyFlags(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03}

	frame, err := BuildAudioOnly(payload, false)
	if err != nil {
		t.Fatalf("BuildAudioOnly(false) error = %v", err)
	}
	if got := frame[0]; got != 0x11 {
		t.Fatalf("version/header byte = 0x%x, want 0x11", got)
	}
	wantTypeFlags := byte(MessageTypeAudioOnlyClient<<4) | byte(FlagNoSequence)
	if got := frame[1]; got != wantTypeFlags {
		t.Fatalf("type/flags byte = 0x%x, want 0x%x", got, wantTypeFlags)
	}
	size := binary.BigEndian.Uint32(frame[4:8])
	if int(size) != len(payload) {
		t.Fatalf("payload size = %d, want %d", size, len(payload))
	}

	lastFrame, err := BuildAudioOnly(payload, true)
	if err != nil {
		t.Fatalf("BuildAudioOnly(true) error = %v", err)
	}
	wantLastFlags := byte(MessageTypeAudioOnlyClient<<4) | byte(FlagNegativeSequence)
	if got := lastFrame[1]; got != wantLastFlags {
		t.Fatalf("last type/flags byte = 0x%x, want 0x%x", got, wantLastFlags)
	}

	parsed, err := ParseServerFrame(lastFrame)
	if err != nil {
		t.Fatalf("ParseServerFrame(audio-last) error = %v", err)
	}
	if parsed.HasSequence {
		t.Fatalf("audio-only frame should not contain sequence")
	}
}

func TestParseServerFrameGzip(t *testing.T) {
	original := []byte(`{"result":{"text":"hello"}}`)
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(original); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	raw := make([]byte, 0, 12+compressed.Len())
	raw = append(raw,
		0x11,
		byte(MessageTypeFullServer<<4)|byte(FlagPositiveSequence),
		byte(SerializationJSON<<4)|byte(CompressionGzip),
		0x00,
	)

	seq := make([]byte, 4)
	binary.BigEndian.PutUint32(seq, 7)
	raw = append(raw, seq...)

	sz := make([]byte, 4)
	binary.BigEndian.PutUint32(sz, uint32(compressed.Len()))
	raw = append(raw, sz...)
	raw = append(raw, compressed.Bytes()...)

	frame, err := ParseServerFrame(raw)
	if err != nil {
		t.Fatalf("ParseServerFrame error = %v", err)
	}
	if !frame.HasSequence || frame.Sequence != 7 {
		t.Fatalf("sequence = (%v,%d), want (true,7)", frame.HasSequence, frame.Sequence)
	}
	if frame.MessageType != MessageTypeFullServer {
		t.Fatalf("message type = %v, want %v", frame.MessageType, MessageTypeFullServer)
	}
	if !bytes.Equal(frame.Payload, original) {
		t.Fatalf("payload mismatch: got %q want %q", string(frame.Payload), string(original))
	}
}

func TestBuildFullClientJSONWithEvent(t *testing.T) {
	payload := []byte(`{"hello":"world"}`)
	raw, err := BuildFullClientJSONWithEvent(100, "session-1", payload)
	if err != nil {
		t.Fatalf("BuildFullClientJSONWithEvent error = %v", err)
	}

	frame, err := ParseServerFrame(raw)
	if err != nil {
		t.Fatalf("ParseServerFrame error = %v", err)
	}

	if !frame.HasEvent {
		t.Fatalf("HasEvent = false, want true")
	}
	if frame.Event != 100 {
		t.Fatalf("Event = %d, want 100", frame.Event)
	}
	if frame.SessionID != "session-1" {
		t.Fatalf("SessionID = %q, want %q", frame.SessionID, "session-1")
	}
	if !bytes.Equal(frame.Payload, payload) {
		t.Fatalf("payload mismatch: got %q, want %q", frame.Payload, payload)
	}
}

func TestParseServerFrameConnectionEventWithConnectID(t *testing.T) {
	raw, err := BuildEventFrame(EventFrame{
		MessageType:   MessageTypeFullServer,
		Flags:         FlagWithEvent,
		Event:         EventConnectionStarted,
		ConnectID:     "connect-1",
		Serialization: SerializationJSON,
		Compression:   CompressionNone,
		Payload:       []byte(`{"status":"ok"}`),
	})
	if err != nil {
		t.Fatalf("BuildEventFrame error = %v", err)
	}

	frame, err := ParseServerFrame(raw)
	if err != nil {
		t.Fatalf("ParseServerFrame error = %v", err)
	}

	if frame.Event != EventConnectionStarted {
		t.Fatalf("Event = %d, want %d", frame.Event, EventConnectionStarted)
	}
	if frame.ConnectID != "connect-1" {
		t.Fatalf("ConnectID = %q, want %q", frame.ConnectID, "connect-1")
	}
	if frame.SessionID != "" {
		t.Fatalf("SessionID = %q, want empty", frame.SessionID)
	}
}

func TestParseServerFrameInvalidSessionIDLength(t *testing.T) {
	raw, err := BuildEventFrame(EventFrame{
		MessageType:   MessageTypeFullServer,
		Flags:         FlagWithEvent,
		Event:         100,
		SessionID:     "s",
		Serialization: SerializationJSON,
		Compression:   CompressionNone,
		Payload:       []byte(`{"ok":true}`),
	})
	if err != nil {
		t.Fatalf("BuildEventFrame error = %v", err)
	}

	// header(4) + event(4) => session id length starts at offset 8.
	binary.BigEndian.PutUint32(raw[8:12], uint32(1000))

	_, err = ParseServerFrame(raw)
	if err == nil {
		t.Fatalf("ParseServerFrame expected error")
	}
	if !strings.Contains(err.Error(), "invalid session id length") {
		t.Fatalf("error = %v, want invalid session id length", err)
	}
}

func TestParseServerFrameTruncatedInputs(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr string
	}{
		{
			name:    "frame too short",
			input:   []byte{0x11, 0x90, 0x10},
			wantErr: "frame too short",
		},
		{
			name: "incomplete header",
			input: []byte{
				0x13, // version=1, header size=3 (12 bytes)
				byte(MessageTypeFullServer << 4),
				byte(SerializationJSON << 4),
				0x00,
				0x00, 0x00, 0x00, 0x00,
			},
			wantErr: "incomplete header",
		},
		{
			name: "missing payload size after sequence",
			input: []byte{
				0x11,
				byte(MessageTypeFullServer<<4) | byte(FlagPositiveSequence),
				byte(SerializationJSON << 4),
				0x00,
				0x00, 0x00, 0x00, 0x01, // sequence
			},
			wantErr: "missing payload size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseServerFrame(tt.input)
			if err == nil {
				t.Fatalf("ParseServerFrame expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want contains %q", err, tt.wantErr)
			}
		})
	}
}
