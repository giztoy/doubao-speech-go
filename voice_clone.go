package doubaospeech

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	voiceCloneUploadPath   = "/api/v1/mega_tts/audio/upload"
	voiceCloneStatusPath   = "/api/v1/mega_tts/status"
	voiceCloneActivatePath = "/api/v1/mega_tts/audio/activate"

	voiceCloneDefaultSource = 2
	voiceCloneModelTypeICL2 = 4
)

// VoiceCloneService provides voice clone training and status operations.
type VoiceCloneService struct {
	client *Client
}

func newVoiceCloneService(c *Client) *VoiceCloneService {
	return &VoiceCloneService{client: c}
}

// VoiceCloneRequest is the request payload for voice clone upload task.
type VoiceCloneRequest struct {
	// VoiceID is a custom voice identifier.
	VoiceID string `json:"voice_id,omitempty" yaml:"voice_id,omitempty"`
	// SpeakerID is an alias of VoiceID for compatibility with official docs.
	SpeakerID string `json:"speaker_id,omitempty" yaml:"speaker_id,omitempty"`

	Audio []byte `json:"-" yaml:"-"`

	AudioFileName    string `json:"audio_file_name,omitempty" yaml:"audio_file_name,omitempty"`
	AudioContentType string `json:"audio_content_type,omitempty" yaml:"audio_content_type,omitempty"`
	AudioFormat      string `json:"audio_format,omitempty" yaml:"audio_format,omitempty"`

	Text      string `json:"text,omitempty" yaml:"text,omitempty"`
	Language  int    `json:"language,omitempty" yaml:"language,omitempty"`
	ModelType int    `json:"model_type,omitempty" yaml:"model_type,omitempty"`
	Source    int    `json:"source,omitempty" yaml:"source,omitempty"`

	ResourceID   string        `json:"resource_id,omitempty" yaml:"resource_id,omitempty"`
	PollInterval time.Duration `json:"-" yaml:"-"`
}

// VoiceCloneStatus is one status snapshot of clone training task.
type VoiceCloneStatus struct {
	TaskID    string `json:"task_id,omitempty"`
	SpeakerID string `json:"speaker_id,omitempty"`
	VoiceID   string `json:"voice_id,omitempty"`

	Status        TaskStatus `json:"status"`
	RawStatus     string     `json:"raw_status,omitempty"`
	RawStatusCode int        `json:"raw_status_code,omitempty"`

	StatusCode    int    `json:"status_code,omitempty"`
	StatusMessage string `json:"status_message,omitempty"`

	Version    string `json:"version,omitempty"`
	DemoAudio  string `json:"demo_audio,omitempty"`
	CreateTime int64  `json:"create_time,omitempty"`
}

// Submit uploads training audio and returns a task handle for polling.
func (s *VoiceCloneService) Submit(ctx context.Context, req *VoiceCloneRequest) (*Task[VoiceCloneStatus], error) {
	return s.Upload(ctx, req)
}

// Upload uploads training audio and returns a task handle for polling.
func (s *VoiceCloneService) Upload(ctx context.Context, req *VoiceCloneRequest) (*Task[VoiceCloneStatus], error) {
	normalized, err := normalizeVoiceCloneRequest(req)
	if err != nil {
		return nil, err
	}

	uploadAudio := voiceCloneUploadAudio{
		AudioBytes: base64.StdEncoding.EncodeToString(normalized.audio),
	}
	audioFormat := normalizeVoiceCloneAudioFormat(normalized.audioFormat, normalized.fileName)
	if audioFormat != "" {
		uploadAudio.AudioFormat = audioFormat
	}
	if normalized.text != "" {
		uploadAudio.Text = normalized.text
	}

	body := voiceCloneUploadBody{
		AppID:     s.client.config.appID,
		SpeakerID: normalized.voiceID,
		Audios:    []voiceCloneUploadAudio{uploadAudio},
		Source:    normalized.source,
	}
	if normalized.modelType > 0 {
		body.ModelType = normalized.modelType
	}
	if normalized.language > 0 {
		body.Language = normalized.language
	}

	resourceID := s.resolveResourceID(normalized.resourceID, normalized.modelType)

	var resp voiceCloneUploadResponse
	if err := s.client.doJSONRequest(
		ctx,
		http.MethodPost,
		voiceCloneUploadPath,
		body,
		&resp,
		resourceID,
	); err != nil {
		return nil, err
	}

	if err := resp.baseRespErr(); err != nil {
		return nil, err
	}

	speakerID := firstNonEmpty(strings.TrimSpace(resp.SpeakerID), strings.TrimSpace(resp.VoiceID), normalized.voiceID)
	if speakerID == "" {
		return nil, newAPIError(CodeServerError, "voice clone upload response missing speaker id")
	}
	taskID := firstNonEmpty(strings.TrimSpace(resp.TaskID), speakerID)

	task := NewTask(taskID, s.pollStatusBySpeaker(speakerID, resourceID), normalized.pollInterval)
	task.SetFailureMapper(func(status TaskStatus, result *VoiceCloneStatus) error {
		if result != nil {
			msg := strings.TrimSpace(result.StatusMessage)
			if msg != "" {
				code := result.StatusCode
				if code == 0 {
					code = CodeServerError
				}
				return &Error{Code: code, Message: msg}
			}
		}
		if status == TaskStatusCancelled {
			return newAPIError(CodeServerError, "voice clone task cancelled")
		}
		return newAPIError(CodeServerError, "voice clone task failed")
	})

	return task, nil
}

// GetStatus queries current voice clone task status.
func (s *VoiceCloneService) GetStatus(ctx context.Context, speakerOrVoiceID string) (*VoiceCloneStatus, error) {
	speakerID := strings.TrimSpace(speakerOrVoiceID)
	if speakerID == "" {
		return nil, newAPIError(CodeParamError, "speaker id is required")
	}
	return s.getStatusBySpeakerID(ctx, speakerID, s.resolveResourceID("", 0))
}

func (s *VoiceCloneService) getStatusBySpeakerID(ctx context.Context, speakerID string, resourceID string) (*VoiceCloneStatus, error) {
	body := map[string]any{
		"appid":      s.client.config.appID,
		"speaker_id": speakerID,
	}

	var resp voiceCloneStatusResponse
	if err := s.client.doJSONRequest(
		ctx,
		http.MethodPost,
		voiceCloneStatusPath,
		body,
		&resp,
		resourceID,
	); err != nil {
		return nil, err
	}

	if err := resp.baseRespErr(); err != nil {
		return nil, err
	}

	status, rawStatus, rawStatusCode := resp.resolveTaskStatus()
	result := &VoiceCloneStatus{
		TaskID:        strings.TrimSpace(resp.TaskID),
		SpeakerID:     firstNonEmpty(strings.TrimSpace(resp.SpeakerID), speakerID),
		VoiceID:       firstNonEmpty(strings.TrimSpace(resp.VoiceID), strings.TrimSpace(resp.SpeakerID), speakerID),
		Status:        status,
		RawStatus:     rawStatus,
		RawStatusCode: rawStatusCode,
		StatusCode:    firstNonZero(resp.StatusCode, resp.BaseResp.StatusCode),
		StatusMessage: firstNonEmpty(strings.TrimSpace(resp.StatusMessage), strings.TrimSpace(resp.Message), strings.TrimSpace(resp.BaseResp.StatusMessage)),
		Version:       strings.TrimSpace(resp.Version),
		DemoAudio:     strings.TrimSpace(resp.DemoAudio),
		CreateTime:    resp.CreateTime,
	}

	return result, nil
}

// Activate formalizes a trained cloned voice.
func (s *VoiceCloneService) Activate(ctx context.Context, voiceID string) error {
	voiceID = strings.TrimSpace(voiceID)
	if voiceID == "" {
		return newAPIError(CodeParamError, "voice id is required")
	}

	body := map[string]any{
		"appid":      s.client.config.appID,
		"speaker_id": voiceID,
	}

	var resp voiceCloneBaseResponse
	if err := s.client.doJSONRequest(
		ctx,
		http.MethodPost,
		voiceCloneActivatePath,
		body,
		&resp,
		s.resolveResourceID("", 0),
	); err != nil {
		return err
	}

	return resp.baseRespErr()
}

func (s *VoiceCloneService) pollStatusBySpeaker(speakerID string, resourceID string) TaskPoller[VoiceCloneStatus] {
	speakerID = strings.TrimSpace(speakerID)
	resourceID = strings.TrimSpace(resourceID)
	return func(ctx context.Context, _ string) (TaskStatus, *VoiceCloneStatus, error) {
		status, err := s.getStatusBySpeakerID(ctx, speakerID, resourceID)
		if err != nil {
			return "", nil, err
		}

		pollStatus := status.Status
		if strings.TrimSpace(string(pollStatus)) == "" {
			pollStatus = TaskStatus(status.RawStatus)
		}
		return pollStatus, status, nil
	}
}

func (s *VoiceCloneService) resolveResourceID(explicit string, modelType int) string {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return explicit
	}

	configured := strings.TrimSpace(s.client.config.resourceID)
	if configured != "" {
		return configured
	}

	return defaultVoiceCloneResourceIDForModel(modelType)
}

func defaultVoiceCloneResourceIDForModel(modelType int) string {
	if modelType == voiceCloneModelTypeICL2 {
		return ResourceVoiceCloneV2
	}

	return ResourceVoiceCloneV1
}

type voiceCloneUploadBody struct {
	AppID     string                  `json:"appid"`
	SpeakerID string                  `json:"speaker_id"`
	Audios    []voiceCloneUploadAudio `json:"audios"`
	Source    int                     `json:"source"`
	Language  int                     `json:"language,omitempty"`
	ModelType int                     `json:"model_type,omitempty"`
}

type voiceCloneUploadAudio struct {
	AudioBytes  string `json:"audio_bytes"`
	AudioFormat string `json:"audio_format,omitempty"`
	Text        string `json:"text,omitempty"`
}

type normalizedVoiceCloneRequest struct {
	voiceID     string
	audio       []byte
	fileName    string
	audioFormat string

	text      string
	language  int
	modelType int
	source    int

	resourceID   string
	pollInterval time.Duration
}

func normalizeVoiceCloneRequest(req *VoiceCloneRequest) (*normalizedVoiceCloneRequest, error) {
	if req == nil {
		return nil, newAPIError(CodeParamError, "request is nil")
	}

	voiceID := firstNonEmpty(strings.TrimSpace(req.VoiceID), strings.TrimSpace(req.SpeakerID))
	if voiceID == "" {
		return nil, newAPIError(CodeParamError, "voice_id is required")
	}

	if len(req.Audio) == 0 {
		return nil, newAPIError(CodeParamError, "audio is required")
	}

	audioCopy := make([]byte, len(req.Audio))
	copy(audioCopy, req.Audio)

	fileName := strings.TrimSpace(req.AudioFileName)
	audioFormat := strings.ToLower(strings.TrimSpace(req.AudioFormat))
	if fileName == "" {
		ext := ""
		if audioFormat != "" {
			ext = "." + audioFormat
		}
		if ext == "" {
			ext = ".wav"
		}
		fileName = voiceID + ext
	}

	pollInterval := req.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultTaskPollInterval
	}

	source := req.Source
	if source == 0 {
		source = voiceCloneDefaultSource
	}

	return &normalizedVoiceCloneRequest{
		voiceID:     voiceID,
		audio:       audioCopy,
		fileName:    fileName,
		audioFormat: audioFormat,
		text:        strings.TrimSpace(req.Text),
		language:    req.Language,
		modelType:   req.ModelType,
		source:      source,
		resourceID:  strings.TrimSpace(req.ResourceID),

		pollInterval: pollInterval,
	}, nil
}

func normalizeVoiceCloneAudioFormat(explicit string, fileName string) string {
	format := strings.ToLower(strings.TrimSpace(explicit))
	if format != "" {
		return format
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(strings.TrimSpace(fileName)), "."))
	switch ext {
	case "wav", "mp3", "ogg", "m4a", "aac", "pcm":
		return ext
	default:
		return ""
	}
}

type voiceCloneBaseResponse struct {
	BaseResp voiceCloneBaseResp `json:"BaseResp"`
}

type voiceCloneBaseResp struct {
	StatusCode    int    `json:"StatusCode"`
	StatusMessage string `json:"StatusMessage"`
}

func (r *voiceCloneBaseResponse) baseRespErr() error {
	if r == nil {
		return nil
	}
	if r.BaseResp.StatusCode == 0 {
		return nil
	}
	msg := strings.TrimSpace(r.BaseResp.StatusMessage)
	if msg == "" {
		msg = "voice clone request failed"
	}
	return &Error{Code: r.BaseResp.StatusCode, Message: msg}
}

type voiceCloneUploadResponse struct {
	voiceCloneBaseResponse

	TaskID    string `json:"task_id"`
	SpeakerID string `json:"speaker_id"`
	VoiceID   string `json:"voice_id"`
}

type voiceCloneStatusResponse struct {
	voiceCloneBaseResponse

	TaskID    string `json:"task_id"`
	SpeakerID string `json:"speaker_id"`
	VoiceID   string `json:"voice_id"`

	Status     json.RawMessage `json:"status"`
	TaskStatus json.RawMessage `json:"task_status"`

	StatusCode    int    `json:"status_code"`
	StatusMessage string `json:"status_message"`
	Message       string `json:"message"`

	Version    string `json:"version"`
	DemoAudio  string `json:"demo_audio"`
	CreateTime int64  `json:"create_time"`
}

func (r *voiceCloneStatusResponse) resolveTaskStatus() (TaskStatus, string, int) {
	primaryStatus, primaryRawText, primaryRawCode := decodeTaskStatusRaw(r.Status)
	if primaryStatus != "" {
		return primaryStatus, primaryRawText, primaryRawCode
	}

	secondaryStatus, secondaryRawText, secondaryRawCode := decodeTaskStatusRaw(r.TaskStatus)
	if secondaryStatus != "" {
		return secondaryStatus, secondaryRawText, secondaryRawCode
	}

	if primaryRawText != "" {
		return primaryStatus, primaryRawText, primaryRawCode
	}

	return secondaryStatus, secondaryRawText, secondaryRawCode
}

func decodeTaskStatusRaw(raw json.RawMessage) (TaskStatus, string, int) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", "", 0
	}

	var intValue int
	if err := json.Unmarshal(raw, &intValue); err == nil {
		return mapTaskStatusCode(intValue), strconv.Itoa(intValue), intValue
	}

	var strValue string
	if err := json.Unmarshal(raw, &strValue); err == nil {
		normalized := normalizeTaskStatus(TaskStatus(strValue))
		if normalized == "" {
			if code, convErr := strconv.Atoi(strings.TrimSpace(strValue)); convErr == nil {
				return mapTaskStatusCode(code), strValue, code
			}
		}
		return normalized, strValue, 0
	}

	return "", trimmed, 0
}

func firstNonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
