package doubaospeech

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/giztoy/doubao-speech-go/internal/auth"
	"github.com/giztoy/doubao-speech-go/internal/protocol"
	"github.com/giztoy/doubao-speech-go/internal/transport"
	"github.com/giztoy/doubao-speech-go/internal/util"
	"github.com/gorilla/websocket"
)

// ASRServiceV2 provides SAUC WebSocket streaming recognition.
type ASRServiceV2 struct {
	client *Client
	dialer transport.WSDialer
}

func newASRServiceV2(c *Client) *ASRServiceV2 {
	return &ASRServiceV2{
		client: c,
		dialer: transport.NewGorillaDialer(nil),
	}
}

// ASRV2Session represents one streaming recognition session.
type ASRV2Session struct {
	conn   transport.WSConn
	client *Client
	cfg    ASRV2Config
	reqID  string

	resultCh chan *ASRV2Result
	errCh    chan error
	closed   chan struct{}
	recvDone chan struct{}

	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

// OpenStreamSession opens a SAUC V2 WebSocket session.
func (s *ASRServiceV2) OpenStreamSession(ctx context.Context, cfg *ASRV2Config) (*ASRV2Session, error) {
	if cfg == nil {
		return nil, newAPIError(CodeParamError, "config is nil")
	}

	normalized, err := normalizeASRV2Config(*cfg)
	if err != nil {
		return nil, err
	}

	resourceID := s.client.resolveResourceID(normalized.ResourceID, ResourceASRStreamV2)
	connectID := util.NewReqID("asr")

	endpoint := strings.TrimRight(s.client.config.wsURL, "/") + "/api/v3/sauc/bigmodel"
	headers := auth.BuildV2WSHeaders(s.client.authCredentials(), resourceID, connectID)

	conn, resp, err := s.dialer.DialContext(ctx, endpoint, headers)
	if err != nil {
		return nil, wsConnectError(err, resp)
	}

	session := &ASRV2Session{
		conn:     conn,
		client:   s.client,
		cfg:      normalized,
		reqID:    connectID,
		resultCh: make(chan *ASRV2Result, 64),
		errCh:    make(chan error, 1),
		closed:   make(chan struct{}),
		recvDone: make(chan struct{}),
	}

	go session.receiveLoop()

	if err := session.sendSessionStart(ctx); err != nil {
		_ = session.Close()
		return nil, wrapError(err, "send session start")
	}

	return session, nil
}

// SendAudio sends one audio chunk.
//
// isLast=true marks the last frame (flags=2).
func (s *ASRV2Session) SendAudio(ctx context.Context, audio []byte, isLast bool) error {
	if len(audio) == 0 && !isLast {
		return newAPIError(CodeParamError, "audio payload is empty")
	}
	if err := s.guardContext(ctx); err != nil {
		return err
	}

	packet, err := protocol.BuildAudioOnly(audio, isLast)
	if err != nil {
		return wrapError(err, "encode audio frame")
	}

	return s.writeBinary(packet)
}

// Recv yields recognition results as a stream.
func (s *ASRV2Session) Recv() iter.Seq2[*ASRV2Result, error] {
	return func(yield func(*ASRV2Result, error) bool) {
		results := s.resultCh
		errs := s.errCh

		for results != nil || errs != nil {
			select {
			case r, ok := <-results:
				if !ok {
					results = nil
					continue
				}
				if !yield(r, nil) {
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
func (s *ASRV2Session) Close() error {
	s.closeOnce.Do(func() {
		// Best-effort finish event; failures should not block close.
		_ = s.sendSessionFinish(context.Background())

		close(s.closed)
		s.closeErr = s.conn.Close()

		select {
		case <-s.recvDone:
		case <-time.After(2 * time.Second):
		}
	})

	return s.closeErr
}

func (s *ASRV2Session) sendSessionStart(ctx context.Context) error {
	req := map[string]any{
		"user": map[string]any{
			"uid": s.client.config.userID,
		},
		"audio": map[string]any{
			"format":      s.cfg.Format,
			"sample_rate": s.cfg.SampleRate,
			"channel":     resolvedChannel(s.cfg),
			"bits":        resolvedBits(s.cfg),
		},
		"request": map[string]any{
			"reqid":           s.reqID,
			"sequence":        1,
			"show_utterances": true,
			"result_type":     normalizedResultType(s.cfg.ResultType),
		},
	}

	requestObj := req["request"].(map[string]any)
	if s.cfg.Language != "" {
		requestObj["language"] = s.cfg.Language
	}
	if s.cfg.EnableITN {
		requestObj["enable_itn"] = true
	}
	if s.cfg.EnablePunc {
		requestObj["enable_punc"] = true
	}
	if s.cfg.EnableDiarization {
		requestObj["enable_diarization"] = true
	}
	if s.cfg.SpeakerNum > 0 {
		requestObj["speaker_num"] = s.cfg.SpeakerNum
	}
	if len(s.cfg.Hotwords) > 0 {
		requestObj["hotwords"] = s.cfg.Hotwords
	}

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return wrapError(err, "marshal start payload")
	}

	packet, err := protocol.BuildFullClientJSON(jsonBody)
	if err != nil {
		return wrapError(err, "encode start frame")
	}

	if err := s.guardContext(ctx); err != nil {
		return err
	}
	return s.writeBinary(packet)
}

func (s *ASRV2Session) sendSessionFinish(ctx context.Context) error {
	finishBody, err := json.Marshal(map[string]any{
		"event": 2,
		"reqid": s.reqID,
	})
	if err != nil {
		return wrapError(err, "marshal finish payload")
	}

	packet, err := protocol.BuildFullClientJSON(finishBody)
	if err != nil {
		return wrapError(err, "encode finish frame")
	}

	if err := s.guardContext(ctx); err != nil {
		return err
	}
	return s.writeBinary(packet)
}

func (s *ASRV2Session) writeBinary(packet []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if s.isClosed() {
		return newAPIError(CodeServerError, "session already closed")
	}

	return s.conn.WriteMessage(websocket.BinaryMessage, packet)
}

func (s *ASRV2Session) receiveLoop() {
	defer close(s.recvDone)
	defer close(s.resultCh)
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
			frame, err := protocol.ParseServerFrame(payload)
			if err != nil {
				s.pushErr(wrapError(err, "parse server frame"))
				continue
			}

			switch frame.MessageType {
			case protocol.MessageTypeFullServer:
				result, err := decodeASRV2Result(frame, s.reqID)
				if err != nil {
					s.pushErr(err)
					continue
				}
				if result == nil {
					continue
				}
				select {
				case s.resultCh <- result:
				case <-s.closed:
					return
				}
			case protocol.MessageTypeError:
				s.pushErr(parseWSErrorPayload(frame.Payload, frame.ErrorCode))
				return
			default:
				// Ignore other frame types in this migration scope.
			}
		default:
			// Ignore unknown message types.
		}
	}
}

func decodeASRV2Result(frame *protocol.ParsedFrame, fallbackReqID string) (*ASRV2Result, error) {
	var payload struct {
		ReqID     string `json:"reqid"`
		Code      int    `json:"code"`
		Message   string `json:"message"`
		AudioInfo struct {
			Duration int `json:"duration"`
		} `json:"audio_info"`
		Result struct {
			Text       string `json:"text"`
			Utterances []struct {
				Text       string  `json:"text"`
				StartTime  int     `json:"start_time"`
				EndTime    int     `json:"end_time"`
				Definite   bool    `json:"definite"`
				SpeakerID  string  `json:"speaker_id"`
				Confidence float64 `json:"confidence"`
				Words      []struct {
					Text      string  `json:"text"`
					StartTime int     `json:"start_time"`
					EndTime   int     `json:"end_time"`
					Conf      float64 `json:"conf"`
				} `json:"words"`
			} `json:"utterances"`
		} `json:"result"`
	}

	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, wrapError(err, "unmarshal asr result")
	}

	if payload.Code != 0 && payload.Code != CodeSuccess && payload.Code != CodeASRSuccess {
		return nil, &Error{Code: payload.Code, Message: payload.Message, ReqID: payload.ReqID}
	}

	if payload.Result.Text == "" && len(payload.Result.Utterances) == 0 {
		// This can be an intermediate control frame.
		return nil, nil
	}

	utterances := make([]ASRV2Utterance, 0, len(payload.Result.Utterances))
	isFinal := frame.Flags == protocol.FlagNegativeSequence || frame.Flags == protocol.FlagNegativeWithSeq
	for _, u := range payload.Result.Utterances {
		words := make([]ASRV2Word, 0, len(u.Words))
		for _, w := range u.Words {
			words = append(words, ASRV2Word{
				Text:      w.Text,
				StartTime: w.StartTime,
				EndTime:   w.EndTime,
				Conf:      w.Conf,
			})
		}

		utterances = append(utterances, ASRV2Utterance{
			Text:       u.Text,
			StartTime:  u.StartTime,
			EndTime:    u.EndTime,
			Definite:   u.Definite,
			SpeakerID:  u.SpeakerID,
			Confidence: u.Confidence,
			Words:      words,
		})

		if u.Definite {
			isFinal = true
		}
	}

	reqID := payload.ReqID
	if reqID == "" {
		reqID = fallbackReqID
	}

	return &ASRV2Result{
		Text:       payload.Result.Text,
		Utterances: utterances,
		Duration:   payload.AudioInfo.Duration,
		IsFinal:    isFinal,
		ReqID:      reqID,
	}, nil
}

func parseWSErrorPayload(payload []byte, fallbackCode uint32) error {
	var e struct {
		Code       int    `json:"code"`
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
		ReqID      string `json:"reqid"`
		Error      string `json:"error"`
	}
	if err := json.Unmarshal(payload, &e); err != nil {
		msg := string(payload)
		if msg == "" {
			msg = "unknown websocket error"
		}
		code := int(fallbackCode)
		if code == 0 {
			code = CodeServerError
		}
		return &Error{Code: code, Message: msg}
	}

	msg := e.Message
	if msg == "" {
		msg = e.Error
	}
	if msg == "" {
		msg = "websocket error"
	}

	code := e.Code
	if code == 0 {
		code = e.StatusCode
	}
	if code == 0 {
		code = int(fallbackCode)
	}
	if code == 0 {
		code = CodeServerError
	}

	return &Error{Code: code, Message: msg, ReqID: e.ReqID}
}

func normalizeASRV2Config(cfg ASRV2Config) (ASRV2Config, error) {
	if cfg.Format == "" {
		cfg.Format = FormatPCM
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = SampleRate16000
	}
	if cfg.Channel == 0 && cfg.Channels == 0 {
		cfg.Channel = 1
	}
	if cfg.Bits == 0 {
		cfg.Bits = 16
	}

	if err := util.ValidateFormat(string(cfg.Format)); err != nil {
		return cfg, newAPIError(CodeParamError, err.Error())
	}
	if err := util.ValidateSampleRate(int(cfg.SampleRate)); err != nil {
		return cfg, newAPIError(CodeParamError, err.Error())
	}
	if err := util.ValidateChannel(resolvedChannel(cfg)); err != nil {
		return cfg, newAPIError(CodeParamError, err.Error())
	}
	if err := util.ValidateBits(resolvedBits(cfg)); err != nil {
		return cfg, newAPIError(CodeParamError, err.Error())
	}
	if err := util.ValidateResultType(cfg.ResultType); err != nil {
		return cfg, newAPIError(CodeParamError, err.Error())
	}

	return cfg, nil
}

func normalizedResultType(rt string) string {
	v := strings.ToLower(strings.TrimSpace(rt))
	if v == "full" {
		return "full"
	}
	return "single"
}

func resolvedChannel(cfg ASRV2Config) int {
	if cfg.Channel > 0 {
		return cfg.Channel
	}
	if cfg.Channels > 0 {
		return cfg.Channels
	}
	return 1
}

func resolvedBits(cfg ASRV2Config) int {
	if cfg.Bits > 0 {
		return cfg.Bits
	}
	return 16
}

func (s *ASRV2Session) guardContext(ctx context.Context) error {
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

func (s *ASRV2Session) isClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *ASRV2Session) pushErr(err error) {
	if err == nil {
		return
	}
	select {
	case s.errCh <- err:
	default:
	}
}

func wsConnectError(baseErr error, resp *http.Response) error {
	if resp == nil || resp.Body == nil {
		return wrapError(baseErr, "websocket connect failed")
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	logID := resp.Header.Get("X-Tt-Logid")
	if logID == "" {
		logID = resp.Header.Get("X-Tt-LogId")
	}

	if readErr == nil && len(body) > 0 {
		if parsed := parseAPIError(resp.StatusCode, body, logID); parsed != nil {
			return wrapError(parsed, "websocket connect failed")
		}
		return fmt.Errorf("websocket connect failed: %w (status=%s, body=%s)", baseErr, resp.Status, string(body))
	}

	return fmt.Errorf("websocket connect failed: %w (status=%s)", baseErr, resp.Status)
}
