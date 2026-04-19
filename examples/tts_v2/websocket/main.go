package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
)

const (
	defaultText       = "Hello, this is the TTS V2 WebSocket example."
	defaultSpeaker    = "zh_female_xiaohe_uranus_bigtts"
	defaultOutputFile = "tts_v2_ws_output.mp3"
	defaultTimeout    = 2 * time.Minute
)

func main() {
	var (
		text               string
		segments           string
		speaker            string
		format             string
		sampleRate         int
		resourceID         string
		outputPath         string
		sessions           int
		reuseConnection    bool
		cancelFirstSession bool
	)

	flag.StringVar(&text, "text", defaultText, "Text to synthesize")
	flag.StringVar(&segments, "segments", "", "Multiple text segments joined by '|' (optional)")
	flag.StringVar(&speaker, "speaker", defaultSpeaker, "TTS speaker/voice type ID")
	flag.StringVar(&format, "format", "mp3", "Audio format: mp3, pcm, ogg_opus")
	flag.IntVar(&sampleRate, "sample-rate", 24000, "Output sample rate")
	flag.StringVar(&resourceID, "resource-id", doubaospeech.ResourceTTSV2, "TTS resource ID")
	flag.StringVar(&outputPath, "output", defaultOutputFile, "Output audio file path")
	flag.IntVar(&sessions, "sessions", 1, "Number of sequential sessions")
	flag.BoolVar(&reuseConnection, "reuse-connection", false, "Reuse one websocket connection for sequential sessions")
	flag.BoolVar(&cancelFirstSession, "cancel-first-session", false, "Cancel the first session after sending its first segment")
	flag.Parse()

	text = strings.TrimSpace(text)
	speaker = strings.TrimSpace(speaker)
	format = strings.TrimSpace(strings.ToLower(format))
	resourceID = strings.TrimSpace(resourceID)
	outputPath = strings.TrimSpace(outputPath)

	if text == "" {
		fmt.Fprintln(os.Stderr, "-text cannot be empty")
		os.Exit(2)
	}
	if speaker == "" {
		fmt.Fprintln(os.Stderr, "-speaker cannot be empty")
		os.Exit(2)
	}
	if resourceID == "" {
		fmt.Fprintln(os.Stderr, "-resource-id cannot be empty")
		os.Exit(2)
	}
	if outputPath == "" {
		fmt.Fprintln(os.Stderr, "-output cannot be empty")
		os.Exit(2)
	}
	if sessions <= 0 {
		fmt.Fprintln(os.Stderr, "-sessions must be greater than 0")
		os.Exit(2)
	}

	textSegments := splitSegments(segments)
	if len(textSegments) == 0 {
		textSegments = []string{text}
	}

	appID := os.Getenv("DOUBAO_APP_ID")
	accessKey := os.Getenv("DOUBAO_ACCESS_KEY")
	if accessKey == "" {
		accessKey = os.Getenv("DOUBAO_TOKEN")
	}
	apiKey := os.Getenv("DOUBAO_API_KEY")

	if appID == "" {
		fmt.Fprintln(os.Stderr, "missing environment variable DOUBAO_APP_ID")
		os.Exit(2)
	}
	if accessKey == "" && apiKey == "" {
		fmt.Fprintln(os.Stderr, "missing DOUBAO_ACCESS_KEY/DOUBAO_TOKEN or DOUBAO_API_KEY")
		os.Exit(2)
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

	if reuseConnection {
		if err := runSequentialSessionsOnSingleConnection(
			client,
			speaker,
			format,
			sampleRate,
			resourceID,
			textSegments,
			sessions,
			outputPath,
			cancelFirstSession,
		); err != nil {
			fmt.Fprintf(os.Stderr, "single-connection run failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	for i := 1; i <= sessions; i++ {
		currentOutput := outputPathForSession(outputPath, i, sessions)
		total, err := runOneSession(
			client,
			speaker,
			format,
			sampleRate,
			resourceID,
			textSegments,
			currentOutput,
			cancelFirstSession && i == 1,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "session %d/%d failed: %v\n", i, sessions, err)
			os.Exit(1)
		}

		fmt.Printf("session %d/%d audio written: %s (%d bytes)\n", i, sessions, currentOutput, total)
	}
}

func runOneSession(
	client *doubaospeech.Client,
	speaker string,
	format string,
	sampleRate int,
	resourceID string,
	segments []string,
	outputPath string,
	cancelSession bool,
) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	session, err := client.TTSV2.OpenStreamSession(ctx, &doubaospeech.TTSV2WSConfig{
		Speaker:    speaker,
		Format:     doubaospeech.AudioFormat(format),
		SampleRate: doubaospeech.SampleRate(sampleRate),
		ResourceID: resourceID,
	})
	if err != nil {
		return 0, fmt.Errorf("open tts session: %w", err)
	}
	defer session.Close()

	return synthesizeOneActiveSession(ctx, session, segments, outputPath, cancelSession)
}

func runSequentialSessionsOnSingleConnection(
	client *doubaospeech.Client,
	speaker string,
	format string,
	sampleRate int,
	resourceID string,
	segments []string,
	sessions int,
	outputPath string,
	cancelFirstSession bool,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	session, err := client.TTSV2.OpenStreamSession(ctx, &doubaospeech.TTSV2WSConfig{
		Speaker:    speaker,
		Format:     doubaospeech.AudioFormat(format),
		SampleRate: doubaospeech.SampleRate(sampleRate),
		ResourceID: resourceID,
	})
	if err != nil {
		return fmt.Errorf("open tts session: %w", err)
	}
	defer session.Close()

	for i := 1; i <= sessions; i++ {
		if i > 1 {
			startCtx, startCancel := context.WithTimeout(context.Background(), defaultTimeout)
			startErr := session.StartNextSession(startCtx)
			startCancel()
			if startErr != nil {
				return fmt.Errorf("start next session %d/%d: %w", i, sessions, startErr)
			}
		}

		currentOutput := outputPathForSession(outputPath, i, sessions)
		runCtx, runCancel := context.WithTimeout(context.Background(), defaultTimeout)
		total, runErr := synthesizeOneActiveSession(
			runCtx,
			session,
			segments,
			currentOutput,
			cancelFirstSession && i == 1,
		)
		runCancel()
		if runErr != nil {
			return fmt.Errorf("session %d/%d failed: %w", i, sessions, runErr)
		}

		fmt.Printf("session %d/%d audio written: %s (%d bytes)\n", i, sessions, currentOutput, total)
	}

	return nil
}

func synthesizeOneActiveSession(
	ctx context.Context,
	session *doubaospeech.TTSV2WSSession,
	segments []string,
	outputPath string,
	cancelSession bool,
) (int64, error) {
	if cancelSession {
		if len(segments) == 0 {
			return 0, fmt.Errorf("no segment available for cancel flow")
		}

		if err := session.SendText(ctx, segments[0], false); err != nil {
			return 0, fmt.Errorf("send first segment before cancel: %w", err)
		}
		if err := session.CancelSession(ctx); err != nil {
			return 0, fmt.Errorf("cancel session: %w", err)
		}
	} else {
		for idx, seg := range segments {
			isLast := idx == len(segments)-1
			if err := session.SendText(ctx, seg, isLast); err != nil {
				return 0, fmt.Errorf("send text segment %d: %w", idx+1, err)
			}
		}
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("create output file: %w", err)
	}
	defer out.Close()

	var total int64
	for chunk, recvErr := range session.Recv() {
		if recvErr != nil {
			return total, fmt.Errorf("receive chunk: %w", recvErr)
		}

		if len(chunk.Audio) > 0 {
			n, writeErr := out.Write(chunk.Audio)
			if writeErr != nil {
				return total, fmt.Errorf("write output audio: %w", writeErr)
			}
			total += int64(n)
		}

		if chunk.IsFinal {
			break
		}
	}

	return total, nil
}

func splitSegments(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, "|")
	segments := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		segments = append(segments, v)
	}

	return segments
}

func outputPathForSession(base string, index int, total int) string {
	if total <= 1 {
		return base
	}

	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	return fmt.Sprintf("%s.s%d%s", name, index, ext)
}
