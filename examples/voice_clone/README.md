# Voice Clone Example

This example demonstrates Voice Clone upload + polling flow:

1. `POST /api/v1/mega_tts/audio/upload`
2. Poll `POST /api/v1/mega_tts/status` until terminal state

Entry:

```text
examples/voice_clone/main.go
```

## Requirements

- `DOUBAO_APP_ID` (required)
- `DOUBAO_TOKEN` (recommended) or `DOUBAO_ACCESS_KEY`
- `DOUBAO_API_KEY` (optional, only if your tenant accepts `x-api-key`)

## Important: model/resource/speaker mapping

- `model_type=1/2/3` -> use `Resource-Id: seed-icl-1.0`
- `model_type=4` -> use `Resource-Id: seed-icl-2.0`

The `speaker_id` must belong to the same model/resource family.

## Run (ICL 2.0)

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_TOKEN=<your_token> \
go run ./examples/voice_clone \
  -speaker-id S_RaTJh1aR1 \
  -audio ./examples/asr_v2_sauc_ws/sample_zh_16k.pcm \
  -auth-mode token \
  -model-type 4 \
  -resource-id seed-icl-2.0
```

If `-resource-id` is omitted, the SDK auto-selects by `-model-type`.

## Run (ICL 1.0)

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_TOKEN=<your_token> \
go run ./examples/voice_clone \
  -speaker-id <your_icl1_speaker_id> \
  -audio /path/to/sample.wav \
  -auth-mode token \
  -model-type 1 \
  -resource-id seed-icl-1.0
```

## Flags

- `-speaker-id`: required, `S_...` voice ID
- `-audio`: required, local training audio file path
- `-auth-mode`: `auto|token|api` (default `auto`)
- `-model-type`: clone model type (default `1`)
- `-resource-id`: optional resource override
- `-timeout-sec`: task wait timeout seconds (default `180`)
- `-poll-interval-ms`: polling interval in milliseconds (default `2000`)

## Common errors

- `parameter license not found for param`
  - Usually model/resource/speaker entitlement mismatch.
- `request and grant appid mismatch`
  - AppID and token are not from the same grant.
- `Invalid X-Api-Key`
  - Your tenant may not allow API key mode for this endpoint; switch to token mode.
