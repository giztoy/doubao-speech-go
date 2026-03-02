package doubaospeech

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestVoiceCloneUploadValidation(t *testing.T) {
	client := NewClient("app-test")

	tests := []struct {
		name string
		req  *VoiceCloneRequest
	}{
		{name: "nil request", req: nil},
		{name: "empty voice id", req: &VoiceCloneRequest{Audio: []byte("a")}},
		{name: "empty audio", req: &VoiceCloneRequest{VoiceID: "voice-1"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.VoiceClone.Upload(context.Background(), tc.req)
			if err == nil {
				t.Fatalf("expected validation error")
			}

			apiErr, ok := AsError(err)
			if !ok {
				t.Fatalf("want *Error, got %T (%v)", err, err)
			}
			if apiErr.Code != CodeParamError {
				t.Fatalf("error code = %d, want %d", apiErr.Code, CodeParamError)
			}
		})
	}
}

func TestVoiceCloneUploadSubmitFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != voiceCloneUploadPath {
			http.NotFound(w, r)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"BaseResp": map[string]any{
				"StatusCode":    1101,
				"StatusMessage": "audio upload failed",
			},
		})
	}))
	defer server.Close()

	client := NewClient(
		"app-test",
		WithBearerToken("token-test"),
		WithBaseURL(server.URL),
	)

	_, err := client.VoiceClone.Upload(context.Background(), &VoiceCloneRequest{
		VoiceID: "voice-a",
		Audio:   []byte("audio-data"),
	})
	if err == nil {
		t.Fatalf("expected submit failure error")
	}

	apiErr, ok := AsError(err)
	if !ok {
		t.Fatalf("want *Error, got %T (%v)", err, err)
	}
	if apiErr.Code != 1101 {
		t.Fatalf("error code = %d, want 1101", apiErr.Code)
	}
	if !strings.Contains(apiErr.Message, "audio upload failed") {
		t.Fatalf("error message = %q, want contains %q", apiErr.Message, "audio upload failed")
	}
}

func TestVoiceCloneUploadAndWaitSuccess(t *testing.T) {
	var statusCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case voiceCloneUploadPath:
			if got := r.Header.Get("Authorization"); got != "Bearer;token-test" {
				t.Fatalf("Authorization = %q, want %q", got, "Bearer;token-test")
			}
			if got := r.Header.Get("Resource-Id"); got != ResourceVoiceCloneV1 {
				t.Fatalf("Resource-Id = %q, want %q", got, ResourceVoiceCloneV1)
			}
			if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
				t.Fatalf("Content-Type = %q, want contains %q", got, "application/json")
			}

			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode upload request error = %v", err)
			}
			if got := body["appid"]; got != "app-test" {
				t.Fatalf("appid = %v, want %q", got, "app-test")
			}
			if got := body["speaker_id"]; got != "voice-a" {
				t.Fatalf("speaker_id = %v, want %q", got, "voice-a")
			}

			audios, ok := body["audios"].([]any)
			if !ok || len(audios) != 1 {
				t.Fatalf("audios = %v, want one item", body["audios"])
			}
			audioItem, ok := audios[0].(map[string]any)
			if !ok {
				t.Fatalf("audio item = %T, want map", audios[0])
			}
			audioBytes, ok := audioItem["audio_bytes"].(string)
			if !ok || strings.TrimSpace(audioBytes) == "" {
				t.Fatalf("audio_bytes = %v, want non-empty string", audioItem["audio_bytes"])
			}
			decoded, err := base64.StdEncoding.DecodeString(audioBytes)
			if err != nil {
				t.Fatalf("DecodeString(audio_bytes) error = %v", err)
			}
			if string(decoded) != "audio-data" {
				t.Fatalf("decoded audio = %q, want %q", string(decoded), "audio-data")
			}
			if got := audioItem["audio_format"]; got != "wav" {
				t.Fatalf("audio_format = %v, want %q", got, "wav")
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"BaseResp":   map[string]any{"StatusCode": 0, "StatusMessage": ""},
				"speaker_id": "voice-a",
			})

		case voiceCloneStatusPath:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode status request error = %v", err)
			}
			if got := body["speaker_id"]; got != "voice-a" {
				t.Fatalf("speaker_id request = %v, want %q", got, "voice-a")
			}

			call := atomic.AddInt32(&statusCalls, 1)
			if call == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"BaseResp":   map[string]any{"StatusCode": 0, "StatusMessage": ""},
					"speaker_id": "voice-a",
					"status":     1,
				})
				return
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"BaseResp":       map[string]any{"StatusCode": 0, "StatusMessage": ""},
				"speaker_id":     "voice-a",
				"status":         2,
				"version":        "V1",
				"demo_audio":     "https://example.com/demo.wav",
				"status_message": "ok",
			})

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(
		"app-test",
		WithBearerToken("token-test"),
		WithBaseURL(server.URL),
	)

	task, err := client.VoiceClone.Upload(context.Background(), &VoiceCloneRequest{
		VoiceID:      "voice-a",
		Audio:        []byte("audio-data"),
		PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Upload error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	status, err := task.Wait(ctx)
	if err != nil {
		t.Fatalf("task.Wait error = %v", err)
	}
	if status == nil {
		t.Fatalf("status is nil")
	}
	if status.Status != TaskStatusSuccess {
		t.Fatalf("status = %q, want %q", status.Status, TaskStatusSuccess)
	}
	if status.DemoAudio != "https://example.com/demo.wav" {
		t.Fatalf("demo_audio = %q, want %q", status.DemoAudio, "https://example.com/demo.wav")
	}
	if atomic.LoadInt32(&statusCalls) < 2 {
		t.Fatalf("status poll calls = %d, want >= 2", atomic.LoadInt32(&statusCalls))
	}
}

func TestVoiceCloneUploadAndWaitWithDifferentTaskAndSpeakerID(t *testing.T) {
	var statusCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case voiceCloneUploadPath:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode upload request error = %v", err)
			}
			if got := body["speaker_id"]; got != "voice-a" {
				t.Fatalf("speaker_id = %v, want %q", got, "voice-a")
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"BaseResp":   map[string]any{"StatusCode": 0, "StatusMessage": ""},
				"task_id":    "task-1",
				"speaker_id": "voice-a",
			})

		case voiceCloneStatusPath:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode status request error = %v", err)
			}
			if got := body["speaker_id"]; got != "voice-a" {
				t.Fatalf("speaker_id request = %v, want %q", got, "voice-a")
			}
			if _, ok := body["task_id"]; ok {
				t.Fatalf("task_id should not be sent in status request: %v", body["task_id"])
			}
			if _, ok := body["voice_id"]; ok {
				t.Fatalf("voice_id should not be sent in status request: %v", body["voice_id"])
			}

			call := atomic.AddInt32(&statusCalls, 1)
			if call == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"BaseResp":   map[string]any{"StatusCode": 0, "StatusMessage": ""},
					"task_id":    "task-1",
					"speaker_id": "voice-a",
					"status":     1,
				})
				return
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"BaseResp":   map[string]any{"StatusCode": 0, "StatusMessage": ""},
				"task_id":    "task-1",
				"speaker_id": "voice-a",
				"status":     2,
			})

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(
		"app-test",
		WithBearerToken("token-test"),
		WithBaseURL(server.URL),
	)

	task, err := client.VoiceClone.Upload(context.Background(), &VoiceCloneRequest{
		VoiceID:      "voice-a",
		Audio:        []byte("audio-data"),
		PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Upload error = %v", err)
	}
	if task.ID != "task-1" {
		t.Fatalf("task.ID = %q, want %q", task.ID, "task-1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	status, err := task.Wait(ctx)
	if err != nil {
		t.Fatalf("task.Wait error = %v", err)
	}
	if status == nil {
		t.Fatalf("status is nil")
	}
	if status.TaskID != "task-1" {
		t.Fatalf("status.TaskID = %q, want %q", status.TaskID, "task-1")
	}
	if status.SpeakerID != "voice-a" {
		t.Fatalf("status.SpeakerID = %q, want %q", status.SpeakerID, "voice-a")
	}
	if atomic.LoadInt32(&statusCalls) < 2 {
		t.Fatalf("status poll calls = %d, want >= 2", atomic.LoadInt32(&statusCalls))
	}
}

func TestVoiceCloneTaskWaitUnknownStatusRetainsRawValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case voiceCloneUploadPath:
			_, _ = io.Copy(io.Discard, r.Body)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"BaseResp":   map[string]any{"StatusCode": 0, "StatusMessage": ""},
				"speaker_id": "voice-u",
			})
		case voiceCloneStatusPath:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"BaseResp":   map[string]any{"StatusCode": 0, "StatusMessage": ""},
				"speaker_id": "voice-u",
				"status":     9,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(
		"app-test",
		WithBearerToken("token-test"),
		WithBaseURL(server.URL),
	)

	task, err := client.VoiceClone.Upload(context.Background(), &VoiceCloneRequest{
		VoiceID:      "voice-u",
		Audio:        []byte("audio-data"),
		PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Upload error = %v", err)
	}

	_, err = task.Wait(context.Background())
	if err == nil {
		t.Fatalf("expected unknown status error")
	}

	apiErr, ok := AsError(err)
	if !ok {
		t.Fatalf("want *Error, got %T (%v)", err, err)
	}
	if apiErr.Code != CodeServerError {
		t.Fatalf("error code = %d, want %d", apiErr.Code, CodeServerError)
	}
	if !strings.Contains(apiErr.Message, `unknown task status: "9"`) {
		t.Fatalf("error message = %q, want contains %q", apiErr.Message, `unknown task status: "9"`)
	}
}

func TestVoiceCloneTaskWaitFailurePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case voiceCloneUploadPath:
			_, _ = io.Copy(io.Discard, r.Body)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"BaseResp":   map[string]any{"StatusCode": 0, "StatusMessage": ""},
				"speaker_id": "voice-b",
			})
		case voiceCloneStatusPath:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"BaseResp":       map[string]any{"StatusCode": 0, "StatusMessage": ""},
				"speaker_id":     "voice-b",
				"status":         3,
				"status_code":    2203,
				"status_message": "training failed",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(
		"app-test",
		WithBearerToken("token-test"),
		WithBaseURL(server.URL),
	)

	task, err := client.VoiceClone.Upload(context.Background(), &VoiceCloneRequest{
		VoiceID:      "voice-b",
		Audio:        []byte("audio-data"),
		PollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Upload error = %v", err)
	}

	_, err = task.Wait(context.Background())
	if err == nil {
		t.Fatalf("expected failure from task wait")
	}

	apiErr, ok := AsError(err)
	if !ok {
		t.Fatalf("want *Error, got %T (%v)", err, err)
	}
	if apiErr.Code != 2203 {
		t.Fatalf("error code = %d, want 2203", apiErr.Code)
	}
	if !strings.Contains(apiErr.Message, "training failed") {
		t.Fatalf("error message = %q, want contains %q", apiErr.Message, "training failed")
	}
}
