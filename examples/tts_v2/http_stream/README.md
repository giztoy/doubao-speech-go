# TTS V2 HTTP Stream Example

This example demonstrates `POST /api/v3/tts/unidirectional` streaming synthesis.

## Run with API Key

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_API_KEY=<your_api_key> \
go run ./examples/tts_v2/http_stream -auth-mode api -text "Hello from Doubao TTS" -output /tmp/tts_v2_output.mp3
```

## Run with Access Key

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_ACCESS_KEY=<your_access_key> \
go run ./examples/tts_v2/http_stream -auth-mode access -speaker zh_female_xiaohe_uranus_bigtts -resource-id seed-tts-2.0
```

`DOUBAO_TOKEN` is also accepted as an alias of `DOUBAO_ACCESS_KEY`.

Optional:

- `DOUBAO_APP_KEY` (used with `-auth-mode access`; defaults to `DOUBAO_APP_ID` when omitted)

By default, `-auth-mode auto` is used and it prefers Access Key over API Key when both are present.

## Important resource/speaker matching

- `seed-tts-2.0` requires `*_uranus_bigtts` speakers.
- `seed-tts-1.0` requires `*_moon_bigtts` speakers.

## Voice list (user-provided, 2026-03-02)

| Display name | Scenario | Lang | Voice ID |
|---|---|---|---|
| vivi 2.0 | General | cn | `zh_female_vv_uranus_bigtts` |
| 大壹 | Video dubbing | cn | `zh_male_dayi_saturn_bigtts` |
| 黑猫侦探社咪仔 | Video dubbing | cn | `zh_female_mizai_saturn_bigtts` |
| 鸡汤女 | Video dubbing | cn | `zh_female_jitangnv_saturn_bigtts` |
| 魅力女友 | Video dubbing | cn | `zh_female_meilinvyou_saturn_bigtts` |
| 流畅女声 | Video dubbing | cn | `zh_female_santongyongns_saturn_bigtts` |
| 儒雅逸辰 | Video dubbing | cn | `zh_male_ruyayichen_saturn_bigtts` |
| 知性灿灿 | Role play | cn | `saturn_zh_female_cancan_tob` |
| 可爱女生 | Role play | cn | `saturn_zh_female_keainvsheng_tob` |
| 调皮公主 | Role play | cn | `saturn_zh_female_tiaopigongzhu_tob` |
| 爽朗少年 | Role play | cn | `saturn_zh_male_shuanglangshaonian_tob` |
| 天才同桌 | Role play | cn | `saturn_zh_male_tiancaitongzhuo_tob` |
| 小何 | General | cn | `zh_female_xiaohe_uranus_bigtts` |
| 云舟 | General | cn | `zh_male_m191_uranus_bigtts` |
| 小天 | General | cn | `zh_male_taocheng_uranus_bigtts` |
| 儿童绘本 | Audiobook | cn | `zh_female_xueayi_saturn_bigtts` |
| Tim | General | en | `en_male_tim_uranus_bigtts` |
| Dacey | General | en | `en_female_dacey_uranus_bigtts` |
| Stokie | General | en | `en_female_stokie_uranus_bigtts` |

For `seed-tts-2.0`, prefer `*_uranus_bigtts` IDs first.
If you use `*_saturn_bigtts` or `saturn_*_tob`, verify account-level availability and resource matching in your tenant.

Common mismatch error:

- `55000000 resource ID is mismatched with speaker related resource`

## Flags

- `-text`: text to synthesize
- `-speaker`: speaker ID
- `-resource-id`: resource ID (default `seed-tts-2.0`)
- `-format`: audio format (`mp3` default)
- `-sample-rate`: sample rate (`24000` default)
- `-output`: output file path (`tts_v2_output.mp3` default)
- `-timeout-sec`: request timeout seconds (`120` default)
- `-auth-mode`: `auto|access|api` (`auto` default)
