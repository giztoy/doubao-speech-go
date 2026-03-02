package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	doubaospeech "github.com/giztoy/doubao-speech-go"
)

func main() {
	var (
		speaker string
		round1  string
		round2  string
	)

	flag.StringVar(&speaker, "speaker", "zh_female_cancan", "TTS speaker/voice ID")
	flag.StringVar(&round1, "round1", "Please give a brief self-introduction.", "First-round user message")
	flag.StringVar(&round2, "round2", "Based on the updated settings, summarize your capability boundaries in two sentences.", "Second-round user message")
	flag.Parse()

	appID := os.Getenv("DOUBAO_APP_ID")
	apiKey := os.Getenv("DOUBAO_API_KEY")
	accessKey := os.Getenv("DOUBAO_ACCESS_KEY")
	if appID == "" {
		fmt.Fprintln(os.Stderr, "missing environment variable DOUBAO_APP_ID")
		os.Exit(2)
	}
	if apiKey == "" && accessKey == "" {
		fmt.Fprintln(os.Stderr, "missing DOUBAO_API_KEY or DOUBAO_ACCESS_KEY")
		os.Exit(2)
	}

	opts := []doubaospeech.Option{
		doubaospeech.WithResourceID(doubaospeech.ResourceRealtime),
		doubaospeech.WithUserID("example-realtime-user"),
	}
	if apiKey != "" {
		opts = append(opts, doubaospeech.WithAPIKey(apiKey))
	} else {
		opts = append(opts, doubaospeech.WithRealtimeAPIKey(accessKey, doubaospeech.AppKeyRealtime))
	}

	client := doubaospeech.NewClient(appID, opts...)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg := doubaospeech.DefaultRealtimeConfig()
	cfg.TTS.Speaker = strings.TrimSpace(speaker)
	cfg.Prompt = doubaospeech.RealtimePromptConfig{
		System: "You are a concise, accurate, and actionable voice assistant.",
		Variables: map[string]string{
			"tone": "professional",
		},
	}
	cfg.Props = doubaospeech.RealtimeGenerationProps{
		Temperature: 0.3,
		TopP:        0.9,
		MaxTokens:   256,
	}

	session, err := client.Realtime.OpenSession(ctx, &cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open realtime session: %v\n", err)
		os.Exit(1)
	}
	defer session.Close()

	fmt.Printf("opened session: %s\n", session.SessionID())

	if err := session.SendUserMessage(ctx, round1); err != nil {
		fmt.Fprintf(os.Stderr, "round1 send failed: %v\n", err)
		os.Exit(1)
	}

	round1Reply, err := recvUntilFinal(ctx, session, "round1")
	if err != nil {
		fmt.Fprintf(os.Stderr, "round1 receive failed: %v\n", err)
		os.Exit(1)
	}

	// Multi-turn update 1: rewrite history before round 2.
	session.UpdateHistory([]doubaospeech.RealtimeConversationMessage{
		{Role: "user", Content: round1},
		{Role: "assistant", Content: round1Reply + " (history revised in example before round 2)"},
	})
	if err := session.ReplaceHistory(1, doubaospeech.RealtimeConversationMessage{
		Role:    "assistant",
		Content: round1Reply + " (ReplaceHistory: second revision before round 2)",
	}); err != nil {
		fmt.Fprintf(os.Stderr, "replace history failed: %v\n", err)
		os.Exit(1)
	}

	// Multi-turn update 2: update prompt.
	session.UpdatePrompt(doubaospeech.RealtimePromptConfig{
		System: "Now state limitations more explicitly and keep the answer within two sentences.",
		Variables: map[string]string{
			"tone": "concise",
		},
	})

	// Multi-turn update 3: update generation props.
	session.UpdateProps(doubaospeech.RealtimeGenerationProps{
		Temperature: 0.1,
		TopP:        0.8,
		MaxTokens:   128,
	})

	if err := session.SendText(ctx, round2); err != nil {
		fmt.Fprintf(os.Stderr, "round2 send failed: %v\n", err)
		os.Exit(1)
	}

	if _, err := recvUntilFinalWithIterator(ctx, session, "round2"); err != nil {
		fmt.Fprintf(os.Stderr, "round2 receive failed: %v\n", err)
		os.Exit(1)
	}

	if err := session.Interrupt(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "interrupt returned error (may be expected on some servers): %v\n", err)
	}

	if err := session.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close failed: %v\n", err)
		os.Exit(1)
	}
	if err := session.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "second close failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("session closed idempotently")
}

func recvUntilFinal(ctx context.Context, session *doubaospeech.RealtimeSession, round string) (string, error) {
	builder := strings.Builder{}

	for {
		evt, err := session.RecvEvent(ctx)
		if err != nil {
			return "", err
		}
		if evt == nil {
			return "", fmt.Errorf("stream closed before final in %s", round)
		}

		switch evt.Type {
		case doubaospeech.EventChatResponse, doubaospeech.EventChatEnded, doubaospeech.EventTTSSegmentEnd, doubaospeech.EventTTSFinished:
			if evt.Text != "" {
				builder.WriteString(evt.Text)
				fmt.Printf("[%s][event=%d][final=%v] %s\n", round, evt.Type, evt.IsFinal, evt.Text)
			}
		default:
			if evt.Text != "" {
				fmt.Printf("[%s][event=%d][final=%v] %s\n", round, evt.Type, evt.IsFinal, evt.Text)
			}
		}

		if evt.IsFinal {
			break
		}
	}

	return builder.String(), nil
}

func recvUntilFinalWithIterator(ctx context.Context, session *doubaospeech.RealtimeSession, round string) (string, error) {
	builder := strings.Builder{}
	type recvItem struct {
		evt *doubaospeech.RealtimeEvent
		err error
	}

	itemCh := make(chan recvItem, 1)
	stopCh := make(chan struct{})
	go func() {
		defer close(itemCh)
		for evt, err := range session.Recv() {
			item := recvItem{evt: evt, err: err}
			select {
			case <-stopCh:
				return
			case itemCh <- item:
			}
			if err != nil {
				return
			}
		}
	}()
	defer close(stopCh)

	for {
		select {
		case <-ctx.Done():
			_ = session.Close()
			return "", ctx.Err()
		case item, ok := <-itemCh:
			if !ok {
				return "", fmt.Errorf("stream closed before final in %s", round)
			}
			if item.err != nil {
				return "", item.err
			}
			evt := item.evt
			if evt == nil {
				continue
			}

			switch evt.Type {
			case doubaospeech.EventChatResponse, doubaospeech.EventChatEnded, doubaospeech.EventTTSSegmentEnd, doubaospeech.EventTTSFinished:
				if evt.Text != "" {
					builder.WriteString(evt.Text)
					fmt.Printf("[%s][event=%d][final=%v] %s\n", round, evt.Type, evt.IsFinal, evt.Text)
				}
			default:
				if evt.Text != "" {
					fmt.Printf("[%s][event=%d][final=%v] %s\n", round, evt.Type, evt.IsFinal, evt.Text)
				}
			}

			if evt.IsFinal {
				return builder.String(), nil
			}
		}
	}
}
