# Censorship Plugin v0.2.6 Functional Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix every confirmed provider-contract gap from the 2026-09-13 audit, preserve explicit selector and machine-field boundaries, and publish verified patch release `v0.2.6`.

**Architecture:** Extend the three existing provider-local collectors with exact discriminators and named natural-language leaves. Reuse the existing JSON Schema description walker and span immutability mechanism; do not change matcher, transform, configuration, ABI, or CLIProxyAPI core. Implement the independent Interactions, OpenAI, and Anthropic test/source pairs concurrently under exclusive file ownership, then update the shared fuzz oracles, documentation, review, verification, merge, tag, push, and monitor sequentially.

**Tech Stack:** Go 1.26, `github.com/tidwall/gjson`, Go testing/fuzzing, CGO shared-library ABI v1, GNU Make under MSYS on Windows, Git, GitHub CLI and GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-13-censorship-v0.2.6-functional-audit-design.md`

## Global Constraints

- Modify only this censorship plugin; do not modify CLIProxyAPI core or unrelated pinned dependency code.
- Keep native ABI v1, CGO ownership, pointer lifetimes, `C.GoBytes`, and the pinned `github.com/router-for-me/CLIProxyAPI/v7` integration unchanged.
- Keep `words` as the strict ordered `block`, `strip`, and `obfs` object and keep execution order `block` -> `strip` -> `obfs`.
- Select only current provider-documented explicit natural-language leaves. Never add a generic recursive string walker or inspect machine fields.
- Preserve raw request bytes outside selected JSON string tokens.
- Keep default roles exactly `system`, `developer`, and `user`; `assistant` and `tool` remain opt-in.
- Add a focused failing test before each behavior implementation.
- Do not read old task plan, spec, or TDD documents. The spec named above and this plan are the only task documents to use.
- Do not hand-edit or commit `.integration/` or `dist/`.
- Coding agents use `claude-sonnet-5[1m]` with `xhigh` effort and mutually exclusive file ownership.
- Review, plan verification, and release decisions use `claude-opus-5[1m]` with `xhigh` effort.

## File map and ownership

- `selectors.go`: source-format role gate. Owned only by the Interactions task.
- `selectors_interactions.go`: Interactions content, step, tool, schema, annotation, and signature selection. Owned only by the Interactions task.
- `selectors_interactions_test.go`: focused Interactions regressions. Owned only by the Interactions task.
- `selectors_openai.go`: Responses local-skill role and MCP approval selection. Owned only by the OpenAI task.
- `selectors_openai_test.go`: focused OpenAI regressions. Owned only by the OpenAI task.
- `selectors_claude.go`: compaction and Beta MCP result selection. Owned only by the Anthropic task.
- `selectors_claude_test.go`: focused Anthropic regressions. Owned only by the Anthropic task.
- `fuzz_test.go`: both independent protocol oracles and deterministic seeds. Owned only by the oracle task after all provider tasks finish.
- `README.md`, `RELEASE_NOTES.md`, `main_test.go`: current contract and `v0.2.6` release documentation. Owned only by the documentation task after code/oracle tests pass.
- `docs/superpowers/specs/2026-09-13-censorship-v0.2.6-functional-audit-design.md` and this plan: task design records, committed before implementation and left unchanged unless self-review finds a concrete mismatch.

---

### Task 1: Preserve the approved task documents and enter an isolated worktree

**Files:**
- Add: `docs/superpowers/specs/2026-09-13-censorship-v0.2.6-functional-audit-design.md`
- Add: `docs/superpowers/plans/2026-09-13-censorship-v0.2.6-functional-audit.md`

**Interfaces:**
- Consumes: clean baseline commit `1e13a967e37774089256bb75df973e896a6445e7` and the two current-task documents.
- Produces: a planning commit on local `main` and an isolated feature worktree based on `origin/main`; the final merge combines both histories.

- [ ] **Step 1: Confirm only current-task files are untracked**

Run:

```powershell
git status --short --branch
git diff --check
git diff --no-index -- /dev/null docs/superpowers/specs/2026-09-13-censorship-v0.2.6-functional-audit-design.md
git diff --no-index -- /dev/null docs/superpowers/plans/2026-09-13-censorship-v0.2.6-functional-audit.md
```

Expected: tracked files are unchanged; only `.claude/` and the two new documents are untracked. The two `git diff --no-index` commands return status 1 because they display intentional additions, with no whitespace error.

- [ ] **Step 2: Commit only the design and plan**

Run:

```powershell
git add -- docs/superpowers/specs/2026-09-13-censorship-v0.2.6-functional-audit-design.md docs/superpowers/plans/2026-09-13-censorship-v0.2.6-functional-audit.md
git diff --cached --check
git diff --cached --stat
git commit -m "docs: plan censorship v0.2.6 audit fixes"
```

Expected: the commit contains exactly the two current-task Markdown files. `.claude/task-memory-2026-09-13-functional-audit.md` remains untracked.

- [ ] **Step 3: Invoke the required worktree skill and create the feature worktree**

Invoke `superpowers:using-git-worktrees`, then create a new worktree named `censorship-v0.2.6-audit-fixes`. Verify its branch starts at `origin/main` commit `1e13a967e37774089256bb75df973e896a6445e7` and that the original local `main` retains the planning commit.

Run inside the worktree:

```powershell
git status --short --branch
git rev-parse HEAD
git merge-base --is-ancestor 1e13a967e37774089256bb75df973e896a6445e7 HEAD
```

Expected: clean feature branch, `HEAD` at the baseline or a descendant of it, and the ancestry command exits 0.

### Task 2: Fix Interactions annotation ownership and documented text surfaces

**Files:**
- Modify: `selectors.go:60-72`
- Modify: `selectors_interactions.go:5-137`
- Test: `selectors_interactions_test.go`

**Interfaces:**
- Consumes: `appendStringSpan`, `appendJSONSchemaDescriptions`, `scanTextPart`, `textSpan.RequiresUnmodified`, and canonical `scopeSet` roles.
- Produces: `appendInteractionTextPart(spans *[]textSpan, part gjson.Result, role string, roles scopeSet, protectAnnotations bool)` plus provider-local exact result/tool/schema collectors used only by `collectInteractions` and `collectInteractionItem`.

- [ ] **Step 1: Add failing annotation-ownership tests**

Add tests with these exact behavioral assertions:

```go
func TestInteractionsCallerAnnotationsRemainRewritable(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
	body := []byte(`{"input":{"type":"text","text":"SECRET user","annotations":[{"type":"url_citation","start_index":0,"end_index":6,"url":"https://example.com"}]}}`)
	want := `{"input":{"type":"text","text":" user","annotations":[{"type":"url_citation","start_index":0,"end_index":6,"url":"https://example.com"}]}}`
	resp := interceptRPC(t, "interactions", body)
	if resp.Terminate || string(resp.Body) != want {
		t.Fatalf("response = %#v, body = %s, want %s", resp, resp.Body, want)
	}
}
```

Keep the existing annotated `model_output.content` rewrite rejection and block tests unchanged as the positive protection control.

- [ ] **Step 2: Add failing tool-result and signed-result tests**

Cover all of these cases under explicit `scope.roles: [tool]`:

```go
{"input":[
  {"type":"function_result","call_id":"SECRET id","name":"SECRET name","result":"SECRET function"},
  {"type":"function_result","result":[{"type":"text","text":"SECRET typed"},{"type":"image","data":"SECRET image"},{"type":"unknown","text":"SECRET unknown"}]},
  {"type":"function_result","content":"SECRET obsolete","result":{"text":"SECRET object"}},
  {"type":"mcp_server_tool_result","call_id":"SECRET id","server_name":"SECRET server","name":"SECRET name","result":"SECRET mcp"},
  {"type":"mcp_server_tool_result","result":[{"type":"text","text":"SECRET mcp typed"},{"type":"image","data":"SECRET image"}]},
  {"type":"code_execution_result","call_id":"SECRET id","result":"SECRET code","is_error":false}
]}
```

For `strip`, expect only `function`, `typed`, `mcp`, `mcp typed`, and unsigned `code` result text to change. Assert every ID/name/server/image/object/unknown/obsolete field remains byte-for-byte unchanged. Add canonical-role `block` rows for each exact result type and expect role `tool`.

Add separate signed tests:

```go
func TestInteractionsSignedCodeExecutionResultRejectsRewrite(t *testing.T) {
	registerConfig(t, "words:\n  obfs: [SECRET]\nscope:\n  roles: [tool]\n")
	body := []byte(`{"input":[{"type":"code_execution_result","call_id":"call","result":"SECRET","signature":"signed"}]}`)
	resp := interceptRPC(t, "interactions", body)
	if !resp.Terminate || resp.StatusCode != 400 || gjson.GetBytes(resp.ResponseBody, "error.message").String() != "censorship cannot rewrite signature-bound text" {
		t.Fatalf("response = %#v", resp)
	}
}
```

Also assert the same body with `block` reports role `tool`, and `signature: null` permits the rewrite.

- [ ] **Step 3: Add failing function-tool and response-schema tests**

Under `scope.roles: [developer]`, use a request containing:

```json
{
  "tools": [
    {
      "type": "function",
      "name": "SECRET function name",
      "description": "SECRET function description",
      "parameters": {
        "type": "object",
        "description": "SECRET root schema",
        "properties": {
          "SECRET key": {
            "type": "string",
            "description": "SECRET property",
            "enum": ["SECRET enum"],
            "default": "SECRET default"
          }
        },
        "required": ["SECRET key"]
      }
    },
    {"type": "google_search", "description": "SECRET excluded"},
    {"description": "SECRET missing type"}
  ],
  "response_format": [
    {"type": "text", "mime_type": "application/json", "schema": {"type": "object", "description": "SECRET response", "properties": {"value": {"description": "SECRET nested response"}}}},
    {"type": "image", "schema": {"description": "SECRET image schema"}},
    {"schema": {"description": "SECRET missing type schema"}}
  ]
}
```

Expect only the function description, JSON Schema description leaves, and exact text response schema description leaves to change. Add a single-object `response_format` test and canonical-role block rows expecting `developer`. Add negative role tests showing user-only scope leaves those fields unchanged.

- [ ] **Step 4: Run the focused tests and confirm red state**

Run:

```powershell
go test . -run '^TestInteractions(CallerAnnotationsRemainRewritable|DocumentedToolResults|SignedCodeExecutionResult|FunctionToolsAndResponseSchemas|SelectorCanonicalRoles)$' -count=1
```

Expected before implementation: failures show caller annotations rejected or omitted output, result/tool/schema leaves unselected, and no signed code-result rejection. Existing model-output annotation tests still pass.

- [ ] **Step 5: Implement explicit annotation ownership**

Change the helper signature and only apply the annotation metadata when requested:

```go
func appendInteractionTextPart(spans *[]textSpan, part gjson.Result, role string, roles scopeSet, protectAnnotations bool) {
	text, allowed, _ := scanTextPart(part, true)
	if !allowed {
		return
	}
	before := len(*spans)
	appendStringSpan(spans, text, role, roles)
	annotations := part.Get("annotations")
	if protectAnnotations && len(*spans) != before && annotations.IsArray() && annotations.Get("#").Int() > 0 {
		span := &(*spans)[len(*spans)-1]
		span.RequiresUnmodified = true
		span.UnmodifiedMessage = "censorship cannot rewrite annotated text"
	}
}
```

Pass `false` for system, direct input, `user_input`, parts, and tool-result text. In `collectInteractionItem`, derive `protectAnnotations := itemType.Type == gjson.String && itemType.Str == "model_output"` and pass it only for that item's `content` object/array. Do not enable it for `parts` or inherited nested items.

- [ ] **Step 6: Implement exact Interactions result steps**

Before ordinary role and content discrimination in `collectInteractionItem`, dispatch the exact type:

```go
func collectInteractionResult(item gjson.Result, roles scopeSet, spans *[]textSpan) bool {
	itemType := item.Get("type")
	if itemType.Type != gjson.String {
		return false
	}
	switch itemType.Str {
	case "function_result", "mcp_server_tool_result":
		appendInteractionResult(spans, item.Get("result"), roles)
		return true
	case "code_execution_result":
		before := len(*spans)
		appendStringSpan(spans, item.Get("result"), "tool", roles)
		if len(*spans) != before && item.Get("signature").Type != gjson.Null {
			(*spans)[len(*spans)-1].RequiresUnmodified = true
		}
		return true
	default:
		return false
	}
}

func appendInteractionResult(spans *[]textSpan, result gjson.Result, roles scopeSet) {
	if !roles.has("tool") {
		return
	}
	if result.Type == gjson.String {
		appendStringSpan(spans, result, "tool", roles)
		return
	}
	if !result.IsArray() {
		return
	}
	result.ForEach(func(_, part gjson.Result) bool {
		partType := part.Get("type")
		if part.IsObject() && partType.Type == gjson.String && partType.Str == "text" {
			appendInteractionTextPart(spans, part, "tool", roles, false)
		}
		return true
	})
}
```

Call `collectInteractionResult(item, roles, spans)` immediately after confirming `item.IsObject()`, and return when it handles the item. `signature` absent has `gjson.Null` type and remains rewritable; present non-null values bind the selected span.

- [ ] **Step 7: Implement exact function and response schema collection**

Add provider-local root collection:

```go
func collectInteractionDefinitions(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	if !roles.has("developer") {
		return
	}
	tools := root.Get("tools")
	if tools.IsArray() {
		tools.ForEach(func(_, tool gjson.Result) bool {
			toolType := tool.Get("type")
			if tool.IsObject() && toolType.Type == gjson.String && toolType.Str == "function" {
				appendStringSpan(spans, tool.Get("description"), "developer", roles)
				appendJSONSchemaDescriptions(spans, tool.Get("parameters"), "developer", roles)
			}
			return true
		})
	}
	format := root.Get("response_format")
	append := func(candidate gjson.Result) {
		candidateType := candidate.Get("type")
		if candidate.IsObject() && candidateType.Type == gjson.String && candidateType.Str == "text" {
			appendJSONSchemaDescriptions(spans, candidate.Get("schema"), "developer", roles)
		}
	}
	if format.IsArray() {
		format.ForEach(func(_, candidate gjson.Result) bool { append(candidate); return true })
	} else {
		append(format)
	}
}
```

Call it at the start of `collectInteractions`. Extend `selectorHasEnabledRole("interactions", roles)` to include `developer` and `tool` without changing any other format gate. Change the collector's input early return to preserve developer-only short-circuiting after root definitions while admitting a tool-only scope:

```go
if !roles.has("user") && !roles.has("assistant") && !roles.has("tool") {
	return
}
```

- [ ] **Step 8: Run Interactions tests to green**

Run:

```powershell
gofmt -w selectors.go selectors_interactions.go selectors_interactions_test.go
go test . -run '^TestInteractions|^TestScanTextPart' -count=1
```

Expected: all Interactions and shared text-part tests pass. Inspect the diff and confirm no generic traversal or machine-field selector was added.

### Task 3: Fix OpenAI additional-tools local-skill roles and MCP approval reasons

**Files:**
- Modify: `selectors_openai.go:58-110,224-338,365-429`
- Test: `selectors_openai_test.go`

**Interfaces:**
- Consumes: existing `collectOpenAIResponsesTool`, `openAIAdditionalToolsRole`, `appendStringSpan`, and canonical roles.
- Produces: role-correct local skill descriptions and exact `mcp_approval_response.reason` selection without changing other Responses item handling.

- [ ] **Step 1: Add failing additional-tools local-skill role tests**

Add a developer-only request and its negative user-only control:

```go
func TestOpenAIResponsesAdditionalToolsLocalSkillUsesItemRole(t *testing.T) {
	body := []byte(`{"input":[{"type":"additional_tools","role":"developer","tools":[{"type":"shell","environment":{"type":"local","skills":[{"name":"SECRET name","path":"/SECRET/path","description":"SECRET description"}]}}]}]}`)
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [developer]\n")
	resp := interceptRPC(t, "openai-response", body)
	want := []byte(`{"input":[{"type":"additional_tools","role":"developer","tools":[{"type":"shell","environment":{"type":"local","skills":[{"name":"SECRET name","path":"/SECRET/path","description":" description"}]}}]}]}`)
	if resp.Terminate || !bytes.Equal(resp.Body, want) {
		t.Fatalf("response = %#v body = %s want = %s", resp, resp.Body, want)
	}
}
```

Run the same body with only `user` enabled and expect no body. Keep `TestOpenAIResponsesTopLevelLocalSkillUsesUserScope` as the regression control. Preserve current rejection of missing/noncanonical additional-tools roles.

- [ ] **Step 2: Add failing MCP approval response tests**

Under user-only scope, assert only `reason` changes:

```go
func TestOpenAIResponsesMCPApprovalReasonUsesUserScope(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
	body := []byte(`{"input":[{"type":"mcp_approval_response","approval_request_id":"SECRET request","approve":true,"id":"SECRET id","reason":"SECRET reason","extra":"SECRET extra"}]}`)
	want := []byte(`{"input":[{"type":"mcp_approval_response","approval_request_id":"SECRET request","approve":true,"id":"SECRET id","reason":" reason","extra":"SECRET extra"}]}`)
	resp := interceptRPC(t, "openai-response", body)
	if resp.Terminate || !bytes.Equal(resp.Body, want) {
		t.Fatalf("response = %#v body = %s want = %s", resp, resp.Body, want)
	}
}
```

Add a `block` canonical-role assertion expecting `user`, a developer-only negative control, and null/non-string reason cases that remain unchanged.

- [ ] **Step 3: Run focused OpenAI tests and confirm red state**

Run:

```powershell
go test . -run '^TestOpenAIResponses(AdditionalToolsLocalSkillUsesItemRole|TopLevelLocalSkillUsesUserScope|MCPApprovalReasonUsesUserScope)$' -count=1
```

Expected before implementation: the additional-tools developer test has no selected text and the approval reason remains unchanged. The existing top-level user-role test passes.

- [ ] **Step 4: Make local skill collection honor the supplied role**

In the local shell branch of `collectOpenAIResponsesTool`, replace the hard-coded user checks and append role with the supplied `role`:

```go
if tool.Get("type").Str == "shell" {
	environment := tool.Get("environment")
	if environment.Get("type").Str != "local" || !roles.has(role) {
		return
	}
	skills := environment.Get("skills")
	if skills.IsArray() {
		skills.ForEach(func(_, skill gjson.Result) bool {
			appendStringSpan(spans, skill.Get("description"), role, roles)
			return true
		})
	}
	return
}
```

At the top-level `root.tools` caller, pass `user` only when the exact tool type is `shell`; pass `developer` for all other existing tool variants:

```go
role := "developer"
if tool.Get("type").Str == "shell" {
	role = "user"
}
collectOpenAIResponsesTool(spans, tool, role, roles)
```

Do not change additional-tools or loaded-tool callers; their existing validated role reaches the helper.

- [ ] **Step 5: Select only MCP approval `reason`**

Add this exact branch to `collectOpenAIResponsesToolOutput` before the default:

```go
case "mcp_approval_response":
	if roles.has("user") {
		appendStringSpan(spans, item.Get("reason"), "user", roles)
	}
	return true
```

Do not inspect or validate `approve`, IDs, or unknown siblings.

- [ ] **Step 6: Run OpenAI tests to green**

Run:

```powershell
gofmt -w selectors_openai.go selectors_openai_test.go
go test . -run '^TestOpenAI' -count=1
```

Expected: all OpenAI Chat and Responses selector tests pass, including top-level local skill role preservation.

### Task 4: Fix Anthropic compaction instructions and Beta MCP tool-result text

**Files:**
- Modify: `selectors_claude.go:5-97`
- Test: `selectors_claude_test.go`

**Interfaces:**
- Consumes: `appendStringSpan`, `appendNonEmptyStringSpan`, existing message role validation, and exact content block discriminators.
- Produces: `compact_20260112.instructions` as `system` and exact Beta MCP content as `tool`, while stable `tool_result` traversal remains unchanged.

- [ ] **Step 1: Add failing compaction instruction tests**

Use a request with no `messages` to prove collection occurs before the early return:

```go
func TestClaudeCompactionInstructionsUseSystemScope(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system]\n")
	body := []byte(`{"context_management":{"edits":[{"type":"compact_20260112","instructions":"SECRET instructions","trigger":{"type":"input_tokens","value":1000},"pause_after_compaction":true},{"type":"future","instructions":"SECRET future"},{"type":"compact_20260112","instructions":null}]}}`)
	want := []byte(`{"context_management":{"edits":[{"type":"compact_20260112","instructions":" instructions","trigger":{"type":"input_tokens","value":1000},"pause_after_compaction":true},{"type":"future","instructions":"SECRET future"},{"type":"compact_20260112","instructions":null}]}}`)
	resp := interceptRPC(t, "claude", body)
	if resp.Terminate || !bytes.Equal(resp.Body, want) {
		t.Fatalf("response = %#v body = %s want = %s", resp, resp.Body, want)
	}
}
```

Add a canonical-role block assertion expecting `system` and a user-only negative control.

- [ ] **Step 2: Add failing Beta MCP result tests**

Under `scope.roles: [tool]`, cover scalar and exact typed content while preserving all machine fields:

```json
{
  "messages": [{
    "role": "user",
    "content": [
      {"type": "mcp_tool_result", "tool_use_id": "SECRET id", "is_error": false, "content": "SECRET scalar", "name": "SECRET name"},
      {"type": "mcp_tool_result", "content": [
        {"type": "text", "text": "SECRET typed"},
        {"type": "image", "source": {"data": "SECRET image"}},
        {"type": "document", "title": "SECRET document"},
        {"type": "unknown", "text": "SECRET unknown"},
        {"text": "SECRET missing type"}
      ]},
      {"type": "mcp_tool_result", "content": {"text": "SECRET object"}}
    ]
  }]
}
```

Expect only scalar and exact typed text to change. Add `block` role `tool`, user-only negative, null and non-string controls. Retain existing stable `tool_result` tests without changing their expectations.

- [ ] **Step 3: Run focused Anthropic tests and confirm red state**

Run:

```powershell
go test . -run '^TestClaude(CompactionInstructionsUseSystemScope|BetaMCPToolResult|TopLevelSystemStringAndToolScope)$' -count=1
```

Expected before implementation: compaction and Beta MCP result leaves remain unchanged; existing stable behavior passes.

- [ ] **Step 4: Add exact compaction collection before message handling**

At the start of `collectClaude`, after ordinary top-level system handling and before reading `messages`, add:

```go
if roles.has("system") {
	edits := root.Get("context_management.edits")
	if edits.IsArray() {
		edits.ForEach(func(_, edit gjson.Result) bool {
			editType := edit.Get("type")
			if edit.IsObject() && editType.Type == gjson.String && editType.Str == "compact_20260112" {
				appendStringSpan(spans, edit.Get("instructions"), "system", roles)
			}
			return true
		})
	}
}
```

This deliberately ignores unknown edit types and every sibling field.

- [ ] **Step 5: Add an exact Beta MCP block branch**

In the message content switch, add a separate branch rather than widening `appendClaudeResultText`:

```go
case "mcp_tool_result":
	if role.Str != "user" || !roles.has("tool") {
		return true
	}
	toolContent := block.Get("content")
	appendStringSpan(spans, toolContent, "tool", roles)
	if !toolContent.IsArray() {
		return true
	}
	toolContent.ForEach(func(_, inner gjson.Result) bool {
		innerType := inner.Get("type")
		if inner.IsObject() && innerType.Type == gjson.String && innerType.Str == "text" {
			appendStringSpan(spans, inner.Get("text"), "tool", roles)
		}
		return true
	})
```

Keep stable `tool_result` logic byte-for-byte except any formatting forced by `gofmt`.

- [ ] **Step 6: Run Anthropic tests to green**

Run:

```powershell
gofmt -w selectors_claude.go selectors_claude_test.go
go test . -run '^TestClaude' -count=1
```

Expected: all Anthropic selector tests pass, including stable search/document/tool-result coverage.

### Task 5: Run the three provider TDD tasks concurrently and integrate their disjoint edits

**Files:**
- Modify only the seven provider-owned files listed in Tasks 2 through 4.

**Interfaces:**
- Consumes: the exact task boundaries and tests above.
- Produces: a combined worktree with three independently green provider selector suites and no shared-file conflict.

- [ ] **Step 1: Invoke required TDD execution skills**

Invoke `superpowers:test-driven-development`, then `superpowers:subagent-driven-development` or `superpowers:executing-plans` as required by the execution mode. Use a workflow with exactly three concurrent `claude-sonnet-5[1m]`, `xhigh` coding agents:

1. Interactions owns only `selectors.go`, `selectors_interactions.go`, `selectors_interactions_test.go` and executes Task 2.
2. OpenAI owns only `selectors_openai.go`, `selectors_openai_test.go` and executes Task 3.
3. Anthropic owns only `selectors_claude.go`, `selectors_claude_test.go` and executes Task 4.

Tell every agent not to edit, stage, commit, or inspect any file outside its ownership. No two agents scan or edit the same scope. Each agent must record the focused command's red failure before source edits and its green result after source edits in its return report.

- [ ] **Step 2: Review the combined unstaged diff**

Run:

```powershell
git status --short
git diff --check
git diff -- selectors.go selectors_interactions.go selectors_interactions_test.go selectors_openai.go selectors_openai_test.go selectors_claude.go selectors_claude_test.go
```

Expected: exactly the seven permitted files changed, no generated files, and no overlapping ownership violation. Verify each new selector is an exact discriminator/leaf selector.

- [ ] **Step 3: Run all provider selector tests together**

Run:

```powershell
go test . -run '^(TestInteractions|TestScanTextPart|TestOpenAI|TestClaude)' -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit provider behavior**

Run:

```powershell
git add -- selectors.go selectors_interactions.go selectors_interactions_test.go selectors_openai.go selectors_openai_test.go selectors_claude.go selectors_claude_test.go
git diff --cached --check
git commit -m "fix: cover documented provider text inputs"
```

Expected: commit contains only the three provider source/test domains.

### Task 6: Synchronize both fuzz protocol oracles

**Files:**
- Modify: `fuzz_test.go:507-721,1239-1490,1588-1716,1838-2110,2183-2308`

**Interfaces:**
- Consumes: final provider selector behavior from Tasks 2 through 5 and the existing standard `encoding/json` oracle plus independent raw-token parser oracle.
- Produces: both oracles agree on selected text, canonical roles, raw ranges, annotation immutability, and signature immutability for every changed provider path under the fuzzer's default role policy.

- [ ] **Step 1: Add deterministic provider seeds and oracle assertions**

Add seeds for:

```go
{format: "openai-response", body: []byte(`{"input":[{"type":"additional_tools","role":"developer","tools":[{"type":"shell","environment":{"type":"local","skills":[{"description":"SECRET"}]}}]},{"type":"mcp_approval_response","approve":true,"approval_request_id":"id","reason":"SECRET"}]}`)},
{format: "claude", body: []byte(`{"context_management":{"edits":[{"type":"compact_20260112","instructions":"SECRET"}]},"messages":[{"role":"user","content":[{"type":"mcp_tool_result","content":[{"type":"text","text":"SECRET"}]}]}]}`)},
{format: "interactions", body: []byte(`{"tools":[{"type":"function","description":"SECRET","parameters":{"description":"SECRET"}}],"response_format":{"type":"text","schema":{"description":"SECRET"}},"input":[{"type":"function_result","result":"SECRET"},{"type":"mcp_server_tool_result","result":[{"type":"text","text":"SECRET"}]},{"type":"code_execution_result","result":"SECRET","signature":"sig"}]}`)},
{format: "interactions", body: []byte(`{"input":{"type":"text","text":"SECRET","annotations":[{"type":"url_citation"}]}}`)},
```

Add a deterministic unit test that directly compares standard and raw oracle spans for the developer/user paths selected by default and confirms caller annotations do not set `RequiresUnmodified`. Tool and assistant paths remain filtered by the fuzz harness's default role set, while focused selector tests cover their opt-in behavior.

- [ ] **Step 2: Update the standard OpenAI and Anthropic oracles**

In `oracleAppendOpenAIResponsesTool`, append local skill descriptions with the supplied `role`, and pass `user` for exact top-level shell tools in `oracleOpenAIResponseSpans`. Add exact `mcp_approval_response.reason` handling before message handling.

In `oracleClaudeSpans`, scan exact `compact_20260112.instructions` before the messages return. Add exact `mcp_tool_result` scalar and typed-text-array handling under canonical role `tool`; do not call the search/document walker for Beta MCP content.

- [ ] **Step 3: Update the standard Interactions oracle**

Add root function declaration and response format schema descriptions as `developer`. In `oracleInteractionItem`, dispatch exact result types before ordinary role/type rejection. For code execution results, append `result` as `tool` and set `RequiresUnmodified` only when a selected span exists and `signature` is present and non-null.

Thread `protectAnnotations bool` through `oracleInteractionPart` and `oracleAppendInteractionString`; pass true only for exact `model_output.content`, false for every other caller. Require exact `type: "text"` for result-array parts.

- [ ] **Step 4: Apply the same logic independently to the raw-token oracle**

Mirror every Task 6 Step 2 and Step 3 path through `oracleRawValue.firstField`, `oracleRawStringField`, and `oracleRawAppendString`. Preserve raw token `Start` and `End`. Thread the same explicit annotation flag, and mark signed code result tokens with the default empty `UnmodifiedMessage` so the transformer emits `censorship cannot rewrite signature-bound text`.

- [ ] **Step 5: Accept Interactions signature-bound invalid results in the differential checker**

Generalize the Interactions invalid-result branch without weakening annotation checks:

```go
if format == "interactions" && (selected == modeStrip || selected == modeObfs) && got.Blocked == nil && len(got.Body) == 0 {
	switch got.InvalidMessage {
	case "censorship cannot rewrite annotated text":
		if oracleInteractionsAnnotatedSpanMatches(body, fold) { return nil }
	case "censorship cannot rewrite signature-bound text":
		if oracleInteractionsSignatureBoundSpanMatches(body, fold) { return nil }
	}
}
```

Implement `oracleInteractionsSignatureBoundSpanMatches` exactly like the existing Gemini helper but using `oracleProtocolSpans("interactions", body)` and requiring `RequiresUnmodified` with an empty `UnmodifiedMessage`.

- [ ] **Step 6: Run oracle unit tests and deterministic fuzz seeds**

Run:

```powershell
gofmt -w fuzz_test.go
go test . -run '^(TestOracle|TestProtocolOracle|TestInteractions|TestOpenAI|TestClaude)' -count=1
go test . -run '^FuzzProtocolTransform$' -count=1
```

Expected: standard and raw oracles agree, deterministic seeds pass in all three modes and both case-fold settings, and existing excluded-token checks remain green.

- [ ] **Step 7: Commit the oracle synchronization**

Run:

```powershell
git add -- fuzz_test.go
git diff --cached --check
git commit -m "test: extend provider protocol oracles"
```

Expected: commit modifies only `fuzz_test.go`.

### Task 7: Update current contract and release documentation

**Files:**
- Modify: `README.md:23,106-129`
- Modify: `RELEASE_NOTES.md:1-18`
- Modify: `main_test.go:639-647`

**Interfaces:**
- Consumes: verified selector behavior and unchanged CLIProxyAPI compatibility target v7.2.152, schema 5, host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`, native ABI v1.
- Produces: accurate `v0.2.6` current-version prose and a regression assertion that release notes name the current target.

- [ ] **Step 1: Make the current-version documentation assertion fail**

Change the test to require the new target line:

```go
func TestReleaseNotesCompatibilityTargetsCurrentVersion(t *testing.T) {
	raw, err := os.ReadFile("RELEASE_NOTES.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("v0.2.6 targets CLIProxyAPI v7.2.152")) {
		t.Fatal("RELEASE_NOTES.md does not target CLIProxyAPI v7.2.152 for v0.2.6")
	}
}
```

Run:

```powershell
go test . -run '^TestReleaseNotesCompatibilityTargetsCurrentVersion$' -count=1
```

Expected: FAIL because release notes still identify v0.2.5 as current.

- [ ] **Step 2: Update README contract prose**

Change the current target line to `v0.2.6` without changing the compatibility values. Update explicit selector bullets to state:

- Interactions function/MCP result strings and exact typed-text array items are `tool`.
- Interactions code execution result strings are `tool`, and a non-null sibling signature permits block but rejects a final rewrite.
- Only non-empty annotations on exact `model_output.content` text protect rewrites; caller-authored annotated text remains rewritable.
- Interactions function declaration descriptions, parameter JSON Schema description leaves, and exact text response schema description leaves are `developer`.
- OpenAI `mcp_approval_response.reason` is `user`; top-level local skills remain `user`, while additional-tools local skills inherit the validated item role.
- Anthropic exact compaction instructions are `system`; Beta MCP string/exact typed-text results are `tool`.

Narrow the existing Interactions function/tool exclusion sentence so it excludes machine fields and unsupported variants rather than the newly documented result and description leaves.

- [ ] **Step 3: Add a v0.2.6 release-notes section**

Make `# Censorship v0.2.6` the title and add `## v0.2.6 fixes` before historical v0.2.5 content. List only the confirmed changes implemented in this plan. Add the exact compatibility line:

```markdown
v0.2.6 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.
```

Retain the complete historical v0.2.5 notes below the new section without rewriting their claims.

- [ ] **Step 4: Run docs assertions to green**

Run:

```powershell
gofmt -w main_test.go
go test . -run '^(TestReleaseNotesCompatibilityTargetsCurrentVersion|TestReadmeDocumentsOperationalContracts)$' -count=1
```

Expected: PASS. If the README contract test requires literal snippets, update its expected list only for text deliberately changed in this task.

- [ ] **Step 5: Commit documentation**

Run:

```powershell
git add -- README.md RELEASE_NOTES.md main_test.go
git diff --cached --check
git commit -m "docs: prepare censorship v0.2.6"
```

Expected: commit contains only the three documentation/test files.

### Task 8: Simplify and review only the changed code

**Files:**
- Review: all files changed since `1e13a967e37774089256bb75df973e896a6445e7`, excluding the two planning documents already reviewed and generated paths.

**Interfaces:**
- Consumes: green focused tests and the approved spec.
- Produces: no unresolved high-confidence correctness, scope, signed-state, role, or byte-preservation finding.

- [ ] **Step 1: Run a changed-code simplification pass**

Invoke `code-simplifier:code-simplifier` only on the newly changed Go code. Instruct it to preserve behavior, avoid abstraction for one use, avoid unrelated edits, and report any suggestion before changing a file. Apply only changes that reduce code while retaining exact discriminators and all focused tests.

- [ ] **Step 2: Invoke the required review skill**

Invoke `superpowers:requesting-code-review`. Use `claude-opus-5[1m]`, `xhigh` review agents with mutually exclusive dimensions rather than duplicate scans:

1. Provider contract and canonical-role correctness.
2. Raw byte preservation, machine exclusions, annotation/signature immutability.
3. Test and fuzz-oracle agreement, missing cases, and regression risk.

Each finding must cite a changed file/line and a reproducible request. Adversarially verify every proposed finding before accepting it.

- [ ] **Step 3: Fix only confirmed findings with TDD**

For each confirmed finding, first add the smallest failing regression to its existing provider test file or `fuzz_test.go`, run it red, make the minimal source/oracle fix, and run it green. Do not implement plausible or speculative suggestions. Re-run:

```powershell
go test . -run '^(TestInteractions|TestScanTextPart|TestOpenAI|TestClaude|TestOracle|TestProtocolOracle)' -count=1
git diff --check
```

Expected: PASS and no whitespace errors.

- [ ] **Step 4: Commit any review fixes**

If there are actual changes, stage only the confirmed review-fix files and commit:

```powershell
git diff --check
git add -- selectors.go selectors_interactions.go selectors_interactions_test.go selectors_openai.go selectors_openai_test.go selectors_claude.go selectors_claude_test.go fuzz_test.go
git diff --cached --check
git commit -m "fix: address v0.2.6 review findings"
```

If there are no confirmed findings or no diff, skip the commit and record that fact in the final report.

### Task 9: Run complete local verification

**Files:**
- Generated but never committed: `.integration/`, `dist/`.

**Interfaces:**
- Consumes: reviewed feature branch.
- Produces: fresh successful evidence for focused tests, full tests, race, vet, integration, native build, and package.

- [ ] **Step 1: Invoke verification-before-completion**

Invoke `superpowers:verification-before-completion` and use its evidence requirements for every claim below.

- [ ] **Step 2: Capture the Windows environment forwarded through MSYS make**

In PowerShell, evaluate these values immediately before invoking Bash/MSYS:

```powershell
$env:USERPROFILE
$env:LOCALAPPDATA
$env:APPDATA
$env:HOME
$env:TEMP
$env:TMP
go env GOCACHE GOPATH GOMODCACHE
```

All must resolve to writable current-user locations. Do not use `C:\WINDOWS` as a Go work/cache directory.

- [ ] **Step 3: Run focused and full unit tests**

Run:

```powershell
go test . -run '^(TestInteractions|TestScanTextPart|TestOpenAI|TestClaude|TestOracle|TestProtocolOracle)' -count=1
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 4: Run every Makefile verification target with explicit environment forwarding**

Run the targets separately in this order through one Bash/MSYS command, preserving each exit status:

```bash
run_make() {
  make \
    USERPROFILE="$USERPROFILE" \
    LOCALAPPDATA="$LOCALAPPDATA" \
    APPDATA="$APPDATA" \
    HOME="$HOME" \
    TEMP="$TEMP" \
    TMP="$TMP" \
    GOCACHE="$(go env GOCACHE)" \
    GOPATH="$(go env GOPATH)" \
    GOMODCACHE="$(go env GOMODCACHE)" \
    "$@"
}
run_make test
run_make race
run_make vet
run_make integration
run_make build
run_make package VERSION=v0.2.6
```

Expected: every target exits 0; host native library and matching archive/checksum are produced.

- [ ] **Step 5: Verify repository state and generated-file exclusion**

Run:

```powershell
git status --short
git diff --check
git log --oneline --decorate -5
git diff --stat 1e13a967e37774089256bb75df973e896a6445e7..HEAD
git ls-files -- .integration dist
git status --short -- .integration dist
```

Expected: no tracked `.integration/` or `dist/` file. Remove generated untracked artifacts only after inspecting the listed paths and confirming they were created by this verification. The feature branch must otherwise be clean.

### Task 10: Merge into local main and create the patch tag

**Files:**
- No source edits.

**Interfaces:**
- Consumes: clean verified feature branch plus local `main` planning commit.
- Produces: local `main` containing the planning documents and all verified implementation commits, annotated tag `v0.2.6` at that exact commit.

- [ ] **Step 1: Confirm remote state before merge or tag**

Run:

```powershell
git fetch origin --prune --tags
git status --short --branch
git rev-parse origin/main
git tag --list v0.2.6
git merge-base --is-ancestor 1e13a967e37774089256bb75df973e896a6445e7 HEAD
```

Expected: feature branch clean, baseline is an ancestor, and `v0.2.6` does not exist locally or remotely. If `origin/main` advanced, inspect only the incoming diff relevant to merge compatibility, merge it into the feature branch, and rerun Task 9 before proceeding. Never overwrite a pre-existing tag.

- [ ] **Step 2: Return to the original working directory and merge**

Leave the worktree while keeping the verified branch. On local `main`, run:

```powershell
git status --short --branch
git merge --no-ff censorship-v0.2.6-audit-fixes -m "merge: censorship v0.2.6 audit fixes"
git status --short --branch
git merge-base --is-ancestor 1e13a967e37774089256bb75df973e896a6445e7 HEAD
```

Expected: merge succeeds without modifying `.claude/`; local `main` consists of the baseline plus the planning commit, feature commits, and merge commit.

- [ ] **Step 3: Remove the temporary task memory before final commit state**

Inspect `.claude/task-memory-2026-09-13-functional-audit.md`, confirm it is the temporary file created for this task, then delete it. Because it is untracked, no commit is needed. Confirm:

```powershell
git status --short --branch
git ls-files -- .claude/task-memory-2026-09-13-functional-audit.md
```

Expected: the temporary file is absent and not tracked; working tree is clean.

- [ ] **Step 4: Verify merged main once more**

Run the focused test and package metadata checks on local `main`:

```powershell
go test . -run '^(TestInteractions|TestScanTextPart|TestOpenAI|TestClaude|TestOracle|TestProtocolOracle|TestReleaseNotesCompatibilityTargetsCurrentVersion)' -count=1
git diff --check 1e13a967e37774089256bb75df973e896a6445e7..HEAD
git status --short --branch
```

Expected: PASS and clean local `main`.

- [ ] **Step 5: Create the annotated patch tag**

Run:

```powershell
git tag -a v0.2.6 -m "Censorship v0.2.6"
git show --no-patch --decorate v0.2.6
git rev-parse v0.2.6^{}
git rev-parse main
```

Expected: dereferenced tag commit equals local `main`.

### Task 11: Push and monitor the real GitHub build/release

**Files:**
- No local source edits.

**Interfaces:**
- Consumes: verified local `main` and unique annotated `v0.2.6` tag.
- Produces: updated `origin/main`, published tag, successful GitHub Actions build, and non-draft release with complete verified assets.

- [ ] **Step 1: Push main, then the tag**

Run:

```powershell
git push origin main
git push origin v0.2.6
```

Expected: both pushes succeed without force. If the second push is interrupted, inspect `git ls-remote --tags origin refs/tags/v0.2.6 refs/tags/v0.2.6^{}` before retrying; do not recreate or move the tag.

- [ ] **Step 2: Identify the exact tag-triggered workflow run**

Run:

```powershell
gh run list --workflow Build --event push --branch v0.2.6 --limit 10 --json databaseId,headBranch,headSha,status,conclusion,url,createdAt
```

Select the run whose `headBranch` is `v0.2.6` and `headSha` equals `git rev-parse main`. Do not confuse it with the separate main-branch push run.

- [ ] **Step 3: Monitor the run to terminal status**

Run:

```powershell
$headSha = git rev-parse main
$runs = gh run list --workflow Build --event push --branch v0.2.6 --limit 10 --json databaseId,headBranch,headSha,status,conclusion,url,createdAt | ConvertFrom-Json
$run = $runs | Where-Object { $_.headBranch -eq 'v0.2.6' -and $_.headSha -eq $headSha } | Select-Object -First 1
if ($null -eq $run) { throw 'tag-triggered v0.2.6 workflow run not found' }
gh run watch $run.databaseId --exit-status
```

Expected: test, five native matrix builds, Windows ARM64, FreeBSD AMD64, and release job all complete successfully. If the CLI/API disconnects, repeat the final command with `$run.databaseId`; do not push another tag.

- [ ] **Step 4: Verify published release metadata and asset set**

Run:

```powershell
gh release view v0.2.6 --json url,isDraft,isPrerelease,tagName,targetCommitish,assets
```

Expected: `isDraft=false`, `isPrerelease=false`, `tagName=v0.2.6`, and exactly seven platform ZIPs, seven matching lowercase `.zip.sha256` files, and `checksums.txt`.

- [ ] **Step 5: Download and verify the published checksums independently**

Create a new temporary directory outside the repository, download the release, and verify every checksum using an available SHA-256 tool. On Bash-capable Windows:

```bash
gh release download v0.2.6 --dir "$VERIFY_DIR"
cd "$VERIFY_DIR"
for checksum in *.zip.sha256; do sha256sum --check "$checksum"; done
sha256sum --check checksums.txt
```

Expected: all seven individual checksum files and every aggregate `checksums.txt` entry report `OK`. Delete the temporary verification directory after inspection.

- [ ] **Step 6: Report exact completion evidence**

Report local main commit, tag commit, push results, exact GitHub Actions run URL/conclusion, release URL/state, asset count, checksum verification result, and all local verification command outcomes. If any remote job fails, report the failing job and logs accurately; do not claim release completion or silently rerun outward-facing actions.
