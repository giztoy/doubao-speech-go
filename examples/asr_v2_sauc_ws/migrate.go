package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

const embeddedAudioFileName = "sample_zh_16k.pcm"

//go:embed sample_zh_16k.pcm
var embeddedSampleAudio []byte

func resolveAudioPath(audioPath string) (string, func(), error) {
	if audioPath != "" {
		return audioPath, func() {}, nil
	}

	if len(embeddedSampleAudio) == 0 {
		return "", func() {}, fmt.Errorf("embedded sample audio is empty")
	}

	tempDir, err := os.MkdirTemp("", "asr-v2-sauc-audio-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp dir: %w", err)
	}

	outputPath := filepath.Join(tempDir, embeddedAudioFileName)
	if err := os.WriteFile(outputPath, embeddedSampleAudio, 0o600); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", func() {}, fmt.Errorf("write embedded sample audio: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	fmt.Printf("未指定 -audio，使用内置样例音频: %s\n", outputPath)
	return outputPath, cleanup, nil
}
