package doubaospeech

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"net/http"
	"strings"

	"github.com/GizClaw/doubao-speech-go/internal/auth"
	"github.com/GizClaw/doubao-speech-go/internal/util"
)

const (
	ttsV2HTTPStreamPath = "/api/v3/tts/unidirectional"
	ttsV2CodeStreamDone = 20000000
)

// TTSV2Request represents a TTS V2 stream request.
type TTSV2Request struct {
	Text    string `json:"text" yaml:"text"`
	Speaker string `json:"speaker" yaml:"speaker"`

	Format     AudioFormat `json:"format,omitempty" yaml:"format,omitempty"`
	SampleRate SampleRate  `json:"sample_rate,omitempty" yaml:"sample_rate,omitempty"`
	BitRate    int         `json:"bit_rate,omitempty" yaml:"bit_rate,omitempty"`

	SpeechRate int `json:"speech_rate,omitempty" yaml:"speech_rate,omitempty"`
	PitchRate  int `json:"pitch_rate,omitempty" yaml:"pitch_rate,omitempty"`
	VolumeRate int `json:"volume_rate,omitempty" yaml:"volume_rate,omitempty"`

	Emotion  string `json:"emotion,omitempty" yaml:"emotion,omitempty"`
	Language string `json:"language,omitempty" yaml:"language,omitempty"`

	ResourceID string           `json:"resource_id,omitempty" yaml:"resource_id,omitempty"`
	MixSpeaker *TTSV2MixSpeaker `json:"mix_speaker,omitempty" yaml:"mix_speaker,omitempty"`
}

// TTSV2MixSpeaker represents mixed-speaker parameters.
type TTSV2MixSpeaker struct {
	Speakers []TTSV2MixSpeakerSource `json:"speakers,omitempty" yaml:"speakers,omitempty"`
}

// TTSV2MixSpeakerSource is one source speaker in a mixed-speaker request.
type TTSV2MixSpeakerSource struct {
	SourceSpeaker string  `json:"source_speaker" yaml:"source_speaker"`
	MixFactor     float64 `json:"mix_factor" yaml:"mix_factor"`
}

// TTSV2Chunk is one stream chunk from TTS V2 HTTP streaming API.
type TTSV2Chunk struct {
	Audio   []byte `json:"-"`
	IsLast  bool   `json:"is_last"`
	ReqID   string `json:"reqid,omitempty"`
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

// Stream synthesizes speech with TTS V2 HTTP streaming endpoint.
func (s *TTSServiceV2) Stream(ctx context.Context, req *TTSV2Request) iter.Seq2[*TTSV2Chunk, error] {
	return func(yield func(*TTSV2Chunk, error) bool) {
		normalized, err := normalizeTTSV2Request(req)
		if err != nil {
			yield(nil, err)
			return
		}

		requestPayload := s.buildStreamRequestBody(normalized)
		bodyBytes, err := json.Marshal(requestPayload)
		if err != nil {
			yield(nil, wrapError(err, "marshal tts stream request"))
			return
		}

		endpoint := strings.TrimRight(s.client.config.baseURL, "/") + ttsV2HTTPStreamPath
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			yield(nil, wrapError(err, "create tts stream request"))
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resourceID := s.client.resolveResourceID(normalized.ResourceID, ResourceTTSV2)
		auth.ApplyV2Headers(httpReq, s.client.authCredentials(), resourceID)

		if isNilHTTPDoer(s.client.config.httpClient) {
			yield(nil, newAPIError(CodeServerError, "http transport is nil"))
			return
		}

		resp, err := s.client.config.httpClient.Do(httpReq)
		if err != nil {
			yield(nil, wrapError(err, "send tts stream request"))
			return
		}
		defer resp.Body.Close()

		logID := resp.Header.Get("X-Tt-Logid")
		if logID == "" {
			logID = resp.Header.Get("X-Tt-LogId")
		}

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			errorBody, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				yield(nil, wrapError(readErr, "read tts stream error response"))
				return
			}
			yield(nil, parseAPIError(resp.StatusCode, errorBody, logID))
			return
		}

		if err := parseTTSV2HTTPStream(resp.Body, yield); err != nil {
			yield(nil, err)
		}
	}
}

func (s *TTSServiceV2) buildStreamRequestBody(req *TTSV2Request) ttsV2HTTPStreamRequest {
	requestBody := ttsV2HTTPStreamRequest{
		User: ttsV2RequestUser{UID: s.client.config.userID},
		ReqParams: ttsV2RequestParams{
			Text:    req.Text,
			Speaker: req.Speaker,
			AudioParams: ttsV2AudioParams{
				Format:     string(req.Format),
				SampleRate: int(req.SampleRate),
				BitRate:    req.BitRate,
				SpeechRate: req.SpeechRate,
				PitchRate:  req.PitchRate,
				VolumeRate: req.VolumeRate,
				Emotion:    req.Emotion,
				Language:   req.Language,
			},
			MixSpeaker: req.MixSpeaker,
		},
	}

	return requestBody
}

func normalizeTTSV2Request(req *TTSV2Request) (*TTSV2Request, error) {
	if req == nil {
		return nil, newAPIError(CodeParamError, "request is nil")
	}

	normalized := *req
	normalized.Text = strings.TrimSpace(normalized.Text)
	if normalized.Text == "" {
		return nil, newAPIError(CodeParamError, "text is required")
	}

	normalized.Speaker = strings.TrimSpace(normalized.Speaker)
	if normalized.Speaker == "" {
		return nil, newAPIError(CodeParamError, "speaker is required")
	}

	if normalized.Format == "" {
		normalized.Format = FormatMP3
	}
	if err := util.ValidateFormat(string(normalized.Format)); err != nil {
		return nil, newAPIError(CodeParamError, err.Error())
	}

	if normalized.SampleRate == 0 {
		normalized.SampleRate = SampleRate24000
	}
	if err := util.ValidateSampleRate(int(normalized.SampleRate)); err != nil {
		return nil, newAPIError(CodeParamError, err.Error())
	}

	normalized.ResourceID = strings.TrimSpace(normalized.ResourceID)

	return &normalized, nil
}

func parseTTSV2HTTPStream(body io.Reader, yield func(*TTSV2Chunk, error) bool) error {
	reader := bufio.NewReader(body)
	seenFinal := false
	lastReqID := ""

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			chunk, isDone, reqID, parseErr := parseTTSV2HTTPStreamLine(line)
			if reqID != "" {
				lastReqID = reqID
			}
			if parseErr != nil {
				return parseErr
			}
			if chunk != nil {
				if !yield(chunk, nil) {
					return nil
				}
			}
			if isDone {
				seenFinal = true
				return nil
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				if seenFinal {
					return nil
				}
				return &Error{Code: CodeServerError, Message: "tts stream ended before final frame", ReqID: lastReqID}
			}
			return wrapError(err, "read tts stream line")
		}
	}
}

func parseTTSV2HTTPStreamLine(line []byte) (*TTSV2Chunk, bool, string, error) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return nil, false, "", nil
	}

	var payload ttsV2HTTPStreamLine
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, false, "", wrapError(err, "unmarshal tts stream line")
	}

	if payload.Code != 0 && payload.Code != ttsV2CodeStreamDone {
		msg := strings.TrimSpace(payload.Message)
		if msg == "" {
			msg = "tts stream request failed"
		}
		return nil, true, payload.ReqID, &Error{Code: payload.Code, Message: msg, ReqID: payload.ReqID}
	}

	audio, hasAudio, err := decodeTTSV2AudioData(payload.Data)
	if err != nil {
		return nil, false, payload.ReqID, err
	}

	isLast := payload.Done || payload.Code == ttsV2CodeStreamDone
	if !hasAudio && !isLast {
		return nil, false, payload.ReqID, nil
	}

	chunk := &TTSV2Chunk{
		Audio:   audio,
		IsLast:  isLast,
		ReqID:   payload.ReqID,
		Code:    payload.Code,
		Message: payload.Message,
	}

	return chunk, isLast, payload.ReqID, nil
}

func decodeTTSV2AudioData(raw json.RawMessage) ([]byte, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, false, nil
	}

	var encoded string
	if err := json.Unmarshal(trimmed, &encoded); err != nil {
		return nil, false, wrapError(err, "decode tts stream data")
	}

	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, false, nil
	}

	audio, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, false, wrapError(err, "decode tts stream audio")
	}

	return audio, true, nil
}

type ttsV2HTTPStreamRequest struct {
	User      ttsV2RequestUser   `json:"user"`
	ReqParams ttsV2RequestParams `json:"req_params"`
}

type ttsV2RequestUser struct {
	UID string `json:"uid"`
}

type ttsV2RequestParams struct {
	Text        string           `json:"text"`
	Speaker     string           `json:"speaker"`
	AudioParams ttsV2AudioParams `json:"audio_params"`
	MixSpeaker  *TTSV2MixSpeaker `json:"mix_speaker,omitempty"`
}

type ttsV2AudioParams struct {
	Format     string `json:"format,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
	BitRate    int    `json:"bit_rate,omitempty"`
	SpeechRate int    `json:"speech_rate,omitempty"`
	PitchRate  int    `json:"pitch_rate,omitempty"`
	VolumeRate int    `json:"volume_rate,omitempty"`
	Emotion    string `json:"emotion,omitempty"`
	Language   string `json:"language,omitempty"`
}

type ttsV2HTTPStreamLine struct {
	ReqID   string          `json:"reqid"`
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Done    bool            `json:"done"`
	Data    json.RawMessage `json:"data"`
}
