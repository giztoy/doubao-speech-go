package doubaospeech

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iter"
	"strings"
	"sync"
	"time"

	"github.com/giztoy/doubao-speech-go/internal/auth"
	"github.com/giztoy/doubao-speech-go/internal/protocol"
	"github.com/giztoy/doubao-speech-go/internal/transport"
	"github.com/giztoy/doubao-speech-go/internal/util"
	"github.com/gorilla/websocket"
)

const (
	realtimeEndpointPath             = "/api/v3/realtime/dialogue"
	realtimeStartSessionEvent  int32 = 100
	realtimeFinishSessionEvent int32 = 102
	realtimeTaskAudioEvent     int32 = 200
	realtimeSayHelloEvent      int32 = 300
	realtimeTTSTextEvent       int32 = 500
	realtimeUserTextEvent      int32 = 501

	defaultRealtimeEventBuffer         = 64
	defaultRealtimeBackpressureTimeout = 2 * time.Second
	defaultRealtimeCloseWaitTimeout    = 2 * time.Second
)

// RealtimeService provides real-time dialogue operations.
type RealtimeService struct {
	client *Client
	dialer transport.WSDialer
}

func newRealtimeService(c *Client) *RealtimeService {
	return &RealtimeService{
		client: c,
		dialer: transport.NewGorillaDialer(nil),
	}
}

// Dial opens a realtime websocket connection and completes StartConnection handshake.
func (s *RealtimeService) Dial(ctx context.Context) (*RealtimeConnection, error) {
	connectReqID := util.NewReqID("rt")
	resourceID := s.client.resolveResourceID("", ResourceRealtime)

	headers := auth.BuildV2WSHeaders(s.client.authCredentials(), resourceID, connectReqID)
	headers.Set("X-Api-Request-Id", connectReqID)

	endpoint := strings.TrimRight(s.client.config.wsURL, "/") + realtimeEndpointPath
	conn, resp, err := s.dialer.DialContext(ctx, endpoint, headers)
	if err != nil {
		return nil, wsConnectError(err, resp)
	}

	rtConn := &RealtimeConnection{
		conn:      conn,
		service:   s,
		connectID: connectReqID,
		closed:    make(chan struct{}),
	}

	if err := rtConn.sendConnectionStart(ctx); err != nil {
		_ = rtConn.Close()
		return nil, wrapError(err, "send start connection")
	}

	frame, err := rtConn.readFrameWithContext(ctx)
	if err != nil {
		_ = rtConn.Close()
		return nil, wrapError(err, "read connection response")
	}
	if frame.MessageType == protocol.MessageTypeError {
		_ = rtConn.Close()
		return nil, wrapError(parseWSErrorPayload(frame.Payload, frame.ErrorCode), "connection failed")
	}
	if !frame.HasEvent || frame.Event != int32(EventConnectionStarted) {
		_ = rtConn.Close()
		return nil, fmt.Errorf("unexpected connection response event: %d", frame.Event)
	}
	if frame.ConnectID != "" {
		rtConn.connectID = frame.ConnectID
	}

	return rtConn, nil
}

// Connect is a convenience method for Dial + StartSession.
func (s *RealtimeService) Connect(ctx context.Context, cfg *RealtimeConfig) (*RealtimeSession, error) {
	conn, err := s.Dial(ctx)
	if err != nil {
		return nil, err
	}

	session, err := conn.StartSession(ctx, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	return session, nil
}

// OpenSession is a compatibility alias for Connect.
func (s *RealtimeService) OpenSession(ctx context.Context, cfg *RealtimeConfig) (*RealtimeSession, error) {
	return s.Connect(ctx, cfg)
}

// RealtimeConnection represents an established realtime websocket connection.
type RealtimeConnection struct {
	conn    transport.WSConn
	service *RealtimeService

	connectID string

	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
	closed    chan struct{}
}

// StartSession starts one realtime session on current connection.
func (c *RealtimeConnection) StartSession(ctx context.Context, cfg *RealtimeConfig) (*RealtimeSession, error) {
	normalized, err := normalizeRealtimeConfig(cfg)
	if err != nil {
		return nil, err
	}

	sessionID := util.NewReqID("session")
	startPayload, err := buildRealtimeStartPayload(normalized)
	if err != nil {
		return nil, wrapError(err, "marshal start session payload")
	}

	packet, err := protocol.BuildFullClientJSONWithEvent(realtimeStartSessionEvent, sessionID, startPayload)
	if err != nil {
		return nil, wrapError(err, "encode start session event")
	}
	if err := c.writeBinary(ctx, packet); err != nil {
		return nil, wrapError(err, "send start session")
	}

	frame, err := c.readFrameWithContext(ctx)
	if err != nil {
		return nil, wrapError(err, "read start session response")
	}
	if frame.MessageType == protocol.MessageTypeError {
		return nil, wrapError(parseWSErrorPayload(frame.Payload, frame.ErrorCode), "start session failed")
	}
	if !frame.HasEvent || frame.Event != int32(EventSessionStarted) {
		return nil, fmt.Errorf("unexpected session response event: %d", frame.Event)
	}
	if frame.SessionID != "" {
		sessionID = frame.SessionID
	}

	session := &RealtimeSession{
		conn:      c,
		sessionID: sessionID,
		eventCh:   make(chan *RealtimeEvent, normalized.EventBuffer),
		errCh:     make(chan error, 1),
		closed:    make(chan struct{}),
		recvDone:  make(chan struct{}),

		backpressureTimeout: normalized.BackpressureTimeout,

		history: cloneConversationHistory(normalized.History),
		prompt:  clonePromptConfig(normalized.Prompt),
		props:   cloneGenerationProps(normalized.Props),
	}

	go session.receiveLoop()

	return session, nil
}

// Close closes websocket connection.
func (c *RealtimeConnection) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		c.closeErr = c.conn.Close()
	})
	return c.closeErr
}

func (c *RealtimeConnection) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func (c *RealtimeConnection) writeBinary(ctx context.Context, packet []byte) error {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.isClosed() {
		return newAPIError(CodeServerError, "realtime connection already closed")
	}

	return c.conn.WriteMessage(websocket.BinaryMessage, packet)
}

func (c *RealtimeConnection) sendConnectionStart(ctx context.Context) error {
	packet, err := protocol.BuildFullClientJSONWithEvent(protocol.EventStartConnection, "", []byte("{}"))
	if err != nil {
		return wrapError(err, "encode start connection event")
	}
	return c.writeBinary(ctx, packet)
}

func (c *RealtimeConnection) sendConnectionFinish(ctx context.Context) error {
	packet, err := protocol.BuildFullClientJSONWithEvent(protocol.EventFinishConnection, "", []byte("{}"))
	if err != nil {
		return wrapError(err, "encode finish connection event")
	}
	return c.writeBinary(ctx, packet)
}

func (c *RealtimeConnection) readFrameWithContext(ctx context.Context) (*protocol.ParsedFrame, error) {
	msgType, payload, err := readWSMessageWithContext(ctx, c.conn)
	if err != nil {
		return nil, err
	}

	switch msgType {
	case websocket.BinaryMessage:
		frame, err := protocol.ParseServerFrame(payload)
		if err != nil {
			return nil, wrapError(err, "parse websocket binary frame")
		}
		return frame, nil
	case websocket.TextMessage:
		return nil, parseWSErrorPayload(payload, 0)
	default:
		return nil, fmt.Errorf("unsupported websocket message type: %d", msgType)
	}
}

// RealtimeSession represents one realtime dialogue session.
type RealtimeSession struct {
	conn      *RealtimeConnection
	sessionID string

	eventCh  chan *RealtimeEvent
	errCh    chan error
	closed   chan struct{}
	recvDone chan struct{}

	backpressureTimeout time.Duration

	stateMu sync.RWMutex
	history []RealtimeConversationMessage
	prompt  RealtimePromptConfig
	props   RealtimeGenerationProps

	turnMu              sync.Mutex
	turnFinalDelivered  bool
	audioTurnInProgress bool

	recvMu   sync.Mutex
	recvBusy bool

	errOnce   sync.Once
	closeOnce sync.Once
	closeErr  error
}

// SessionID returns current session ID.
func (s *RealtimeSession) SessionID() string {
	return s.sessionID
}

// SendAudio sends one audio chunk (event=200).
func (s *RealtimeSession) SendAudio(ctx context.Context, audio []byte) error {
	if len(audio) == 0 {
		return newAPIError(CodeParamError, "audio payload is empty")
	}
	if err := s.guardSend(ctx); err != nil {
		return err
	}

	s.beginAudioTurn()

	packet, err := protocol.BuildAudioOnlyWithEvent(realtimeTaskAudioEvent, s.sessionID, audio)
	if err != nil {
		return wrapError(err, "encode audio event")
	}

	return s.conn.writeBinary(ctx, packet)
}

// SendText sends user text (event=501).
func (s *RealtimeSession) SendText(ctx context.Context, text string) error {
	return s.SendUserMessage(ctx, text)
}

// SendUserMessage sends one user text with current history/prompt/props snapshot.
func (s *RealtimeSession) SendUserMessage(ctx context.Context, text string) error {
	content := strings.TrimSpace(text)
	if content == "" {
		return newAPIError(CodeParamError, "text is empty")
	}
	if err := s.guardSend(ctx); err != nil {
		return err
	}

	s.resetTurnFinalState()

	history, prompt, props := s.snapshotSessionState()
	payload := map[string]any{
		"content": content,
	}
	if len(history) > 0 {
		payload["history"] = history
	}
	if prompt.System != "" || len(prompt.Variables) > 0 {
		payload["prompt"] = prompt
	}
	if hasRealtimeProps(props) {
		payload["props"] = props
	}

	if err := s.sendJSONEvent(ctx, realtimeUserTextEvent, payload); err != nil {
		return err
	}

	s.appendHistory(RealtimeConversationMessage{Role: "user", Content: content})
	return nil
}

// SayHello sends SayHello event (event=300).
func (s *RealtimeSession) SayHello(ctx context.Context, content string) error {
	if strings.TrimSpace(content) == "" {
		return newAPIError(CodeParamError, "hello content is empty")
	}
	if err := s.guardSend(ctx); err != nil {
		return err
	}

	s.resetTurnFinalState()
	return s.sendJSONEvent(ctx, realtimeSayHelloEvent, map[string]any{"content": content})
}

// Interrupt interrupts current generation (event=102).
func (s *RealtimeSession) Interrupt(ctx context.Context) error {
	if err := s.guardSend(ctx); err != nil {
		return err
	}
	return s.sendJSONEvent(ctx, realtimeFinishSessionEvent, map[string]any{"session_id": s.sessionID})
}

// SendTTSText sends incremental TTS text (event=500).
func (s *RealtimeSession) SendTTSText(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		return newAPIError(CodeParamError, "tts text is empty")
	}
	if err := s.guardSend(ctx); err != nil {
		return err
	}

	s.resetTurnFinalState()
	return s.sendJSONEvent(ctx, realtimeTTSTextEvent, map[string]any{"content": text})
}

// UpdateHistory replaces the whole local history snapshot used by future turns.
func (s *RealtimeSession) UpdateHistory(history []RealtimeConversationMessage) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.history = cloneConversationHistory(history)
}

// ReplaceHistory replaces one item in local history by index.
func (s *RealtimeSession) ReplaceHistory(index int, message RealtimeConversationMessage) error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if index < 0 || index >= len(s.history) {
		return newAPIError(CodeParamError, "history index out of range")
	}
	s.history[index] = message
	return nil
}

// UpdatePrompt replaces current prompt config used by future turns.
func (s *RealtimeSession) UpdatePrompt(prompt RealtimePromptConfig) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.prompt = clonePromptConfig(prompt)
}

// UpdateProps replaces current generation props used by future turns.
func (s *RealtimeSession) UpdateProps(props RealtimeGenerationProps) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.props = cloneGenerationProps(props)
}

// Recv returns a streaming iterator. Concurrent Recv is not supported.
func (s *RealtimeSession) Recv() iter.Seq2[*RealtimeEvent, error] {
	return func(yield func(*RealtimeEvent, error) bool) {
		if err := s.beginRecv(); err != nil {
			yield(nil, err)
			return
		}
		defer s.endRecv()

		events := s.eventCh
		errs := s.errCh

		for events != nil || errs != nil {
			select {
			case evt, ok := <-events:
				if !ok {
					events = nil
					continue
				}
				if !yield(evt, nil) {
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

// RecvEvent receives one event. Concurrent Recv/RecvEvent is not supported.
func (s *RealtimeSession) RecvEvent(ctx context.Context) (*RealtimeEvent, error) {
	if err := s.beginRecv(); err != nil {
		return nil, err
	}
	defer s.endRecv()

	events := s.eventCh
	errs := s.errCh

	for events != nil || errs != nil {
		select {
		case evt, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			return evt, nil
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, nil
}

// Close closes current session. It is idempotent.
func (s *RealtimeSession) Close() error {
	s.closeOnce.Do(func() {
		// Best-effort finish signals.
		_ = s.sendJSONEvent(context.Background(), realtimeFinishSessionEvent, map[string]any{"session_id": s.sessionID})
		_ = s.conn.sendConnectionFinish(context.Background())

		close(s.closed)
		s.closeErr = s.conn.Close()

		select {
		case <-s.recvDone:
		case <-time.After(defaultRealtimeCloseWaitTimeout):
		}
	})

	return s.closeErr
}

func (s *RealtimeSession) isClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *RealtimeSession) guardSend(ctx context.Context) error {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	if s.isClosed() {
		return newAPIError(CodeServerError, "realtime session already closed")
	}
	return nil
}

func (s *RealtimeSession) sendJSONEvent(ctx context.Context, event int32, body map[string]any) error {
	if body == nil {
		body = map[string]any{}
	}
	if body["session_id"] == nil {
		body["session_id"] = s.sessionID
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return wrapError(err, "marshal realtime event payload")
	}

	packet, err := protocol.BuildFullClientJSONWithEvent(event, s.sessionID, payload)
	if err != nil {
		return wrapError(err, "encode realtime event")
	}

	if err := s.conn.writeBinary(ctx, packet); err != nil {
		return wrapError(err, "send realtime event")
	}
	return nil
}

func (s *RealtimeSession) beginRecv() error {
	s.recvMu.Lock()
	defer s.recvMu.Unlock()

	if s.recvBusy {
		return newAPIError(CodeParamError, "concurrent Recv is not supported")
	}
	s.recvBusy = true
	return nil
}

func (s *RealtimeSession) endRecv() {
	s.recvMu.Lock()
	defer s.recvMu.Unlock()

	s.recvBusy = false
}

func (s *RealtimeSession) receiveLoop() {
	defer close(s.recvDone)
	defer close(s.eventCh)
	defer close(s.errCh)

	for {
		if s.isClosed() {
			return
		}

		msgType, payload, err := s.conn.conn.ReadMessage()
		if err != nil {
			if s.isClosed() {
				return
			}
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			s.pushErr(wrapError(err, "realtime websocket read"))
			return
		}

		switch msgType {
		case websocket.BinaryMessage:
			frame, err := protocol.ParseServerFrame(payload)
			if err != nil {
				s.pushErr(wrapError(err, "parse realtime frame"))
				return
			}

			evt, fatalErr := s.decodeFrame(frame)
			if evt != nil {
				if err := s.enqueueEvent(evt); err != nil {
					s.pushErr(err)
					return
				}
			}
			if fatalErr != nil {
				s.pushErr(fatalErr)
				return
			}
		case websocket.TextMessage:
			s.pushErr(parseWSErrorPayload(payload, 0))
			return
		default:
			// Ignore unsupported message types.
		}
	}
}

func (s *RealtimeSession) decodeFrame(frame *protocol.ParsedFrame) (*RealtimeEvent, error) {
	evt := &RealtimeEvent{
		Payload: copyBytes(frame.Payload),
	}

	if frame.HasSequence {
		evt.Sequence = frame.Sequence
	}
	if frame.HasEvent {
		evt.Type = RealtimeEventType(frame.Event)
	}
	if frame.SessionID != "" {
		evt.SessionID = frame.SessionID
	}
	if frame.ConnectID != "" {
		evt.ConnectID = frame.ConnectID
	}

	if frame.MessageType == protocol.MessageTypeError {
		parsedErr := parseWSErrorPayload(frame.Payload, frame.ErrorCode)
		if apiErr, ok := AsError(parsedErr); ok {
			evt.Error = apiErr
			evt.ReqID = apiErr.ReqID
			evt.TraceID = apiErr.TraceID
		}
		if evt.Type == 0 {
			evt.Type = EventSessionFailed
		}
		return evt, wrapRealtimeEventError(evt.Type, parsedErr)
	}

	if frame.MessageType == protocol.MessageTypeAudioOnlyServer {
		evt.Audio = copyBytes(frame.Payload)
		if evt.Type == 0 {
			evt.Type = EventTTSAudioData
		}
	}

	decodeEventPayload(evt)
	s.markFinalOnce(evt)

	if evt.Error != nil {
		return evt, wrapRealtimeEventError(evt.Type, evt.Error)
	}
	if evt.Type == EventSessionFailed || evt.Type == EventConnectionFailed {
		return evt, wrapRealtimeEventError(evt.Type, newAPIError(CodeServerError, "realtime session failed"))
	}

	return evt, nil
}

func (s *RealtimeSession) enqueueEvent(evt *RealtimeEvent) error {
	timeout := s.backpressureTimeout
	if timeout <= 0 {
		timeout = defaultRealtimeBackpressureTimeout
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case s.eventCh <- evt:
		return nil
	case <-s.closed:
		return nil
	case <-timer.C:
		return newAPIError(CodeServerError, "realtime event buffer full")
	}
}

func (s *RealtimeSession) pushErr(err error) {
	if err == nil {
		return
	}
	s.errOnce.Do(func() {
		select {
		case s.errCh <- err:
		case <-s.closed:
		}
	})
}

func (s *RealtimeSession) resetTurnFinalState() {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()

	s.turnFinalDelivered = false
	s.audioTurnInProgress = false
}

func (s *RealtimeSession) beginAudioTurn() {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()

	if !s.audioTurnInProgress || s.turnFinalDelivered {
		s.turnFinalDelivered = false
	}
	s.audioTurnInProgress = true
}

func (s *RealtimeSession) markFinalOnce(evt *RealtimeEvent) {
	if evt == nil {
		return
	}

	candidate := evt.IsFinal ||
		evt.Type == EventASREnded ||
		evt.Type == EventTTSFinished ||
		evt.Type == EventChatEnded ||
		evt.Type == EventSessionFinished
	if !candidate {
		return
	}

	s.turnMu.Lock()
	defer s.turnMu.Unlock()

	if s.turnFinalDelivered {
		evt.IsFinal = false
		return
	}

	evt.IsFinal = true
	s.turnFinalDelivered = true
	s.audioTurnInProgress = false
}

func (s *RealtimeSession) appendHistory(message RealtimeConversationMessage) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	s.history = append(s.history, message)
}

func (s *RealtimeSession) snapshotSessionState() ([]RealtimeConversationMessage, RealtimePromptConfig, RealtimeGenerationProps) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	return cloneConversationHistory(s.history), clonePromptConfig(s.prompt), cloneGenerationProps(s.props)
}

func normalizeRealtimeConfig(cfg *RealtimeConfig) (RealtimeConfig, error) {
	base := DefaultRealtimeConfig()
	if cfg != nil {
		base = *cfg
	}

	if base.TTS.AudioConfig.Format == "" {
		base.TTS.AudioConfig.Format = FormatPCM
	}
	if base.TTS.AudioConfig.SampleRate == 0 {
		base.TTS.AudioConfig.SampleRate = SampleRate16000
	}
	if base.TTS.AudioConfig.Channel == 0 {
		base.TTS.AudioConfig.Channel = 1
	}
	if base.TTS.AudioConfig.Bits == 0 {
		base.TTS.AudioConfig.Bits = 16
	}
	if base.ASR.Language == "" {
		base.ASR.Language = LanguageZhCN
	}
	if base.EventBuffer <= 0 {
		base.EventBuffer = defaultRealtimeEventBuffer
	}
	if base.BackpressureTimeout <= 0 {
		base.BackpressureTimeout = defaultRealtimeBackpressureTimeout
	}

	if strings.TrimSpace(base.TTS.Speaker) == "" {
		return base, newAPIError(CodeParamError, "tts.speaker is required")
	}
	if err := util.ValidateFormat(string(base.TTS.AudioConfig.Format)); err != nil {
		return base, newAPIError(CodeParamError, err.Error())
	}
	if err := util.ValidateSampleRate(int(base.TTS.AudioConfig.SampleRate)); err != nil {
		return base, newAPIError(CodeParamError, err.Error())
	}
	if err := util.ValidateChannel(base.TTS.AudioConfig.Channel); err != nil {
		return base, newAPIError(CodeParamError, err.Error())
	}
	if err := util.ValidateBits(base.TTS.AudioConfig.Bits); err != nil {
		return base, newAPIError(CodeParamError, err.Error())
	}

	base.History = cloneConversationHistory(base.History)
	base.Prompt = clonePromptConfig(base.Prompt)
	base.Props = cloneGenerationProps(base.Props)

	return base, nil
}

func buildRealtimeStartPayload(cfg RealtimeConfig) ([]byte, error) {
	payload := map[string]any{
		"asr": map[string]any{
			"language": cfg.ASR.Language,
		},
		"tts": map[string]any{
			"speaker": cfg.TTS.Speaker,
			"audio_config": map[string]any{
				"channel":     cfg.TTS.AudioConfig.Channel,
				"format":      cfg.TTS.AudioConfig.Format,
				"sample_rate": cfg.TTS.AudioConfig.SampleRate,
				"bits":        cfg.TTS.AudioConfig.Bits,
			},
		},
		"dialog": map[string]any{},
	}

	asr := payload["asr"].(map[string]any)
	for k, v := range cfg.ASR.Extra {
		asr[k] = v
	}

	tts := payload["tts"].(map[string]any)
	for k, v := range cfg.TTS.Extra {
		tts[k] = v
	}

	dialog := payload["dialog"].(map[string]any)
	if cfg.Dialog.BotName != "" {
		dialog["bot_name"] = cfg.Dialog.BotName
	}
	if cfg.Dialog.SystemRole != "" {
		dialog["system_role"] = cfg.Dialog.SystemRole
	}
	if cfg.Dialog.SpeakingStyle != "" {
		dialog["speaking_style"] = cfg.Dialog.SpeakingStyle
	}
	if cfg.Dialog.CharacterManifest != "" {
		dialog["character_manifest"] = cfg.Dialog.CharacterManifest
	}
	for k, v := range cfg.Dialog.Extra {
		dialog[k] = v
	}

	if cfg.Prompt.System != "" || len(cfg.Prompt.Variables) > 0 {
		payload["prompt"] = cfg.Prompt
	}
	if hasRealtimeProps(cfg.Props) {
		payload["props"] = cfg.Props
	}
	if len(cfg.History) > 0 {
		payload["history"] = cfg.History
	}

	return json.Marshal(payload)
}

func decodeEventPayload(evt *RealtimeEvent) {
	if evt == nil || len(evt.Payload) == 0 {
		return
	}

	var payload struct {
		ReqID     string `json:"reqid"`
		TraceID   string `json:"trace_id"`
		SessionID string `json:"session_id"`

		Text    string `json:"text"`
		Content string `json:"content"`
		Audio   string `json:"audio"`

		IsFinal bool `json:"is_final"`

		Code    int    `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`

		ASRInfo *struct {
			Text    string `json:"text"`
			IsFinal bool   `json:"is_final"`
		} `json:"asr_info,omitempty"`
		TTSInfo *struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		} `json:"tts_info,omitempty"`
	}

	if err := json.Unmarshal(evt.Payload, &payload); err != nil {
		return
	}

	if payload.SessionID != "" {
		evt.SessionID = payload.SessionID
	}
	if payload.ReqID != "" {
		evt.ReqID = payload.ReqID
	}
	if payload.TraceID != "" {
		evt.TraceID = payload.TraceID
	}

	if payload.Content != "" {
		evt.Text = payload.Content
	} else if payload.Text != "" {
		evt.Text = payload.Text
	}

	if payload.ASRInfo != nil {
		if payload.ASRInfo.Text != "" {
			evt.Text = payload.ASRInfo.Text
		}
		evt.IsFinal = evt.IsFinal || payload.ASRInfo.IsFinal
	}
	if payload.TTSInfo != nil {
		if payload.TTSInfo.Content != "" {
			evt.Text = payload.TTSInfo.Content
		} else if payload.TTSInfo.Text != "" {
			evt.Text = payload.TTSInfo.Text
		}
	}

	evt.IsFinal = evt.IsFinal || payload.IsFinal

	if evt.Audio == nil && payload.Audio != "" {
		decoded, err := base64.StdEncoding.DecodeString(payload.Audio)
		if err == nil {
			evt.Audio = decoded
		}
	}

	if payload.Code != 0 {
		message := payload.Message
		if message == "" {
			message = payload.Error
		}
		if message == "" {
			message = "realtime event error"
		}
		evt.Error = &Error{
			Code:    payload.Code,
			Message: message,
			ReqID:   payload.ReqID,
			TraceID: payload.TraceID,
		}
	}
}

func hasRealtimeProps(props RealtimeGenerationProps) bool {
	return props.Temperature != 0 ||
		props.TopP != 0 ||
		props.MaxTokens != 0 ||
		props.PresencePenalty != 0 ||
		props.FrequencyPenalty != 0 ||
		len(props.Extra) > 0
}

func cloneConversationHistory(history []RealtimeConversationMessage) []RealtimeConversationMessage {
	if len(history) == 0 {
		return nil
	}
	out := make([]RealtimeConversationMessage, len(history))
	copy(out, history)
	return out
}

func clonePromptConfig(prompt RealtimePromptConfig) RealtimePromptConfig {
	out := prompt
	if len(prompt.Variables) > 0 {
		out.Variables = make(map[string]string, len(prompt.Variables))
		for k, v := range prompt.Variables {
			out.Variables[k] = v
		}
	}
	return out
}

func cloneGenerationProps(props RealtimeGenerationProps) RealtimeGenerationProps {
	out := props
	if len(props.Extra) > 0 {
		out.Extra = make(map[string]any, len(props.Extra))
		for k, v := range props.Extra {
			out.Extra[k] = v
		}
	}
	return out
}

func readWSMessageWithContext(ctx context.Context, conn transport.WSConn) (int, []byte, error) {
	if ctx == nil {
		return conn.ReadMessage()
	}

	type readResult struct {
		msgType int
		payload []byte
		err     error
	}

	resultCh := make(chan readResult, 1)
	go func() {
		msgType, payload, err := conn.ReadMessage()
		resultCh <- readResult{msgType: msgType, payload: payload, err: err}
	}()

	select {
	case <-ctx.Done():
		_ = conn.Close()
		return 0, nil, ctx.Err()
	case result := <-resultCh:
		return result.msgType, result.payload, result.err
	}
}

func wrapRealtimeEventError(eventType RealtimeEventType, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("realtime event %d: %w", eventType, err)
}

func copyBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}
