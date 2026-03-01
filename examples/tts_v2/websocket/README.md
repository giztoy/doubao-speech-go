# TTS V2 WebSocket Example

This directory is reserved for the TTS V2 WebSocket example implementation.

- Unidirectional WS: `WSS /api/v3/tts/unidirectional`
- Bidirectional WS: `WSS /api/v3/tts/bidirection`

## Official TTS V2 VoiceType IDs (documented set)

Last updated: **2026-03-01**

The list below includes the VoiceType IDs explicitly documented in the official TTS 2.0 materials currently referenced by this repository's migration sources.

### A) General voices

| VoiceType ID | Language | Notes |
|---|---|---|
| `zh_female_cancan` | zh-CN | General female voice |
| `zh_female_shuangshuan` | zh-CN | General female voice |
| `zh_female_qingxin` | zh-CN | General female voice |
| `zh_female_tianmei` | zh-CN | General female voice |
| `zh_male_yangguang` | zh-CN | General male voice |
| `zh_male_wenzhong` | zh-CN | General male voice |
| `zh_male_qingsong` | zh-CN | General male voice |
| `en_female_sweet` | en | General female voice |
| `en_male_warm` | en | General male voice |
| `ja_female_warm` | ja | General female voice |
| `ko_female_sweet` | ko | General female voice |

### B) BigModel voices explicitly documented for TTS V2

| VoiceType ID | Typical Resource Match | Notes |
|---|---|---|
| `zh_female_shuangkuaisisi_moon_bigtts` | `seed-tts-1.0` / `seed-tts-1.0-concurr` | `_moon_bigtts` family |
| `zh_female_xiaohe_uranus_bigtts` | `seed-tts-2.0` / `seed-tts-2.0-concurr` | `_uranus_bigtts` family |
| `zh_female_vv_uranus_bigtts` | `seed-tts-2.0` / `seed-tts-2.0-concurr` | `_uranus_bigtts` family |
| `zh_male_taocheng_uranus_bigtts` | `seed-tts-2.0` / `seed-tts-2.0-concurr` | `_uranus_bigtts` family |
| `zh_female_cancan_mars_bigtts` | permission-dependent | Officially shown as an example ID |

### C) Voice IDs shown in official mix-speaker examples

| VoiceType ID | Notes |
|---|---|
| `zh_male_bvlazysheep` | Appears in mix-speaker sample |
| `BV120_streaming` | Appears in mix-speaker sample |
| `zh_male_ahu_conversation_wvae_bigtts` | Appears in mix-speaker sample |

### Special (not a normal voice ID)

| Value | Purpose |
|---|---|
| `custom_mix_bigtts` | Use as `speaker` when sending a `mix_speaker` request |

## Resource ID matching rule (important)

| Resource ID | Expected speaker pattern |
|---|---|
| `seed-tts-2.0`, `seed-tts-2.0-concurr` | `*_uranus_bigtts` |
| `seed-tts-1.0`, `seed-tts-1.0-concurr` | `*_moon_bigtts` |

If Resource ID and speaker family do not match, the service typically returns a mismatch error (for example code `55000000`).

## Notes on completeness

This file lists the official IDs explicitly published in the currently referenced documentation snapshot.
For the complete and latest account-specific official catalog (including newly released or permission-gated voices), use the official `ListSpeakers` API and paginate through all results.

## Sources

- TTS 2.0 overview and voice references:
  - https://www.volcengine.com/docs/6561/1257584
  - https://www.volcengine.com/docs/6561/1257544
- TTS 2.0 WebSocket docs:
  - https://www.volcengine.com/docs/6561/1719100
  - https://www.volcengine.com/docs/6561/1329505
- Official speaker listing API:
  - https://www.volcengine.com/docs/6561/2160690
