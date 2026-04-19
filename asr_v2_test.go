package doubaospeech

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/doubao-speech-go/internal/protocol"
	"github.com/GizClaw/doubao-speech-go/internal/transport"
	"github.com/gorilla/websocket"
)

type wsReadItem struct {
	msgType int
	payload []byte
	err     error
}

type fakeWSConn struct {
	mu        sync.Mutex
	writes    [][]byte
	reads     chan wsReadItem
	closeOnce sync.Once
}

func newFakeWSConn() *fakeWSConn {
	return &fakeWSConn{reads: make(chan wsReadItem, 16)}
}

func (c *fakeWSConn) WriteMessage(_ int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := make([]byte, len(data))
	copy(b, data)
	c.writes = append(c.writes, b)
	return nil
}

func (c *fakeWSConn) ReadMessage() (int, []byte, error) {
	item, ok := <-c.reads
	if !ok {
		return 0, nil, io.EOF
	}
	if item.err != nil {
		return 0, nil, item.err
	}
	return item.msgType, item.payload, nil
}

func (c *fakeWSConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.reads)
	})
	return nil
}

func (c *fakeWSConn) enqueue(msgType int, payload []byte) {
	c.reads <- wsReadItem{msgType: msgType, payload: payload}
}

func (c *fakeWSConn) writesSnapshot() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]byte, 0, len(c.writes))
	for _, w := range c.writes {
		b := make([]byte, len(w))
		copy(b, w)
		out = append(out, b)
	}
	return out
}

type fakeDialer struct {
	conn    transport.WSConn
	err     error
	resp    *http.Response
	url     string
	headers http.Header
}

func (d *fakeDialer) DialContext(_ context.Context, url string, requestHeader http.Header) (transport.WSConn, *http.Response, error) {
	d.url = url
	d.headers = requestHeader.Clone()
	if d.err != nil {
		return nil, d.resp, d.err
	}
	return d.conn, d.resp, nil
}

func TestOpenStreamSessionSendsStartFrame(t *testing.T) {
	client := NewClient("test-app", WithV2APIKey("test-ak", "test-app"), WithUserID("tester"))
	conn := newFakeWSConn()
	dialer := &fakeDialer{conn: conn}

	svc := newASRServiceV2(client)
	svc.dialer = dialer

	session, err := svc.OpenStreamSession(context.Background(), &ASRV2Config{
		Format:     FormatPCM,
		SampleRate: SampleRate16000,
	})
	if err != nil {
		t.Fatalf("OpenStreamSession error = %v", err)
	}
	defer session.Close()

	writes := conn.writesSnapshot()
	if len(writes) == 0 {
		t.Fatalf("no writes sent")
	}

	frame, err := protocol.ParseServerFrame(writes[0])
	if err != nil {
		t.Fatalf("parse start frame: %v", err)
	}
	if frame.MessageType != protocol.MessageTypeFullClient {
		t.Fatalf("start frame type = %v, want full client", frame.MessageType)
	}

	var body map[string]any
	if err := json.Unmarshal(frame.Payload, &body); err != nil {
		t.Fatalf("unmarshal start payload: %v", err)
	}
	audio, ok := body["audio"].(map[string]any)
	if !ok {
		t.Fatalf("audio field missing")
	}
	if got := audio["format"]; got != string(FormatPCM) {
		t.Fatalf("audio.format = %v, want %s", got, FormatPCM)
	}
	if got := dialer.headers.Get("X-Api-Resource-Id"); got != ResourceASRStreamV2 {
		t.Fatalf("X-Api-Resource-Id = %q, want %q", got, ResourceASRStreamV2)
	}
}

func TestOpenStreamSessionAuthFailureErrorStructure(t *testing.T) {
	client := NewClient("test-app", WithV2APIKey("bad-ak", "test-app"), WithUserID("tester"))

	dialer := &fakeDialer{
		err: errors.New("bad handshake"),
		resp: &http.Response{
			StatusCode: http.StatusUnauthorized,
			Status:     "401 Unauthorized",
			Header: http.Header{
				"X-Tt-Logid": []string{"log-auth-1"},
			},
			Body: io.NopCloser(strings.NewReader(`{"code":3002,"message":"auth failed","reqid":"req-auth-1"}`)),
		},
	}

	svc := newASRServiceV2(client)
	svc.dialer = dialer

	_, err := svc.OpenStreamSession(context.Background(), &ASRV2Config{Format: FormatPCM, SampleRate: SampleRate16000})
	if err == nil {
		t.Fatalf("OpenStreamSession expected error")
	}

	apiErr, ok := AsError(err)
	if !ok {
		t.Fatalf("want *Error, got %T (%v)", err, err)
	}
	if apiErr.Code != CodeAuthError {
		t.Fatalf("code = %d, want %d", apiErr.Code, CodeAuthError)
	}
	if apiErr.Message != "auth failed" {
		t.Fatalf("message = %q, want %q", apiErr.Message, "auth failed")
	}
	if apiErr.ReqID != "req-auth-1" {
		t.Fatalf("reqid = %q, want %q", apiErr.ReqID, "req-auth-1")
	}
	if apiErr.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("http status = %d, want %d", apiErr.HTTPStatus, http.StatusUnauthorized)
	}
}

func TestSendAudioBoundary(t *testing.T) {
	client := NewClient("test-app", WithV2APIKey("test-ak", "test-app"))
	conn := newFakeWSConn()

	svc := newASRServiceV2(client)
	svc.dialer = &fakeDialer{conn: conn}

	session, err := svc.OpenStreamSession(context.Background(), &ASRV2Config{Format: FormatPCM, SampleRate: SampleRate16000})
	if err != nil {
		t.Fatalf("OpenStreamSession error = %v", err)
	}
	defer session.Close()

	if err := session.SendAudio(context.Background(), nil, false); err == nil {
		t.Fatalf("SendAudio(nil,false) expected error")
	}

	if err := session.SendAudio(context.Background(), nil, true); err != nil {
		t.Fatalf("SendAudio(nil,true) should be allowed, got %v", err)
	}
}

func TestRecvResultAndError(t *testing.T) {
	client := NewClient("test-app", WithV2APIKey("test-ak", "test-app"))
	conn := newFakeWSConn()

	svc := newASRServiceV2(client)
	svc.dialer = &fakeDialer{conn: conn}

	session, err := svc.OpenStreamSession(context.Background(), &ASRV2Config{Format: FormatPCM, SampleRate: SampleRate16000})
	if err != nil {
		t.Fatalf("OpenStreamSession error = %v", err)
	}
	defer session.Close()

	resultPayload := []byte(`{"reqid":"r1","audio_info":{"duration":1200},"result":{"text":"hello","utterances":[{"text":"hello","start_time":0,"end_time":1000,"definite":true}]}}`)
	conn.enqueue(websocket.BinaryMessage, buildServerFrame(protocol.MessageTypeFullServer, protocol.FlagNegativeWithSeq, resultPayload))

	var gotResult *ASRV2Result
	for result, err := range session.Recv() {
		if err != nil {
			t.Fatalf("unexpected recv error: %v", err)
		}
		gotResult = result
		break
	}

	if gotResult == nil {
		t.Fatalf("no result received")
	}
	if gotResult.Text != "hello" || !gotResult.IsFinal {
		t.Fatalf("unexpected result: %+v", gotResult)
	}

	errPayload := []byte(`{"code":3002,"message":"auth failed","reqid":"r2"}`)
	conn.enqueue(websocket.BinaryMessage, buildServerErrorFrame(3002, errPayload))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	gotErr := waitRecvError(ctx, session.Recv())
	if gotErr == nil {
		t.Fatalf("expected error frame")
	}
	apiErr, ok := AsError(gotErr)
	if !ok {
		t.Fatalf("want *Error, got %T (%v)", gotErr, gotErr)
	}
	if apiErr.Code != 3002 {
		t.Fatalf("error code = %d, want 3002", apiErr.Code)
	}
}

func TestDecodeASRV2ResultFinalByNegativeSequence(t *testing.T) {
	payload := []byte(`{"reqid":"r-final","audio_info":{"duration":1200},"result":{"text":"hello","utterances":[{"text":"hello","start_time":0,"end_time":1000,"definite":false}]}}`)
	raw := buildServerFrame(protocol.MessageTypeFullServer, protocol.FlagNegativeSequence, payload)
	frame, err := protocol.ParseServerFrame(raw)
	if err != nil {
		t.Fatalf("ParseServerFrame error = %v", err)
	}

	result, err := decodeASRV2Result(frame, "fallback")
	if err != nil {
		t.Fatalf("decodeASRV2Result error = %v", err)
	}
	if result == nil {
		t.Fatalf("result is nil")
	}
	if !result.IsFinal {
		t.Fatalf("result.IsFinal = false, want true when flags=FlagNegativeSequence")
	}
}

func TestParseWSErrorPayloadCodeAndStatusCodePriority(t *testing.T) {
	tests := []struct {
		name         string
		payload      string
		fallbackCode uint32
		wantCode     int
		wantReqID    string
	}{
		{
			name:         "prefer code over status_code",
			payload:      `{"code":3002,"status_code":55000000,"message":"auth failed","reqid":"r-code"}`,
			fallbackCode: 0,
			wantCode:     3002,
			wantReqID:    "r-code",
		},
		{
			name:         "use status_code when code missing",
			payload:      `{"status_code":45000001,"message":"invalid request","reqid":"r-status"}`,
			fallbackCode: 0,
			wantCode:     45000001,
			wantReqID:    "r-status",
		},
		{
			name:         "use fallback when code and status_code missing",
			payload:      `{"message":"fallback path","reqid":"r-fallback"}`,
			fallbackCode: 7788,
			wantCode:     7788,
			wantReqID:    "r-fallback",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := parseWSErrorPayload([]byte(tc.payload), tc.fallbackCode)
			apiErr, ok := AsError(err)
			if !ok {
				t.Fatalf("want *Error, got %T (%v)", err, err)
			}
			if apiErr.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d", apiErr.Code, tc.wantCode)
			}
			if apiErr.ReqID != tc.wantReqID {
				t.Fatalf("reqid = %q, want %q", apiErr.ReqID, tc.wantReqID)
			}
		})
	}
}

func waitRecvError(ctx context.Context, seq iter.Seq2[*ASRV2Result, error]) error {
	ch := make(chan error, 1)
	go func() {
		for _, err := range seq {
			if err != nil {
				ch <- err
				return
			}
		}
		ch <- nil
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-ch:
		return err
	}
}

func buildServerFrame(msgType protocol.MessageType, flags protocol.MessageFlags, payload []byte) []byte {
	b := make([]byte, 12+len(payload))
	b[0] = 0x11
	b[1] = byte(msgType<<4) | byte(flags)
	b[2] = byte(protocol.SerializationJSON<<4) | byte(protocol.CompressionNone)
	b[3] = 0
	binary.BigEndian.PutUint32(b[4:8], 1)
	binary.BigEndian.PutUint32(b[8:12], uint32(len(payload)))
	copy(b[12:], payload)
	return b
}

func buildServerErrorFrame(code uint32, payload []byte) []byte {
	b := make([]byte, 16+len(payload))
	b[0] = 0x11
	b[1] = byte(protocol.MessageTypeError<<4) | byte(protocol.FlagPositiveSequence)
	b[2] = byte(protocol.SerializationJSON<<4) | byte(protocol.CompressionNone)
	b[3] = 0
	binary.BigEndian.PutUint32(b[4:8], 1)
	binary.BigEndian.PutUint32(b[8:12], code)
	binary.BigEndian.PutUint32(b[12:16], uint32(len(payload)))
	copy(b[16:], payload)
	return b
}
