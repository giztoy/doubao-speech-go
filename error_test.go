package doubaospeech

import (
	"net/http"
	"testing"
)

func TestParseAPIErrorNestedHeaderPayload(t *testing.T) {
	err := parseAPIError(http.StatusUnauthorized, []byte(`{"header":{"reqid":"req-nested-1","code":45000010,"message":"Invalid X-Api-Key"}}`), "log-nested-1")
	if err == nil {
		t.Fatalf("parseAPIError returned nil")
	}

	apiErr, ok := AsError(err)
	if !ok {
		t.Fatalf("want *Error, got %T", err)
	}

	if apiErr.Code != 45000010 {
		t.Fatalf("code = %d, want 45000010", apiErr.Code)
	}
	if apiErr.Message != "Invalid X-Api-Key" {
		t.Fatalf("message = %q, want %q", apiErr.Message, "Invalid X-Api-Key")
	}
	if apiErr.ReqID != "req-nested-1" {
		t.Fatalf("reqid = %q, want %q", apiErr.ReqID, "req-nested-1")
	}
	if apiErr.LogID != "log-nested-1" {
		t.Fatalf("logid = %q, want %q", apiErr.LogID, "log-nested-1")
	}
}
