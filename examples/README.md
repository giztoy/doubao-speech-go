# examples

Convention: **one subdirectory per API**.

## Current examples

- `asr_v2_sauc_ws/`: ASR V2 SAUC WebSocket streaming recognition
  (open session -> send audio -> receive results -> close)

## Run

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_API_KEY=<your_api_key> \
go run ./examples/asr_v2_sauc_ws
```

This example embeds a sample PCM fixture (`sample_zh_16k.pcm`).
If `-audio` is not provided, it uses the embedded sample automatically.
In embedded mode, `-sample-rate` is forced to `16000`.
Because this sample is stored via Git LFS, run `git lfs pull` after cloning.

This example currently supports `pcm` format only.
If `-format` is set to another value, it exits with an error.

Use Access Key auth instead:

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_ACCESS_KEY=<your_access_key> \
go run ./examples/asr_v2_sauc_ws -audio /path/to/audio.pcm -resource-id volc.bigasr.sauc.duration
```

If you see `resourceId ... is not allowed`, switch to a resource your account can access using `-resource-id`.
