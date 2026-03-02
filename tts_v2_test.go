package doubaospeech

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTTSV2HTTPStreamChunkSequenceAndFinalFrame(t *testing.T) {
	type capturedRequest struct {
		Method     string
		Path       string
		AppID      string
		AccessKey  string
		ResourceID string
		Body       ttsV2HTTPStreamRequest
	}

	requestCh := make(chan capturedRequest, 1)

	firstAudio := []byte("chunk-audio-1")
	secondAudio := []byte("chunk-audio-2")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body ttsV2HTTPStreamRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		requestCh <- capturedRequest{
			Method:     r.Method,
			Path:       r.URL.Path,
			AppID:      r.Header.Get("X-Api-App-Id"),
			AccessKey:  r.Header.Get("X-Api-Access-Key"),
			ResourceID: r.Header.Get("X-Api-Resource-Id"),
			Body:       body,
		}

		w.Header().Set("Content-Type", "application/json")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "flush is not supported", http.StatusInternalServerError)
			return
		}

		_, _ = fmt.Fprintf(
			w,
			`{"reqid":"req-stream-1","code":0,"message":"","data":"%s"}`+"\n",
			base64.StdEncoding.EncodeToString(firstAudio),
		)
		flusher.Flush()

		_, _ = fmt.Fprintf(
			w,
			`{"reqid":"req-stream-1","code":0,"message":"","data":"%s"}`+"\n",
			base64.StdEncoding.EncodeToString(secondAudio),
		)
		flusher.Flush()

		_, _ = fmt.Fprintln(w, `{"reqid":"req-stream-1","code":20000000,"message":"ok","data":null}`)
		flusher.Flush()
	}))
	defer server.Close()

	client := NewClient(
		"app-test",
		WithV2APIKey("ak-test", "app-test"),
		WithBaseURL(server.URL),
		WithUserID("stream-user"),
	)

	chunks, err := collectTTSV2HTTPStreamChunks(client.TTSV2.Stream(context.Background(), &TTSV2Request{
		Text:       "hello stream",
		Speaker:    "zh_female_xiaohe_uranus_bigtts",
		Format:     FormatPCM,
		SampleRate: SampleRate16000,
		BitRate:    64000,
		SpeechRate: 10,
		PitchRate:  -5,
		VolumeRate: 8,
		ResourceID: ResourceTTSV2,
	}))
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}

	if len(chunks) != 3 {
		t.Fatalf("chunk count = %d, want 3", len(chunks))
	}

	combinedAudio := bytes.Join([][]byte{chunks[0].Audio, chunks[1].Audio}, nil)
	if !bytes.Equal(combinedAudio, append(firstAudio, secondAudio...)) {
		t.Fatalf("unexpected combined audio: got %q", string(combinedAudio))
	}

	if chunks[0].IsLast {
		t.Fatalf("first chunk should not be final")
	}
	if chunks[1].IsLast {
		t.Fatalf("second chunk should not be final")
	}
	if !chunks[2].IsLast {
		t.Fatalf("last chunk should be final")
	}
	if len(chunks[2].Audio) != 0 {
		t.Fatalf("final chunk audio len = %d, want 0", len(chunks[2].Audio))
	}

	captured := <-requestCh
	if captured.Method != http.MethodPost {
		t.Fatalf("method = %s, want %s", captured.Method, http.MethodPost)
	}
	if captured.Path != ttsV2HTTPStreamPath {
		t.Fatalf("path = %s, want %s", captured.Path, ttsV2HTTPStreamPath)
	}
	if captured.AppID != "app-test" {
		t.Fatalf("X-Api-App-Id = %q, want %q", captured.AppID, "app-test")
	}
	if captured.AccessKey != "ak-test" {
		t.Fatalf("X-Api-Access-Key = %q, want %q", captured.AccessKey, "ak-test")
	}
	if captured.ResourceID != ResourceTTSV2 {
		t.Fatalf("X-Api-Resource-Id = %q, want %q", captured.ResourceID, ResourceTTSV2)
	}

	if captured.Body.User.UID != "stream-user" {
		t.Fatalf("user.uid = %q, want %q", captured.Body.User.UID, "stream-user")
	}
	if captured.Body.ReqParams.Text != "hello stream" {
		t.Fatalf("req_params.text = %q, want %q", captured.Body.ReqParams.Text, "hello stream")
	}
	if captured.Body.ReqParams.Speaker != "zh_female_xiaohe_uranus_bigtts" {
		t.Fatalf("req_params.speaker = %q", captured.Body.ReqParams.Speaker)
	}
	if captured.Body.ReqParams.AudioParams.Format != string(FormatPCM) {
		t.Fatalf("audio_params.format = %q, want %q", captured.Body.ReqParams.AudioParams.Format, FormatPCM)
	}
	if captured.Body.ReqParams.AudioParams.SampleRate != int(SampleRate16000) {
		t.Fatalf("audio_params.sample_rate = %d, want %d", captured.Body.ReqParams.AudioParams.SampleRate, SampleRate16000)
	}
	if captured.Body.ReqParams.AudioParams.BitRate != 64000 {
		t.Fatalf("audio_params.bit_rate = %d, want 64000", captured.Body.ReqParams.AudioParams.BitRate)
	}
	if captured.Body.ReqParams.AudioParams.SpeechRate != 10 {
		t.Fatalf("audio_params.speech_rate = %d, want 10", captured.Body.ReqParams.AudioParams.SpeechRate)
	}
	if captured.Body.ReqParams.AudioParams.PitchRate != -5 {
		t.Fatalf("audio_params.pitch_rate = %d, want -5", captured.Body.ReqParams.AudioParams.PitchRate)
	}
	if captured.Body.ReqParams.AudioParams.VolumeRate != 8 {
		t.Fatalf("audio_params.volume_rate = %d, want 8", captured.Body.ReqParams.AudioParams.VolumeRate)
	}
}

func TestTTSV2HTTPStreamOnlyFinalFrame(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, `{"reqid":"req-final-only","code":20000000,"message":"ok","data":null}`)
	}))
	defer server.Close()

	client := NewClient(
		"app-test",
		WithV2APIKey("ak-test", "app-test"),
		WithBaseURL(server.URL),
	)

	chunks, err := collectTTSV2HTTPStreamChunks(client.TTSV2.Stream(context.Background(), &TTSV2Request{
		Text:    "final only",
		Speaker: "zh_female_vv_uranus_bigtts",
	}))
	if err != nil {
		t.Fatalf("Stream error = %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("chunk count = %d, want 1", len(chunks))
	}
	if !chunks[0].IsLast {
		t.Fatalf("single chunk should be final")
	}
	if len(chunks[0].Audio) != 0 {
		t.Fatalf("single final chunk audio len = %d, want 0", len(chunks[0].Audio))
	}
}

func TestTTSV2HTTPStreamResourceSpeakerMismatchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, `{"reqid":"req-mismatch","code":55000000,"message":"resource ID is mismatched with speaker related resource"}`)
	}))
	defer server.Close()

	client := NewClient(
		"app-test",
		WithV2APIKey("ak-test", "app-test"),
		WithBaseURL(server.URL),
	)

	_, err := collectTTSV2HTTPStreamChunks(client.TTSV2.Stream(context.Background(), &TTSV2Request{
		Text:       "mismatch test",
		Speaker:    "zh_female_shuangkuaisisi_moon_bigtts",
		ResourceID: ResourceTTSV2,
	}))
	if err == nil {
		t.Fatalf("expected mismatch error")
	}

	apiErr, ok := AsError(err)
	if !ok {
		t.Fatalf("want *Error, got %T (%v)", err, err)
	}
	if apiErr.Code != 55000000 {
		t.Fatalf("error code = %d, want 55000000", apiErr.Code)
	}
	if !strings.Contains(apiErr.Message, "resource ID is mismatched with speaker related resource") {
		t.Fatalf("unexpected error message = %q", apiErr.Message)
	}
	if apiErr.ReqID != "req-mismatch" {
		t.Fatalf("reqid = %q, want %q", apiErr.ReqID, "req-mismatch")
	}
}

func TestTTSV2HTTPStreamEOFWithoutFinalFrameReturnsError(t *testing.T) {
	partialAudio := []byte("partial-audio")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(
			w,
			`{"reqid":"req-partial","code":0,"message":"","data":"%s"}`+"\n",
			base64.StdEncoding.EncodeToString(partialAudio),
		)
	}))
	defer server.Close()

	client := NewClient(
		"app-test",
		WithV2APIKey("ak-test", "app-test"),
		WithBaseURL(server.URL),
	)

	chunks, err := collectTTSV2HTTPStreamChunks(client.TTSV2.Stream(context.Background(), &TTSV2Request{
		Text:    "eof without final",
		Speaker: "zh_female_vv_uranus_bigtts",
	}))
	if err == nil {
		t.Fatalf("expected error when stream ended without final frame")
	}

	if len(chunks) != 1 {
		t.Fatalf("chunk count = %d, want 1", len(chunks))
	}
	if !bytes.Equal(chunks[0].Audio, partialAudio) {
		t.Fatalf("unexpected audio chunk = %q", string(chunks[0].Audio))
	}
	if chunks[0].IsLast {
		t.Fatalf("partial chunk should not be final")
	}

	apiErr, ok := AsError(err)
	if !ok {
		t.Fatalf("want *Error, got %T (%v)", err, err)
	}
	if apiErr.Code != CodeServerError {
		t.Fatalf("error code = %d, want %d", apiErr.Code, CodeServerError)
	}
	if !strings.Contains(apiErr.Message, "ended before final frame") {
		t.Fatalf("unexpected error message = %q", apiErr.Message)
	}
	if apiErr.ReqID != "req-partial" {
		t.Fatalf("reqid = %q, want %q", apiErr.ReqID, "req-partial")
	}
}

func collectTTSV2HTTPStreamChunks(seq iter.Seq2[*TTSV2Chunk, error]) ([]*TTSV2Chunk, error) {
	chunks := make([]*TTSV2Chunk, 0)
	for chunk, err := range seq {
		if err != nil {
			return chunks, err
		}
		if chunk == nil {
			continue
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}
