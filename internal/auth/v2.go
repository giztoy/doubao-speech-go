package auth

import "net/http"

// ApplyV2Headers sets V2/V3 authentication headers.
func ApplyV2Headers(req *http.Request, creds Credentials, resourceID string) {
	headers := BuildV2WSHeaders(creds, resourceID, "")
	for key, values := range headers {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}
}

// BuildV2WSHeaders builds V2/V3 WebSocket authentication headers.
func BuildV2WSHeaders(creds Credentials, resourceID, connectID string) http.Header {
	headers := http.Header{}

	appKey := creds.AppKey
	if appKey == "" {
		appKey = creds.AppID
	}

	if appKey != "" {
		headers.Set("X-Api-App-Key", appKey)
	}
	if creds.AppID != "" {
		headers.Set("X-Api-App-Id", creds.AppID)
	}

	if creds.AccessKey != "" {
		headers.Set("X-Api-Access-Key", creds.AccessKey)
	} else if creds.AccessToken != "" {
		headers.Set("X-Api-Access-Key", creds.AccessToken)
	} else if creds.APIKey != "" {
		headers.Set("x-api-key", creds.APIKey)
	}

	resolvedResourceID := resourceID
	if resolvedResourceID == "" {
		resolvedResourceID = creds.DefaultResourceID
	}
	if resolvedResourceID != "" {
		headers.Set("X-Api-Resource-Id", resolvedResourceID)
	}
	if connectID != "" {
		headers.Set("X-Api-Connect-Id", connectID)
	}

	return headers
}
