# Voice Clone Usage

This document describes the voice clone API implemented in this repository.

## API endpoints

- Upload training audio: `POST /api/v1/mega_tts/audio/upload`
- Query training status: `POST /api/v1/mega_tts/status`
- Activate cloned voice: `POST /api/v1/mega_tts/audio/activate`

## Protocol notes

Voice clone V1 APIs use **JSON** payloads.

- Upload request body is JSON with `audios[].audio_bytes` (base64-encoded bytes)
- Status request body uses `speaker_id` as the primary identifier
- Headers include `Authorization: Bearer;{token}` or `x-api-key`, plus `Resource-Id`:
  - `seed-icl-1.0` for `model_type=1/2/3`
  - `seed-icl-2.0` for `model_type=4`

## SDK entry

```go
client := doubaospeech.NewClient(appID, ...)

task, err := client.VoiceClone.Upload(ctx, &doubaospeech.VoiceCloneRequest{
    VoiceID: "S_xxx",
    Audio:   audioBytes,
})
if err != nil {
    return err
}

status, err := task.Wait(ctx)
if err != nil {
    return err
}

// Query by speaker/voice id
latest, err := client.VoiceClone.GetStatus(ctx, "S_xxx")
if err != nil {
    return err
}

_ = latest
_ = status
```

## Authentication

Voice clone endpoints are V1 HTTP APIs.

Supported credential modes:

- Bearer token (`Authorization: Bearer;{token}`)
- API key (`x-api-key`)

## Request model

`VoiceCloneRequest` key fields:

- `VoiceID` / `SpeakerID` (required, one of them)
- `Audio` (required)
- `AudioFileName` / `AudioContentType` / `AudioFormat` (optional)
- `Text` / `Language` / `ModelType` / `Source` (optional)
- `ResourceID` (optional; auto-defaults by `model_type`)
- `PollInterval` (optional)

`model_type` notes:

- `1/2/3`: ICL 1.0 / DiT variants, default `Resource-Id = seed-icl-1.0`
- `4`: ICL 2.0, default `Resource-Id = seed-icl-2.0`

Upload mapping:

- `Audio` -> `audios[0].audio_bytes` (base64)
- `AudioFormat` (or inferred from file extension) -> `audios[0].audio_format`
- `Text` -> `audios[0].text`
- `VoiceID`/`SpeakerID` -> `speaker_id`

## Task semantics

`Upload` returns a generic `Task[VoiceCloneStatus]`.

- `task.Wait(ctx)` polls status immediately once, then continues with interval polling.
- Status mapping supports numeric and string task states.
- Terminal status:
  - success: returns `*VoiceCloneStatus`
  - failed/cancelled: returns structured `*doubaospeech.Error`

## Status response model

`VoiceCloneStatus` includes:

- `TaskID`
- `SpeakerID`
- `VoiceID`
- `Status` (`pending` / `processing` / `success` / `failed` / `cancelled`)
- `StatusCode`, `StatusMessage`
- `Version`, `DemoAudio`, `CreateTime`

## Example path and command

Example directory:

```text
examples/voice_clone
```

Run with Bearer token:

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_TOKEN=<your_token> \
go run ./examples/voice_clone -speaker-id <speaker_id> -audio /path/to/sample.wav
```
