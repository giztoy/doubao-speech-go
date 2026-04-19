package doubaospeech

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/doubao-speech-go/internal/protocol"
	"github.com/gorilla/websocket"
)

func TestRealtimeOpenSessionSendsLifecycleFrames(t *testing.T) {
	client := NewClient("test-app", WithV2APIKey("test-ak", AppKeyRealtime), WithUserID("tester"))
	conn := newFakeWSConn()
	dialer := &fakeDialer{conn: conn}

	svc := newRealtimeService(client)
	svc.dialer = dialer

	conn.enqueue(websocket.BinaryMessage, mustBuildRealtimeServerEventFrame(t, protocol.EventConnectionStarted, "", "connect-1", []byte(`{"ok":true}`)))
	conn.enqueue(websocket.BinaryMessage, mustBuildRealtimeServerEventFrame(t, int32(EventSessionStarted), "session-from-server", "", []byte(`{"ok":true}`)))

	session, err := svc.OpenSession(context.Background(), nil)
	if err != nil {
		t.Fatalf("OpenSession error = %v", err)
	}
	defer session.Close()

	writes := conn.writesSnapshot()
	if len(writes) < 2 {
		t.Fatalf("writes count = %d, want >= 2", len(writes))
	}

	startConnFrame, err := protocol.ParseServerFrame(writes[0])
	if err != nil {
		t.Fatalf("parse start-connection frame: %v", err)
	}
	if !startConnFrame.HasEvent || startConnFrame.Event != protocol.EventStartConnection {
		t.Fatalf("start connection event = %d, want %d", startConnFrame.Event, protocol.EventStartConnection)
	}

	startSessionFrame, err := protocol.ParseServerFrame(writes[1])
	if err != nil {
		t.Fatalf("parse start-session frame: %v", err)
	}
	if !startSessionFrame.HasEvent || startSessionFrame.Event != realtimeStartSessionEvent {
		t.Fatalf("start session event = %d, want %d", startSessionFrame.Event, realtimeStartSessionEvent)
	}
	if startSessionFrame.SessionID == "" {
		t.Fatalf("start session frame should contain session ID")
	}
}

func TestRealtimeSendUserMessageIncludesUpdatedState(t *testing.T) {
	session, conn := newOpenedRealtimeSessionForTest(t, nil)
	defer session.Close()

	session.UpdateHistory([]RealtimeConversationMessage{{Role: "assistant", Content: "old answer"}})
	session.UpdatePrompt(RealtimePromptConfig{System: "you are concise", Variables: map[string]string{"style": "short"}})
	session.UpdateProps(RealtimeGenerationProps{Temperature: 0.3, TopP: 0.7, MaxTokens: 64})

	if err := session.SendUserMessage(context.Background(), "hello"); err != nil {
		t.Fatalf("SendUserMessage error = %v", err)
	}

	writes := conn.writesSnapshot()
	if len(writes) < 3 {
		t.Fatalf("writes count = %d, want >= 3", len(writes))
	}

	userFrame, err := protocol.ParseServerFrame(writes[len(writes)-1])
	if err != nil {
		t.Fatalf("parse user text frame: %v", err)
	}
	if userFrame.Event != realtimeUserTextEvent {
		t.Fatalf("event = %d, want %d", userFrame.Event, realtimeUserTextEvent)
	}

	var payload map[string]any
	if err := json.Unmarshal(userFrame.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got := payload["content"]; got != "hello" {
		t.Fatalf("content = %v, want hello", got)
	}
	if _, ok := payload["history"]; !ok {
		t.Fatalf("history is missing")
	}
	if _, ok := payload["prompt"]; !ok {
		t.Fatalf("prompt is missing")
	}
	if _, ok := payload["props"]; !ok {
		t.Fatalf("props is missing")
	}
}

func TestRealtimeRecvFinalThenErrorOrder(t *testing.T) {
	session, conn := newOpenedRealtimeSessionForTest(t, nil)
	defer session.Close()

	conn.enqueue(websocket.BinaryMessage, mustBuildRealtimeServerEventFrame(t, int32(EventChatEnded), session.SessionID(), "", []byte(`{"content":"done"}`)))
	conn.enqueue(websocket.BinaryMessage, mustBuildRealtimeServerErrorFrame(t, int32(EventSessionFailed), session.SessionID(), 3005, []byte(`{"code":3005,"message":"boom","reqid":"req-1"}`)))

	seenFinal := false
	for evt, err := range session.Recv() {
		if err != nil {
			if !seenFinal {
				t.Fatalf("error arrived before final event: %v", err)
			}
			if !strings.Contains(err.Error(), "realtime event") {
				t.Fatalf("error = %v, want wrapped realtime event error", err)
			}
			return
		}

		if evt != nil && evt.Type == EventChatEnded {
			if !evt.IsFinal {
				t.Fatalf("chat-ended event should be final: %+v", evt)
			}
			seenFinal = true
		}
	}

	t.Fatalf("Recv ended unexpectedly without terminal error")
}

func TestRealtimeFinalDeliveredOncePerTurn(t *testing.T) {
	session, conn := newOpenedRealtimeSessionForTest(t, nil)
	defer session.Close()

	conn.enqueue(websocket.BinaryMessage, mustBuildRealtimeServerEventFrame(t, int32(EventChatEnded), session.SessionID(), "", []byte(`{"content":"turn done"}`)))
	conn.enqueue(websocket.BinaryMessage, mustBuildRealtimeServerEventFrame(t, int32(EventTTSFinished), session.SessionID(), "", []byte(`{"content":"tts done"}`)))

	finalCount := 0
	seen := 0
	for evt, err := range session.Recv() {
		if err != nil {
			t.Fatalf("Recv error = %v", err)
		}
		seen++
		if evt.IsFinal {
			finalCount++
		}
		if seen == 2 {
			break
		}
	}

	if finalCount != 1 {
		t.Fatalf("final event count = %d, want 1", finalCount)
	}
}

func TestRealtimeSendAudioResetsFinalStateBetweenTurns(t *testing.T) {
	session, conn := newOpenedRealtimeSessionForTest(t, nil)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := session.SendAudio(ctx, []byte{0x01, 0x02}); err != nil {
		t.Fatalf("round1 SendAudio error = %v", err)
	}
	conn.enqueue(websocket.BinaryMessage, mustBuildRealtimeServerEventFrame(t, int32(EventASREnded), session.SessionID(), "", []byte(`{"text":"round1"}`)))

	r1, err := session.RecvEvent(ctx)
	if err != nil {
		t.Fatalf("round1 RecvEvent error = %v", err)
	}
	if r1 == nil || !r1.IsFinal {
		t.Fatalf("round1 event = %+v, want final event", r1)
	}

	if err := session.SendAudio(ctx, []byte{0x03, 0x04}); err != nil {
		t.Fatalf("round2 SendAudio error = %v", err)
	}
	conn.enqueue(websocket.BinaryMessage, mustBuildRealtimeServerEventFrame(t, int32(EventASREnded), session.SessionID(), "", []byte(`{"text":"round2"}`)))

	r2, err := session.RecvEvent(ctx)
	if err != nil {
		t.Fatalf("round2 RecvEvent error = %v", err)
	}
	if r2 == nil || !r2.IsFinal {
		t.Fatalf("round2 event = %+v, want final event", r2)
	}
}

func TestRealtimeConcurrentRecvNotSupported(t *testing.T) {
	session, conn := newOpenedRealtimeSessionForTest(t, nil)
	defer session.Close()

	firstDone := make(chan error, 1)
	go func() {
		_, err := session.RecvEvent(context.Background())
		firstDone <- err
	}()

	time.Sleep(20 * time.Millisecond)

	_, err := session.RecvEvent(context.Background())
	if err == nil {
		t.Fatalf("second RecvEvent expected error")
	}
	if !strings.Contains(err.Error(), "concurrent Recv") {
		t.Fatalf("error = %v, want concurrent Recv message", err)
	}

	conn.enqueue(websocket.BinaryMessage, mustBuildRealtimeServerEventFrame(t, int32(EventChatResponse), session.SessionID(), "", []byte(`{"content":"ok"}`)))

	select {
	case recvErr := <-firstDone:
		if recvErr != nil {
			t.Fatalf("first RecvEvent error = %v", recvErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("first RecvEvent did not finish")
	}
}

func TestRealtimeBackpressureReturnsError(t *testing.T) {
	cfg := &RealtimeConfig{
		TTS: RealtimeTTSConfig{
			Speaker: "zh_female_cancan",
			AudioConfig: RealtimeAudioConfig{
				Channel:    1,
				Format:     FormatPCM,
				SampleRate: SampleRate16000,
				Bits:       16,
			},
		},
		EventBuffer:         1,
		BackpressureTimeout: 30 * time.Millisecond,
	}

	session, conn := newOpenedRealtimeSessionForTest(t, cfg)
	defer session.Close()

	conn.enqueue(websocket.BinaryMessage, mustBuildRealtimeServerEventFrame(t, int32(EventChatResponse), session.SessionID(), "", []byte(`{"content":"1"}`)))
	conn.enqueue(websocket.BinaryMessage, mustBuildRealtimeServerEventFrame(t, int32(EventChatResponse), session.SessionID(), "", []byte(`{"content":"2"}`)))

	time.Sleep(120 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	firstEvt, err := session.RecvEvent(ctx)
	if err != nil {
		if !strings.Contains(err.Error(), "buffer full") {
			t.Fatalf("first RecvEvent error = %v, want buffer full", err)
		}
		return
	}
	if firstEvt == nil {
		t.Fatalf("first RecvEvent got nil event and nil error")
	}

	_, err = session.RecvEvent(ctx)
	if err == nil {
		t.Fatalf("second RecvEvent expected backpressure error")
	}
	if !strings.Contains(err.Error(), "buffer full") {
		t.Fatalf("error = %v, want buffer full", err)
	}
}

func TestRealtimeCloseIdempotentAndRaceSafe(t *testing.T) {
	session, _ := newOpenedRealtimeSessionForTest(t, nil)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = session.Close()
		}()
	}

	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("concurrent Close did not finish")
	}

	if err := session.Close(); err != nil {
		t.Fatalf("Close after race error = %v", err)
	}
}

func TestRealtimeOpenCloseLoopCleansReceiveLoop(t *testing.T) {
	const loops = 20

	for i := 0; i < loops; i++ {
		session, _ := newOpenedRealtimeSessionForTest(t, nil)
		if err := session.Close(); err != nil {
			t.Fatalf("loop %d close error = %v", i, err)
		}

		select {
		case <-session.recvDone:
		case <-time.After(1 * time.Second):
			t.Fatalf("loop %d recv loop did not exit", i)
		}
	}
}

func newOpenedRealtimeSessionForTest(t *testing.T, cfg *RealtimeConfig) (*RealtimeSession, *fakeWSConn) {
	t.Helper()

	client := NewClient("test-app", WithV2APIKey("test-ak", AppKeyRealtime), WithUserID("tester"))
	conn := newFakeWSConn()
	dialer := &fakeDialer{conn: conn}

	svc := newRealtimeService(client)
	svc.dialer = dialer

	conn.enqueue(websocket.BinaryMessage, mustBuildRealtimeServerEventFrame(t, protocol.EventConnectionStarted, "", "connect-1", []byte(`{"ok":true}`)))
	conn.enqueue(websocket.BinaryMessage, mustBuildRealtimeServerEventFrame(t, int32(EventSessionStarted), "session-1", "", []byte(`{"ok":true}`)))

	session, err := svc.OpenSession(context.Background(), cfg)
	if err != nil {
		t.Fatalf("OpenSession error = %v", err)
	}

	return session, conn
}

func mustBuildRealtimeServerEventFrame(t *testing.T, event int32, sessionID, connectID string, payload []byte) []byte {
	t.Helper()

	raw, err := protocol.BuildEventFrame(protocol.EventFrame{
		MessageType:   protocol.MessageTypeFullServer,
		Flags:         protocol.FlagWithEvent,
		Event:         event,
		SessionID:     sessionID,
		ConnectID:     connectID,
		Serialization: protocol.SerializationJSON,
		Compression:   protocol.CompressionNone,
		Payload:       payload,
	})
	if err != nil {
		t.Fatalf("BuildEventFrame error = %v", err)
	}
	return raw
}

func mustBuildRealtimeServerErrorFrame(t *testing.T, event int32, sessionID string, code uint32, payload []byte) []byte {
	t.Helper()

	raw, err := protocol.BuildEventFrame(protocol.EventFrame{
		MessageType:   protocol.MessageTypeError,
		Flags:         protocol.FlagWithEvent,
		Event:         event,
		SessionID:     sessionID,
		ErrorCode:     code,
		Serialization: protocol.SerializationJSON,
		Compression:   protocol.CompressionNone,
		Payload:       payload,
	})
	if err != nil {
		t.Fatalf("BuildEventFrame error = %v", err)
	}
	return raw
}
