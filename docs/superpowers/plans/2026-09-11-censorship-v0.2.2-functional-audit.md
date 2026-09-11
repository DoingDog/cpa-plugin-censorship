# Censorship v0.2.2 Functional Audit Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all verified v0.2.1 functional audit defects and release v0.2.2 without changing CPA core or broadening selector policy.

**Architecture:** Extend only existing explicit provider selectors, preserve the single gjson/raw-span transform pipeline, reuse existing package alias validation, and restore Make's missing-version default by reordering existing assignments. Keep successful interception body-only and reject only changed Claude fields whose documented provider contract requires a non-empty value.

**Tech Stack:** Go 1.26, CGO, GNU Make, gjson, CLIProxyAPI v7.2.152 native ABI v1/RPC schema v5, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-11-censorship-v0.2.2-functional-audit-design.md`

## Global Constraints

- Do not modify CLIProxyAPI core, native ABI v1, RPC schema v5, the pinned `github.com/router-for-me/CLIProxyAPI/v7` dependency, or `C.GoBytes` in `cliproxyPluginCall`.
- Keep `words` as a strict Object with optional ordered `block`, `strip`, and `obfs` arrays. Legacy `words` sequence and global `mode` remain handwritten-YAML-only behavior; the panel must not emit global `mode`.
- Preserve rule execution order exactly: `block` -> `strip` -> `obfs`.
- Select only the provider-documented natural-language paths listed in the spec. Do not add generic recursive traversal, generic unmarshalling, or a second JSON parse.
- Successful rewrites return only `RequestInterceptResponse.Body`; do not set or clear request headers. CPA remains responsible for the replacement body's `Content-Length`.
- Keep the plugin request-only. Do not register response, stream-chunk, WebSocket-response, Management API, model-router, or executor capabilities.
- Do not hand-edit `.integration/` or `dist/` artifacts.
- Release archives retain `censorship_<version>_<goos>_<goarch>.zip`, matching lowercase SHA-256 files, and aggregate `checksums.txt`.
- Every behavioral change starts with a focused uncached failing test. Make only the minimum root-cause change required to pass it.
- Planning and final review use `claude-opus-5[1m]` at `xhigh`; implementation workers use `claude-sonnet-5[1m]` at `xhigh`.

## File Map and Execution Boundaries

| Unit | Files | Responsibility |
| --- | --- | --- |
| U1 OpenAI Responses | `selectors_openai_test.go:235-314`, `selectors_openai.go:145-210` | Correct explicit replay-item leaves and `tool` role gating. |
| U2 Claude selectors and rewrite validity | `selectors_claude_test.go:78-174`, `selectors_claude.go:5-138`, `selectors.go:113-123`, `transform.go:10-72`, `main.go:108-140` | Add documented Claude leaves, annotate minimum-length spans, and produce the local 400 response. |
| U3 Aggregate packaging | `.github/scripts/package-release_test.go:701-832`, `.github/scripts/package-release.go:68-97` | Reproduce aggregate archive/library aliasing and reuse direct-mode validation. |
| U4 Make default version | `.github/scripts/package-release_test.go:834-971`, `Makefile:1-7` | Test a truly absent version and reorder the existing assignments. |
| U5 Documentation | `main_test.go:472-568`, `.github/scripts/package-release_test.go:973-993`, `README.md:104-180`, `RELEASE_NOTES.md:1-end` | Publish the exact v0.2.2 selector, invalid-rewrite, packaging, build, compatibility, and stream-transparency contract. |
| U6 Verification and release | no source changes unless a verified review finding requires a new TDD cycle | Run all checks, review once, merge, tag, push, and verify the release assets. |

U1, U2, and U3 have disjoint file ownership and run concurrently in isolated worktrees. Cherry-pick those commits onto `fix/v0.2.2-functional-audit`, then run U4 because U3 and U4 both modify `.github/scripts/package-release_test.go`. Run U5 only after U1 through U4 are integrated so documentation describes final behavior. No two concurrent workers may read or edit the same owned source file.

---

### Task 0: Commit This Implementation Plan

**Files:**
- Create: `docs/superpowers/plans/2026-09-11-censorship-v0.2.2-functional-audit.md`

**Interfaces:**
- Consumes: the approved current-task spec at `docs/superpowers/specs/2026-09-11-censorship-v0.2.2-functional-audit-design.md`.
- Produces: the task boundaries, test names, commands, commits, review gate, and release procedure used by every implementation worker.

- [ ] **Step 1: Confirm the plan contains no incomplete instructions**

Run:

```bash
git diff --check -- docs/superpowers/plans/2026-09-11-censorship-v0.2.2-functional-audit.md
```

Expected: exit 0 with no output.

- [ ] **Step 2: Confirm only the new plan is uncommitted**

Run:

```bash
git status --short
git diff --name-only
```

Expected: only `docs/superpowers/plans/2026-09-11-censorship-v0.2.2-functional-audit.md` appears.

- [ ] **Step 3: Commit the plan**

```bash
git add docs/superpowers/plans/2026-09-11-censorship-v0.2.2-functional-audit.md
git commit -m "docs: plan v0.2.2 functional audit fixes"
```

Expected: one documentation commit and a clean worktree.

---

### Task 1: Correct OpenAI Responses Replay-Item Selection (U1)

**Files:**
- Modify: `selectors_openai_test.go:235-314`
- Modify: `selectors_openai.go:145-210`

**Interfaces:**
- Consumes: `appendStringSpan(spans *[]textSpan, value gjson.Result, role string, roles scopeSet)` and the existing `collectOpenAIResponsesToolOutput` switch.
- Produces: explicit `tool` spans for `program_output.result`, `mcp_call.output`, approved `mcp_call.error.message`, `mcp_list_tools.error`, `file_search_call.results[*].text`, and `code_interpreter_call.outputs[type=logs].logs`.
- Excludes: `program_result`, unsupported MCP error objects, non-log code interpreter outputs, and all IDs, arguments, code, scores, URLs, status, metadata, and unknown variants.

- [ ] **Step 1: Replace stale fixtures and add current-contract cases**

In `TestOpenAIResponsesOutputRoleGate`, retain the existing function/custom/shell/apply-patch cases and replace the stale MCP/program cases with these table entries:

```go
{
	name: "mcp call",
	body: []byte(`{"input":[{"type":"mcp_call","id":"SECRET mcp id","name":"SECRET mcp name","arguments":"SECRET mcp arguments","server_label":"SECRET mcp server","output":"SECRET mcp output","error":{"type":"mcp_protocol_error","message":"SECRET protocol message","code":"SECRET protocol code"}},{"type":"mcp_call","output":null,"error":{"type":"http_error","message":"SECRET HTTP message","status_code":500}},{"type":"mcp_call","error":{"type":"future_error","message":"SECRET future message"}}]}`),
	replacements: []rawReplacement{
		{Before: `"SECRET mcp output"`, After: `" mcp output"`},
		{Before: `"SECRET protocol message"`, After: `" protocol message"`},
		{Before: `"SECRET HTTP message"`, After: `" HTTP message"`},
	},
},
{
	name: "mcp list tools error",
	body: []byte(`{"input":[{"type":"mcp_list_tools","id":"SECRET list id","server_label":"SECRET server","error":"SECRET list error","tools":[{"name":"SECRET tool","description":"SECRET description","input_schema":{"note":"SECRET schema"}}]}]}`),
	replacements: []rawReplacement{
		{Before: `"SECRET list error"`, After: `" list error"`},
	},
},
{
	name: "program output",
	body: []byte(`{"input":[{"type":"program_output","id":"SECRET program id","result":"SECRET program result","status":"SECRET program status"},{"type":"program_result","result":"SECRET legacy result"},{"type":"unknown_result","result":"SECRET unknown result"}]}`),
	replacements: []rawReplacement{
		{Before: `"SECRET program result"`, After: `" program result"`},
	},
},
{
	name: "file search results",
	body: []byte(`{"input":[{"type":"file_search_call","id":"SECRET file search id","queries":["SECRET query"],"results":[{"file_id":"SECRET file id","filename":"SECRET filename","score":0.75,"text":"SECRET first result","attributes":{"note":"SECRET attribute"}},{"text":"SECRET second result"},{"text":123}]}]}`),
	replacements: []rawReplacement{
		{Before: `"SECRET first result"`, After: `" first result"`},
		{Before: `"SECRET second result"`, After: `" second result"`},
	},
},
{
	name: "code interpreter logs",
	body: []byte(`{"input":[{"type":"code_interpreter_call","id":"SECRET code id","code":"SECRET code","container_id":"SECRET container","outputs":[{"type":"logs","logs":"SECRET logs"},{"type":"image","url":"SECRET image URL","logs":"SECRET image logs"},{"type":"future","logs":"SECRET future logs"}]}]}`),
	replacements: []rawReplacement{
		{Before: `"SECRET logs"`, After: `" logs"`},
	},
},
```

Keep the existing test loop unchanged. It already executes every fixture with `roles: [tool]` and `roles: [user]`, compares the enabled result against `replaceRawTokens`, and requires a nil body when `tool` is disabled. Exact raw-body comparison proves all unlisted machine siblings remain byte-identical.

- [ ] **Step 2: Run the focused test and observe RED**

Run:

```bash
go test -count=1 -run '^TestOpenAIResponsesOutputRoleGate$' .
```

Expected: FAIL. The current selector misses `program_output.result`, object-valued MCP error messages, MCP list errors, file-search result text, and code-interpreter logs. The old `program_result` expectation no longer masks `program_output`.

- [ ] **Step 3: Add only the explicit switch branches**

In `collectOpenAIResponsesToolOutput`, replace the current `mcp_call` and `program_result` branches and add the three missing item branches:

```go
case "mcp_call":
	if roles.has("tool") {
		appendStringSpan(spans, item.Get("output"), "tool", roles)
		errorValue := item.Get("error")
		if errorValue.IsObject() {
			switch errorValue.Get("type").Str {
			case "mcp_protocol_error", "http_error":
				appendStringSpan(spans, errorValue.Get("message"), "tool", roles)
			}
		}
	}
	return true
case "mcp_list_tools":
	if roles.has("tool") {
		appendStringSpan(spans, item.Get("error"), "tool", roles)
	}
	return true
case "program_output":
	if roles.has("tool") {
		appendStringSpan(spans, item.Get("result"), "tool", roles)
	}
	return true
case "file_search_call":
	if !roles.has("tool") {
		return true
	}
	results := item.Get("results")
	if results.IsArray() {
		results.ForEach(func(_, result gjson.Result) bool {
			if result.IsObject() {
				appendStringSpan(spans, result.Get("text"), "tool", roles)
			}
			return true
		})
	}
	return true
case "code_interpreter_call":
	if !roles.has("tool") {
		return true
	}
	outputs := item.Get("outputs")
	if outputs.IsArray() {
		outputs.ForEach(func(_, output gjson.Result) bool {
			if output.IsObject() && output.Get("type").Str == "logs" {
				appendStringSpan(spans, output.Get("logs"), "tool", roles)
			}
			return true
		})
	}
	return true
```

Do not introduce a shared result walker. `appendStringSpan` enforces string type, enabled role, non-empty source text, and valid raw offsets.

- [ ] **Step 4: Format and run GREEN tests**

Run:

```bash
gofmt -w selectors_openai.go selectors_openai_test.go
go test -count=1 -run '^(TestOpenAIResponsesOutputRoleGate|TestOpenAIResponsesSelectorRowsAndExclusions|TestOpenAIResponsesAssistantOutputShapesUseAssistantScope)$' .
```

Expected: PASS. The enabled cases change only listed replacements; disabled and unknown variants remain untouched.

- [ ] **Step 5: Commit U1**

```bash
git add selectors_openai.go selectors_openai_test.go
git commit -m "fix: cover OpenAI Responses replay results (U1)"
```

Expected: a self-contained U1 commit with no other files.

---

### Task 2: Complete Claude Result Selection and Reject Empty Required Text (U2)

**Files:**
- Modify: `selectors_claude_test.go:3-174`
- Modify: `selectors_claude.go:5-138`
- Modify: `selectors.go:113-123`
- Modify: `transform.go:10-72`
- Modify: `main.go:108-140`

**Interfaces:**
- Consumes: existing `textSpan`, `appendStringSpan`, `appendClaudeResultText`, `applyMode`, `rebuildBody`, and `terminatedRequest` behavior.
- Produces: `textSpan.RequiresNonEmpty bool`, `appendNonEmptyStringSpan`, and `transformResult.InvalidMessage string`.
- Error contract: changed required Claude text becoming empty returns HTTP 400 JSON with code `censorship_invalid_request` and message `censorship rewrite would make a text field invalid`.
- Existing invalid-JSON contract remains `request body must be a JSON object`.

- [ ] **Step 1: Add direct-user and nested-tool selector cases**

Append these two entries to the table in `TestClaudeResultText`:

```go
{
	name:          "direct user search and document metadata",
	roles:         "[user]",
	disabledRoles: "[tool]",
	body:          `{"messages":[{"role":"user","content":[{"type":"search_result","source":"SECRET search source","title":"SECRET search title","content":[{"type":"text","text":"SECRET search text"}]},{"type":"document","title":"SECRET document title","context":"SECRET document context","source":{"type":"content","content":"SECRET scalar document"}}]}]}`,
	replacements: []rawReplacement{
		{Before: `"SECRET search title"`, After: `" search title"`},
		{Before: `"SECRET search text"`, After: `" search text"`},
		{Before: `"SECRET document title"`, After: `" document title"`},
		{Before: `"SECRET document context"`, After: `" document context"`},
		{Before: `"SECRET scalar document"`, After: `" scalar document"`},
	},
},
{
	name:          "nested tool search and document metadata",
	roles:         "[tool]",
	disabledRoles: "[user]",
	body:          `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"SECRET tool id","content":[{"type":"search_result","source":"SECRET nested source","title":"SECRET nested search title","content":[{"type":"text","text":"SECRET nested search text"}]},{"type":"document","title":"SECRET nested document title","context":"SECRET nested document context","source":{"type":"content","content":"SECRET nested scalar document"}}]}]}]}`,
	replacements: []rawReplacement{
		{Before: `"SECRET nested search title"`, After: `" nested search title"`},
		{Before: `"SECRET nested search text"`, After: `" nested search text"`},
		{Before: `"SECRET nested document title"`, After: `" nested document title"`},
		{Before: `"SECRET nested document context"`, After: `" nested document context"`},
		{Before: `"SECRET nested scalar document"`, After: `" nested scalar document"`},
	},
},
```

The existing enabled/disabled loop proves direct blocks inherit `user`, nested `tool_result.content` inherits `tool`, and no replacement body is returned for the disabled canonical role.

- [ ] **Step 2: Correct the existing exact-body expectations**

In `TestClaudeSelectorRowsAndExclusions`, add this replacement because `document.title` is now selected even when the document source is base64:

```go
rawReplacement{Before: `"SECRET document"`, After: `" document"`},
```

In `TestClaudeResultTextPreservesMachineFields`:

1. Add unique `context` values to direct and nested `document` objects.
2. Add direct and nested `source.type == "content"` documents whose `source.content` is a scalar string.
3. Add replacements for `search_result.title`, every `document.title`, every added `document.context`, and both scalar content strings.
4. Retain source IDs/URLs, media types, base64, file IDs, citations, cache metadata, thinking, signatures, image content, `tool_use_id`, and unknown blocks without replacement entries.

Use `replaceRawTokens` for each complete expected body. Do not compare decoded maps, because these tests must detect any byte change outside selected JSON string tokens.

- [ ] **Step 3: Add full-strip rejection tests**

Add imports for `net/http` and `github.com/tidwall/gjson`, then add:

```go
func TestClaudeMinimumLengthFieldsRejectFullStrip(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{name: "typed text", body: []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"SECRET"}]}]}`)},
		{name: "document title", body: []byte(`{"messages":[{"role":"user","content":[{"type":"document","title":"SECRET","source":{"type":"text","data":"clean"}}]}]}`)},
		{name: "document context", body: []byte(`{"messages":[{"role":"user","content":[{"type":"document","context":"SECRET","source":{"type":"text","data":"clean"}}]}]}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
			resp := interceptRPC(t, "claude", tc.body)
			if !resp.Terminate || resp.StatusCode != http.StatusBadRequest || resp.ResponseHeaders.Get("Content-Type") != "application/json" || len(resp.Body) != 0 {
				t.Fatalf("response = %#v", resp)
			}
			if got := gjson.GetBytes(resp.ResponseBody, "error.code").String(); got != "censorship_invalid_request" {
				t.Fatalf("error code = %q, body = %s", got, resp.ResponseBody)
			}
			if got := gjson.GetBytes(resp.ResponseBody, "error.message").String(); got != "censorship rewrite would make a text field invalid" {
				t.Fatalf("error message = %q, body = %s", got, resp.ResponseBody)
			}
		})
	}
}
```

- [ ] **Step 4: Add partial-strip exact-body tests**

Add:

```go
func TestClaudeMinimumLengthFieldsAllowPartialStrip(t *testing.T) {
	cases := []struct {
		name string
		body []byte
		want []byte
	}{
		{
			name: "typed text",
			body: []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"SECRET tail"}]}]}`),
			want: []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":" tail"}]}]}`),
		},
		{
			name: "document title",
			body: []byte(`{"messages":[{"role":"user","content":[{"type":"document","title":"SECRET title","source":{"type":"text","data":"clean"}}]}]}`),
			want: []byte(`{"messages":[{"role":"user","content":[{"type":"document","title":" title","source":{"type":"text","data":"clean"}}]}]}`),
		},
		{
			name: "document context",
			body: []byte(`{"messages":[{"role":"user","content":[{"type":"document","context":"SECRET context","source":{"type":"text","data":"clean"}}]}]}`),
			want: []byte(`{"messages":[{"role":"user","content":[{"type":"document","context":" context","source":{"type":"text","data":"clean"}}]}]}`),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
			resp := interceptRPC(t, "claude", tc.body)
			if resp.Terminate || !bytes.Equal(resp.Body, tc.want) {
				t.Fatalf("response = %#v, want body %s", resp, tc.want)
			}
		})
	}
}
```

- [ ] **Step 5: Run the Claude tests and observe RED**

Run:

```bash
go test -count=1 -run '^(TestClaudeResultText|TestClaudeResultTextPreservesMachineFields|TestClaudeMinimumLengthFieldsRejectFullStrip|TestClaudeMinimumLengthFieldsAllowPartialStrip)$' .
```

Expected: FAIL. Titles, contexts, and scalar content documents bypass selection; a fully stripped typed text is returned as an empty string instead of a local 400.

- [ ] **Step 6: Add minimum-length span metadata without changing generic selection**

Add the boolean to `textSpan` in `transform.go`:

```go
type textSpan struct {
	RawStart        int
	RawEnd          int
	Text            string
	Role            string
	Changed         bool
	SkipFoldRewrite bool
	RequiresNonEmpty bool
}
```

Align fields with `gofmt`. In `selectors.go`, add this single-purpose wrapper immediately after `appendStringSpan`:

```go
func appendNonEmptyStringSpan(spans *[]textSpan, value gjson.Result, role string, roles scopeSet) {
	before := len(*spans)
	appendStringSpan(spans, value, role, roles)
	if len(*spans) > before {
		(*spans)[len(*spans)-1].RequiresNonEmpty = true
	}
}
```

This wrapper delegates all eligibility and raw-offset checks to the existing selector and marks only a span that was actually appended.

- [ ] **Step 7: Use the helper only for Claude provider-required strings**

In `selectors_claude.go`:

1. Replace `appendStringSpan` with `appendNonEmptyStringSpan` for `text` blocks in top-level system arrays, message content arrays, nested `tool_result.content` arrays, `search_result.content` typed-text arrays, and `document.source.content` typed-text arrays.
2. Keep string-valued top-level `system`, message `content`, tool-result `content`, `document.source.data`, and scalar `document.source.content` on ordinary `appendStringSpan`.
3. In `appendClaudeResultText`, append `search_result.title` with ordinary `appendStringSpan` before scanning content.
4. For a `document`, append `title` and `context` with `appendNonEmptyStringSpan` before inspecting `source`.
5. For `source.type == "content"`, call ordinary `appendStringSpan` on scalar `source.content`; if it is an array, retain the existing explicit typed-text loop using `appendNonEmptyStringSpan`.

The resulting `appendClaudeResultText` switch has this shape:

```go
switch blockType.Str {
case "search_result":
	appendStringSpan(spans, block.Get("title"), role, roles)
	content := block.Get("content")
	if !content.IsArray() {
		return
	}
	content.ForEach(func(_, part gjson.Result) bool {
		if part.IsObject() && part.Get("type").Str == "text" {
			appendNonEmptyStringSpan(spans, part.Get("text"), role, roles)
		}
		return true
	})
case "document":
	appendNonEmptyStringSpan(spans, block.Get("title"), role, roles)
	appendNonEmptyStringSpan(spans, block.Get("context"), role, roles)
	source := block.Get("source")
	if !source.IsObject() {
		return
	}
	switch source.Get("type").Str {
	case "text":
		appendStringSpan(spans, source.Get("data"), role, roles)
	case "content":
		content := source.Get("content")
		appendStringSpan(spans, content, role, roles)
		if !content.IsArray() {
			return
		}
		content.ForEach(func(_, part gjson.Result) bool {
			if part.IsObject() && part.Get("type").Str == "text" {
				appendNonEmptyStringSpan(spans, part.Get("text"), role, roles)
			}
			return true
		})
	}
}
```

Do not mark `search_result.title`, document source text, scalar content documents, or generic string content as minimum-length fields. The spec limits the marker to Claude `TextBlockParam.text`, `document.title`, and `document.context`.

- [ ] **Step 8: Reject a changed required span before rebuilding JSON**

Extend `transformResult`:

```go
type transformResult struct {
	Body           []byte
	Blocked        *blockMatch
	Invalid        bool
	InvalidMessage string
}
```

In `transformRequest`, after `changed` is known true and before `rebuildBody`, add:

```go
for _, span := range spans {
	if span.Changed && span.RequiresNonEmpty && span.Text == "" {
		return transformResult{
			Invalid:        true,
			InvalidMessage: "censorship rewrite would make a text field invalid",
		}, nil
	}
}
```

This scans the already selected spans once. Do not parse or traverse the body again, delete a property, insert whitespace, or return the original body.

- [ ] **Step 9: Route the specific message through the existing 400 response**

In `interceptBeforeAuth`, resolve the invalid message locally while retaining the existing response type, code, status, and headers:

```go
case result.Invalid:
	message := result.InvalidMessage
	if message == "" {
		message = "request body must be a JSON object"
	}
	return terminatedRequest(censorshipError{
		Type:    "invalid_request_error",
		Code:    "censorship_invalid_request",
		Message: message,
	})
```

Do not change `terminatedRequest` or the successful `Body` branch.

- [ ] **Step 10: Format and run GREEN tests**

Run:

```bash
gofmt -w selectors_claude.go selectors_claude_test.go selectors.go transform.go main.go
go test -count=1 -run '^(TestClaudeSelectorRowsAndExclusions|TestClaudeTopLevelSystemStringAndToolScope|TestClaudeSelectorCanonicalRoles|TestClaudeResultText|TestClaudeResultTextPreservesMachineFields|TestClaudeMinimumLengthFieldsRejectFullStrip|TestClaudeMinimumLengthFieldsAllowPartialStrip|TestBeforeAuthRejectsEnabledInvalidJSON)$' .
```

Expected: PASS. Existing malformed/deep/duplicate JSON continues to use its original message; full required-text stripping terminates locally; partial stripping remains body-only and exact.

- [ ] **Step 11: Commit U2**

```bash
git add selectors_claude.go selectors_claude_test.go selectors.go transform.go main.go
git commit -m "fix: validate Claude text rewrites (U2)"
```

Expected: a self-contained U2 commit with no unrelated changes.

---

### Task 3: Protect Aggregate Packaging from Output Aliases (U3)

**Files:**
- Modify: `.github/scripts/package-release_test.go:701-832`
- Modify: `.github/scripts/package-release.go:68-97`

**Interfaces:**
- Consumes: `artifactSpec.binaryPath`, `snapshotFiles`, `assertFilesUnchanged`, and `validateDirectPackagePaths(libraryPath, archivePath, checksumPath) (string, string, string, error)`.
- Produces: aggregate mode validates each discovered library/archive/checksum triple before opening any of those paths.

- [ ] **Step 1: Write the aggregate hard-link regression test**

Add immediately before `TestPackageExistingArtifactsHashesEachArchiveOnce`:

```go
func TestPackageExistingArtifactsRejectsArchiveAliasWithoutChangingFiles(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	out := filepath.Join(t.TempDir(), "out")
	artifact := artifactSpec{osName: "linux", arch: "amd64"}
	library := artifact.binaryPath(dist)
	if err := os.MkdirAll(filepath.Dir(library), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(library, []byte("library contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(out, "censorship_1.2.3_linux_amd64.zip")
	if err := os.Link(library, archive); err != nil {
		t.Fatal(err)
	}
	checksum := archive + ".sha256"
	aggregate := filepath.Join(out, "checksums.txt")
	before := snapshotFiles(t, library, archive, checksum, aggregate)

	if err := packageExistingArtifacts("1.2.3", dist, out); err == nil {
		t.Fatal("aggregate packaging accepted an archive alias of its input library")
	}
	assertFilesUnchanged(t, before)
}
```

This test uses the expected aggregate archive name, snapshots both hard-link paths and all possible checksum outputs, and checks content, timestamps, symlink state, and absence preservation.

- [ ] **Step 2: Run the focused test and observe RED**

Run:

```bash
go test -count=1 .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^TestPackageExistingArtifactsRejectsArchiveAliasWithoutChangingFiles$'
```

Expected: FAIL because aggregate packaging succeeds, truncates the source inode through `os.Create`, and creates checksum output.

- [ ] **Step 3: Reuse direct-mode validation inside the aggregate loop**

In `packageExistingArtifacts`, after computing `zipPath` and before `packageLibrary`, replace raw paths with validator outputs:

```go
zipPath := filepath.Join(outDir, zipName)
library, archive, checksum, err := validateDirectPackagePaths(binaryPath, zipPath, zipPath+".sha256")
if err != nil {
	return err
}
if err := packageLibrary(library, archive); err != nil {
	return err
}
line, err := writeChecksum(checksum, archive)
```

Do not add a second canonicalization helper or modify `packageLibrary`. Returning at validation ensures no library/archive/checksum path is changed for the rejected platform and prevents aggregate `checksums.txt` from being written.

- [ ] **Step 4: Format and run GREEN packaging tests**

Run:

```bash
gofmt -w .github/scripts/package-release.go .github/scripts/package-release_test.go
go test -count=1 .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^(TestPackageExistingArtifactsRejectsArchiveAliasWithoutChangingFiles|TestPackagerRejectsPathCollisionsBeforeChangingFiles|TestPackageExistingArtifactsHashesEachArchiveOnce)$'
```

Expected: PASS. Existing direct alias and single-hash behavior remains unchanged.

- [ ] **Step 5: Commit U3**

```bash
git add .github/scripts/package-release.go .github/scripts/package-release_test.go
git commit -m "fix: reject aggregate package aliases (U3)"
```

Expected: a self-contained U3 commit with no other files.

---

### Task 4: Restore the Missing-Version Make Default (U4)

**Files:**
- Modify: `.github/scripts/package-release_test.go:834-971`
- Modify: `Makefile:1-7`

**Interfaces:**
- Consumes: the real repository `Makefile`, `validate-version`, and environment handling used by packaging tests.
- Produces: missing `VERSION` -> `PACKAGER_VERSION=0.0.0-dev`, while explicit empty and unsafe values remain rejected.

- [ ] **Step 1: Add a helper that truly removes inherited variables**

Add next to `withEnvironment`:

```go
func withoutEnvironment(base []string, keys ...string) []string {
	out := make([]string, 0, len(base))
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		remove := false
		for _, key := range keys {
			if strings.EqualFold(name, key) {
				remove = true
				break
			}
		}
		if !remove {
			out = append(out, entry)
		}
	}
	return out
}
```

Use this only where a test must distinguish an absent environment variable from an explicitly empty one.

- [ ] **Step 2: Add the real missing-version validation test**

Add before `TestMakeVersionValidationContract`:

```go
func TestMakeMissingVersionUsesDevelopmentDefault(t *testing.T) {
	makefile, err := filepath.Abs(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("make", "-f", makefile, "validate-version")
	cmd.Dir = t.TempDir()
	cmd.Env = withoutEnvironment(os.Environ(), "VERSION", "PACKAGER_VERSION", "NORMALIZED_VERSION")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make validate-version without VERSION: %v\n%s", err, output)
	}
}
```

Also add `{value: ""}` to the unsafe command-line-value table in `TestMakeVersionValidationContract`. That invokes `make ... VERSION=` and proves an explicitly empty value is not treated as absent.

- [ ] **Step 3: Run the focused tests and observe RED**

Run:

```bash
go test -count=1 .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^(TestMakeMissingVersionUsesDevelopmentDefault|TestMakeVersionValidationContract)$'
```

Expected: FAIL only in `TestMakeMissingVersionUsesDevelopmentDefault`, with `VERSION must normalize to a safe non-empty release version`. Explicit empty and all existing unsafe cases must already fail as expected.

- [ ] **Step 4: Reorder the existing Make assignments**

Replace `Makefile:3-6` with exactly:

```make
RAW_VERSION := $(value VERSION)
PACKAGER_VERSION := $(if $(filter undefined,$(origin VERSION)),0.0.0-dev,$(RAW_VERSION))
unexport VERSION
NORMALIZED_VERSION = $(patsubst v%,%,$(PACKAGER_VERSION))
```

Do not change validation recipes, recursive make arguments, normalization, archive names, or any version source. The immediate assignments must inspect the original `VERSION` origin before `unexport VERSION` creates the file-origin empty variable.

- [ ] **Step 5: Run GREEN Make contract tests**

Run:

```bash
go test -count=1 .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^(TestMakeMissingVersionUsesDevelopmentDefault|TestMakeVersionValidationContract|TestMakeBuildIgnoresTargetOverrides)$'
make validate-version
```

Expected: PASS. The direct command exits 0 with all three version variables absent from the invoking environment.

- [ ] **Step 6: Commit U4**

```bash
git add Makefile .github/scripts/package-release_test.go
git commit -m "fix: restore Make development version default (U4)"
```

Expected: a self-contained U4 commit built on the integrated U3 test file.

---

### Task 5: Document the v0.2.2 Contract (U5)

**Files:**
- Modify: `main_test.go:472-568`
- Modify: `.github/scripts/package-release_test.go:973-993`
- Modify: `README.md:104-180`
- Modify: `RELEASE_NOTES.md:1-end`

**Interfaces:**
- Consumes: final U1 through U4 behavior and all compatibility exclusions from the current-task spec.
- Produces: exact searchable documentation for users and the tag-triggered GitHub release notes.

- [ ] **Step 1: Extend documentation assertions before editing prose**

Add these exact tokens to the `required` slice in `TestDocumentationListsConfigAndLimits` so both `README.md` and `RELEASE_NOTES.md` must contain them:

```go
"`program_output.result`",
"`mcp_call.error.message`",
"`mcp_list_tools.error`",
"`file_search_call.results[*].text`",
"`code_interpreter_call.outputs[type=logs].logs`",
"`search_result.title`",
"`document.title`",
"`document.context`",
"scalar `document.source.content`",
"censorship rewrite would make a text field invalid",
"0.0.0-dev",
"aggregate packaging",
"signed-history prefix",
```

In `TestDocumentationSelectorContractIncludesToolResults`, retain the current compatibility phrases and add the same exact path tokens that distinguish current OpenAI Responses items from the stale `program_result` fixture.

- [ ] **Step 2: Run documentation tests and observe RED**

Run:

```bash
go test -count=1 -run '^TestDocumentationListsConfigAndLimits$' .
go test -count=1 .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^TestDocumentationSelectorContractIncludesToolResults$'
```

Expected: FAIL because the v0.2.1 documents do not list all new paths, the new invalid-rewrite message, aggregate alias protection, development default, or signed-history limitation wording.

- [ ] **Step 3: Update `README.md` surgically**

In the existing selector and limitation sections:

1. Extend OpenAI Responses result-output text with exact paths: `program_output.result`, `mcp_call.output`, `mcp_call.error.message` only for `mcp_protocol_error` and `http_error`, scalar `mcp_list_tools.error`, `file_search_call.results[*].text`, and `code_interpreter_call.outputs[type=logs].logs`.
2. State that all these leaves use canonical `tool`; unsupported error/output variants and machine siblings remain unchanged.
3. Extend Claude result text with `search_result.title`, `document.title`, `document.context`, and scalar `document.source.content` for `source.type == "content"`; state direct user blocks use `user` and nested tool-result blocks use `tool`.
4. State that a rewrite which empties Claude `TextBlockParam.text`, present `document.title`, or present `document.context` terminates locally with `censorship_invalid_request` and `censorship rewrite would make a text field invalid`.
5. State that bare `make build` and host-artifact `make package` use `0.0.0-dev`; explicit empty or unsafe `VERSION` remains invalid.
6. State that direct and aggregate packaging reject library/archive/checksum aliases before changing those paths.
7. Document the signed-history prefix limitation from the spec: provider-controlled Claude signed history can bind preceding input, the BeforeAuth hook cannot reliably know final model/binding controls, and the plugin therefore neither mutates thinking/signatures nor adds an overbroad runtime rejection.
8. Retain the request-only statement and existing HTTP JSON EOF, SSE, and Responses WebSocket transparency statements.

Do not add response filtering, generic selector claims, or guarantees about CPA's own header normalization.

- [ ] **Step 4: Update `RELEASE_NOTES.md` for v0.2.2**

Change the heading to:

```markdown
# Censorship v0.2.2
```

Add a concise v0.2.2 fixes section covering U1 through U4, then retain the complete configuration, selector, ABI, packaging, compatibility, request-only, and stream-transparency contract currently enforced by `TestDocumentationListsConfigAndLimits`. Update stale OpenAI and Claude selector prose to the same exact paths and role rules as `README.md`. Include the same Claude invalid-rewrite message and signed-history prefix limitation.

Do not delete required v0.2.1-era compatibility statements merely because this is a patch release; `RELEASE_NOTES.md` is the release's complete user contract and is used directly by `gh release create/edit`.

- [ ] **Step 5: Run GREEN documentation tests**

Run:

```bash
gofmt -w main_test.go .github/scripts/package-release_test.go
go test -count=1 -run '^TestDocumentationListsConfigAndLimits$' .
go test -count=1 .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^(TestDocumentationSelectorContractIncludesToolResults|TestDocumentationDoesNotDenyOptionalLicense)$'
```

Expected: PASS for both documents without weakening existing documentation assertions.

- [ ] **Step 6: Commit U5**

```bash
git add main_test.go .github/scripts/package-release_test.go README.md RELEASE_NOTES.md
git commit -m "docs: publish v0.2.2 behavior and limits (U5)"
```

Expected: the release documentation and its executable assertions are in one commit.

---

### Task 6: Integrate Parallel Units and Run Focused Verification

**Files:**
- Integrate: commits produced by U1, U2, and U3 isolated worktrees
- Modify only if a cherry-pick conflict exposes overlapping ownership, which should not occur under the file map

**Interfaces:**
- Consumes: one commit hash from each isolated implementation worker.
- Produces: `fix/v0.2.2-functional-audit` containing the plan plus U1 through U5 in order.

- [ ] **Step 1: Run the first implementation wave concurrently**

Use one `claude-sonnet-5[1m]` `xhigh` isolated worker for each of U1, U2, and U3. Each worker reads only the current spec, this plan, and its owned files, follows the listed RED -> minimal GREEN -> commit sequence, and returns its commit hash plus exact red/green outputs. Do not let any worker edit documentation or files owned by another unit.

Expected: three independent commits with disjoint path sets.

- [ ] **Step 2: Cherry-pick U1, U2, and U3 onto the feature branch**

```bash
git checkout fix/v0.2.2-functional-audit
git cherry-pick <U1-commit> <U2-commit> <U3-commit>
```

Expected: all three apply without conflicts. Verify paths before proceeding:

```bash
git show --stat --oneline HEAD~2..HEAD
git status --short
```

Expected: only the owned files appear and the worktree is clean.

- [ ] **Step 3: Execute U4 from the integrated branch**

Dispatch one fresh `claude-sonnet-5[1m]` `xhigh` isolated worker from the now-integrated feature branch. It owns only `Makefile` and `.github/scripts/package-release_test.go`, follows U4 RED/GREEN, commits, and returns its hash. Cherry-pick that hash onto the feature branch.

- [ ] **Step 4: Execute U5 from the integrated branch**

Dispatch one fresh `claude-sonnet-5[1m]` `xhigh` isolated worker after U4 is integrated. It owns only `main_test.go`, `.github/scripts/package-release_test.go`, `README.md`, and `RELEASE_NOTES.md`, follows U5 RED/GREEN, commits, and returns its hash. Cherry-pick that hash onto the feature branch.

- [ ] **Step 5: Run all focused tests together without cache**

```bash
go test -count=1 -run '^(TestOpenAIResponsesOutputRoleGate|TestClaudeResultText|TestClaudeResultTextPreservesMachineFields|TestClaudeMinimumLengthFieldsRejectFullStrip|TestClaudeMinimumLengthFieldsAllowPartialStrip|TestBeforeAuthRejectsEnabledInvalidJSON|TestDocumentationListsConfigAndLimits)$' .
go test -count=1 .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^(TestPackageExistingArtifactsRejectsArchiveAliasWithoutChangingFiles|TestPackagerRejectsPathCollisionsBeforeChangingFiles|TestPackageExistingArtifactsHashesEachArchiveOnce|TestMakeMissingVersionUsesDevelopmentDefault|TestMakeVersionValidationContract|TestMakeBuildIgnoresTargetOverrides|TestDocumentationSelectorContractIncludesToolResults|TestDocumentationDoesNotDenyOptionalLicense)$'
```

Expected: both commands PASS.

- [ ] **Step 6: Check the integrated diff**

```bash
git diff --check ce11ebb598add04c0b3038eb52ebe82e8c761152..HEAD
git diff --name-only ce11ebb598add04c0b3038eb52ebe82e8c761152..HEAD
git status --short
```

Expected changed paths only:

```plaintext
.github/scripts/package-release.go
.github/scripts/package-release_test.go
Makefile
README.md
RELEASE_NOTES.md
docs/superpowers/plans/2026-09-11-censorship-v0.2.2-functional-audit.md
docs/superpowers/specs/2026-09-11-censorship-v0.2.2-functional-audit-design.md
main.go
main_test.go
selectors.go
selectors_claude.go
selectors_claude_test.go
selectors_openai.go
selectors_openai_test.go
transform.go
```

No CLIProxyAPI core, `.integration/`, `dist/`, dependency, ABI, or unrelated file may appear. The worktree must be clean.

---

### Task 7: Run Full Verification and Opus Review (U6)

**Files:**
- Review: all paths in `ce11ebb598add04c0b3038eb52ebe82e8c761152..HEAD`
- Modify: only files tied to a confirmed review finding, with a new focused failing test first

**Interfaces:**
- Consumes: integrated U1 through U5 commits and the current-task spec.
- Produces: fresh full-suite evidence and one complete Opus review with every surviving finding resolved or explicitly shown not to reproduce.

- [ ] **Step 1: Run the required verification sequence**

Run each command separately and retain its exit code and output:

```bash
make test
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
make race
make vet
make integration
make build
make package
```

Expected: every command exits 0. In particular:

- `make integration` continues to pass HTTP fixed-length JSON EOF, chunk/trailer handling, SSE output trace, request-body propagation, Responses WebSocket transparency, and ABI response ownership checks.
- Bare `make build` embeds `0.0.0-dev` and creates only ignored `dist/` output.
- Bare `make package` accepts the missing-version default and writes valid archives/checksums for discovered artifacts.

If a command fails, diagnose the root cause before editing. Add or strengthen one focused failing test, make the minimum correction, rerun its focused command, then restart this full sequence from `make test`.

- [ ] **Step 2: Run one complete Opus review**

Dispatch one `claude-opus-5[1m]` `xhigh` reviewer over `ce11ebb598add04c0b3038eb52ebe82e8c761152..HEAD`. Give it the current spec and plan. Require it to inspect:

1. Exact OpenAI and Claude selector inclusions/exclusions and canonical roles.
2. No generic walker, no second JSON parse, and no mutation outside selected raw spans.
3. Claude full-strip invalid flow, unchanged malformed JSON message, body-only success, and HTTP 400 JSON termination.
4. Aggregate alias preservation and Make absent-versus-empty semantics.
5. ABI/dependency/config/panel/request-only compatibility and absence of generated/core edits.
6. Test quality, README/release-note accuracy, and patch-release artifact contract.

Use one reviewer rather than overlapping file-level reviewers so review reads are not duplicated. Require concrete file/line, reproduction, impact, and minimal fix for every finding; reject speculative hardening and already excluded findings.

- [ ] **Step 3: Resolve only confirmed findings through TDD**

For each reported finding:

1. Reproduce it on the reviewed branch with the smallest focused test or command.
2. If it does not reproduce or contradicts the approved exclusions, record the evidence and do not edit.
3. If confirmed, dispatch one fresh `claude-sonnet-5[1m]` `xhigh` worker with exclusive ownership of the affected files. Require RED, minimal GREEN, focused pass, and a named commit.
4. Return the resulting diff to the same Opus reviewer for a focused re-review.

Do not combine unrelated review findings in one implementation commit.

- [ ] **Step 4: Re-run verification after any review fix**

If no code changed during review, retain Step 1 evidence. If any code changed, rerun all seven Step 1 commands from the beginning. Then run:

```bash
git diff --check ce11ebb598add04c0b3038eb52ebe82e8c761152..HEAD
git status --short
```

Expected: no whitespace errors and a clean worktree.

- [ ] **Step 5: Confirm version and release metadata before merge**

Run:

```bash
git log --oneline --decorate ce11ebb598add04c0b3038eb52ebe82e8c761152..HEAD
git tag --list v0.2.2
git status --short --branch
```

Expected: spec, plan, implementation, documentation, and any verified review-fix commits are present; `v0.2.2` does not yet exist; the feature branch is clean.

---

### Task 8: Merge, Tag, Push, and Verify the GitHub Release (U6)

**Files:**
- No source edits
- Create git tag: `v0.2.2`
- Publish: local `main`, tag-triggered workflow, and GitHub release assets

**Interfaces:**
- Consumes: a clean, fully verified `fix/v0.2.2-functional-audit` branch and authenticated `origin`/`gh` access.
- Produces: local and remote `main` at the verified commit, annotated tag `v0.2.2`, successful GitHub Actions run, and 15 release assets.

- [ ] **Step 1: Refresh remote state without changing the verified branch**

```bash
git fetch origin --prune --tags
git status --short
git rev-parse main
git rev-parse origin/main
git merge-base --is-ancestor origin/main fix/v0.2.2-functional-audit
```

Expected: clean worktree; local and remote `main` still match or local `main` can be fast-forwarded; `origin/main` is an ancestor of the feature branch. If remote `main` moved, integrate it without force-pushing, rerun Task 7, and only then continue.

- [ ] **Step 2: Fast-forward local `main` to the verified branch**

```bash
git checkout main
git merge --ff-only fix/v0.2.2-functional-audit
git status --short --branch
```

Expected: local `main` points at the verified feature tip and the worktree is clean.

- [ ] **Step 3: Create the annotated patch tag**

```bash
git tag -a v0.2.2 -m "censorship v0.2.2"
git cat-file -t v0.2.2
git rev-parse v0.2.2^{}
git rev-parse main
```

Expected: tag type is `tag`; peeled tag commit equals local `main`.

- [ ] **Step 4: Push the verified main commit and tag without force**

```bash
git push origin main
git push origin v0.2.2
```

Expected: both pushes succeed. Do not use `--force`, bypass hooks, or amend published commits.

- [ ] **Step 5: Locate and watch the tag workflow**

Set `release_sha` to `git rev-parse v0.2.2^{}`. Query the `build.yml` push runs for that commit and select the run whose `headBranch` is `v0.2.2`:

```bash
gh run list --workflow build.yml --commit "$release_sha" --event push --limit 10 --json databaseId,headBranch,headSha,status,conclusion,url
gh run watch "$release_run_id" --exit-status
```

Expected: the tag run reaches `completed/success`. The main-branch run may share the same commit but is not a substitute for the tag run because only `refs/tags/v*` executes the release job.

- [ ] **Step 6: Verify release metadata and the exact asset set**

Run:

```bash
gh release view v0.2.2 --json tagName,isDraft,isPrerelease,url,assets
```

Expected: tag `v0.2.2`, `isDraft=false`, `isPrerelease=false`, and these 15 assets:

```plaintext
censorship_0.2.2_linux_amd64.zip
censorship_0.2.2_linux_amd64.zip.sha256
censorship_0.2.2_linux_arm64.zip
censorship_0.2.2_linux_arm64.zip.sha256
censorship_0.2.2_darwin_amd64.zip
censorship_0.2.2_darwin_amd64.zip.sha256
censorship_0.2.2_darwin_arm64.zip
censorship_0.2.2_darwin_arm64.zip.sha256
censorship_0.2.2_windows_amd64.zip
censorship_0.2.2_windows_amd64.zip.sha256
censorship_0.2.2_windows_arm64.zip
censorship_0.2.2_windows_arm64.zip.sha256
censorship_0.2.2_freebsd_amd64.zip
censorship_0.2.2_freebsd_amd64.zip.sha256
checksums.txt
```

- [ ] **Step 7: Download and verify all published checksums**

Download the release into a new temporary directory, then run `sha256sum -c checksums.txt` and `sha256sum -c` once for each of the seven `.zip.sha256` files from that directory.

Expected:

- `checksums.txt` contains exactly seven lines, one for each archive basename.
- Every digest is exactly 64 lowercase hexadecimal characters followed by two spaces and the archive basename.
- All aggregate and individual checksum validations report `OK`.
- No missing, duplicate, or extra archive/checksum asset exists.

- [ ] **Step 8: Confirm final local and remote refs**

```bash
git status --short --branch
git rev-parse main
git rev-parse origin/main
git rev-parse v0.2.2^{}
git ls-remote --tags origin refs/tags/v0.2.2 refs/tags/v0.2.2^{}
```

Expected: clean local `main`; local `main`, `origin/main`, and peeled `v0.2.2` resolve to the same verified commit; the remote annotated tag and peeled commit are present.

## Completion Record

Record the following in the final response only after all steps pass:

- U1 through U5 commit hashes and any review-fix commits.
- Focused RED observation for each unit and focused GREEN command result.
- Results of all seven full verification commands.
- Opus review outcome and any confirmed findings resolved.
- Local/remote `main` commit and peeled `v0.2.2` commit.
- GitHub Actions tag-run URL and conclusion.
- GitHub release URL, exact 15-asset count, and successful aggregate/individual checksum verification.
- Explicit confirmation that no CPA core, dependency pin, ABI, `.integration/`, or tracked `dist/` artifact changed.
