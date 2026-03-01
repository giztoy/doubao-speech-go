# ASR V2 SAUC WebSocket Usage

This document describes the minimum runnable flow for the `ASR V2 SAUC WebSocket` API in this repository.

## Example Path

```text
examples/asr_v2_sauc_ws
```

## Required Environment Variables

- `DOUBAO_APP_ID` (required)
- `DOUBAO_API_KEY` (recommended) **or** `DOUBAO_ACCESS_KEY`

## Minimum Run Command

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_API_KEY=<your_api_key> \
go run ./examples/asr_v2_sauc_ws
```

If `-audio` is omitted, the example uses the embedded fixture audio (`sample_zh_16k.pcm`).
In that mode, `-format` and `-sample-rate` are ignored and forced to `pcm/16000`.

Since the embedded sample file is managed by Git LFS, run `git lfs pull` after cloning before using default embedded mode.

This example currently supports **PCM only**.
If `-format` is set to a non-PCM value, the example exits with an error.

If you prefer Access Key auth:

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_ACCESS_KEY=<your_access_key> \
go run ./examples/asr_v2_sauc_ws -audio /path/to/audio.pcm -resource-id volc.bigasr.sauc.duration
```

## Audio Input Requirements

- Recommended input: `pcm` 16kHz, mono, 16-bit
- You can override via flags:
  - `-format` (`pcm` only)
  - `-sample-rate` (default: `16000`, and forced to `16000` when using embedded sample)
  - `-chunk-size` (default: `3200` bytes)
  - `-resource-id` (default: `volc.seedasr.sauc.duration`)

## Expected Flow

1. Open WebSocket session
2. Send audio chunks (`isLast=true` on last chunk)
3. Receive interim/final text results
4. Close session

## Common Errors

- Missing credentials:
  - `missing DOUBAO_APP_ID`
  - `missing DOUBAO_ACCESS_KEY or DOUBAO_API_KEY`
- Authentication failure:
  - API returns structured error with `code/message` (and `reqid` when available)
- Resource not allowed:
  - If you see `resourceId ... is not allowed`, retry with a permitted resource via `-resource-id`, e.g. `volc.bigasr.sauc.duration`
- Unsupported audio parameters:
  - format/sample_rate/channel/bits validation errors

## Known Limitations

- Example requires real network connectivity and valid cloud credentials.
- Final-result behavior depends on server frame flags and utterance definiteness.
- This repository validates functionality with local Go tests (`go test ./...`).
