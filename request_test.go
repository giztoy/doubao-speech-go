package doubaospeech

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoJSONRequestNon2xxMapsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Tt-Logid", "log-429")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"code":3003,"message":"rate limited","reqid":"req-429"}`)
	}))
	defer server.Close()

	client := NewClient(
		"app-test",
		WithBaseURL(server.URL),
		WithAPIKey("key-test"),
	)

	err := client.doJSONRequest(context.Background(), http.MethodPost, "/test", map[string]any{"k": "v"}, nil, "")
	if err == nil {
		t.Fatalf("expected non-2xx error")
	}

	apiErr, ok := AsError(err)
	if !ok {
		t.Fatalf("want *Error, got %T (%v)", err, err)
	}
	if apiErr.Code != 3003 {
		t.Fatalf("error code = %d, want 3003", apiErr.Code)
	}
	if apiErr.HTTPStatus != http.StatusTooManyRequests {
		t.Fatalf("http status = %d, want %d", apiErr.HTTPStatus, http.StatusTooManyRequests)
	}
	if apiErr.LogID != "log-429" {
		t.Fatalf("log id = %q, want %q", apiErr.LogID, "log-429")
	}
	if apiErr.ReqID != "req-429" {
		t.Fatalf("req id = %q, want %q", apiErr.ReqID, "req-429")
	}
}

type stubHTTPDoer struct {
	req *http.Request
}

func (d *stubHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	d.req = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}, nil
}

func TestWithHTTPTransportOverridesDefaultHTTPClient(t *testing.T) {
	stub := &stubHTTPDoer{}
	client := NewClient(
		"app-test",
		WithBaseURL("https://example.com"),
		WithAPIKey("key-test"),
		WithHTTPTransport(stub),
	)

	var out struct {
		OK bool `json:"ok"`
	}

	if err := client.doJSONRequest(context.Background(), http.MethodPost, "/custom/path", map[string]any{"x": 1}, &out, ""); err != nil {
		t.Fatalf("doJSONRequest error = %v", err)
	}
	if !out.OK {
		t.Fatalf("response ok = false, want true")
	}
	if stub.req == nil {
		t.Fatalf("stub request was not captured")
	}
	if got := stub.req.URL.String(); got != "https://example.com/custom/path" {
		t.Fatalf("request URL = %q, want %q", got, "https://example.com/custom/path")
	}
	if got := stub.req.Header.Get("x-api-key"); got != "key-test" {
		t.Fatalf("x-api-key = %q, want %q", got, "key-test")
	}
}
