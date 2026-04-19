package doubaospeech

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/doubao-speech-go/internal/auth"
	"github.com/GizClaw/doubao-speech-go/internal/transport"
	"github.com/GizClaw/doubao-speech-go/internal/util"
	"github.com/gorilla/websocket"
)

const (
	ttsV2WSMsgTypeFullClient      byte = 0x1
	ttsV2WSMsgTypeAudioOnlyServer byte = 0xB
	ttsV2WSMsgTypeError           byte = 0xF

	ttsV2WSFlagNoSequence       byte = 0x0
	ttsV2WSFlagPositiveSequence byte = 0x1
	ttsV2WSFlagNegativeSequence byte = 0x2
	ttsV2WSFlagNegativeWithSeq  byte = 0x3
	ttsV2WSFlagWithEvent        byte = 0x4

	ttsV2WSSerializationRaw  byte = 0x0
	ttsV2WSSerializationJSON byte = 0x1

	ttsV2WSCompressionNone byte = 0x0
	ttsV2WSCompressionGzip byte = 0x1

	ttsV2StatusSuccess = 20000000

	ttsV2WSMaxGzipDecodedSize int64 = 10 * 1024 * 1024

	ttsV2EventStartConnection  int32 = 1
	ttsV2EventFinishConnection int32 = 2
	ttsV2EventStartSession     int32 = 100
	ttsV2EventSessionCancel    int32 = 101
	ttsV2EventSessionFinish    int32 = 102
	ttsV2EventTaskRequest      int32 = 200

	ttsV2EventConnectionStarted  int32 = 50
	ttsV2EventConnectionFailed   int32 = 51
	ttsV2EventConnectionFinished int32 = 52
	ttsV2EventSessionStarted     int32 = 150
	ttsV2EventSessionCanceled    int32 = 151
	ttsV2EventSessionFinished    int32 = 152
	ttsV2EventSessionFailed      int32 = 153
	ttsV2EventSentenceStart      int32 = 350
	ttsV2EventSentenceEnd        int32 = 351
	ttsV2EventResponse           int32 = 352
)

// TTSServiceV2 provides TTS V2 WebSocket streaming synthesis.
type TTSServiceV2 struct {
	client *Client
	dialer transport.WSDialer
}

func newTTSServiceV2(c *Client) *TTSServiceV2 {
	return &TTSServiceV2{
		client: c,
		dialer: transport.NewGorillaDialer(nil),
	}
}

// TTSV2WSSession represents one bidirectional TTS V2 WebSocket session.
type TTSV2WSSession struct {
	conn      transport.WSConn
	client    *Client
	cfg       TTSV2WSConfig
	reqID     string
	sessionID string

	stateMu       sync.Mutex
	sessionActive bool

	chunkCh  chan *TTSV2WSChunk
	errCh    chan error
	closed   chan struct{}
	recvDone chan struct{}

	connStartedCh    chan struct{}
	sessionStartedCh chan struct{}

	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

// OpenStreamSession opens a TTS V2 bidirectional WebSocket stream session.
func (s *TTSServiceV2) OpenStreamSession(ctx context.Context, cfg *TTSV2WSConfig) (*TTSV2WSSession, error) {
	if cfg == nil {
		return nil, newAPIError(CodeParamError, "config is nil")
	}

	normalized, err := normalizeTTSV2WSConfig(*cfg)
	if err != nil {
		return nil, err
	}

	resourceID := s.client.resolveResourceID(normalized.ResourceID, ResourceTTSV2)
	connectID := util.NewReqID("tts")
	sessionID, err := newTTSV2SessionID(12)
	if err != nil {
		return nil, wrapError(err, "generate session id")
	}

	endpoint := strings.TrimRight(s.client.config.wsURL, "/") + "/api/v3/tts/bidirection"
	headers := auth.BuildV2WSHeaders(s.client.authCredentials(), resourceID, connectID)

	conn, resp, err := s.dialer.DialContext(ctx, endpoint, headers)
	if err != nil {
		return nil, wsConnectError(err, resp)
	}

	session := &TTSV2WSSession{
		conn:             conn,
		client:           s.client,
		cfg:              normalized,
		reqID:            connectID,
		sessionID:        sessionID,
		chunkCh:          make(chan *TTSV2WSChunk, 64),
		errCh:            make(chan error, 1),
		closed:           make(chan struct{}),
		recvDone:         make(chan struct{}),
		connStartedCh:    make(chan struct{}, 1),
		sessionStartedCh: make(chan struct{}, 1),
	}

	go session.receiveLoop()

	if err := session.sendStartConnection(ctx); err != nil {
		_ = session.Close()
		return nil, wrapError(err, "send start connection")
	}
	if err := session.waitForLifecycleEvent(ctx, session.connStartedCh); err != nil {
		_ = session.Close()
		return nil, wrapError(err, "wait connection started")
	}

	if err := session.sendStartSession(ctx); err != nil {
		_ = session.Close()
		return nil, wrapError(err, "send start session")
	}
	if err := session.waitForLifecycleEvent(ctx, session.sessionStartedCh); err != nil {
		_ = session.Close()
		return nil, wrapError(err, "wait session started")
	}

	return session, nil
}

// SendText sends one text piece. When isLast=true, it also sends FinishSession.
func (s *TTSV2WSSession) SendText(ctx context.Context, text string, isLast bool) error {
	text = strings.TrimSpace(text)
	if text == "" && !isLast {
		return newAPIError(CodeParamError, "text is empty")
	}
	if err := s.guardContext(ctx); err != nil {
		return err
	}
	if !s.isSessionActive() {
		return newAPIError(CodeServerError, "session is not active; call StartNextSession after final event")
	}

	if text != "" {
		payload := map[string]any{
			"user": map[string]any{
				"uid": s.client.config.userID,
			},
			"event": ttsV2EventTaskRequest,
			"req_params": map[string]any{
				"text": text,
			},
		}

		if err := s.sendSessionEvent(ctx, ttsV2EventTaskRequest, payload); err != nil {
			return wrapError(err, "send task request")
		}
	}

	if isLast {
		if err := s.sendFinishSession(ctx); err != nil {
			return wrapError(err, "send finish session")
		}
	}

	return nil
}

// StartNextSession starts a new session on the same WebSocket connection.
func (s *TTSV2WSSession) StartNextSession(ctx context.Context) error {
	if err := s.guardContext(ctx); err != nil {
		return err
	}
	if s.isSessionActive() {
		return newAPIError(CodeParamError, "current session is still active")
	}

	sessionID, err := newTTSV2SessionID(12)
	if err != nil {
		return wrapError(err, "generate session id")
	}
	s.setSessionID(sessionID)

	if err := s.sendStartSession(ctx); err != nil {
		return wrapError(err, "send start session")
	}
	if err := s.waitForLifecycleEvent(ctx, s.sessionStartedCh); err != nil {
		return wrapError(err, "wait session started")
	}

	return nil
}

// CancelSession cancels the current session.
func (s *TTSV2WSSession) CancelSession(ctx context.Context) error {
	if err := s.guardContext(ctx); err != nil {
		return err
	}
	if !s.isSessionActive() {
		return newAPIError(CodeParamError, "current session is not active")
	}

	if err := s.sendSessionEvent(ctx, ttsV2EventSessionCancel, map[string]any{}); err != nil {
		return wrapError(err, "send cancel session")
	}

	return nil
}

// Recv yields TTS output chunks.
func (s *TTSV2WSSession) Recv() iter.Seq2[*TTSV2WSChunk, error] {
	return func(yield func(*TTSV2WSChunk, error) bool) {
		chunks := s.chunkCh
		errs := s.errCh

		for chunks != nil || errs != nil {
			select {
			case c, ok := <-chunks:
				if !ok {
					chunks = nil
					continue
				}
				if !yield(c, nil) {
					return
				}
				if c.IsFinal {
					return
				}
			case err, ok := <-errs:
				if !ok {
					errs = nil
					continue
				}
				yield(nil, err)
				return
			}
		}
	}
}

// Close closes the session.
func (s *TTSV2WSSession) Close() error {
	s.closeOnce.Do(func() {
		if s.isSessionActive() {
			_ = s.sendFinishSession(context.Background())
		}
		_ = s.sendFinishConnection(context.Background())

		close(s.closed)
		s.closeErr = s.conn.Close()

		select {
		case <-s.recvDone:
		case <-time.After(2 * time.Second):
		}
	})

	return s.closeErr
}

func (s *TTSV2WSSession) sendStartConnection(ctx context.Context) error {
	return s.sendConnectEvent(ctx, ttsV2EventStartConnection, map[string]any{
		"namespace": "BidirectionalTTS",
	})
}

func (s *TTSV2WSSession) sendFinishConnection(ctx context.Context) error {
	return s.sendConnectEvent(ctx, ttsV2EventFinishConnection, map[string]any{})
}

func (s *TTSV2WSSession) sendStartSession(ctx context.Context) error {
	payload := map[string]any{
		"user": map[string]any{
			"uid": s.client.config.userID,
		},
		"event": ttsV2EventStartSession,
		"req_params": map[string]any{
			"speaker": s.cfg.Speaker,
			"audio_params": map[string]any{
				"format":      string(s.cfg.Format),
				"sample_rate": int(s.cfg.SampleRate),
			},
		},
	}

	return s.sendSessionEvent(ctx, ttsV2EventStartSession, payload)
}

func (s *TTSV2WSSession) sendFinishSession(ctx context.Context) error {
	if !s.isSessionActive() {
		return nil
	}
	return s.sendSessionEvent(ctx, ttsV2EventSessionFinish, map[string]any{})
}

func (s *TTSV2WSSession) sendConnectEvent(ctx context.Context, event int32, payload any) error {
	if err := s.guardContext(ctx); err != nil {
		return err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return wrapError(err, "marshal connect payload")
	}
	frame, err := buildTTSV2ConnectEventFrame(event, body)
	if err != nil {
		return err
	}

	return s.writeBinary(frame)
}

func (s *TTSV2WSSession) sendSessionEvent(ctx context.Context, event int32, payload any) error {
	if err := s.guardContext(ctx); err != nil {
		return err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return wrapError(err, "marshal session payload")
	}
	frame, err := buildTTSV2SessionEventFrame(s.getSessionID(), event, body)
	if err != nil {
		return err
	}

	return s.writeBinary(frame)
}

func (s *TTSV2WSSession) writeBinary(packet []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if s.isClosed() {
		return newAPIError(CodeServerError, "session already closed")
	}

	return s.conn.WriteMessage(websocket.BinaryMessage, packet)
}

func (s *TTSV2WSSession) waitForLifecycleEvent(ctx context.Context, ch <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	case err, ok := <-s.errCh:
		if !ok {
			return newAPIError(CodeServerError, "session closed before lifecycle event")
		}
		if err == nil {
			return newAPIError(CodeServerError, "session lifecycle failed")
		}
		return err
	}
}

func (s *TTSV2WSSession) receiveLoop() {
	defer close(s.recvDone)
	defer close(s.chunkCh)
	defer close(s.errCh)

	for {
		if s.isClosed() {
			return
		}

		msgType, payload, err := s.conn.ReadMessage()
		if err != nil {
			if s.isClosed() {
				return
			}
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			s.pushErr(wrapError(err, "websocket read"))
			return
		}

		switch msgType {
		case websocket.TextMessage:
			s.pushErr(parseWSErrorPayload(payload, 0))
			return
		case websocket.BinaryMessage:
			frame, err := parseTTSV2WSFrame(payload)
			if err != nil {
				s.pushErr(wrapError(err, "parse tts websocket frame"))
				continue
			}

			if frame.messageType == ttsV2WSMsgTypeError {
				s.pushErr(parseWSErrorPayload(frame.payload, frame.errorCode))
				return
			}

			if frame.messageType == ttsV2WSMsgTypeAudioOnlyServer {
				chunk, err := decodeTTSV2Chunk(frame, s.reqID)
				if err != nil {
					s.pushErr(err)
					return
				}
				if chunk == nil {
					continue
				}
				select {
				case s.chunkCh <- chunk:
				case <-s.closed:
					return
				}
				continue
			}

			if !frame.hasEvent {
				continue
			}

			switch frame.event {
			case ttsV2EventConnectionStarted:
				notifyEvent(s.connStartedCh)
			case ttsV2EventSessionStarted:
				s.setSessionActive(true)
				notifyEvent(s.sessionStartedCh)
			case ttsV2EventResponse:
				chunk, err := decodeTTSV2Chunk(frame, s.reqID)
				if err != nil {
					s.pushErr(err)
					return
				}
				if chunk == nil {
					continue
				}
				select {
				case s.chunkCh <- chunk:
				case <-s.closed:
					return
				}
			case ttsV2EventSessionFinished:
				if err := validateTTSV2SessionFinished(frame.payload); err != nil {
					s.pushErr(err)
					return
				}
				s.setSessionActive(false)
				select {
				case s.chunkCh <- &TTSV2WSChunk{IsFinal: true, Event: frame.event, ReqID: s.reqID}:
				case <-s.closed:
				}
				continue
			case ttsV2EventSessionCanceled:
				if err := validateTTSV2SessionFinished(frame.payload); err != nil {
					s.pushErr(err)
					return
				}
				s.setSessionActive(false)
				select {
				case s.chunkCh <- &TTSV2WSChunk{IsFinal: true, Event: frame.event, ReqID: s.reqID}:
				case <-s.closed:
				}
				continue
			case ttsV2EventSessionFailed:
				s.setSessionActive(false)
				s.pushErr(parseWSErrorPayload(frame.payload, 0))
				return
			case ttsV2EventConnectionFailed:
				s.pushErr(parseWSErrorPayload(frame.payload, 0))
				return
			case ttsV2EventSentenceStart, ttsV2EventSentenceEnd, ttsV2EventConnectionFinished:
				// Ignore sentence boundary / connection-finished events.
			default:
				// Ignore unknown events in this migration scope.
			}
		default:
			// Ignore unknown message types.
		}
	}
}

func (s *TTSV2WSSession) guardContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if s.isClosed() {
		return newAPIError(CodeServerError, "session already closed")
	}
	return nil
}

func (s *TTSV2WSSession) isSessionActive() bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.sessionActive
}

func (s *TTSV2WSSession) setSessionActive(v bool) {
	s.stateMu.Lock()
	s.sessionActive = v
	s.stateMu.Unlock()
}

func (s *TTSV2WSSession) getSessionID() string {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.sessionID
}

func (s *TTSV2WSSession) setSessionID(id string) {
	s.stateMu.Lock()
	s.sessionID = id
	s.stateMu.Unlock()
}

func (s *TTSV2WSSession) isClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *TTSV2WSSession) pushErr(err error) {
	if err == nil {
		return
	}
	select {
	case s.errCh <- err:
	default:
	}
}

type ttsV2WSParsedFrame struct {
	messageType   byte
	flags         byte
	serialization byte
	compression   byte

	hasSequence bool
	sequence    int32

	hasEvent  bool
	event     int32
	eventID   string
	errorCode uint32
	payload   []byte
}

func parseTTSV2WSFrame(data []byte) (*ttsV2WSParsedFrame, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("frame too short: %d", len(data))
	}

	headerUnits := int(data[0] & 0x0F)
	if headerUnits <= 0 {
		return nil, fmt.Errorf("invalid header units: %d", headerUnits)
	}

	headerSize := headerUnits * 4
	if len(data) < headerSize {
		return nil, fmt.Errorf("incomplete header: need %d, got %d", headerSize, len(data))
	}

	frame := &ttsV2WSParsedFrame{
		messageType:   (data[1] >> 4) & 0x0F,
		flags:         data[1] & 0x0F,
		serialization: (data[2] >> 4) & 0x0F,
		compression:   data[2] & 0x0F,
	}

	offset := headerSize

	if hasTTSV2Sequence(frame.flags) {
		if len(data) < offset+4 {
			return nil, fmt.Errorf("missing sequence field")
		}
		frame.hasSequence = true
		frame.sequence = int32(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
	}

	if frame.flags&ttsV2WSFlagWithEvent != 0 {
		if len(data) < offset+4 {
			return nil, fmt.Errorf("missing event field")
		}
		frame.hasEvent = true
		frame.event = int32(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4

		if shouldReadTTSV2EventID(frame.event) {
			id, next, err := parseTTSV2EventID(data, offset)
			if err != nil {
				return nil, err
			}
			frame.eventID = id
			offset = next
		}
	}

	if frame.messageType == ttsV2WSMsgTypeError {
		if len(data) < offset+4 {
			return nil, fmt.Errorf("missing error code")
		}
		frame.errorCode = binary.BigEndian.Uint32(data[offset : offset+4])
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
	if frame.compression == ttsV2WSCompressionGzip {
		decoded, err := gzipDecode(payload)
		if err != nil {
			return nil, wrapError(err, "gzip decode payload")
		}
		payload = decoded
	}

	frame.payload = payload
	return frame, nil
}

func shouldReadTTSV2EventID(event int32) bool {
	if event == ttsV2EventConnectionStarted || event == ttsV2EventConnectionFailed || event == ttsV2EventConnectionFinished {
		return true
	}
	return event >= ttsV2EventSessionStarted
}

func hasTTSV2Sequence(flags byte) bool {
	v := flags & 0x3
	return v == ttsV2WSFlagPositiveSequence || v == ttsV2WSFlagNegativeSequence || v == ttsV2WSFlagNegativeWithSeq
}

func parseTTSV2EventID(data []byte, offset int) (string, int, error) {
	if len(data) < offset+4 {
		return "", offset, fmt.Errorf("missing event id length")
	}
	idSize := int(binary.BigEndian.Uint32(data[offset : offset+4]))
	offset += 4
	if idSize < 0 || len(data) < offset+idSize {
		return "", offset, fmt.Errorf("invalid event id size: %d", idSize)
	}
	id := string(data[offset : offset+idSize])
	offset += idSize
	return id, offset, nil
}

func buildTTSV2ConnectEventFrame(event int32, payload []byte) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 12+len(payload)))
	buf.WriteByte(0x11)
	buf.WriteByte(byte((ttsV2WSMsgTypeFullClient << 4) | ttsV2WSFlagWithEvent))
	buf.WriteByte(byte((ttsV2WSSerializationJSON << 4) | ttsV2WSCompressionNone))
	buf.WriteByte(0x00)

	if err := binary.Write(buf, binary.BigEndian, event); err != nil {
		return nil, wrapError(err, "write event")
	}
	if err := binary.Write(buf, binary.BigEndian, uint32(len(payload))); err != nil {
		return nil, wrapError(err, "write payload size")
	}
	if _, err := buf.Write(payload); err != nil {
		return nil, wrapError(err, "write payload")
	}

	return buf.Bytes(), nil
}

func buildTTSV2SessionEventFrame(sessionID string, event int32, payload []byte) ([]byte, error) {
	sid := []byte(sessionID)
	buf := bytes.NewBuffer(make([]byte, 0, 16+len(sid)+len(payload)))
	buf.WriteByte(0x11)
	buf.WriteByte(byte((ttsV2WSMsgTypeFullClient << 4) | ttsV2WSFlagWithEvent))
	buf.WriteByte(byte((ttsV2WSSerializationJSON << 4) | ttsV2WSCompressionNone))
	buf.WriteByte(0x00)

	if err := binary.Write(buf, binary.BigEndian, event); err != nil {
		return nil, wrapError(err, "write event")
	}
	if err := binary.Write(buf, binary.BigEndian, uint32(len(sid))); err != nil {
		return nil, wrapError(err, "write session id size")
	}
	if _, err := buf.Write(sid); err != nil {
		return nil, wrapError(err, "write session id")
	}
	if err := binary.Write(buf, binary.BigEndian, uint32(len(payload))); err != nil {
		return nil, wrapError(err, "write payload size")
	}
	if _, err := buf.Write(payload); err != nil {
		return nil, wrapError(err, "write payload")
	}

	return buf.Bytes(), nil
}

func decodeTTSV2Chunk(frame *ttsV2WSParsedFrame, reqID string) (*TTSV2WSChunk, error) {
	if frame == nil {
		return nil, newAPIError(CodeServerError, "tts frame is nil")
	}

	if frame.messageType == ttsV2WSMsgTypeAudioOnlyServer || frame.serialization == ttsV2WSSerializationRaw {
		if len(frame.payload) == 0 {
			return nil, nil
		}
		return &TTSV2WSChunk{
			Audio: frame.payload,
			Event: frame.event,
			ReqID: reqID,
		}, nil
	}

	if frame.serialization != ttsV2WSSerializationJSON {
		return nil, newAPIError(CodeServerError, fmt.Sprintf("unsupported serialization: %d", frame.serialization))
	}

	var payload struct {
		Data  string `json:"data"`
		ReqID string `json:"reqid"`
	}
	if err := json.Unmarshal(frame.payload, &payload); err != nil {
		return nil, wrapError(err, "unmarshal tts response payload")
	}
	if payload.Data == "" {
		return nil, nil
	}

	audio, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		return nil, wrapError(err, "decode base64 audio")
	}

	chunkReqID := reqID
	if payload.ReqID != "" {
		chunkReqID = payload.ReqID
	}

	return &TTSV2WSChunk{
		Audio: audio,
		Event: frame.event,
		ReqID: chunkReqID,
	}, nil
}

func validateTTSV2SessionFinished(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}

	var p struct {
		StatusCode int    `json:"status_code"`
		Code       int    `json:"code"`
		Message    string `json:"message"`
		Error      string `json:"error"`
		ReqID      string `json:"reqid"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil
	}

	status := p.StatusCode
	if status == 0 {
		status = p.Code
	}
	if status == 0 || status == ttsV2StatusSuccess {
		return nil
	}

	msg := p.Message
	if msg == "" {
		msg = p.Error
	}
	if msg == "" {
		msg = "session finished with non-success status"
	}

	if p.ReqID != "" {
		return &Error{Code: status, Message: msg, ReqID: p.ReqID}
	}
	return &Error{Code: status, Message: msg}
}

func normalizeTTSV2WSConfig(cfg TTSV2WSConfig) (TTSV2WSConfig, error) {
	if strings.TrimSpace(cfg.Speaker) == "" {
		return cfg, newAPIError(CodeParamError, "speaker is empty")
	}

	if cfg.Format == "" {
		cfg.Format = FormatMP3
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = SampleRate24000
	}

	if err := util.ValidateFormat(string(cfg.Format)); err != nil {
		return cfg, newAPIError(CodeParamError, err.Error())
	}
	if err := util.ValidateSampleRate(int(cfg.SampleRate)); err != nil {
		return cfg, newAPIError(CodeParamError, err.Error())
	}

	return cfg, nil
}

func newTTSV2SessionID(length int) (string, error) {
	if length <= 0 {
		length = 12
	}

	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	out := make([]byte, length)
	for i := range raw {
		out[i] = alphabet[int(raw[i])%len(alphabet)]
	}

	return string(out), nil
}

func gzipDecode(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	b, err := io.ReadAll(io.LimitReader(r, ttsV2WSMaxGzipDecodedSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > ttsV2WSMaxGzipDecodedSize {
		return nil, fmt.Errorf("gzip decoded payload exceeds limit: %d", ttsV2WSMaxGzipDecodedSize)
	}

	return b, nil
}

func notifyEvent(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
