package doubaospeech

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/giztoy/doubao-speech-go/internal/auth"
)

// doJSONRequest sends a JSON HTTP request.
//
// Notes:
//   - path is joined with baseURL;
//   - /api/v3 paths use V2 auth headers by default;
//   - non-2xx responses are converted to *Error.
func (c *Client) doJSONRequest(ctx context.Context, method, path string, body any, out any, resourceID string) error {
	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return wrapError(err, "marshal request body")
		}
		bodyReader = bytes.NewReader(payload)
	}

	endpoint := c.buildEndpoint(path)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return wrapError(err, "create request")
	}
	req.Header.Set("Content-Type", "application/json")
	c.applyAuthHeaders(req, path, resourceID)

	return c.doRequest(req, out)
}

func (c *Client) doMultipartRequest(
	ctx context.Context,
	method string,
	path string,
	fields map[string]string,
	files []MultipartFile,
	out any,
	resourceID string,
) error {
	contentType, body, err := buildMultipartBody(fields, files)
	if err != nil {
		return err
	}

	endpoint := c.buildEndpoint(path)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return wrapError(err, "create request")
	}
	req.Header.Set("Content-Type", contentType)
	c.applyAuthHeaders(req, path, resourceID)

	return c.doRequest(req, out)
}

func (c *Client) buildEndpoint(path string) string {
	return strings.TrimRight(c.config.baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func (c *Client) applyAuthHeaders(req *http.Request, path string, resourceID string) {
	creds := c.authCredentials()
	if strings.HasPrefix(path, "/api/v3/") {
		auth.ApplyV2Headers(req, creds, resourceID)
		return
	}

	auth.ApplyV1Headers(req, creds)

	resourceID = strings.TrimSpace(resourceID)
	if resourceID != "" {
		req.Header.Set("Resource-Id", resourceID)
		req.Header.Set("X-Api-Resource-Id", resourceID)
	}
}

func (c *Client) doRequest(req *http.Request, out any) error {
	if isNilHTTPDoer(c.config.httpClient) {
		return newAPIError(CodeServerError, "http transport is nil")
	}

	resp, err := c.config.httpClient.Do(req)
	if err != nil {
		return wrapError(err, "send request")
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return wrapError(err, "read response")
	}

	logID := resp.Header.Get("X-Tt-Logid")
	if logID == "" {
		logID = resp.Header.Get("X-Tt-LogId")
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return parseAPIError(resp.StatusCode, respBody, logID)
	}

	if out == nil || len(respBody) == 0 {
		return nil
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return wrapError(err, "unmarshal response")
	}

	return nil
}
