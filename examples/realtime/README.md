# Realtime multi-turn example

This example demonstrates one **single realtime session** with multiple turns:

1. Open session with initial prompt + generation props
2. Send round-1 user message and receive streaming events until final
3. Update history before round-2
4. Update prompt before round-2
5. Update generation props before round-2
6. Send round-2 user message and receive events until final
7. Trigger one interrupt request
8. Close session twice to verify idempotent close

### API coverage in this example

Covered directly in `main.go`:

- `OpenSession`
- `SendUserMessage`
- `SendText` (alias)
- `RecvEvent`
- `Recv` (iterator form)
- `UpdateHistory`
- `ReplaceHistory`
- `UpdatePrompt`
- `UpdateProps`
- `Interrupt`
- `Close` (idempotent)

Not covered in this single example (recommended scenarios):

- `Dial` + `StartSession`: when you need explicit connection lifecycle control
- `Connect`: when you prefer one-shot connect+session API (equivalent semantics to `OpenSession`)
- `SendAudio`: microphone/PCM streaming scenarios
- `SayHello`: greeting bootstrap flow before user turn
- `SendTTSText`: server-side TTS text streaming scenarios

## Requirements

- `DOUBAO_APP_ID` (required)
- `DOUBAO_API_KEY` (recommended) **or** `DOUBAO_ACCESS_KEY`

## Run

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_API_KEY=<your_api_key> \
go run ./examples/realtime
```

Use Access Key auth:

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_ACCESS_KEY=<your_access_key> \
go run ./examples/realtime -speaker zh_female_cancan
```

## Key flags

- `-speaker`: TTS speaker/voice ID (default: `zh_female_cancan`)
- `-round1`: first user message
- `-round2`: second user message

## Voice list (realtime-compatible references)

> Table update date: **2026-03-02**

| voice_id | Language | Gender / style | Remark |
|---|---|---|---|
| `zh_female_cancan` | Chinese (Mandarin) | Female / standard | Default in this example, commonly used in VolcEngine realtime samples |
| `BV700_streaming` | Chinese (Mandarin) | Female / standard (Cancan) | BytePlus Speech "Supported voice and languages" |
| `BV701_streaming` | Chinese (Mandarin) | Male / expressive (Qingcang) | Supports multi-emotion in official docs |
| `BV138_streaming` | English (US) | Female / expressive (Lawrence) | Dialog expressive voice in official docs |
| `BV027_streaming` | English (US) | Female / formal (Amelia) | General English voice |
| `BV520_streaming` | Japanese | Female / outgoing (Himari) | Japanese voice option |

## Official sources

- Realtime API entry (VolcEngine):
  - https://www.volcengine.com/docs/6561/1594356
- Realtime/TTS voice list references:
  - https://docs.byteplus.com/en/docs/speech/docs-voice-parameters-1
  - https://www.volcengine.com/docs/6561/1257544

## Default speaker note

`main.go` uses `zh_female_cancan` by default (`-speaker` flag).
If your account is configured with BytePlus/BV voice IDs, pass a BV ID explicitly, for example:

```bash
go run ./examples/realtime -speaker BV700_streaming
```
