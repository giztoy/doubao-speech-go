# doubao-speech-go

[![CI Pass](https://github.com/giztoy/doubao-speech-go/actions/workflows/ci.yml/badge.svg)](https://github.com/giztoy/doubao-speech-go/actions/workflows/ci.yml)
[![Code Scan](https://github.com/giztoy/doubao-speech-go/actions/workflows/codeql.yml/badge.svg)](https://github.com/giztoy/doubao-speech-go/actions/workflows/codeql.yml)
[![Go Pass A+](https://goreportcard.com/badge/github.com/giztoy/doubao-speech-go)](https://goreportcard.com/report/github.com/giztoy/doubao-speech-go)

Go SDK for Doubao/Volc speech APIs.

## Features

- ASR V2 SAUC WebSocket streaming
- TTS V2 HTTP stream
- TTS V2 WebSocket (unidirectional/bidirectional flows)
- Realtime session API
- Voice clone upload + polling workflow

## Requirements

- Go `1.26+`
- Git LFS (for embedded audio fixture)

```bash
git lfs pull
```

## Install

```bash
go get github.com/giztoy/doubao-speech-go
```

## Quick Start

Run the ASR V2 example:

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_API_KEY=<your_api_key> \
go run ./examples/asr_v2_sauc_ws
```

Run the Voice Clone example:

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_TOKEN=<your_token> \
go run ./examples/voice_clone -speaker-id <speaker_id> -audio /path/to/sample.wav
```

## Development

Format:

```bash
gofmt -w *.go internal/auth/*.go internal/protocol/*.go internal/transport/*.go internal/util/*.go examples/asr_v2_sauc_ws/*.go examples/realtime/*.go examples/tts_v2/http_stream/*.go examples/tts_v2/websocket/*.go examples/voice_clone/*.go
```

Build / test / vet:

```bash
go build ./...
go test ./...
go vet ./...
```

Run one test:

```bash
go test . -run TestVoiceCloneUploadAndWaitSuccess -count=1 -v
```

## Documentation

- `docs/asr_v2_sauc_ws.md`
- `docs/tts_v2_http_stream.md`
- `docs/tts_v2_websocket.md`
- `docs/voice_clone.md`

## License

This project is licensed under the MIT License. See [LICENSE](./LICENSE).
