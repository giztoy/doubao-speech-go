# TTS V2 HTTP Stream Usage

This document describes how to use the SDK TTS V2 HTTP stream API implemented in this repository.

## API endpoint

- HTTP: `POST /api/v3/tts/unidirectional`

## SDK entry

```go
client := doubaospeech.NewClient(appID, ...)
for chunk, err := range client.TTSV2.Stream(ctx, req) {
    // handle chunk / error
}
```

## Authentication

Set one of the following credential pairs:

- Recommended:
  - `DOUBAO_APP_ID`
  - `DOUBAO_ACCESS_KEY` (or `DOUBAO_TOKEN` alias)
  - Optional: `DOUBAO_APP_KEY` (defaults to `DOUBAO_APP_ID`)
- Or:
  - `DOUBAO_APP_ID`
  - `DOUBAO_API_KEY`

Example auth selection notes:

- `-auth-mode auto` (default): prefers access key auth when both are present
- `-auth-mode access`: force V3 access key auth
- `-auth-mode api`: force API key auth

The SDK sends V2/V3 headers (`X-Api-App-Id`, `X-Api-Access-Key` or `x-api-key`, and `X-Api-Resource-Id`).

## Request model

`TTSV2Request` fields:

- `Text` (required)
- `Speaker` (required)
- `Format` (optional, default `mp3`)
- `SampleRate` (optional, default `24000`)
- `BitRate` (optional)
- `SpeechRate` / `PitchRate` / `VolumeRate` (optional)
- `Emotion` / `Language` (optional)
- `ResourceID` (optional; resolution order: request value > `WithResourceID` > `seed-tts-2.0`)
- `MixSpeaker` (optional)

The SDK request body aligns with:

- `user.uid`
- `req_params.text`
- `req_params.speaker`
- `req_params.audio_params`
- `req_params.mix_speaker` (optional)

## Stream response semantics

The service returns newline-delimited JSON frames.

Success states:

- `code == 0`: normal chunk
- `code == 20000000`: stream finished

Audio data handling:

- `data` is base64 audio bytes
- SDK decodes `data` and exposes `TTSV2Chunk.Audio`

Final-chunk handling:

- `TTSV2Chunk.IsLast` is true when `done=true` or `code=20000000`
- End-only frame (no `data`) is treated as valid finish

Error handling:

- Any `code` not in `{0, 20000000}` is returned as structured `*doubaospeech.Error`
- Error includes at least `code/message`, and `reqid` when provided by server

## Resource / speaker matching rule

- `seed-tts-2.0` must use `*_uranus_bigtts`
- `seed-tts-1.0` must use `*_moon_bigtts`

Verified `seed-tts-2.0` speakers in repository docs/examples:

- `zh_female_xiaohe_uranus_bigtts`
- `zh_female_vv_uranus_bigtts`
- `zh_male_taocheng_uranus_bigtts`

Common mismatch error:

- `55000000 resource ID is mismatched with speaker related resource`

## Example path and run command

Example directory:

```text
examples/tts_v2/http_stream
```

Run with Access Key:

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_ACCESS_KEY=<your_access_key> \
go run ./examples/tts_v2/http_stream -auth-mode access -text "Hello from Doubao TTS" -output /tmp/tts_v2_output.mp3
```

Run with API key:

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_API_KEY=<your_api_key> \
go run ./examples/tts_v2/http_stream -auth-mode api -text "Hello from Doubao TTS" -output /tmp/tts_v2_output.mp3
```

The example writes all streamed audio chunks into a local output file.
