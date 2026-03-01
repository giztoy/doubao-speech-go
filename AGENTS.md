# AGENTS.md

Agent operating guide for this repository.

## Repository profile
- Language: Go only.
- Module: `github.com/giztoy/doubao-speech-go`.
- Go version: `1.26`.
- Build system: no Bazel.
- Workflow: local development + direct merge.
- No mandatory PR or CI gate before merge.

## Cursor/Copilot rule files
Checked and currently **not present**:
- `.cursor/rules/**`
- `.cursorrules`
- `.github/copilot-instructions.md`
If these files are added later, they become additional constraints.

## Commands: build / lint / test
Run from repository root.

### Format
```bash
gofmt -w *.go internal/auth/*.go internal/protocol/*.go internal/transport/*.go internal/util/*.go examples/asr_v2_sauc_ws/*.go
```

### Build
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

### Run all tests
```bash
go test ./...
```

### Run tests in one package
```bash
go test . -v
go test ./internal/protocol -v
```

### Run a single test (important)
```bash
go test . -run TestOpenStreamSessionAuthFailureErrorStructure -count=1 -v
go test ./internal/protocol -run TestParseServerFrameGzip -count=1 -v
```

### Run a single subtest
```bash
go test ./... -run 'TestName/SubCase' -count=1 -v
```

### Disable test cache
```bash
go test ./... -count=1
```

### Race test (recommended for concurrency edits)
```bash
go test ./... -race
```

## Example commands
Primary example directory:
```text
examples/asr_v2_sauc_ws/
```

Run with API key:
```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_API_KEY=<your_api_key> \
go run ./examples/asr_v2_sauc_ws
```

Run with Access Key:
```bash
DOUBAO_APP_ID=<your_app_id> \
DOUBAO_ACCESS_KEY=<your_access_key> \
go run ./examples/asr_v2_sauc_ws -resource-id volc.bigasr.sauc.duration
```

Example behavior:
- If `-audio` is omitted, embedded sample audio is used.
- Embedded fixture: `sample_zh_16k.pcm`.
- Example currently supports `pcm` format only.
- In embedded mode, sample rate is forced to `16000`.

## Git LFS requirements
The embedded fixture is tracked by Git LFS:
- `examples/asr_v2_sauc_ws/sample_zh_16k.pcm`

After cloning, run:
```bash
git lfs pull
```

If LFS content is missing, embedded mode may detect an LFS pointer payload and exit with guidance.

## Code style guidelines

### Formatting and imports
- Always run `gofmt -w` before finishing.
- Keep imports gofmt-ordered.
- Prefer stdlib imports first, then third-party, then module-internal imports.

### Naming
- Exported names: `PascalCase`.
- Unexported names: `camelCase`.
- File names: lowercase; use underscores only when helpful.
- Keep common acronyms conventional (`ID`, `URL`, `API`).

### Types and API design
- Keep public types stable and explicit.
- Continue using option-pattern config for client/service APIs.
- Keep implementation-only code under `internal/`.
- Keep one API example per subdirectory under `examples/`.

### Context usage
- Put `context.Context` first for I/O/network APIs.
- Respect cancellation and timeout paths.

### Error handling
- Return errors; avoid panic for normal control flow.
- Wrap errors with operation context.
- Keep API-facing errors aligned with `error.go` (`code/message`, plus reqid/trace/log when available).
- Do not silently swallow network/protocol errors.

### Streaming behavior
- Keep result/error delivery deterministic.
- Keep final-frame semantics explicit and regression-tested.
- Ensure close/cleanup paths are idempotent.

### Testing
- Keep tests deterministic and focused.
- Use clear failure messages with expected vs actual values.
- Add regression tests for protocol flags, auth failures, and edge cases.

### Language in code
- Keep Go code comments and CLI/user-facing strings in English.

## Security and secrets
- Never commit secrets (keys, tokens, credentials).
- Never print secrets in logs or errors.
- Use environment variables for local credentials.

## Commit guidance
- Commit only when explicitly requested.
- Preferred commit message format: `{module/submodule}: {subject}`.
- Subject should start with lowercase.

## Minimal completion checklist
Before declaring work complete:
1. Run `gofmt -w` on changed Go files.
2. Run `go test ./...`.
3. Run `go build ./...`.
4. Run affected example(s) when behavior changed.
5. Verify no secrets were introduced.

If all checks pass, the change is ready for direct merge in this repo model.
