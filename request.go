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

// doJSONRequest 发送 JSON HTTP 请求。
//
// 说明：
//   - path 会拼接到 baseURL；
//   - 对 /api/v3 路径默认应用 V2 鉴权；
//   - 非 2xx 响应将统一转换为 *Error。
func (c *Client) doJSONRequest(ctx context.Context, method, path string, body any, out any, resourceID string) error {
	endpoint := strings.TrimRight(c.config.baseURL, "/") + "/" + strings.TrimLeft(path, "/")

	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return wrapError(err, "marshal request body")
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return wrapError(err, "create request")
	}
	req.Header.Set("Content-Type", "application/json")

	creds := c.authCredentials()
	if strings.HasPrefix(path, "/api/v3/") {
		auth.ApplyV2Headers(req, creds, resourceID)
	} else {
		auth.ApplyV1Headers(req, creds)
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
