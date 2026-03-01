package util

import (
	"fmt"
	"strings"
)

var supportedFormats = map[string]struct{}{
	"pcm":      {},
	"wav":      {},
	"mp3":      {},
	"ogg_opus": {},
	"aac":      {},
	"m4a":      {},
}

var supportedSampleRates = map[int]struct{}{
	8000:  {},
	16000: {},
	22050: {},
	24000: {},
	32000: {},
	44100: {},
	48000: {},
}

// NormalizeFormat normalizes the audio format string.
func NormalizeFormat(format string) string {
	return strings.ToLower(strings.TrimSpace(format))
}

func ValidateFormat(format string) error {
	if format == "" {
		return fmt.Errorf("audio format is required")
	}
	if _, ok := supportedFormats[NormalizeFormat(format)]; !ok {
		return fmt.Errorf("unsupported audio format: %s", format)
	}
	return nil
}

func ValidateSampleRate(sampleRate int) error {
	if sampleRate <= 0 {
		return fmt.Errorf("sample_rate must be > 0")
	}
	if _, ok := supportedSampleRates[sampleRate]; !ok {
		return fmt.Errorf("unsupported sample_rate: %d", sampleRate)
	}
	return nil
}

func ValidateChannel(channel int) error {
	if channel != 1 && channel != 2 {
		return fmt.Errorf("channel must be 1 or 2")
	}
	return nil
}

func ValidateBits(bits int) error {
	if bits <= 0 {
		return fmt.Errorf("bits must be > 0")
	}
	if bits != 8 && bits != 16 && bits != 24 && bits != 32 {
		return fmt.Errorf("bits must be one of [8,16,24,32]")
	}
	return nil
}

func ValidateResultType(resultType string) error {
	rt := strings.TrimSpace(strings.ToLower(resultType))
	if rt == "" || rt == "single" || rt == "full" {
		return nil
	}
	return fmt.Errorf("result_type must be one of [single,full]")
}
