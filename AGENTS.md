# AGENTS.md

Agent guide for `github.com/giztoy/doubao-speech-go`.

## 1) Repository profile
- Language: Go only
- Module: `github.com/giztoy/doubao-speech-go`
- Go version: `1.26` (`go.mod`)
- Build/test toolchain: `go build`, `go test`, `go vet`
- Workflow: local development + direct merge
- No mandatory PR/CI gate before merge in this repo model

## 2) Cursor/Copilot rules
Checked and currently not present:
- `.cursor/rules/**`
- `.cursorrules`
- `.github/copilot-instructions.md`
If these files are added later, they become hard constraints.

## 3) Layout snapshot
- Public SDK APIs: root `*.go` (`client.go`, `asr_v2.go`, `tts_v2.go`, `tts_v2_ws.go`, `realtime.go`, `voice_clone.go`)
- Internal details: `internal/auth`, `internal/transport`, `internal/protocol`, `internal/util`
- Examples: `examples/asr_v2_sauc_ws`, `examples/tts_v2/http_stream`, `examples/tts_v2/websocket`, `examples/realtime`, `examples/voice_clone`
- Docs: `docs/*.md`

## 4) Setup notes
Run commands from repository root.
Git LFS is required for embedded fixture:
```bash
git lfs pull
```
LFS file:
- `examples/asr_v2_sauc_ws/sample_zh_16k.pcm`

## 5) Build / lint / test commands

### Format
```bash
gofmt -w *.go internal/auth/*.go internal/protocol/*.go internal/transport/*.go internal/util/*.go examples/asr_v2_sauc_ws/*.go examples/realtime/*.go examples/tts_v2/http_stream/*.go examples/tts_v2/websocket/*.go examples/voice_clone/*.go
```

### Build all
```bash
go build ./...
```

### Lint / static checks
Required:
```bash
go vet ./...
```
Optional (if installed):
```bash
staticcheck ./...
```

### Test all
```bash
go test ./...
```

### Test one package
```bash
go test . -v
go test ./internal/protocol -v
```

### Run a single test (important)
Root package examples:
```bash
go test . -run TestVoiceCloneUploadAndWaitSuccess -count=1 -v
go test . -run TestRealtimeBackpressureReturnsError -count=1 -v
go test . -run TestOpenStreamSessionAuthFailureErrorStructure -count=1 -v
```
Internal package example:
```bash
go test ./internal/protocol -run TestParseServerFrameGzip -count=1 -v
```

### Run a single subtest
```bash
go test ./... -run 'TestName/SubCase' -count=1 -v
```

### Disable test cache / race mode
```bash
go test ./... -count=1
go test ./... -race
```

## 6) Example run commands
See API-specific docs for full env/flags:
- `docs/asr_v2_sauc_ws.md`
- `docs/tts_v2_http_stream.md`
- `docs/tts_v2_websocket.md`
- `docs/voice_clone.md`

Quick smoke test:
```bash
DOUBAO_APP_ID=<your_app_id> DOUBAO_API_KEY=<your_api_key> go run ./examples/asr_v2_sauc_ws
```

## 7) Code style guidelines

### Formatting and imports
- Always run `gofmt -w` before finishing
- Keep imports gofmt-ordered
- Import grouping: stdlib, third-party, module-internal

### Naming and files
- Exported identifiers: `PascalCase`
- Unexported identifiers: `camelCase`
- Acronyms use Go conventions: `ID`, `URL`, `API`, `HTTP`, `WS`
- File names lowercase; use underscores only when useful

### API and type design
- Keep public API stable and explicit
- Prefer option pattern via `NewClient(..., opts...)`
- Put implementation-only logic in `internal/`

### Context, cancellation, and concurrency
- Put `context.Context` first in I/O/network APIs
- Respect cancellation/timeouts in HTTP and WS flows
- Ensure close paths are idempotent and deterministic

### Error handling
- Return errors; do not use panic for expected failures
- Wrap with operation context (`wrapError`) where useful
- Keep API-facing errors aligned with `error.go` (`code/message` + optional `reqid/trace/log/http_status`)
- Never swallow network/protocol errors silently

### Streaming / async behavior
- Keep event/chunk ordering deterministic
- Make final-frame/final-event semantics explicit and regression-tested
- For async tasks, define terminal states and unknown-state behavior clearly

### Testing standards
- Prefer table-driven tests for validation/mapping logic
- Keep tests deterministic and focused
- Use clear failure messages (expected vs actual)

### Language policy
- Code comments and user-facing CLI strings should be English
- Docs/examples must match actual API behavior and flags

## 8) Security and secrets
- Never commit tokens/keys/credentials
- Never print secrets in logs/errors
- Use environment variables for local credentials

## 9) Commit guidance
- Commit only when explicitly requested
- Preferred message format: `{module/submodule}: {subject}`
- Subject should start with lowercase

## 10) Minimal completion checklist
1. `gofmt -w` on changed Go files
2. `go test ./...`
3. `go build ./...`
4. `go vet ./...`
5. Run affected examples when behavior changes
6. Verify no secrets were introduced

If all checks pass, the change is ready for direct merge.
