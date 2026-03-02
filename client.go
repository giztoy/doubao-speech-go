package doubaospeech

import (
	"net/http"
	"time"

	"github.com/giztoy/doubao-speech-go/internal/auth"
)

const (
	defaultBaseURL = "https://openspeech.bytedance.com"
	defaultWSURL   = "wss://openspeech.bytedance.com"
	defaultTimeout = 30 * time.Second
)

// V2/V3 fixed app keys (official constants, not user credentials).
const (
	AppKeyRealtime = "PlgvMymc7f3tQnJ6"
	AppKeyPodcast  = "aGjiRDfUWi"
)

// V2/V3 Resource IDs.
const (
	ResourceTTSV1        = "seed-tts-1.0"
	ResourceTTSV1Concurr = "seed-tts-1.0-concurr"
	ResourceTTSV2        = "seed-tts-2.0"
	ResourceTTSV2Concurr = "seed-tts-2.0-concurr"
	ResourceVoiceCloneV1 = "seed-icl-1.0"
	ResourceVoiceCloneV2 = "seed-icl-2.0"

	ResourceASRStream   = "volc.bigasr.sauc.duration"
	ResourceASRStreamV2 = "volc.seedasr.sauc.duration"
	ResourceASRFile     = "volc.bigasr.auc.duration"

	ResourceRealtime    = "volc.speech.dialog"
	ResourcePodcast     = "volc.service_type.10050"
	ResourceTranslation = "volc.megatts.simt"
)

// Client is the SDK entry point.
//
// In this migration stage, ASR V2 SAUC WS and Realtime are implemented.
type Client struct {
	// ASR V2 streaming recognition.
	ASR   *ASRServiceV2
	ASRV2 *ASRServiceV2

	// Realtime dialogue.
	Realtime *RealtimeService

	config *clientConfig
}

type clientConfig struct {
	appID       string
	accessKey   string // X-Api-Access-Key
	accessToken string // Bearer token (fallback to X-Api-Access-Key)
	appKey      string // X-Api-App-Key (defaults to appID)
	apiKey      string // x-api-key

	cluster    string
	resourceID string

	baseURL    string
	wsURL      string
	httpClient *http.Client
	timeout    time.Duration
	userID     string
}

// Option configures Client.
type Option func(*clientConfig)

// NewClient creates an SDK client.
func NewClient(appID string, opts ...Option) *Client {
	cfg := &clientConfig{
		appID:   appID,
		baseURL: defaultBaseURL,
		wsURL:   defaultWSURL,
		timeout: defaultTimeout,
		userID:  "default_user",
	}

	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.httpClient == nil {
		cfg.httpClient = &http.Client{Timeout: cfg.timeout}
	}

	c := &Client{config: cfg}
	asrV2 := newASRServiceV2(c)
	c.ASR = asrV2
	c.ASRV2 = asrV2
	c.Realtime = newRealtimeService(c)

	return c
}

// WithBearerToken sets Bearer token.
// V1 header format is `Authorization: Bearer;{token}` (historical convention).
func WithBearerToken(token string) Option {
	return func(c *clientConfig) {
		c.accessToken = token
	}
}

// WithAPIKey sets x-api-key.
func WithAPIKey(apiKey string) Option {
	return func(c *clientConfig) {
		c.apiKey = apiKey
	}
}

// WithV2APIKey sets V2/V3 authentication.
func WithV2APIKey(accessKey, appKey string) Option {
	return func(c *clientConfig) {
		c.accessKey = accessKey
		c.appKey = appKey
	}
}

// WithRealtimeAPIKey is a compatibility alias.
func WithRealtimeAPIKey(accessKey, appKey string) Option {
	return WithV2APIKey(accessKey, appKey)
}

// WithResourceID sets the default resource_id.
func WithResourceID(resourceID string) Option {
	return func(c *clientConfig) {
		c.resourceID = resourceID
	}
}

// WithCluster sets the V1 cluster (kept for backward compatibility).
func WithCluster(cluster string) Option {
	return func(c *clientConfig) {
		c.cluster = cluster
	}
}

// WithBaseURL sets the HTTP base URL.
func WithBaseURL(url string) Option {
	return func(c *clientConfig) {
		c.baseURL = url
	}
}

// WithWebSocketURL sets the WebSocket base URL.
func WithWebSocketURL(url string) Option {
	return func(c *clientConfig) {
		c.wsURL = url
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *clientConfig) {
		c.httpClient = client
	}
}

// WithTimeout sets request timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *clientConfig) {
		c.timeout = timeout
	}
}

// WithUserID sets user.uid.
func WithUserID(userID string) Option {
	return func(c *clientConfig) {
		c.userID = userID
	}
}

func (c *Client) authCredentials() auth.Credentials {
	appKey := c.config.appKey
	if appKey == "" {
		appKey = c.config.appID
	}

	return auth.Credentials{
		AppID:             c.config.appID,
		AppKey:            appKey,
		AccessToken:       c.config.accessToken,
		AccessKey:         c.config.accessKey,
		APIKey:            c.config.apiKey,
		DefaultResourceID: c.config.resourceID,
	}
}

func (c *Client) resolveResourceID(explicit string, fallback string) string {
	if explicit != "" {
		return explicit
	}
	if c.config.resourceID != "" {
		return c.config.resourceID
	}
	return fallback
}
