# Censorship Plugin v0.2.3 Functional Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复两个 provider selector 漏洞、changed-body header 不一致，并把 HTTP/WebSocket integration oracle 改为严格验证完整 JSON，发布 `v0.2.3`。

**Architecture:** 保持现有 request-only pipeline 和 native ABI v1。复用 selector span helpers 处理两个 provider schema 差异；changed body 通过 ABI 已有 `ClearHeaders` 删除旧 entity/framing metadata；integration 只新增 typed JSON decoders 和必要的 request-header capture，不修改 CPA core 或实际 response/stream path。

**Tech Stack:** Go 1.26、CGO c-shared、CLIProxyAPI v7.2.152/schema 5/native ABI v1、GJSON、Gorilla WebSocket、klauspost zstd、Go `encoding/json`/`net/http`/`testing`。

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
- `selectors_openai.go`、`selectors_openai_test.go`：OpenAI Responses string `mcp_call.error`。
- `selectors_claude.go`、`selectors_claude_test.go`：Claude required-nonempty `search_result.title`。
- `integration/http_test.go`：strict HTTP error decoder、其 unit cases、zstd end-to-end request。
- `integration/harness_test.go`：mock upstream 同步 capture body 和 cloned headers。
- `integration/websocket_test.go`：strict typed Responses WebSocket event decoder 和所有 event assertions。
- `README.md`、`RELEASE_NOTES.md`：v0.2.3 verified behavior 与 compatibility。
- `docs/superpowers/specs/2026-09-12-censorship-v0.2.3-functional-fixes-design.md`、本文件：本次唯一 spec/plan。

Tasks 1-4 文件互斥，可并行 worktree 执行；Task 5 在 Tasks 1-4 commit 集成后执行，避免文档与 `main_test.go` 冲突。

---

### Task 1: Changed-body Headers、Strict HTTP Oracle 和 zstd Integration

**Files:**
- Modify: `main.go:108-145`
- Modify: `main_test.go:194-228`
- Modify: `integration/http_test.go:1-51`
- Modify: `integration/harness_test.go:375-489`

**Interfaces:**
- Consumes: `pluginapi.RequestInterceptResponse{Body, ClearHeaders}`；CLIProxyAPI 在 BeforeAuth callback 中传入已解码 `Body` 和原 request `Headers`。
- Produces: changed body response 的 `ClearHeaders == []string{"Content-Encoding", "Content-Length", "Transfer-Encoding"}`；`decodeCensorshipError([]byte) (integrationCensorshipErrorResponse, error)`；`(*upstreamCapture).lastHeader() http.Header`。

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

Add `encoding/json`, `net/http`, and `github.com/klauspost/compress/zstd` imports as later steps need them. Add:

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

- [ ] **Step 5: 在 `integration/harness_test.go` 增加 header capture**

Change capture state and recorder without altering synchronization:

```go
type upstreamCapture struct {
	mu       sync.Mutex
	arrivals int
	requests [][]byte
	headers  []http.Header
}

func (c *upstreamCapture) record(body []byte, header http.Header) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, bytes.Clone(body))
	c.headers = append(c.headers, header.Clone())
}

func (c *upstreamCapture) lastHeader() http.Header {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.headers) == 0 {
		return nil
	}
	return c.headers[len(c.headers)-1].Clone()
}
```

Change the only call from `capture.record(body)` to `capture.record(body, r.Header)`.

- [ ] **Step 6: 在 `integration/http_test.go` 写 zstd RED integration**

Add `TestHTTPRewrittenZstdRequestClearsContentEncoding`:

```go
func TestHTTPRewrittenZstdRequestClearsContentEncoding(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: strip\nwords: [SECRET]\n")
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	compressed := encoder.EncodeAll(chatBody("SECRET input", false), nil)
	encoder.Close()

	request, err := http.NewRequest(http.MethodPost, cpa.baseURL+"/v1/chat/completions", bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+downstreamKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Encoding", "zstd")
	response, err := integrationHTTPClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.StatusCode, responseBody)
	}
	if got := gjson.GetBytes(upstream.lastRequest(), "messages.0.content").String(); got != " input" {
		t.Fatalf("upstream content = %q, body = %s", got, upstream.lastRequest())
	}
	if got := upstream.lastHeader().Get("Content-Encoding"); got != "" {
		t.Fatalf("upstream Content-Encoding = %q, want empty", got)
	}
}
```

- [ ] **Step 7: 运行 real integration test 并记录 RED**

Use the repository integration runner so the pinned CPA and native library are rebuilt:

```powershell
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
go run ./.github/scripts/integration-runner.go -run '^TestHTTPRewrittenZstdRequestClearsContentEncoding$'
```

If runner has no `-run` option, run `go run ./.github/scripts/integration-runner.go` and retain the named failing test output. Expected: FAIL because upstream still receives `Content-Encoding: zstd`.

- [ ] **Step 8: 写最小 production fix**

Change only the changed-body branch in `main.go`:

```go
case len(result.Body) != 0:
	return okEnvelope(pluginapi.RequestInterceptResponse{
		Body:         result.Body,
		ClearHeaders: []string{"Content-Encoding", "Content-Length", "Transfer-Encoding"},
	})
```

Do not decode `request.Body` again and do not change no-op/termination branches.

- [ ] **Step 9: gofmt 并运行 Task 1 GREEN tests**

```powershell
gofmt -w main.go main_test.go integration/http_test.go integration/harness_test.go
go test . -run '^TestBeforeAuthChangedBodyClearsStaleEntityHeaders$' -count=1
go test -tags=integration ./integration -run '^TestDecodeCensorshipError' -count=1
go run ./.github/scripts/integration-runner.go
```

Expected: all PASS; zstd upstream body is plaintext transformed JSON and header is absent.

- [ ] **Step 10: Commit Task 1**

```powershell
git add main.go main_test.go integration/http_test.go integration/harness_test.go
git commit -m "fix: clear stale request body headers"
```

---

### Task 2: OpenAI Responses string `mcp_call.error`

**Files:**
- Modify: `selectors_openai_test.go:235-337`
- Modify: `selectors_openai.go:117-128`

**Interfaces:**
- Consumes: `appendStringSpan(*[]textSpan, gjson.Result, "tool", scopeSet)` and existing `collectOpenAIResponsesToolOutput` dispatch.
- Produces: string `mcp_call.error` selected as tool text; structured supported error objects remain unchanged.

- [ ] **Step 1: 扩展现有 MCP row test**

In the `name: "mcp call"` row, append a string-error item while retaining machine siblings:

```json
{"type":"mcp_call","id":"SECRET string error id","name":"SECRET string error name","arguments":"SECRET string error arguments","server_label":"SECRET string error server","error":"SECRET string error"}
```

Add only this expected replacement:

```go
{Before: `"SECRET string error"`, After: `" string error"`},
```

Existing `/tool disabled` subtest must continue expecting nil body.

- [ ] **Step 2: Run RED**

```powershell
go test . -run '^TestOpenAIResponsesOutputRoleGate/mcp_call' -count=1
```

Expected: tool-enabled body still contains `SECRET string error`.

- [ ] **Step 3: 写最小 selector fix**

Immediately after `errorValue := item.Get("error")`, add:

```go
appendStringSpan(spans, errorValue, "tool", roles)
```

Keep the object `type` switch and `message` selection intact. `appendStringSpan` already ignores non-string values.

- [ ] **Step 4: Run GREEN and adjacent OpenAI tests**

```powershell
gofmt -w selectors_openai.go selectors_openai_test.go
go test . -run '^TestOpenAI' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 2**

```powershell
git add selectors_openai.go selectors_openai_test.go
git commit -m "fix: inspect OpenAI MCP string errors"
```

---

### Task 3: Claude required `search_result.title`

**Files:**
- Modify: `selectors_claude_test.go:226-283`
- Modify: `selectors_claude.go:95-121`

**Interfaces:**
- Consumes: `appendNonEmptyStringSpan` and shared `appendClaudeResultText` used by direct user and nested tool search results.
- Produces: full strip of a present string `search_result.title` returns existing `censorship_invalid_request`; partial strip stays valid.

- [ ] **Step 1: Add full-strip RED row**

Add to `TestClaudeMinimumLengthFieldsRejectFullStrip`:

```go
{name: "search result title", body: []byte(`{"messages":[{"role":"user","content":[{"type":"search_result","title":"SECRET","source":"search","content":[{"type":"text","text":"clean"}]}]}]}`)},
```

Existing `TestClaudeResultText` already proves a partial title strip remains non-empty and is forwarded.

- [ ] **Step 2: Run RED**

```powershell
go test . -run '^TestClaudeMinimumLengthFieldsRejectFullStrip/search_result_title$' -count=1
```

Expected: FAIL because current response is non-terminating with `"title":""`.

- [ ] **Step 3: Replace the one incorrect helper call**

```go
case "search_result":
	appendNonEmptyStringSpan(spans, block.Get("title"), role, roles)
```

No other Claude field changes.

- [ ] **Step 4: Run GREEN and adjacent Claude tests**

```powershell
gofmt -w selectors_claude.go selectors_claude_test.go
go test . -run '^TestClaude' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 3**

```powershell
git add selectors_claude.go selectors_claude_test.go
git commit -m "fix: preserve required Claude search titles"
```

---

### Task 4: Strict Responses WebSocket Event Oracle

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

- [ ] **Step 6: Commit Task 4**

```powershell
git add integration/websocket_test.go
git commit -m "test: validate complete WebSocket events"
```

---

### Task 5: Integrate Tasks 1-4 and Update v0.2.3 Documentation

**Files:**
- Modify: `README.md:104-150`
- Modify: `RELEASE_NOTES.md:1-16`
- Modify: `main_test.go:472-594`

**Interfaces:**
- Consumes: committed Tasks 1-4 behavior and current spec.
- Produces: documentation tokens that match selector/header/oracle behavior and compatibility test that targets v0.2.3.

- [ ] **Step 1: Integrate all four implementation commits**

Cherry-pick Tasks 1-4 commits onto `fix/v0.2.3-functional-audit`，按 1、2、3、4 顺序使用四个 worker 返回的完整 commit hash 作为一次 `git cherry-pick` 的参数。四个域没有重叠 production files：Task 1 owns `main*` and HTTP/harness，Task 2 owns OpenAI，Task 3 owns Claude，Task 4 owns WebSocket。执行前逐个运行 `git show --stat --oneline` 并传入实际 hash，核对返回 hash 的文件边界，再运行 cherry-pick。

- [ ] **Step 2: Update version-sensitive test first**

In `TestReleaseNotesCompatibilityTargetsCurrentVersion`, require:

```go
if !bytes.Contains(raw, []byte("v0.2.3 targets CLIProxyAPI v7.2.152")) {
	t.Fatal("RELEASE_NOTES.md does not target CLIProxyAPI v7.2.152 for v0.2.3")
}
if bytes.Contains(raw, []byte("v0.2.2 targets CLIProxyAPI v7.2.152")) {
	t.Fatal("RELEASE_NOTES.md still targets CLIProxyAPI v7.2.152 for v0.2.2")
}
```

Add required documentation tokens for string `mcp_call.error`, `search_result.title` non-empty behavior, and clearing stale request body headers.

- [ ] **Step 3: Run docs RED**

```powershell
go test . -run '^(TestDocumentationListsConfigAndLimits|TestReleaseNotesCompatibilityTargetsCurrentVersion)$' -count=1
```

Expected: FAIL until README and release notes are updated.

- [ ] **Step 4: Update README behavior exactly**

- In OpenAI explicit paths, state that scalar `mcp_call.error` and supported structured `mcp_call.error.message` are selected as `tool`.
- In invalid rewrites, add present `search_result.title` to the Claude fields that may not become empty.
- In ABI/integration section, state that changed request bodies clear `Content-Encoding`, `Content-Length`, and `Transfer-Encoding`; no-op requests preserve headers.
- Preserve request-only and output/stream non-modification claims.

- [ ] **Step 5: Replace release notes heading and fixes**

Use:

```markdown
# Censorship v0.2.3

## v0.2.3 fixes

- Inspects documented scalar OpenAI Responses `mcp_call.error` text under canonical `tool` scope while preserving machine siblings and supported structured errors.
- Rejects rewrites that empty required Claude `search_result.title` fields.
- Clears stale `Content-Encoding`, `Content-Length`, and `Transfer-Encoding` whenever censorship replaces a decoded request body.
- Strictly validates complete, correctly typed HTTP and Responses WebSocket JSON in the integration oracle.
```

Change compatibility sentence to `v0.2.3 targets CLIProxyAPI v7.2.152`; update the copied explicit-path, invalid-field, ABI/header, and integration statements so `README.md` and `RELEASE_NOTES.md` remain aligned.

- [ ] **Step 6: Run docs GREEN and full unit tests**

```powershell
gofmt -w main_test.go
go test . -run '^(TestDocumentationListsConfigAndLimits|TestReleaseNotesCompatibilityTargetsCurrentVersion)$' -count=1
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit Task 5**

```powershell
git add README.md RELEASE_NOTES.md main_test.go
git commit -m "docs: prepare censorship v0.2.3"
```

---

### Task 6: Spec Compliance Review and Verification

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

- [ ] **Step 3: Run selector and oracle focused regressions explicitly**

```powershell
go test . -run '^(TestBeforeAuthChangedBodyClearsStaleEntityHeaders|TestOpenAIResponsesOutputRoleGate|TestClaudeMinimumLengthFieldsRejectFullStrip|TestDocumentationListsConfigAndLimits|TestReleaseNotesCompatibilityTargetsCurrentVersion)$' -count=1
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

### Task 7: Merge、Tag 和 Push

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
