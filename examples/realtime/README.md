# realtime example

This directory contains a single example: **multi-turn realtime dialogue**.

## Example goals

The example should cover the full session lifecycle and multi-turn capabilities:

1. Open connection and start session
2. Send/receive turn 1 (streaming events + final event)
3. Update conversation history
4. Update system prompt
5. Update generation props (for example: `temperature`, `top_p`, `max_tokens`)
6. Continue with turn 2 and verify updated settings are effective
7. Optional interrupt + idempotent close (`Close` called twice)

## Run (placeholder)

```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_API_KEY=<your_api_key> \
go run ./examples/realtime
```

> Note: Runtime flags (for example `-speaker`) must match what `main.go` actually supports.

## Voice list for Realtime model (`volc.speech.dialog`)

Last updated: **2026-03-01**

| voice_id | Language | Gender / Style | Source |
|---|---|---|---|
| `zh_female_vv_jupiter_bigtts` | zh-CN | female / realtime conversation | realtime model config (default) |
| `zh_male_yunzhou_jupiter_bigtts` | zh-CN | male / realtime conversation | realtime model config |
| `zh_female_cancan_jupiter_bigtts` | zh-CN | female / lively | voice list example |
| `BV700_streaming_jupiter_bigtts` | zh-CN | female / classic Càncàn style | voice list example |
| `zh_female_qingxin_moon_bigtts` | zh-CN | female / fresh | voice list example |
| `zh_female_shuangkuaisisi_moon_bigtts` | zh-CN | female / energetic | voice list example |

The list above is the current set used in upstream docs/examples/configurations.
For your own account, validate availability in console and by a quick runtime smoke test.

## References

- Realtime API access documentation:
  - https://www.volcengine.com/docs/6561/1594356
- TTS voice catalog:
  - https://www.volcengine.com/docs/6561/1257544
- Speaker listing API (console side):
  - https://www.volcengine.com/docs/6561/2160690

> Availability may vary by account permissions. If you get speaker permission errors, verify your console entitlement and latest official documentation.
