# TTS V2 WebSocket (V3) Example

This directory provides a runnable TTS V2 WebSocket **bidirectional** example and a migration-safe voice reference.

## Example entry

```text
examples/tts_v2/websocket/main.go
```

## Minimum run command

Use either `DOUBAO_ACCESS_KEY` (or `DOUBAO_TOKEN`) **or** `DOUBAO_API_KEY`.

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_ACCESS_KEY=<your_access_key> \
go run ./examples/tts_v2/websocket \
  -text "hello from tts websocket example" \
  -speaker zh_female_xiaohe_uranus_bigtts \
  -resource-id seed-tts-2.0 \
  -format mp3 \
  -sample-rate 24000 \
  -output ./tts_v2_ws_output.mp3
```

If your tenant allows `x-api-key`, you can also try API key mode:

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_API_KEY=<your_api_key> \
go run ./examples/tts_v2/websocket
```

Some accounts may reject this mode with `Invalid X-Api-Key`.
In that case, use `DOUBAO_ACCESS_KEY` (or `DOUBAO_TOKEN`) instead.

## Flags

- `-text`: input text to synthesize
- `-speaker`: voice type ID
- `-resource-id`: resource family (default `seed-tts-2.0`)
- `-format`: `mp3`, `pcm`, `ogg_opus`
- `-sample-rate`: output sample rate
- `-output`: audio output file path
- `-segments`: optional multi-segment input, split by `|`
- `-sessions`: optional sequential session count (default `1`)
- `-reuse-connection`: reuse one WebSocket connection for sequential sessions
- `-cancel-first-session`: cancel the first session after sending its first segment

This sample keeps a thin CLI surface like `examples/asr_v2_sauc_ws`, but adds optional advanced knobs:

- multi-segment text in one session (`-segments`)
- sequential multi-session run (`-sessions`)
- sequential sessions on the **same connection** (`-reuse-connection`)
- cancellation path demo (`-cancel-first-session`)

### Advanced run example

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_ACCESS_KEY=<your_access_key> \
go run ./examples/tts_v2/websocket \
  -segments "hello|this is segment two|final segment" \
  -sessions 2 \
  -reuse-connection \
  -speaker zh_female_vv_uranus_bigtts \
  -resource-id seed-tts-2.0 \
  -output ./tts_v2_ws_output.mp3
```

With `-sessions 2`, outputs are written as `tts_v2_ws_output.s1.mp3` and `tts_v2_ws_output.s2.mp3`.

Cancel flow example:

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_ACCESS_KEY=<your_access_key> \
go run ./examples/tts_v2/websocket \
  -segments "hello|unused segment" \
  -cancel-first-session \
  -output ./tts_v2_cancel_output.mp3
```

In cancel mode, the first output can be empty (`0 bytes`) depending on server-side cancel timing.

## Coverage boundary

Covered by this example:

- bidirectional endpoint
- multi-segment text
- sequential sessions (new connection mode and same-connection mode)
- `CancelSession` flow (first session)

Not covered in this example:

- unidirectional endpoint runtime flow
- `custom_mix_bigtts` with `mix_speaker` payload

## WebSocket endpoints

| Flow type | URL | Path |
| --- | --- | --- |
| Unidirectional stream | `wss://openspeech.bytedance.com/api/v3/tts/unidirectional` | `/api/v3/tts/unidirectional` |
| Bidirectional stream | `wss://openspeech.bytedance.com/api/v3/tts/bidirection` | `/api/v3/tts/bidirection` |

This runnable example currently uses the **bidirectional** endpoint.

## VoiceType IDs (updated with latest shared list)

> Last updated: 2026-03-02 (from user-provided list + existing migration baseline).
> This section is still a maintained subset. Use ListSpeakers API for complete/latest catalog.

### Chinese (`cn`)

| Display name | Scenario | VoiceType ID |
| --- | --- | --- |
| vivi 2.0 | general | `zh_female_vv_uranus_bigtts` |
| 大壹 | video dubbing | `zh_male_dayi_saturn_bigtts` |
| 黑猫侦探社咪仔 | video dubbing | `zh_female_mizai_saturn_bigtts` |
| 鸡汤女 | video dubbing | `zh_female_jitangnv_saturn_bigtts` |
| 魅力女友 | video dubbing | `zh_female_meilinvyou_saturn_bigtts` |
| 流畅女声 | video dubbing | `zh_female_santongyongns_saturn_bigtts` |
| 儒雅逸辰 | video dubbing | `zh_male_ruyayichen_saturn_bigtts` |
| 知性灿灿 | role-play | `saturn_zh_female_cancan_tob` |
| 可爱女生 | role-play | `saturn_zh_female_keainvsheng_tob` |
| 调皮公主 | role-play | `saturn_zh_female_tiaopigongzhu_tob` |
| 爽朗少年 | role-play | `saturn_zh_male_shuanglangshaonian_tob` |
| 天才同桌 | role-play | `saturn_zh_male_tiancaitongzhuo_tob` |
| 小何 | general | `zh_female_xiaohe_uranus_bigtts` |
| 云舟 | general | `zh_male_m191_uranus_bigtts` |
| 小天 | general | `zh_male_taocheng_uranus_bigtts` |
| 儿童绘本 | audiobook | `zh_female_xueayi_saturn_bigtts` |
| 爽快思思 (legacy baseline) | general | `zh_female_shuangkuaisisi_moon_bigtts` |

### English (`en`)

| Display name | Scenario | VoiceType ID |
| --- | --- | --- |
| Tim | general | `en_male_tim_uranus_bigtts` |
| Dacey | general | `en_female_dacey_uranus_bigtts` |
| Stokie | general | `en_female_stokie_uranus_bigtts` |

### Special-purpose identifier

- `custom_mix_bigtts`
  - Special-use identifier for mixed/custom scenarios.
  - Do **not** treat it as a regular standalone built-in voice.

## Resource ID and speaker suffix matching

| Resource ID family | Required speaker suffix | Example speaker |
| --- | --- | --- |
| `seed-tts-2.0` | `*_uranus_bigtts` | `zh_female_xiaohe_uranus_bigtts` |
| `seed-tts-1.0` | `*_moon_bigtts` | `zh_female_shuangkuaisisi_moon_bigtts` |

For `*_saturn_bigtts` / `saturn_*_tob` entries, required resource family may vary by account entitlement and product line.
Always verify with your tenant's ListSpeakers output and a runtime smoke test.

### Smoke-tested in this repo workspace

- `zh_female_vv_uranus_bigtts` + `seed-tts-2.0` -> pass
- `zh_male_dayi_saturn_bigtts` + `seed-tts-2.0` -> pass

If the resource family and speaker suffix do not match, the server typically returns an error like:

`55000000 resource ID is mismatched with speaker related resource`

For `custom_mix_bigtts`, you must provide `req_params.mix_speaker` in the request.
The current runnable sample demonstrates a regular single-speaker flow.

## Official sources and update path

- TTS V2 voice list (official): <https://www.volcengine.com/docs/6561/1257544>
- TTS V2 WebSocket unidirectional (official): <https://www.volcengine.com/docs/6561/1719100>
- TTS V2 WebSocket bidirectional (official): <https://www.volcengine.com/docs/6561/1329505>
- ListSpeakers API (official, paginated source of truth): <https://www.volcengine.com/docs/6561/2160690>
- Migration trace in this repo: `openteam/design_proposal.md`

## Notes on completeness

This README intentionally keeps a traceable subset that is stable for migration/testing.
For the complete and latest speaker list, call the official **ListSpeakers** API and iterate all pages, then refresh this document from API output.
