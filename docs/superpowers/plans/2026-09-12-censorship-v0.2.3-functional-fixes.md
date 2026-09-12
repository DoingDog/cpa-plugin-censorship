# Censorship Plugin v0.2.3 Functional Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 changed-body header 不一致，并把 HTTP/WebSocket integration oracle 改为严格验证完整 JSON，发布 `v0.2.3`。

**Architecture:** 保持现有 request-only pipeline 和 native ABI v1。changed body 通过 ABI 已有 `ClearHeaders` 删除旧 entity/framing metadata；integration 只新增 stdlib typed JSON decoders，不修改 CPA core 或实际 response/stream path。

**Tech Stack:** Go 1.26、CGO c-shared、CLIProxyAPI v7.2.152/schema 5/native ABI v1、GJSON、Gorilla WebSocket、Go `encoding/json`/`net/http`/`testing`。

**Spec:** `docs/superpowers/specs/2026-09-12-censorship-v0.2.3-functional-fixes-design.md`

## Global Constraints

- 只修改 censorship plugin repository；绝不修改 module cache 中的 CLIProxyAPI 源码。
- 保留 `C.GoBytes`、ABI ownership 和 pointer-lifetime 行为。
- `words`、legacy YAML、`block -> strip -> obfs`、term 顺序和精确字节保持不变。
- selector 只能进入明确记录的 natural-language paths；禁止 recursive string walker。
- plugin capability 仍只有 `request_interceptor`；禁止新增 model response、SSE 或 WebSocket output rewrite。
- 每项 behavior change 必须先出现 focused RED test，再写最小实现，再得到 GREEN。
- 编码执行使用 Sonnet 1M/xhigh；plan、review 使用 Opus 1M/xhigh。
- 禁止读取任何旧 spec、plan、TDD 文档；只允许读取本计划和本次 spec。
- 不手工编辑 `.integration/` 或 `dist/`；它们只能由 runner/build/package 生成。
- 不添加依赖；`github.com/klauspost/compress` 已由 pinned CPA dependency graph 提供。

## File Map

- `main.go`：changed-body `RequestInterceptResponse` 增加精确 `ClearHeaders`。
- `main_test.go`：changed/no-op header contract 和 release compatibility token。
- `abi_cgo_test.go`：native ABI response lifetime test 的 changed-body header expectation。
- `integration/http_test.go`：strict HTTP error decoder、其 unit cases和转换后的 HTTP assertions。
- `integration/websocket_test.go`：strict typed Responses WebSocket event decoder 和所有 event assertions。
- `README.md`、`RELEASE_NOTES.md`：v0.2.3 verified behavior 与 compatibility。
- `docs/superpowers/specs/2026-09-12-censorship-v0.2.3-functional-fixes-design.md`、本文件：本次唯一 spec/plan。

Tasks 1-2 文件互斥，可并行 worktree 执行；Task 3 在 Tasks 1-2 commit 集成后执行，避免文档与 `main_test.go` 冲突。

---

### Task 1: Changed-body Headers 和 Strict HTTP Oracle

**Files:**
- Modify: `main.go:108-145`
- Modify: `main_test.go:194-228`
- Modify: `abi_cgo_test.go:210-240`
- Modify: `integration/http_test.go:1-124`

**Interfaces:**
- Consumes: `pluginapi.RequestInterceptResponse{Body, ClearHeaders}`；CLIProxyAPI 在 BeforeAuth callback 中传入已解码 `Body` 和原 request `Headers`。
- Produces: changed body response 的 `ClearHeaders == []string{"Content-Encoding", "Content-Length", "Transfer-Encoding"}`；`decodeCensorshipError([]byte) (integrationCensorshipErrorResponse, error)`。

- [ ] **Step 1: 在 `main_test.go` 写 changed/no-op header RED test**

增加 `TestBeforeAuthChangedBodyClearsStaleEntityHeaders`。使用 direct `pluginapi.RequestInterceptRequest`，避免修改现有 `callIntercept` helper：

```go
func TestBeforeAuthChangedBodyClearsStaleEntityHeaders(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\n")
	raw, err := json.Marshal(pluginapi.RequestInterceptRequest{
		RequestID:    "header-test",
		SourceFormat: "openai",
		Headers: http.Header{
			"Content-Encoding":  {"zstd"},
			"Content-Length":    {"123"},
			"Transfer-Encoding": {"chunked"},
			"Content-Type":      {"application/json"},
		},
		Body: []byte(`{"messages":[{"role":"user","content":"SECRET text"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var env pluginabi.Envelope
	decodeEnvelope(t, mustHandle(t, pluginabi.MethodRequestInterceptBefore, raw), &env)
	resp := decodeResult[pluginapi.RequestInterceptResponse](t, env)
	wantClear := []string{"Content-Encoding", "Content-Length", "Transfer-Encoding"}
	if !reflect.DeepEqual(resp.ClearHeaders, wantClear) {
		t.Fatalf("ClearHeaders = %#v, want %#v", resp.ClearHeaders, wantClear)
	}
	if got := string(resp.Body); got != `{"messages":[{"role":"user","content":" text"}]}` {
		t.Fatalf("body = %s", resp.Body)
	}

	noChange := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"user","content":"clean"}]}`))
	if noChange.ClearHeaders != nil {
		t.Fatalf("no-op ClearHeaders = %#v, want nil", noChange.ClearHeaders)
	}
}
```

Add `reflect` to imports.

- [ ] **Step 2: 运行 focused unit test 并记录 RED**

Run:

```powershell
go test . -run '^TestBeforeAuthChangedBodyClearsStaleEntityHeaders$' -count=1
```

Expected: FAIL because `resp.ClearHeaders` is nil.

- [ ] **Step 3: 在 `integration/http_test.go` 写 strict decoder RED test 和调用点**

Add the `encoding/json` import. Add:

```go
type integrationCensorshipErrorResponse struct {
	Error struct {
		Code string `json:"code"`
		Term string `json:"term"`
		Role string `json:"role"`
	} `json:"error"`
}

func decodeCensorshipError(body []byte) (integrationCensorshipErrorResponse, error) {
	var response integrationCensorshipErrorResponse
	err := json.Unmarshal(body, &response)
	return response, err
}
```

Add a table test covering complete JSON, missing closing braces, trailing non-whitespace, and numeric `error.term`. It must require only complete, correctly typed JSON to decode. Replace GJSON error field reads in `TestHTTPBlockIncludesTermAndRole`, `TestHTTPRejectsDuplicateJSONMembers`, and `TestLegacyCompletionsPromptUsesConvertedUserRole` with `decodeCensorshipError` and typed comparisons.

- [ ] **Step 4: 运行 local integration helper tests并记录 RED/GREEN boundary**

The decoder does not exist before Step 3, so compile failure is the first RED. After adding only the decoder and focused test, run:

```powershell
go test -tags=integration ./integration -run '^TestDecodeCensorshipError' -count=1
```

Expected: PASS for strict decoder cases. The production header test remains RED.

- [ ] **Step 5: 写最小 production fix**

Change only the changed-body branch in `main.go`:

```go
case len(result.Body) != 0:
	return okEnvelope(pluginapi.RequestInterceptResponse{
		Body:         result.Body,
		ClearHeaders: []string{"Content-Encoding", "Content-Length", "Transfer-Encoding"},
	})
```

Do not decode `request.Body` again and do not change no-op/termination branches.

- [ ] **Step 6: gofmt 并运行 Task 1 GREEN tests**

```powershell
gofmt -w main.go main_test.go integration/http_test.go
go test . -run '^TestBeforeAuthChangedBodyClearsStaleEntityHeaders$' -count=1
go test -tags=integration ./integration -run '^TestDecodeCensorshipError' -count=1
go test -tags=integration ./integration -run '^(TestHTTPBlockIncludesTermAndRole|TestHTTPRejectsDuplicateJSONMembers|TestLegacyCompletionsPromptUsesConvertedUserRole)$' -count=1
```

Expected: all PASS; changed/no-op response headers remain distinct and HTTP error assertions reject malformed JSON.

- [ ] **Step 7: Run the full unit suite and capture the dependent RED**

```powershell
go test ./...
```

Expected before updating `abi_cgo_test.go`: FAIL in `TestBorrowedABIResponseSurvivesHostRequestPoison` because its exact changed-body response still expects `clear_headers:null`.

- [ ] **Step 8: Update the native ABI lifetime test's exact response**

Keep the request-poisoning and byte-lifetime assertions unchanged. Update only the expected response:

```go
Result: pluginapi.RequestInterceptResponse{
	Body:         []byte(`{"messages":[{"role":"user","content":""}]}`),
	ClearHeaders: []string{"Content-Encoding", "Content-Length", "Transfer-Encoding"},
},
```

- [ ] **Step 9: Run dependent GREEN tests**

```powershell
gofmt -w abi_cgo_test.go
go test . -run '^TestBorrowedABIResponseSurvivesHostRequestPoison$' -count=1
go test ./...
```

Expected: all PASS.

- [ ] **Step 10: Commit Task 1**

```powershell
git add main.go main_test.go abi_cgo_test.go integration/http_test.go
git commit -m "fix: clear stale request body headers"
```

---

### Task 2: Strict Responses WebSocket Event Oracle

**Files:**
- Modify: `integration/websocket_test.go:1-303`

**Interfaces:**
- Consumes: Gorilla `ReadMessage()` logical message `(opcode int, payload []byte)`.
- Produces: `decodeResponsesWebSocketEvent(int, []byte) (responsesWebSocketEvent, error)` and strict use in block/status/completion tests.

- [ ] **Step 1: Add typed decoder contract test before decoder implementation**

Add `encoding/json` import, then add this shape and a table-driven test declaration. The first run may fail to compile until Step 3, which is the RED boundary:

```go
type responsesWebSocketEvent struct {
	Type   string `json:"type"`
	Status int    `json:"status"`
	Error  struct {
		Term json.RawMessage `json:"term"`
		Role json.RawMessage `json:"role"`
	} `json:"error"`
}
```

Cases:

```go
{name: "valid text", opcode: websocket.TextMessage, payload: `{"type":"response.completed","status":400}`, wantType: "response.completed", wantStatus: 400},
{name: "truncated", opcode: websocket.TextMessage, payload: `{"status":400`, wantErr: true},
{name: "string status", opcode: websocket.TextMessage, payload: `{"status":"400"}`, wantErr: true},
{name: "fractional status", opcode: websocket.TextMessage, payload: `{"status":400.9}`, wantErr: true},
{name: "binary JSON", opcode: websocket.BinaryMessage, payload: `{"status":400}`, wantErr: true},
```

- [ ] **Step 2: Run RED**

```powershell
go test -tags=integration ./integration -run '^TestDecodeResponsesWebSocketEvent' -count=1
```

Expected: compile FAIL because `decodeResponsesWebSocketEvent` is undefined.

- [ ] **Step 3: Implement the minimum strict decoder**

```go
func decodeResponsesWebSocketEvent(opcode int, payload []byte) (responsesWebSocketEvent, error) {
	if opcode != websocket.TextMessage {
		return responsesWebSocketEvent{}, fmt.Errorf("unexpected WebSocket message opcode %d", opcode)
	}
	var event responsesWebSocketEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return responsesWebSocketEvent{}, fmt.Errorf("decode Responses WebSocket event: %w", err)
	}
	return event, nil
}
```

Do not use `DisallowUnknownFields`.

- [ ] **Step 4: Replace tolerant assertions**

- In `TestTerminalWebSocketTimeoutIsNotPeerClose`, decode `(opcode, event)` and compare `decoded.Status`.
- In `TestResponsesWebSocketBlockReturnsStatus400ThenCloses`, decode and compare `decoded.Status`; require `len(decoded.Error.Term) == 0` and `len(decoded.Error.Role) == 0`.
- In `readUntilCompletedWithTimeout`, decode every `ReadMessage` result before appending/terminating; return the decode error; terminate only when `decoded.Type == "response.completed"`.

- [ ] **Step 5: gofmt and run GREEN/adjacent tests**

```powershell
gofmt -w integration/websocket_test.go
go test -tags=integration ./integration -run '^(TestDecodeResponsesWebSocketEvent|TestReadUntilCompletedReturnsOnDeadline|TestTerminalWebSocketTimeoutIsNotPeerClose)$' -count=1
go run ./.github/scripts/integration-runner.go
```

Expected: all PASS; existing enabled/disabled WebSocket message traces remain exactly equal.

- [ ] **Step 6: Commit Task 2**

```powershell
git add integration/websocket_test.go
git commit -m "test: validate complete WebSocket events"
```

---

### Task 3: Integrate Tasks 1-2 and Update v0.2.3 Documentation

**Files:**
- Modify: `README.md:104-150`
- Modify: `RELEASE_NOTES.md:1-16`
- Modify: `main_test.go:472-594`

**Interfaces:**
- Consumes: accepted and reviewed Tasks 1-2 commits plus the current spec.
- Produces: documentation tokens that match header/oracle behavior and compatibility tests that target v0.2.3.

- [ ] **Step 1: Integrate the two accepted implementation commits**

Cherry-pick the reviewed Task 1 and Task 2 commits onto `fix/v0.2.3-functional-audit` in numeric order, using the complete final commit hashes returned by their workers. Before cherry-pick, run `git show --stat --oneline` for each hash and verify that Task 1 owns `main.go`, `main_test.go`, `abi_cgo_test.go`, and `integration/http_test.go` while Task 2 owns only `integration/websocket_test.go`. Do not cherry-pick either rejected selector commit.

- [ ] **Step 2: Update version-sensitive tests first**

In `TestReleaseNotesCompatibilityTargetsCurrentVersion`, require:

```go
if !bytes.Contains(raw, []byte("v0.2.3 targets CLIProxyAPI v7.2.152")) {
	t.Fatal("RELEASE_NOTES.md does not target CLIProxyAPI v7.2.152 for v0.2.3")
}
if bytes.Contains(raw, []byte("v0.2.2 targets CLIProxyAPI v7.2.152")) {
	t.Fatal("RELEASE_NOTES.md still targets CLIProxyAPI v7.2.152 for v0.2.2")
}
```

Extend `TestDocumentationListsConfigAndLimits` only as needed to require the three cleared header names and the changed-body/no-op distinction. Do not add OpenAI or Claude selector tokens because provider-contract recovery rejected those findings.

- [ ] **Step 3: Run docs RED**

```powershell
go test . -run '^(TestDocumentationListsConfigAndLimits|TestReleaseNotesCompatibilityTargetsCurrentVersion)$' -count=1
```

Expected: FAIL until README and release notes are updated.

- [ ] **Step 4: Update README behavior exactly**

- In the ABI/integration section, state that changed request bodies clear `Content-Encoding`, `Content-Length`, and `Transfer-Encoding`; no-op requests preserve headers.
- Preserve existing request-only and output/stream non-modification claims.
- Do not document behavior for rejected scalar `mcp_call.error` or non-empty `search_result.title` findings.

- [ ] **Step 5: Replace release notes heading and fixes**

Use:

```markdown
# Censorship v0.2.3

## v0.2.3 fixes

- Clears stale `Content-Encoding`, `Content-Length`, and `Transfer-Encoding` whenever censorship replaces a decoded request body.
- Strictly validates complete, correctly typed HTTP and Responses WebSocket JSON in the integration oracle.
```

Change the compatibility sentence to `v0.2.3 targets CLIProxyAPI v7.2.152`; update the copied ABI/header and integration statements so `README.md` and `RELEASE_NOTES.md` remain aligned. Do not include either rejected selector claim.

- [ ] **Step 6: Run docs GREEN and full unit tests**

```powershell
gofmt -w main_test.go
go test . -run '^(TestDocumentationListsConfigAndLimits|TestReleaseNotesCompatibilityTargetsCurrentVersion)$' -count=1
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit Task 3**

```powershell
git add README.md RELEASE_NOTES.md main_test.go
git commit -m "docs: prepare censorship v0.2.3"
```

---

### Task 4: Spec Compliance Review and Verification

**Files:**
- Review only: every path changed since `b6d0c92787fea81a52b69549dd00b3b7566200a6`
- Generated only: `.integration/**`, `dist/**`

**Interfaces:**
- Consumes: integrated feature branch.
- Produces: one Opus 1M/xhigh review result, verified build/package outputs, clean tracked worktree.

- [ ] **Step 1: Run one non-duplicated review pass**

Use one Opus 1M/xhigh reviewer. It must read only the current spec, current plan, and final diff; verify behavior, ABI, exact paths, header semantics, JSON strictness, test coverage, and scope. It must not repeat the original whole-repository scan. Apply only confirmed findings with focused TDD; if changes are made, rerun the smallest affected tests.

- [ ] **Step 2: Run verification commands with fresh output**

PowerShell equivalents are acceptable when Git Bash cannot propagate Windows Go environment variables:

```powershell
go test ./...
go test -race ./...
go vet ./...
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
go run ./.github/scripts/integration-runner.go
$env:CGO_ENABLED='1'; $env:GOOS=(go env GOHOSTOS); $env:GOARCH=(go env GOHOSTARCH); go build -trimpath -buildmode=c-shared -ldflags='-s -w -X main.pluginVersion=0.2.3' -o 'dist/windows_amd64/censorship.dll' .
go run ./.github/scripts/package-release.go -dist dist -out dist -version v0.2.3
```

Expected: every command exits 0. Inspect generated `.zip.sha256` and `checksums.txt` through package tests; do not hand-edit them.

- [ ] **Step 3: Run focused regressions explicitly**

```powershell
go test . -run '^(TestBeforeAuthChangedBodyClearsStaleEntityHeaders|TestDocumentationListsConfigAndLimits|TestReleaseNotesCompatibilityTargetsCurrentVersion)$' -count=1
go test -tags=integration ./integration -run '^(TestDecodeCensorshipError|TestDecodeResponsesWebSocketEvent)' -count=1
```

Expected: PASS.

- [ ] **Step 4: Verify repository scope and generated-file discipline**

```powershell
git status --short
git diff --check b6d0c92787fea81a52b69549dd00b3b7566200a6..HEAD
git diff --name-only b6d0c92787fea81a52b69549dd00b3b7566200a6..HEAD
git log --oneline --decorate b6d0c92787fea81a52b69549dd00b3b7566200a6..HEAD
```

Expected: only planned tracked files changed; no `.integration/` or `dist/` tracked; no whitespace errors; feature branch clean.

---

### Task 5: Merge、Tag 和 Push

**Files:**
- Git refs only; no new source edits.

**Interfaces:**
- Consumes: clean, reviewed, verified `fix/v0.2.3-functional-audit` branch.
- Produces: local `main` containing all commits, annotated `v0.2.3`, pushed `origin/main` and `origin/v0.2.3` that triggers `.github/workflows/build.yml`.

- [ ] **Step 1: Confirm remote and release preconditions**

```powershell
git fetch origin --tags
git status --short --branch
git rev-parse origin/main
git tag --list v0.2.3
git remote get-url origin
```

Expected: tracked worktree clean; `origin/main` has no unintegrated commit; `v0.2.3` absent; origin is `https://github.com/DoingDog/cpa-plugin-censorship.git`.

- [ ] **Step 2: Merge into local main**

```powershell
git switch main
git merge --no-ff fix/v0.2.3-functional-audit -m "merge: release censorship v0.2.3"
```

Run `go test ./...` once on merged `main`; expected PASS.

- [ ] **Step 3: Create annotated patch tag**

```powershell
git tag -a v0.2.3 -m "Censorship v0.2.3"
```

- [ ] **Step 4: Push main and tag**

```powershell
git push origin main
git push origin v0.2.3
```

Do not force push and do not bypass hooks or signing.

- [ ] **Step 5: Verify workflow trigger and refs**

```powershell
git ls-remote --heads origin main
git ls-remote --tags origin v0.2.3
git status --short --branch
```

If GitHub CLI is authenticated, inspect the newly triggered `build.yml` run once with `gh run list --workflow build.yml --limit 3`; do not wait indefinitely. Report the run URL/status exactly. A failed remote workflow is not reported as success; finish every local task and state the remote failure output.
