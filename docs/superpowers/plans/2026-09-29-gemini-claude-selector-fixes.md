# Gemini and Claude Selector Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Inspect model-visible Gemini tool descriptions and prevent a full strip from producing invalid Claude MCP typed text.

**Architecture:** Keep the provider-specific collectors and the shared `textSpan`/`transformRequest` behavior. Map Gemini tool definitions to canonical `developer`, reuse `appendJSONSchemaDescriptions` at four explicit schema roots, and mark Claude MCP typed text with the existing `RequiresNonEmpty` flag. No new generic walker or ABI changes.

**Tech Stack:** Go, `gjson`, existing plugin RPC tests, PowerShell/Make, CPA v7.2.152.

**Spec:** `docs/superpowers/specs/2026-09-29-gemini-claude-selector-fixes-design.md`

## Global Constraints

- Change only the plugin; preserve native ABI v1 and the pinned `github.com/router-for-me/CLIProxyAPI/v7` v7.2.152 contract.
- Schema version 5 and current `C.GoBytes` input handling remain unchanged.
- Preserve Object `words`, canonical roles, `block` -> `strip` -> `obfs`, and exact machine-field bytes.
- Only documented explicit natural-language paths; no generic recursive string walker or single-object Gemini `contents`/`parts` compatibility guess.
- Do not hand-edit `.integration/` or `dist/`; use PowerShell for `make` commands on this machine.
- First prove each behavior change fails on the original code, then implement, rerun, and commit focused changes.

## Review Focus

- `scope.roles: [developer]` without any content role: Gemini function description still blocks. Task 1 tests this role gate.
- Gemini schema descriptions in nested `properties` and `items`: text rewrites without changing sibling machine fields. Task 1 tests these paths.
- Gemini `functionCall.args`, names, enum values, and arbitrary tool metadata containing the term: no rewrite. Task 1 compares original tokens outside contracted leaves.
- `scope.roles: [user]` with Gemini tools: definition descriptions stay untouched. Task 1 tests disabled role.
- Claude MCP exact typed text versus a block missing `type`/disabled `tool` role: only selected text rejects an empty rewrite. Task 2 tests all cases.

---

### Task 1: Gemini function-definition descriptions

**Files:**
- Modify: `selectors.go:60-75` (Gemini role gate)
- Modify: `selectors_gemini.go:17-69` (explicit definition paths)
- Test: `selectors_gemini_test.go` (new focused tests)

**Interfaces:** Consumes `selectTextSpans([]byte, string, scopeSet)`, `appendStringSpan(*[]textSpan, gjson.Result, string, scopeSet)`, `appendJSONSchemaDescriptions(*[]textSpan, gjson.Result, string, scopeSet)`; produces additional `developer` spans for existing `transformRequest` and plugin RPC.

- [ ] **Step 1: Add focused failing RPC tests to `selectors_gemini_test.go`.** Use `registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: [developer]\n")` and `interceptRPC(t, "gemini", []byte(...))` with `{"tools":[{"functionDeclarations":[{"name":"lookup","description":"SECRET lookup"}]}],"contents":[{"parts":[{"text":"hello"}]}]}`; assert `Terminate`, HTTP 400 and `gjson.GetBytes(resp.ResponseBody, "error.role").Str == "developer"`. For `strip`, use developer-only scope and one definition with `description`, `parameters.properties.x.description`, `parametersJsonSchema.items.description`, `response.description`, `responseJsonSchema.properties.y.description`, each containing a different `SECRET <suffix>` token. Include `name`, schema `enum`, arbitrary `metadata.description`, and `contents.parts.functionCall.args` with `SECRET` in machine-only fields. Build expected bytes with `replaceRawTokens(t, body, rawReplacement{Before: "\"SECRET <suffix>\"", After: "\" <suffix>\""}, ...)`, assert `!Terminate` and `bytes.Equal(resp.Body, want)`; separately register `roles: [user]` and assert no replacement for a tools-only request. Add a nested `items` description on a parameter or response schema to pin schema traversal.
- [ ] **Step 2: Run red tests.** `go test -count=1 -run 'TestGeminiTool' .` under PowerShell; expect the block test to receive no terminal response and the strip test to receive no replacement body.
- [ ] **Step 3: Implement only explicit paths.** In `selectorHasEnabledRole`, include `roles.has("developer")` for Gemini. In `collectGemini`, before the system/contents logic, if `roles.has("developer")`, iterate `root.Get("tools")` only when it is an array; iterate `tool.Get("functionDeclarations")` only when it is an array and the tool is an object; for each object declaration call `appendStringSpan` on `description` with role `developer`, then `appendJSONSchemaDescriptions` on exactly `parameters`, `parametersJsonSchema`, `response`, and `responseJsonSchema`, each with role `developer`. Retain existing system and contents flow unchanged.
- [ ] **Step 4: Run green tests and nearby selector checks.** `go test -count=1 -run 'TestGeminiTool|TestGeminiSelector|TestGeminiDisabledRoles' .`; all must pass. Inspect the diff to ensure no field outside these paths was added.
- [ ] **Step 5: Commit only Task 1 files.** `git add selectors.go selectors_gemini.go selectors_gemini_test.go && git commit -m "fix: inspect Gemini function declaration descriptions"`.

### Task 2: Claude MCP typed text validity

**Files:**
- Modify: `selectors_claude.go:105-120` (one typed text call)
- Test: `selectors_claude_test.go` (new focused test)

**Interfaces:** Consumes `appendNonEmptyStringSpan(*[]textSpan, gjson.Result, string, scopeSet)` and existing `transformRequest` `RequiresNonEmpty` rejection; produces the same selected text spans with an added nonempty requirement.

- [ ] **Step 1: Add a focused failing test to `selectors_claude_test.go`.** With `words:\n  strip: [SECRET]\nscope:\n  roles: [tool]\n`, send `{"messages":[{"role":"user","content":[{"type":"mcp_tool_result","content":[{"type":"text","text":"SECRET"}]}]}]}` through `interceptRPC`; assert `Terminate`, HTTP 400, no `Body`, and `error.code == "censorship_invalid_request"` plus `error.message == "censorship rewrite would make a text field invalid"`. Add subtests for `"SECRET tail"` producing `" tail"`, an inner block lacking exact `type: "text"` leaving the body unchanged, and `roles: [user]` leaving the exact typed block untouched.
- [ ] **Step 2: Run red test.** `go test -count=1 -run 'TestClaudeMCPNonEmpty' .` under PowerShell; expect only the full-strip subtest to fail because the current plugin returns `text:""`.
- [ ] **Step 3: Implement one-call fix.** In the `mcp_tool_result` exact `type: "text"` branch, replace `appendStringSpan(spans, inner.Get("text"), "tool", roles)` with `appendNonEmptyStringSpan(spans, inner.Get("text"), "tool", roles)`; leave scalar MCP content and all other branches untouched.
- [ ] **Step 4: Run green tests.** `go test -count=1 -run 'TestClaudeMCPNonEmpty|TestClaudeBetaMCPToolResult|TestClaudeMinimumLengthFields' .`; all must pass.
- [ ] **Step 5: Commit only Task 2 files.** `git add selectors_claude.go selectors_claude_test.go && git commit -m "fix: reject empty Claude MCP typed text after strip"`.

### Task 3: Documentation, release checks and integration

**Files:**
- Modify: `README.md:138-150` (Gemini selected-text contract)
- Modify: `RELEASE_NOTES.md:1-9` (new v0.3.5 patch section)
- Modify: `main_test.go:1190-1215` (current-release section expectations)

**Interfaces:** Documents Task 1 and Task 2 behavior; keeps the existing version injection via build flags and tag, without changing `main.go`.

- [ ] **Step 1: Update docs and release assertion.** Append to the Gemini paragraph that `tools[*].functionDeclarations[*].description` and description leaves in `parameters`, `parametersJsonSchema`, `response`, `responseJsonSchema` use canonical `developer`, while names, enum values, call arguments, and signatures are excluded. Add a v0.3.5 release note with both fixes and the existing pinned CPA/ABI statement; update `main_test.go`'s current-release section to v0.3.5 including its target-compatibility string. Do not edit prior release sections.
- [ ] **Step 2: Verify documentation checks and full suite.** Run `go test -count=1 -run '^TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions$' .`, then `make test`, `make vet`, `make race`, `make build`, `make integration`, and `make package` in PowerShell. Run `go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go` and `go test .github/scripts/package-release.go .github/scripts/package-release_test.go` separately; the directory has two standalone `main` commands. Inspect release ZIPs and matching lowercase SHA-256 sidecars plus `checksums.txt`; inspect registration output from integration. Any failure must be diagnosed and fixed before proceeding.
- [ ] **Step 3: Inspect only this branch's diff and commit docs.** `git diff --check && git status --short && git diff main...HEAD` to review already committed changes; inspect uncommitted README/release test diff separately, then `git add README.md RELEASE_NOTES.md main_test.go && git commit -m "docs: describe v0.3.5 selector fixes"`. Have an independent reviewer inspect the full branch and repair only confirmed errors, rerunning affected checks.
- [ ] **Step 4: Merge and publish after checks.** Verify clean branch and `origin/main` ancestry, switch to local `main`, merge with `git merge --ff-only fix/gemini-claude-selector-text`, rerun release-note and registration checks, and ensure `v0.3.5` does not exist locally or remotely. Existing `v0.3.4` is an annotated tag; create `git tag -a v0.3.5 -m "Censorship v0.3.5"` and push `main` and this tag without force. Inspect the GitHub Actions run and release artifacts with `gh`, including lowercase `.sha256` sidecars and `checksums.txt`. Do not claim remote success until the run and checksums are verified.
