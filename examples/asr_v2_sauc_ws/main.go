package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	doubaospeech "github.com/giztoy/doubao-speech-go"
)

func main() {
	var (
		audioPath  string
		sampleRate int
		format     string
		chunkSize  int
		resourceID string
	)

	flag.StringVar(&audioPath, "audio", "", "本地音频文件路径（建议 16k PCM）")
	flag.IntVar(&sampleRate, "sample-rate", 16000, "音频采样率")
	flag.StringVar(&format, "format", "pcm", "音频格式：pcm/wav/mp3/... ")
	flag.IntVar(&chunkSize, "chunk-size", 3200, "每次发送字节数")
	flag.StringVar(&resourceID, "resource-id", doubaospeech.ResourceASRStreamV2, "ASR 资源 ID，例如 volc.seedasr.sauc.duration 或 volc.bigasr.sauc.duration")
	flag.Parse()

	resolvedAudioPath, cleanupAudioFile, err := resolveAudioPath(audioPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "准备音频失败: %v\n", err)
		os.Exit(2)
	}
	defer cleanupAudioFile()

	appID := os.Getenv("DOUBAO_APP_ID")
	accessKey := os.Getenv("DOUBAO_ACCESS_KEY")
	apiKey := os.Getenv("DOUBAO_API_KEY")
	if appID == "" {
		fmt.Fprintln(os.Stderr, "缺少环境变量 DOUBAO_APP_ID")
		os.Exit(2)
	}
	if accessKey == "" && apiKey == "" {
		fmt.Fprintln(os.Stderr, "缺少 DOUBAO_ACCESS_KEY 或 DOUBAO_API_KEY")
		os.Exit(2)
	}

	audio, err := os.ReadFile(resolvedAudioPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取音频失败: %v\n", err)
		os.Exit(1)
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
		fmt.Fprintf(os.Stderr, "创建会话失败: %v\n", err)
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
			fmt.Fprintf(os.Stderr, "发送音频失败: %v\n", err)
			os.Exit(1)
		}
	}

	for result, err := range session.Recv() {
		if err != nil {
			fmt.Fprintf(os.Stderr, "接收失败: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[final=%v] %s\n", result.IsFinal, result.Text)
		if result.IsFinal {
			// 示例场景：拿到最终结果后退出。
			break
		}
	}
}
