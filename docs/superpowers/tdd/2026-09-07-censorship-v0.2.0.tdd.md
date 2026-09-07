# Censorship v0.2.0 TDD Record

## Baseline, 2026-09-07

- `make integration`: PASS; the pinned CPA HTTP, SSE, reload, Responses WebSocket, and ABI scenarios completed.
- `go vet ./...`: PASS.
- `make test`: FAIL only at `TestDocumentationListsConfigAndLimits`; `RELEASE_NOTES.md` lacks the exact expected configuration block.
- `make race`: FAIL only at the same documentation assertion.
- Baseline benchmark output: `%TEMP%/censorship-v020-before-bench.txt`.

Every task below records its literal RED, GREEN, and REFACTOR command, exit code, and decisive output.

## Task 2: Upgrade CPA SDK and Integration Host Atomically, 2026-09-07

### RED

- `go test . -run '^TestSupportedPluginSchema$' -count=1`
  - exit code: `1`
  - decisive output: `main_test.go:21: plugin schema = 4, want 5`
- `go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -run '^TestPinnedCPARevision$' -count=1`
  - exit code: `1`
  - decisive output: `integration-runner_test.go:44: cpaSHA = "81e1b5374f99c212f196f34956eeed964a46b8fa", want "c76dfd4e0edabab9000628b1560ab8ab379eadb8"`

### GREEN

- `go get github.com/router-for-me/CLIProxyAPI/v7@v7.2.152`
  - exit code: `0`
  - decisive output: `go: upgraded github.com/router-for-me/CLIProxyAPI/v7 v7.2.147-0.20260831020448-81e1b5374f99 => v7.2.152`
- `go mod download github.com/gorilla/websocket@v1.5.3`
  - exit code: `0`
  - decisive output: no output.
- `go mod tidy`
  - exit code: `0`
  - decisive output: no output.
- `go test . -run '^TestSupportedPluginSchema$' -count=1`
  - exit code: `0`
  - decisive output: `ok   github.com/DoingDog/cpa-plugin-censorship  0.050s`
- `go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -run '^TestPinnedCPARevision$' -count=1`
  - exit code: `0`
  - decisive output: `ok   command-line-arguments  0.034s`
- `go list -m -json github.com/router-for-me/CLIProxyAPI/v7`
  - exit code: `0`
  - decisive output: `"Version": "v7.2.152"`, `"Sum": "h1:FkvGzpOCvuDGswaOyoVfbY5Ua7OlP/wMXw3agiNMUQI="`, and `"GoModSum": "h1:lTHwMAGajc1wKGQiRtDvYbwV0FWsM7sy+N0ZU5/gxJQ="`.
- `go mod verify`
  - exit code: `0`
  - decisive output: `all modules verified`
- `go test . -run '^(TestPluginRegistration|TestHandlePluginRegister|TestHandlePluginReconfigure)' -count=1`
  - exit code: `0`
  - decisive output: `ok   github.com/DoingDog/cpa-plugin-censorship  0.054s [no tests to run]`
- `go vet ./...`
  - exit code: `0`
  - decisive output: no output.

### REFACTOR

- No production refactor was needed.
- `git diff --check`
  - exit code: `0`
  - decisive output: no whitespace errors; Git printed only CRLF conversion warnings.
