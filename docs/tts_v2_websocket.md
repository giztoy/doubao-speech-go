# TTS V2 WebSocket Usage

This document describes the minimum runnable flow for the `TTS V2 WebSocket` API in this repository.

## Example Path

```text
examples/tts_v2/websocket
```

## Supported Endpoints

- Unidirectional: `wss://openspeech.bytedance.com/api/v3/tts/unidirectional`
- Bidirectional: `wss://openspeech.bytedance.com/api/v3/tts/bidirection`

The current runnable example uses the **bidirectional** endpoint.

## Required Environment Variables

- `DOUBAO_APP_ID` (required)
- `DOUBAO_ACCESS_KEY` (recommended) **or** `DOUBAO_TOKEN`
- `DOUBAO_API_KEY` (optional, only if your tenant accepts `x-api-key` for this endpoint)
- `DOUBAO_APP_KEY` (optional, defaults to `DOUBAO_APP_ID`)

## Minimum Run Command

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_ACCESS_KEY=<your_access_key> \
go run ./examples/tts_v2/websocket \
  -text "hello from tts v2 websocket" \
  -speaker zh_female_xiaohe_uranus_bigtts \
  -resource-id seed-tts-2.0 \
  -format mp3 \
  -sample-rate 24000 \
  -output ./tts_v2_ws_output.mp3
```

API key mode:

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_API_KEY=<your_api_key> \
go run ./examples/tts_v2/websocket
```

If you see `Invalid X-Api-Key`, switch to `DOUBAO_ACCESS_KEY`/`DOUBAO_TOKEN` mode.

## Optional Advanced Flags

- `-segments`: send multiple text segments in one session, split by `|`
- `-sessions`: run sequential sessions (default `1`)
- `-reuse-connection`: reuse one WebSocket connection for sequential sessions
- `-cancel-first-session`: cancel the first session after sending its first segment

Advanced example:

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_ACCESS_KEY=<your_access_key> \
go run ./examples/tts_v2/websocket \
  -segments "hello|second segment|third segment" \
  -sessions 2 \
  -reuse-connection \
  -speaker zh_female_vv_uranus_bigtts \
  -resource-id seed-tts-2.0 \
  -output ./tts_v2_ws_output.mp3
```

When `-sessions > 1`, output files are suffixed as `.s1`, `.s2`, etc.

Cancel flow example:

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_ACCESS_KEY=<your_access_key> \
go run ./examples/tts_v2/websocket \
  -segments "hello|unused segment" \
  -cancel-first-session \
  -output ./tts_v2_cancel_output.mp3
```

In cancel mode, the first output may be empty (`0 bytes`) depending on cancel timing.

## Voice / Resource Matching Rule

Match speaker suffix to resource family:

- `seed-tts-2.0` -> `*_uranus_bigtts`
- `seed-tts-1.0` -> `*_moon_bigtts`

If mismatched, the server typically returns:

`55000000 resource ID is mismatched with speaker related resource`

## Bidirectional Event Flow

1. `StartConnection` (`event=1`)
2. Wait `ConnectionStarted` (`event=50`)
3. `StartSession` (`event=100`)
4. Wait `SessionStarted` (`event=150`)
5. `TaskRequest` (`event=200`, send text)
6. Receive sentence/audio events (`350`, `351`, `352`)
7. `FinishSession` (`event=102`)
8. Wait `SessionFinished` (`event=152`)
9. Best-effort `FinishConnection` (`event=2`)

## Output

The example writes synthesized audio to `-output` and prints per-session output summary.

## Coverage boundary

Covered by this example:

- bidirectional endpoint
- multi-segment text
- sequential sessions with optional single-connection reuse
- `CancelSession` (first-session demo)

Not covered:

- unidirectional endpoint runtime flow
- `custom_mix_bigtts` + `mix_speaker` payload

## Common Errors

- Missing credentials:
  - `missing environment variable DOUBAO_APP_ID`
  - `missing DOUBAO_ACCESS_KEY/DOUBAO_TOKEN or DOUBAO_API_KEY`
- Auth failures:
  - server error payload in WebSocket error frame or text frame
- Resource/speaker mismatch:
  - `55000000 resource ID is mismatched with speaker related resource`
- Permission mismatch:
  - resource not enabled for your account

## Known Limitations

- This example focuses on single-speaker synthesis.
- `custom_mix_bigtts` requires additional `mix_speaker` request fields and is not covered by this sample flow.
- Runtime behavior depends on network connectivity and cloud-side account permissions.

## References

- TTS V2 WebSocket unidirectional: <https://www.volcengine.com/docs/6561/1719100>
- TTS V2 WebSocket bidirectional: <https://www.volcengine.com/docs/6561/1329505>
- TTS V2 voice list: <https://www.volcengine.com/docs/6561/1257544>
- ListSpeakers API: <https://www.volcengine.com/docs/6561/2160690>
