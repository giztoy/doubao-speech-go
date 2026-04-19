package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
)

func main() {
	var (
		text       string
		speaker    string
		resourceID string
		format     string
		sampleRate int
		outputPath string
		timeoutSec int
		authMode   string
	)

	flag.StringVar(&text, "text", "Hello, this is a TTS V2 HTTP stream example.", "Text to synthesize")
	flag.StringVar(&speaker, "speaker", "zh_female_xiaohe_uranus_bigtts", "TTS V2 speaker ID")
	flag.StringVar(&resourceID, "resource-id", doubaospeech.ResourceTTSV2, "TTS resource ID")
	flag.StringVar(&format, "format", "mp3", "Audio format: pcm/mp3/wav/ogg_opus/aac/m4a")
	flag.IntVar(&sampleRate, "sample-rate", 24000, "Audio sample rate")
	flag.StringVar(&outputPath, "output", "tts_v2_output.mp3", "Output audio file path")
	flag.IntVar(&timeoutSec, "timeout-sec", 120, "Request timeout in seconds")
	flag.StringVar(&authMode, "auth-mode", "auto", "Auth mode: auto|access|api (auto prefers access key)")
	flag.Parse()

	appID := strings.TrimSpace(os.Getenv("DOUBAO_APP_ID"))
	accessKey := strings.TrimSpace(os.Getenv("DOUBAO_ACCESS_KEY"))
	if accessKey == "" {
		accessKey = strings.TrimSpace(os.Getenv("DOUBAO_TOKEN"))
	}
	apiKey := strings.TrimSpace(os.Getenv("DOUBAO_API_KEY"))
	appKey := strings.TrimSpace(os.Getenv("DOUBAO_APP_KEY"))
	if appKey == "" {
		appKey = appID
	}

	if appID == "" {
		fmt.Fprintln(os.Stderr, "missing environment variable DOUBAO_APP_ID")
		os.Exit(2)
	}
	authMode = strings.ToLower(strings.TrimSpace(authMode))
	if authMode == "" {
		authMode = "auto"
	}
	if authMode != "auto" && authMode != "access" && authMode != "api" {
		fmt.Fprintln(os.Stderr, "-auth-mode must be one of: auto, access, api")
		os.Exit(2)
	}

	if accessKey == "" && apiKey == "" {
		fmt.Fprintln(os.Stderr, "missing credentials: set DOUBAO_ACCESS_KEY/DOUBAO_TOKEN or DOUBAO_API_KEY")
		os.Exit(2)
	}

	normalizedFormat := strings.ToLower(strings.TrimSpace(format))
	if normalizedFormat == "" {
		fmt.Fprintln(os.Stderr, "-format cannot be empty")
		os.Exit(2)
	}
	if sampleRate <= 0 {
		fmt.Fprintln(os.Stderr, "-sample-rate must be > 0")
		os.Exit(2)
	}
	if timeoutSec <= 0 {
		timeoutSec = 120
	}

	resourceID = strings.TrimSpace(resourceID)

	opts := []doubaospeech.Option{
		doubaospeech.WithResourceID(resourceID),
		doubaospeech.WithUserID("example-user"),
	}

	selectedAuthMode := authMode
	if selectedAuthMode == "auto" {
		if accessKey != "" {
			selectedAuthMode = "access"
		} else {
			selectedAuthMode = "api"
		}
	}

	switch selectedAuthMode {
	case "access":
		if accessKey == "" {
			fmt.Fprintln(os.Stderr, "-auth-mode=access requires DOUBAO_ACCESS_KEY or DOUBAO_TOKEN")
			os.Exit(2)
		}
		opts = append(opts, doubaospeech.WithV2APIKey(accessKey, appKey))
	case "api":
		if apiKey == "" {
			fmt.Fprintln(os.Stderr, "-auth-mode=api requires DOUBAO_API_KEY")
			os.Exit(2)
		}
		opts = append(opts, doubaospeech.WithAPIKey(apiKey))
	default:
		fmt.Fprintln(os.Stderr, "unexpected auth mode")
		os.Exit(2)
	}

	fmt.Printf("using auth mode=%s resource_id=%s speaker=%s\n", selectedAuthMode, resourceID, speaker)

	client := doubaospeech.NewClient(appID, opts...)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	req := &doubaospeech.TTSV2Request{
		Text:       text,
		Speaker:    speaker,
		Format:     doubaospeech.AudioFormat(normalizedFormat),
		SampleRate: doubaospeech.SampleRate(sampleRate),
		ResourceID: resourceID,
	}

	var (
		audioBuffer bytes.Buffer
		chunkCount  int
		lastReqID   string
	)

	for chunk, err := range client.TTSV2.Stream(ctx, req) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "stream synthesis failed: %v\n", err)
			os.Exit(1)
		}
		if chunk == nil {
			continue
		}
		if chunk.ReqID != "" {
			lastReqID = chunk.ReqID
		}
		if len(chunk.Audio) > 0 {
			_, _ = audioBuffer.Write(chunk.Audio)
			chunkCount++
		}
		if chunk.IsLast {
			break
		}
	}

	if audioBuffer.Len() == 0 {
		fmt.Fprintln(os.Stderr, "no audio data received from stream")
		os.Exit(1)
	}

	if err := ensureOutputDir(outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "prepare output directory failed: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputPath, audioBuffer.Bytes(), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write output file failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("stream synthesis finished: chunks=%d bytes=%d reqid=%s output=%s\n", chunkCount, audioBuffer.Len(), lastReqID, outputPath)
}

func ensureOutputDir(outputPath string) error {
	dir := filepath.Dir(outputPath)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
