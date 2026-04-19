package main

import (
	"bytes"
	"context"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
)

//go:embed sample_zh_16k.pcm
var embeddedSampleAudio []byte

const (
	embeddedSampleFormat     = "pcm"
	embeddedSampleSampleRate = 16000
)

func main() {
	var (
		audioPath  string
		sampleRate int
		format     string
		chunkSize  int
		resourceID string
	)

	flag.StringVar(&audioPath, "audio", "", "Local audio file path (recommended 16k PCM)")
	flag.IntVar(&sampleRate, "sample-rate", 16000, "Audio sample rate")
	flag.StringVar(&format, "format", "pcm", "Audio format (currently pcm only)")
	flag.IntVar(&chunkSize, "chunk-size", 3200, "Bytes per audio chunk")
	flag.StringVar(&resourceID, "resource-id", doubaospeech.ResourceASRStreamV2, "ASR resource ID, e.g. volc.seedasr.sauc.duration or volc.bigasr.sauc.duration")
	flag.Parse()

	normalizedFormat := strings.ToLower(strings.TrimSpace(format))
	if normalizedFormat != embeddedSampleFormat {
		fmt.Fprintf(os.Stderr, "unsupported -format %q: this example currently supports pcm only\n", format)
		os.Exit(2)
	}

	appID := os.Getenv("DOUBAO_APP_ID")
	accessKey := os.Getenv("DOUBAO_ACCESS_KEY")
	apiKey := os.Getenv("DOUBAO_API_KEY")
	if appID == "" {
		fmt.Fprintln(os.Stderr, "missing environment variable DOUBAO_APP_ID")
		os.Exit(2)
	}
	if accessKey == "" && apiKey == "" {
		fmt.Fprintln(os.Stderr, "missing DOUBAO_ACCESS_KEY or DOUBAO_API_KEY")
		os.Exit(2)
	}

	var (
		audio []byte
		err   error
	)
	if audioPath == "" {
		if len(embeddedSampleAudio) == 0 {
			fmt.Fprintln(os.Stderr, "embedded sample audio is empty")
			os.Exit(2)
		}
		if isLFSPointerContent(embeddedSampleAudio) {
			fmt.Fprintln(os.Stderr, "embedded sample audio is currently a Git LFS pointer; run `git lfs pull` first or pass -audio with a local file")
			os.Exit(2)
		}
		audio = embeddedSampleAudio
		fmt.Fprintln(os.Stderr, "-audio is not provided, using embedded sample: sample_zh_16k.pcm")

		if normalizedFormat != embeddedSampleFormat || sampleRate != embeddedSampleSampleRate {
			fmt.Fprintf(
				os.Stderr,
				"when using embedded sample, -format/-sample-rate are ignored and forced to %s/%d\n",
				embeddedSampleFormat,
				embeddedSampleSampleRate,
			)
		}
		format = embeddedSampleFormat
		sampleRate = embeddedSampleSampleRate
	} else {
		audio, err = os.ReadFile(audioPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read audio file: %v\n", err)
			os.Exit(1)
		}
	}

	if chunkSize <= 0 {
		chunkSize = 3200
	}

	opts := []doubaospeech.Option{
		doubaospeech.WithResourceID(resourceID),
		doubaospeech.WithUserID("example-user"),
	}
	if apiKey != "" {
		opts = append(opts, doubaospeech.WithAPIKey(apiKey))
	} else {
		opts = append(opts, doubaospeech.WithV2APIKey(accessKey, appID))
	}

	client := doubaospeech.NewClient(appID, opts...)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	session, err := client.ASRV2.OpenStreamSession(ctx, &doubaospeech.ASRV2Config{
		Format:     doubaospeech.AudioFormat(format),
		SampleRate: doubaospeech.SampleRate(sampleRate),
		ResultType: "full",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open session: %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	for offset := 0; offset < len(audio); offset += chunkSize {
		end := offset + chunkSize
		if end > len(audio) {
			end = len(audio)
		}

		chunk := audio[offset:end]
		isLast := end == len(audio)
		if err := session.SendAudio(ctx, chunk, isLast); err != nil {
			fmt.Fprintf(os.Stderr, "failed to send audio: %v\n", err)
			os.Exit(1)
		}
	}

	for result, err := range session.Recv() {
		if err != nil {
			fmt.Fprintf(os.Stderr, "receive error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[final=%v] %s\n", result.IsFinal, result.Text)
		if result.IsFinal {
			// For this example, exit once final result arrives.
			break
		}
	}
}

func isLFSPointerContent(data []byte) bool {
	const lfsPointerPrefix = "version https://git-lfs.github.com/spec/v1"
	trimmed := bytes.TrimSpace(data)
	return bytes.HasPrefix(trimmed, []byte(lfsPointerPrefix))
}
