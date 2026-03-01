package doubaospeech

// AudioFormat represents audio encoding format.
type AudioFormat string

const (
	FormatPCM AudioFormat = "pcm"
	FormatWAV AudioFormat = "wav"
	FormatMP3 AudioFormat = "mp3"
	FormatOGG AudioFormat = "ogg_opus"
	FormatAAC AudioFormat = "aac"
	FormatM4A AudioFormat = "m4a"
)

// SampleRate represents audio sample rate.
type SampleRate int

const (
	SampleRate8000  SampleRate = 8000
	SampleRate16000 SampleRate = 16000
	SampleRate22050 SampleRate = 22050
	SampleRate24000 SampleRate = 24000
	SampleRate32000 SampleRate = 32000
	SampleRate44100 SampleRate = 44100
	SampleRate48000 SampleRate = 48000
)

// Language represents recognition language.
type Language string

const (
	LanguageZhCN Language = "zh-CN"
	LanguageEnUS Language = "en-US"
	LanguageJaJP Language = "ja-JP"
	LanguageKoKR Language = "ko-KR"
)

// TaskStatus is async task status.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusSuccess    TaskStatus = "success"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

// ASRV2Config is SAUC V2 streaming session config.
type ASRV2Config struct {
	Format     AudioFormat `json:"format" yaml:"format"`
	SampleRate SampleRate  `json:"sample_rate" yaml:"sample_rate"`
	Channel    int         `json:"channel,omitempty" yaml:"channel,omitempty"`
	Channels   int         `json:"channels,omitempty" yaml:"channels,omitempty"` // Backward-compatible alias field.
	Bits       int         `json:"bits,omitempty" yaml:"bits,omitempty"`
	Language   Language    `json:"language,omitempty" yaml:"language,omitempty"`

	EnableITN         bool     `json:"enable_itn,omitempty" yaml:"enable_itn,omitempty"`
	EnablePunc        bool     `json:"enable_punc,omitempty" yaml:"enable_punc,omitempty"`
	EnableDiarization bool     `json:"enable_diarization,omitempty" yaml:"enable_diarization,omitempty"`
	SpeakerNum        int      `json:"speaker_num,omitempty" yaml:"speaker_num,omitempty"`
	Hotwords          []string `json:"hotwords,omitempty" yaml:"hotwords,omitempty"`
	ResultType        string   `json:"result_type,omitempty" yaml:"result_type,omitempty"` // single/full

	ResourceID string `json:"resource_id,omitempty" yaml:"resource_id,omitempty"`
}

// ASRV2Result is one parsed server response.
type ASRV2Result struct {
	Text       string           `json:"text"`
	Utterances []ASRV2Utterance `json:"utterances,omitempty"`
	IsFinal    bool             `json:"is_final"`
	Duration   int              `json:"duration,omitempty"`
	ReqID      string           `json:"reqid,omitempty"`
}

// ASRV2Utterance contains utterance-level info.
type ASRV2Utterance struct {
	Text       string      `json:"text"`
	StartTime  int         `json:"start_time"`
	EndTime    int         `json:"end_time"`
	Definite   bool        `json:"definite"`
	SpeakerID  string      `json:"speaker_id,omitempty"`
	Words      []ASRV2Word `json:"words,omitempty"`
	Confidence float64     `json:"confidence,omitempty"`
}

// ASRV2Word contains word-level timing info.
type ASRV2Word struct {
	Text      string  `json:"text"`
	StartTime int     `json:"start_time"`
	EndTime   int     `json:"end_time"`
	Conf      float64 `json:"conf,omitempty"`
}

// Backward-compatible aliases mapped to V2 types.
type StreamASRConfig = ASRV2Config
type ASRChunk = ASRV2Result
type Utterance = ASRV2Utterance
type Word = ASRV2Word
