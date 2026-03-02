package doubaospeech

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestParseTTSV2WSFrameWithSequence(t *testing.T) {
	payload := []byte(`{"hello":"world"}`)
	raw := buildTTSV2SeqFrame(payload, 7)

	frame, err := parseTTSV2WSFrame(raw)
	if err != nil {
		t.Fatalf("parseTTSV2WSFrame error = %v", err)
	}

	if !frame.hasSequence {
		t.Fatalf("hasSequence = false, want true")
	}
	if frame.sequence != 7 {
		t.Fatalf("sequence = %d, want 7", frame.sequence)
	}
	if !bytes.Equal(frame.payload, payload) {
		t.Fatalf("payload mismatch: got=%q want=%q", string(frame.payload), string(payload))
	}
}

func TestTTSV2WSSessionRecvAudioOnlyNoEvent(t *testing.T) {
	client := NewClient("test-app", WithV2APIKey("test-ak", "test-app"), WithUserID("tester"))
	conn := newFakeWSConn()

	svc := newTTSServiceV2(client)
	svc.dialer = &fakeDialer{conn: conn}

	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventConnectionStarted, "conn1", []byte(`{}`)))
	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventSessionStarted, "sid1", []byte(`{}`)))

	session, err := svc.OpenStreamSession(context.Background(), &TTSV2WSConfig{
		Speaker:    "zh_female_xiaohe_uranus_bigtts",
		Format:     FormatMP3,
		SampleRate: SampleRate24000,
	})
	if err != nil {
		t.Fatalf("OpenStreamSession error = %v", err)
	}
	defer session.Close()

	audioPayload := []byte{1, 2, 3, 4, 5}
	conn.enqueue(websocket.BinaryMessage, buildTTSV2AudioOnlyFrame(audioPayload))
	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventSessionFinished, "sid1", []byte(`{"status_code":20000000,"message":"ok"}`)))

	var (
		gotAudio bool
		gotFinal bool
	)
	for chunk, recvErr := range session.Recv() {
		if recvErr != nil {
			t.Fatalf("Recv error = %v", recvErr)
		}

		if len(chunk.Audio) > 0 {
			gotAudio = true
			if !bytes.Equal(chunk.Audio, audioPayload) {
				t.Fatalf("audio mismatch: got=%v want=%v", chunk.Audio, audioPayload)
			}
		}

		if chunk.IsFinal {
			gotFinal = true
			break
		}
	}

	if !gotAudio {
		t.Fatalf("expected audio chunk from audio-only frame")
	}
	if !gotFinal {
		t.Fatalf("expected final chunk from SessionFinished event")
	}
}

func TestTTSV2WSSessionRecvErrorFrame(t *testing.T) {
	client := NewClient("test-app", WithV2APIKey("test-ak", "test-app"), WithUserID("tester"))
	conn := newFakeWSConn()

	svc := newTTSServiceV2(client)
	svc.dialer = &fakeDialer{conn: conn}

	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventConnectionStarted, "conn1", []byte(`{}`)))
	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventSessionStarted, "sid1", []byte(`{}`)))

	session, err := svc.OpenStreamSession(context.Background(), &TTSV2WSConfig{
		Speaker:    "zh_female_xiaohe_uranus_bigtts",
		Format:     FormatMP3,
		SampleRate: SampleRate24000,
	})
	if err != nil {
		t.Fatalf("OpenStreamSession error = %v", err)
	}
	defer session.Close()

	conn.enqueue(websocket.BinaryMessage, buildTTSV2ErrorFrame(3002, []byte(`{"code":3002,"message":"auth failed","reqid":"r2"}`)))

	var gotErr error
	for _, recvErr := range session.Recv() {
		gotErr = recvErr
		break
	}
	if gotErr == nil {
		t.Fatalf("expected error from error frame")
	}

	apiErr, ok := AsError(gotErr)
	if !ok {
		t.Fatalf("want *Error, got %T (%v)", gotErr, gotErr)
	}
	if apiErr.Code != 3002 {
		t.Fatalf("error code = %d, want 3002", apiErr.Code)
	}
}

func TestTTSV2WSSessionFailedEventUsesStatusCode(t *testing.T) {
	client := NewClient("test-app", WithV2APIKey("test-ak", "test-app"), WithUserID("tester"))
	conn := newFakeWSConn()

	svc := newTTSServiceV2(client)
	svc.dialer = &fakeDialer{conn: conn}

	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventConnectionStarted, "conn1", []byte(`{}`)))
	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventSessionStarted, "sid1", []byte(`{}`)))

	session, err := svc.OpenStreamSession(context.Background(), &TTSV2WSConfig{
		Speaker:    "zh_female_xiaohe_uranus_bigtts",
		Format:     FormatMP3,
		SampleRate: SampleRate24000,
	})
	if err != nil {
		t.Fatalf("OpenStreamSession error = %v", err)
	}
	defer session.Close()

	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventSessionFailed, "sid1", []byte(`{"status_code":55000001,"message":"session failed","reqid":"s-failed"}`)))

	var gotErr error
	for _, recvErr := range session.Recv() {
		gotErr = recvErr
		break
	}
	if gotErr == nil {
		t.Fatalf("expected error from session failed event")
	}

	apiErr, ok := AsError(gotErr)
	if !ok {
		t.Fatalf("want *Error, got %T (%v)", gotErr, gotErr)
	}
	if apiErr.Code != 55000001 {
		t.Fatalf("error code = %d, want 55000001", apiErr.Code)
	}
	if apiErr.ReqID != "s-failed" {
		t.Fatalf("reqid = %q, want %q", apiErr.ReqID, "s-failed")
	}
}

func TestTTSV2WSSessionCancelSessionFlow(t *testing.T) {
	client := NewClient("test-app", WithV2APIKey("test-ak", "test-app"), WithUserID("tester"))
	conn := newFakeWSConn()

	svc := newTTSServiceV2(client)
	svc.dialer = &fakeDialer{conn: conn}

	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventConnectionStarted, "conn1", []byte(`{}`)))
	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventSessionStarted, "sid1", []byte(`{}`)))

	session, err := svc.OpenStreamSession(context.Background(), &TTSV2WSConfig{
		Speaker:    "zh_female_xiaohe_uranus_bigtts",
		Format:     FormatMP3,
		SampleRate: SampleRate24000,
	})
	if err != nil {
		t.Fatalf("OpenStreamSession error = %v", err)
	}
	defer session.Close()

	if err := session.CancelSession(context.Background()); err != nil {
		t.Fatalf("CancelSession error = %v", err)
	}

	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventSessionCanceled, "sid1", []byte(`{"status_code":20000000,"message":"ok"}`)))

	var gotFinal bool
	for chunk, recvErr := range session.Recv() {
		if recvErr != nil {
			t.Fatalf("Recv error = %v", recvErr)
		}
		if chunk.IsFinal {
			gotFinal = true
			if chunk.Event != ttsV2EventSessionCanceled {
				t.Fatalf("final event = %d, want %d", chunk.Event, ttsV2EventSessionCanceled)
			}
			break
		}
	}
	if !gotFinal {
		t.Fatalf("expected final chunk from SessionCanceled event")
	}

	writes := conn.writesSnapshot()
	if len(writes) == 0 {
		t.Fatalf("no writes captured")
	}

	last := writes[len(writes)-1]
	frame, parseErr := parseTTSV2WSFrame(last)
	if parseErr != nil {
		t.Fatalf("parse last frame error = %v", parseErr)
	}
	if !frame.hasEvent || frame.event != ttsV2EventSessionCancel {
		t.Fatalf("last frame event = %d (hasEvent=%v), want cancel event=%d", frame.event, frame.hasEvent, ttsV2EventSessionCancel)
	}
}

func TestTTSV2WSSessionStartNextSessionSameConnection(t *testing.T) {
	client := NewClient("test-app", WithV2APIKey("test-ak", "test-app"), WithUserID("tester"))
	conn := newFakeWSConn()

	svc := newTTSServiceV2(client)
	svc.dialer = &fakeDialer{conn: conn}

	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventConnectionStarted, "conn1", []byte(`{}`)))
	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventSessionStarted, "sid1", []byte(`{}`)))

	session, err := svc.OpenStreamSession(context.Background(), &TTSV2WSConfig{
		Speaker:    "zh_female_xiaohe_uranus_bigtts",
		Format:     FormatMP3,
		SampleRate: SampleRate24000,
	})
	if err != nil {
		t.Fatalf("OpenStreamSession error = %v", err)
	}
	defer session.Close()

	if err := session.SendText(context.Background(), "first", true); err != nil {
		t.Fatalf("SendText first session error = %v", err)
	}
	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventSessionFinished, "sid1", []byte(`{"status_code":20000000,"message":"ok"}`)))

	for chunk, recvErr := range session.Recv() {
		if recvErr != nil {
			t.Fatalf("Recv first session error = %v", recvErr)
		}
		if chunk.IsFinal {
			break
		}
	}

	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventSessionStarted, "sid2", []byte(`{}`)))
	if err := session.StartNextSession(context.Background()); err != nil {
		t.Fatalf("StartNextSession error = %v", err)
	}

	if err := session.SendText(context.Background(), "second", true); err != nil {
		t.Fatalf("SendText second session error = %v", err)
	}
	conn.enqueue(websocket.BinaryMessage, buildTTSV2EventFrame(ttsV2EventSessionFinished, "sid2", []byte(`{"status_code":20000000,"message":"ok"}`)))

	for chunk, recvErr := range session.Recv() {
		if recvErr != nil {
			t.Fatalf("Recv second session error = %v", recvErr)
		}
		if chunk.IsFinal {
			break
		}
	}
}

func TestParseTTSV2WSFrameGzipLimit(t *testing.T) {
	payload := bytes.Repeat([]byte("a"), int(ttsV2WSMaxGzipDecodedSize+1))
	compressed := gzipBytesForTest(t, payload)

	raw := make([]byte, 0, 8+len(compressed))
	raw = append(raw, 0x11)
	raw = append(raw, byte((0x9<<4)|ttsV2WSFlagNoSequence))
	raw = append(raw, byte((ttsV2WSSerializationRaw<<4)|ttsV2WSCompressionGzip))
	raw = append(raw, 0x00)

	size := make([]byte, 4)
	binary.BigEndian.PutUint32(size, uint32(len(compressed)))
	raw = append(raw, size...)
	raw = append(raw, compressed...)

	_, err := parseTTSV2WSFrame(raw)
	if err == nil {
		t.Fatalf("expected gzip size limit error")
	}
	if !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func buildTTSV2SeqFrame(payload []byte, seq int32) []byte {
	b := make([]byte, 12+len(payload))
	b[0] = 0x11
	b[1] = byte((0x9 << 4) | ttsV2WSFlagPositiveSequence)
	b[2] = byte((ttsV2WSSerializationJSON << 4) | ttsV2WSCompressionNone)
	b[3] = 0
	binary.BigEndian.PutUint32(b[4:8], uint32(seq))
	binary.BigEndian.PutUint32(b[8:12], uint32(len(payload)))
	copy(b[12:], payload)
	return b
}

func buildTTSV2AudioOnlyFrame(payload []byte) []byte {
	b := make([]byte, 8+len(payload))
	b[0] = 0x11
	b[1] = byte((ttsV2WSMsgTypeAudioOnlyServer << 4) | ttsV2WSFlagNoSequence)
	b[2] = byte((ttsV2WSSerializationRaw << 4) | ttsV2WSCompressionNone)
	b[3] = 0
	binary.BigEndian.PutUint32(b[4:8], uint32(len(payload)))
	copy(b[8:], payload)
	return b
}

func buildTTSV2EventFrame(event int32, id string, payload []byte) []byte {
	idBytes := []byte(id)
	b := make([]byte, 16+len(idBytes)+len(payload))
	b[0] = 0x11
	b[1] = byte((0x9 << 4) | ttsV2WSFlagWithEvent)
	b[2] = byte((ttsV2WSSerializationJSON << 4) | ttsV2WSCompressionNone)
	b[3] = 0
	binary.BigEndian.PutUint32(b[4:8], uint32(event))
	binary.BigEndian.PutUint32(b[8:12], uint32(len(idBytes)))
	copy(b[12:12+len(idBytes)], idBytes)
	offset := 12 + len(idBytes)
	binary.BigEndian.PutUint32(b[offset:offset+4], uint32(len(payload)))
	copy(b[offset+4:], payload)
	return b
}

func buildTTSV2ErrorFrame(code uint32, payload []byte) []byte {
	b := make([]byte, 12+len(payload))
	b[0] = 0x11
	b[1] = byte((ttsV2WSMsgTypeError << 4) | ttsV2WSFlagNoSequence)
	b[2] = byte((ttsV2WSSerializationJSON << 4) | ttsV2WSCompressionNone)
	b[3] = 0
	binary.BigEndian.PutUint32(b[4:8], code)
	binary.BigEndian.PutUint32(b[8:12], uint32(len(payload)))
	copy(b[12:], payload)
	return b
}

func gzipBytesForTest(t *testing.T, in []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(in); err != nil {
		t.Fatalf("gzip write error: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close error: %v", err)
	}

	return buf.Bytes()
}
