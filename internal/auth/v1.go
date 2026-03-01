package auth

import "net/http"

// Credentials 是鉴权所需最小凭证集合。
type Credentials struct {
	AppID             string
	AppKey            string
	AccessToken       string
	AccessKey         string
	APIKey            string
	DefaultResourceID string
}

// ApplyV1Headers 设置 V1 请求鉴权头。
// 优先级：x-api-key > Authorization(Bearer;) > X-Api-Access-Key.
func ApplyV1Headers(req *http.Request, creds Credentials) {
	if creds.APIKey != "" {
		req.Header.Set("x-api-key", creds.APIKey)
		return
	}

	if creds.AccessToken != "" {
		// 官方历史格式：Bearer;{token}
		req.Header.Set("Authorization", "Bearer;"+creds.AccessToken)
		return
	}

	if creds.AccessKey != "" {
		req.Header.Set("X-Api-Access-Key", creds.AccessKey)
		if creds.AppKey != "" {
			req.Header.Set("X-Api-App-Key", creds.AppKey)
		}
	}
}
