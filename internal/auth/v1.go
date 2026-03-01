package auth

import "net/http"

// Credentials is the minimal credential set for authentication.
type Credentials struct {
	AppID             string
	AppKey            string
	AccessToken       string
	AccessKey         string
	APIKey            string
	DefaultResourceID string
}

// ApplyV1Headers sets V1 request authentication headers.
// Priority: x-api-key > Authorization(Bearer;) > X-Api-Access-Key.
func ApplyV1Headers(req *http.Request, creds Credentials) {
	if creds.APIKey != "" {
		req.Header.Set("x-api-key", creds.APIKey)
		return
	}

	if creds.AccessToken != "" {
		// Historical official format: Bearer;{token}
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
