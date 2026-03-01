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

// V2/V3 固定 App Key（官方文档约定，非用户凭证）。
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

// Client 是豆包语音 SDK 入口。
//
// 首次迁移阶段仅实现 ASR V2 SAUC WS；其余服务在后续迁移补齐。
type Client struct {
	// ASR V2 大模型流式识别。
	ASR   *ASRServiceV2
	ASRV2 *ASRServiceV2

	config *clientConfig
}

type clientConfig struct {
	appID       string
	accessKey   string // X-Api-Access-Key
	accessToken string // Bearer token（可回退到 X-Api-Access-Key）
	appKey      string // X-Api-App-Key（默认 appID）
	apiKey      string // x-api-key

	cluster    string
	resourceID string

	baseURL    string
	wsURL      string
	httpClient *http.Client
	timeout    time.Duration
	userID     string
}

// Option 用于配置 Client。
type Option func(*clientConfig)

// NewClient 创建 SDK Client。
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

	return c
}

// WithBearerToken 设置 Bearer Token。
// V1 Header 格式是 `Authorization: Bearer;{token}`（官方历史约定）。
func WithBearerToken(token string) Option {
	return func(c *clientConfig) {
		c.accessToken = token
	}
}

// WithAPIKey 设置 x-api-key。
func WithAPIKey(apiKey string) Option {
	return func(c *clientConfig) {
		c.apiKey = apiKey
	}
}

// WithV2APIKey 设置 V2/V3 鉴权。
func WithV2APIKey(accessKey, appKey string) Option {
	return func(c *clientConfig) {
		c.accessKey = accessKey
		c.appKey = appKey
	}
}

// WithRealtimeAPIKey 兼容别名。
func WithRealtimeAPIKey(accessKey, appKey string) Option {
	return WithV2APIKey(accessKey, appKey)
}

// WithResourceID 设置默认 resource_id。
func WithResourceID(resourceID string) Option {
	return func(c *clientConfig) {
		c.resourceID = resourceID
	}
}

// WithCluster 设置 V1 cluster（保留以兼容历史用法）。
func WithCluster(cluster string) Option {
	return func(c *clientConfig) {
		c.cluster = cluster
	}
}

// WithBaseURL 设置 HTTP Base URL。
func WithBaseURL(url string) Option {
	return func(c *clientConfig) {
		c.baseURL = url
	}
}

// WithWebSocketURL 设置 WebSocket Base URL。
func WithWebSocketURL(url string) Option {
	return func(c *clientConfig) {
		c.wsURL = url
	}
}

// WithHTTPClient 设置自定义 HTTP 客户端。
func WithHTTPClient(client *http.Client) Option {
	return func(c *clientConfig) {
		c.httpClient = client
	}
}

// WithTimeout 设置请求超时时间。
func WithTimeout(timeout time.Duration) Option {
	return func(c *clientConfig) {
		c.timeout = timeout
	}
}

// WithUserID 设置 user.uid。
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
