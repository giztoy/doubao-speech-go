# AGENTS.md

## Repository Scope

- This repository is a **pure Go package**.
- **Bazel is not used** (no Bazel build/test targets).
- Changes are merged directly after local development, **without a PR workflow**.
- There is currently no CI gate required before merge.

## Review Strategy (Current)

This repository follows a **local self-check + direct merge** model.
There is no separate Code Review approval flow.

### Required Local Checks

1. Code formatting
   - `gofmt -w` (or equivalent Go formatting)
2. Unit tests
   - `go test ./...`
3. Example usability
   - API examples under `examples/` must run with the required environment variables.

### Change Principles

- Every feature change must include runnable code. No TODO/placeholder implementations.
- Use a consistent error structure (prefer returning errors with `code/message`).
- Keep `examples/` organized as **one subdirectory per API**.

## Directory Convention (Example)

```text
examples/
  asr_v2_sauc_ws/   # ASR V2 SAUC WebSocket API example
```

When adding new APIs, follow the same rule and create a dedicated subdirectory under `examples/`.
