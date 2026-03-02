package doubaospeech

import "time"

// RealtimeEventType represents realtime websocket event ID.
type RealtimeEventType int32

const (
	// Connection events.
	EventConnectionStarted RealtimeEventType = 50
	EventConnectionFailed  RealtimeEventType = 51
	EventConnectionEnded   RealtimeEventType = 52

	// Session events.
	EventSessionStarted  RealtimeEventType = 150
	EventSessionFinished RealtimeEventType = 152
	EventSessionFailed   RealtimeEventType = 153
	EventUsageResponse   RealtimeEventType = 154

	// ASR events.
	EventASRInfo     RealtimeEventType = 450
	EventASRResponse RealtimeEventType = 451
	EventASREnded    RealtimeEventType = 459

	// TTS events.
	EventTTSStarted    RealtimeEventType = 350
	EventTTSSegmentEnd RealtimeEventType = 351
	EventTTSAudioData  RealtimeEventType = 352
	EventTTSFinished   RealtimeEventType = 359

	// Chat events.
	EventChatResponse RealtimeEventType = 550
	EventChatEnded    RealtimeEventType = 559
)

// RealtimeConfig represents one realtime session config.
type RealtimeConfig struct {
	ASR    RealtimeASRConfig    `json:"asr" yaml:"asr"`
	TTS    RealtimeTTSConfig    `json:"tts" yaml:"tts"`
	Dialog RealtimeDialogConfig `json:"dialog" yaml:"dialog"`

	Prompt  RealtimePromptConfig          `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Props   RealtimeGenerationProps       `json:"props,omitempty" yaml:"props,omitempty"`
	History []RealtimeConversationMessage `json:"history,omitempty" yaml:"history,omitempty"`

	ResourceID string `json:"resource_id,omitempty" yaml:"resource_id,omitempty"`

	// Local runtime controls (not sent to server).
	EventBuffer         int           `json:"-" yaml:"-"`
	BackpressureTimeout time.Duration `json:"-" yaml:"-"`
}

// RealtimeASRConfig configures ASR behavior.
type RealtimeASRConfig struct {
	Language Language       `json:"language,omitempty" yaml:"language,omitempty"`
	Extra    map[string]any `json:"extra,omitempty" yaml:"extra,omitempty"`
}

// RealtimeTTSConfig configures TTS behavior.
type RealtimeTTSConfig struct {
	Speaker     string              `json:"speaker" yaml:"speaker"`
	AudioConfig RealtimeAudioConfig `json:"audio_config" yaml:"audio_config"`
	Extra       map[string]any      `json:"extra,omitempty" yaml:"extra,omitempty"`
}

// RealtimeAudioConfig describes audio IO parameters.
type RealtimeAudioConfig struct {
	Channel    int         `json:"channel" yaml:"channel"`
	Format     AudioFormat `json:"format" yaml:"format"`
	SampleRate SampleRate  `json:"sample_rate" yaml:"sample_rate"`
	Bits       int         `json:"bits,omitempty" yaml:"bits,omitempty"`
}

// RealtimeDialogConfig configures dialogue behavior.
type RealtimeDialogConfig struct {
	BotName           string         `json:"bot_name,omitempty" yaml:"bot_name,omitempty"`
	SystemRole        string         `json:"system_role,omitempty" yaml:"system_role,omitempty"`
	SpeakingStyle     string         `json:"speaking_style,omitempty" yaml:"speaking_style,omitempty"`
	CharacterManifest string         `json:"character_manifest,omitempty" yaml:"character_manifest,omitempty"`
	Extra             map[string]any `json:"extra,omitempty" yaml:"extra,omitempty"`
}

// RealtimePromptConfig controls prompt and prompt variables.
type RealtimePromptConfig struct {
	System    string            `json:"system,omitempty" yaml:"system,omitempty"`
	Variables map[string]string `json:"variables,omitempty" yaml:"variables,omitempty"`
}

// RealtimeGenerationProps controls generation params.
type RealtimeGenerationProps struct {
	Temperature      float64        `json:"temperature,omitempty" yaml:"temperature,omitempty"`
	TopP             float64        `json:"top_p,omitempty" yaml:"top_p,omitempty"`
	MaxTokens        int            `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
	PresencePenalty  float64        `json:"presence_penalty,omitempty" yaml:"presence_penalty,omitempty"`
	FrequencyPenalty float64        `json:"frequency_penalty,omitempty" yaml:"frequency_penalty,omitempty"`
	Extra            map[string]any `json:"extra,omitempty" yaml:"extra,omitempty"`
}

// RealtimeConversationMessage is one dialog history entry.
type RealtimeConversationMessage struct {
	Role    string `json:"role" yaml:"role"`
	Content string `json:"content" yaml:"content"`
}

// RealtimeEvent is one parsed server event.
type RealtimeEvent struct {
	Type      RealtimeEventType `json:"type"`
	SessionID string            `json:"session_id,omitempty"`
	ConnectID string            `json:"connect_id,omitempty"`
	Sequence  int32             `json:"sequence,omitempty"`

	Text    string `json:"text,omitempty"`
	Audio   []byte `json:"audio,omitempty"`
	Payload []byte `json:"payload,omitempty"`

	Error   *Error `json:"error,omitempty"`
	IsFinal bool   `json:"is_final,omitempty"`

	ReqID   string `json:"reqid,omitempty"`
	TraceID string `json:"trace_id,omitempty"`
}

// DefaultRealtimeConfig returns a baseline realtime config.
func DefaultRealtimeConfig() RealtimeConfig {
	return RealtimeConfig{
		ASR: RealtimeASRConfig{
			Language: LanguageZhCN,
		},
		TTS: RealtimeTTSConfig{
			Speaker: "zh_female_cancan",
			AudioConfig: RealtimeAudioConfig{
				Channel:    1,
				Format:     FormatPCM,
				SampleRate: SampleRate16000,
				Bits:       16,
			},
		},
		Dialog: RealtimeDialogConfig{},
	}
}
