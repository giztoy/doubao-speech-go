package doubaospeech

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type panicOnNilDoer struct{}

func (d *panicOnNilDoer) Do(req *http.Request) (*http.Response, error) {
	if d == nil {
		panic("panicOnNilDoer: nil receiver")
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}, nil
}

func TestWithHTTPClientNilKeepsDefaultTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	client := NewClient(
		"app-test",
		WithBaseURL(server.URL),
		WithAPIKey("key-test"),
		WithHTTPClient(nil),
	)

	var out struct {
		OK bool `json:"ok"`
	}

	if err := client.doJSONRequest(context.Background(), http.MethodPost, "/health", map[string]any{"ping": true}, &out, ""); err != nil {
		t.Fatalf("doJSONRequest error = %v", err)
	}
	if !out.OK {
		t.Fatalf("response ok = false, want true")
	}
}

func TestWithHTTPTransportTypedNilKeepsDefaultTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	var typedNil *panicOnNilDoer
	client := NewClient(
		"app-test",
		WithBaseURL(server.URL),
		WithAPIKey("key-test"),
		WithHTTPTransport(typedNil),
	)

	var out struct {
		OK bool `json:"ok"`
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("unexpected panic: %v", recovered)
		}
	}()

	if err := client.doJSONRequest(context.Background(), http.MethodPost, "/health", map[string]any{"ping": true}, &out, ""); err != nil {
		t.Fatalf("doJSONRequest error = %v", err)
	}
	if !out.OK {
		t.Fatalf("response ok = false, want true")
	}
}

func TestDoJSONRequestWithTypedNilTransportReturnsStructuredError(t *testing.T) {
	client := NewClient(
		"app-test",
		WithBaseURL("https://example.com"),
		WithAPIKey("key-test"),
	)

	var typedNil *panicOnNilDoer
	client.config.httpClient = typedNil

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("unexpected panic: %v", recovered)
		}
	}()

	err := client.doJSONRequest(context.Background(), http.MethodPost, "/health", map[string]any{"ping": true}, nil, "")
	if err == nil {
		t.Fatalf("expected structured error for typed-nil transport")
	}

	apiErr, ok := AsError(err)
	if !ok {
		t.Fatalf("want *Error, got %T (%v)", err, err)
	}
	if apiErr.Code != CodeServerError {
		t.Fatalf("error code = %d, want %d", apiErr.Code, CodeServerError)
	}
	if !strings.Contains(apiErr.Message, "http transport is nil") {
		t.Fatalf("error message = %q, want contains %q", apiErr.Message, "http transport is nil")
	}
}

func TestTTSV2StreamWithTypedNilTransportReturnsStructuredError(t *testing.T) {
	client := NewClient(
		"app-test",
		WithV2APIKey("ak-test", "app-test"),
		WithBaseURL("https://example.com"),
	)

	var typedNil *panicOnNilDoer
	client.config.httpClient = typedNil

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("unexpected panic: %v", recovered)
		}
	}()

	_, err := collectTTSV2HTTPStreamChunks(client.TTSV2.Stream(context.Background(), &TTSV2Request{
		Text:    "typed nil transport",
		Speaker: "zh_female_xiaoyuan_bigtts",
	}))
	if err == nil {
		t.Fatalf("expected structured error for typed-nil transport")
	}

	apiErr, ok := AsError(err)
	if !ok {
		t.Fatalf("want *Error, got %T (%v)", err, err)
	}
	if apiErr.Code != CodeServerError {
		t.Fatalf("error code = %d, want %d", apiErr.Code, CodeServerError)
	}
	if !strings.Contains(apiErr.Message, "http transport is nil") {
		t.Fatalf("error message = %q, want contains %q", apiErr.Message, "http transport is nil")
	}
}
