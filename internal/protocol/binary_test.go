package protocol

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
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
	original := []byte(`{"result":{"text":"你好"}}`)
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
