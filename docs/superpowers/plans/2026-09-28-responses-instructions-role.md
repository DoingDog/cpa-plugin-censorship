# OpenAI Responses `instructions` Role Coverage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect and transform a Responses `instructions` string when only `developer` is enabled, without changing existing `system` behavior.

**Architecture:** The only production change is in `collectOpenAIResponses(root gjson.Result, roles scopeSet, spans *[]textSpan)`. Select the `instructions` span once with `system` precedence and `developer` fallback; the existing selector, matcher, rewrite, and RPC paths then apply unchanged. Prove the regression at the RPC layer and through a real CPA HTTP request.

**Tech Stack:** Go, CGO, `gjson`, native plugin ABI v1, CLIProxyAPI v7.2.152, `make`, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-28-responses-instructions-role-design.md`

## Global Constraints

- Modify only this plugin repository; never change CLIProxyAPI core or the pinned dependency.
- Preserve native ABI v1, request pointer ownership, and `C.GoBytes` in `cliproxyPluginCall`.
- `words` is a strict Object; retain `block -> strip -> obfs` and the documented explicit text paths only.
- Keep `system` precedence for `instructions` when both `system` and `developer` are configured; never append the same raw span twice.
- Do not hand-edit generated `.integration/` or `dist/` artifacts, nor modify the unrelated second-turn WebSocket deadline.
- Add a failing focused behavior test before changing production code. Preserve existing formatting and unrelated documentation.

## Review Focus

1. `developer`-only `block` with `instructions: "SECRET"`: block locally and report `developer`.
2. `developer`-only `strip` with metadata containing the same term: rewrite only `instructions`, not metadata.
3. `system`-only or combined `system, developer`: one span with the existing `system` role, not duplicates or an invalid-span error.
4. `user`-only with a matching `instructions`: return a no-op response.
5. Non-string `instructions` with `developer` enabled: do not select or rewrite a machine-shaped value.

---

### Task 1: Add a focused failing RPC regression

**Files:**
- Modify: `selectors_openai_test.go:364-377`

**Interfaces:**
- Consumes: existing `registerConfig(t, yaml)`, `interceptRPC(t, "openai-response", body)`, `gjson.GetBytes`, and the existing `RequestInterceptResponse` fields.
- Produces: `TestOpenAIResponsesInstructionsScope`, covering all five Review Focus cases in the plugin's request path.

- [ ] **Step 1: Start an isolated branch and append the regression test after `TestOpenAIResponsesSelectorCanonicalRoles`.** Do not edit existing canonical expectations.

```bash
git switch -c fix/responses-instructions-developer
```

```go
func TestOpenAIResponsesInstructionsScope(t *testing.T) {
	for _, tc := range []struct {
		name, roles, wantRole string
	}{
		{name: "developer only", roles: "developer", wantRole: "developer"},
		{name: "system only", roles: "system", wantRole: "system"},
		{name: "system and developer", roles: "system, developer", wantRole: "system"},
	} {
		t.Run(tc.name+" block", func(t *testing.T) {
			registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: ["+tc.roles+"]\n")
			resp := interceptRPC(t, "openai-response", []byte(`{"instructions":"SECRET","input":"safe"}`))
			if !resp.Terminate || resp.StatusCode != 400 || gjson.GetBytes(resp.ResponseBody, "error.code").String() != "censorship_blocked" || gjson.GetBytes(resp.ResponseBody, "error.term").String() != "SECRET" || gjson.GetBytes(resp.ResponseBody, "error.role").String() != tc.wantRole {
				t.Fatalf("response = %#v, body = %s", resp, resp.ResponseBody)
			}
		})
	}
	t.Run("developer only strip", func(t *testing.T) {
		registerConfig(t, "words:\n  strip: [SECRET]\nscope:\n  roles: [developer]\n")
		resp := interceptRPC(t, "openai-response", []byte(`{"instructions":"SECRET plan","input":"safe","metadata":{"note":"SECRET"}}`))
		want := `{"instructions":" plan","input":"safe","metadata":{"note":"SECRET"}}`
		if resp.Terminate || string(resp.Body) != want {
			t.Fatalf("response = %#v, want body %s", resp, want)
		}
	})
	t.Run("user only", func(t *testing.T) {
		registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: [user]\n")
		resp := interceptRPC(t, "openai-response", []byte(`{"instructions":"SECRET","input":"safe"}`))
		if resp.Terminate || len(resp.Body) != 0 {
			t.Fatalf("response = %#v, want no-op", resp)
		}
	})
	t.Run("nonstring instructions", func(t *testing.T) {
		registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: [developer]\n")
		resp := interceptRPC(t, "openai-response", []byte(`{"instructions":{"text":"SECRET"},"input":"safe"}`))
		if resp.Terminate || len(resp.Body) != 0 {
			t.Fatalf("response = %#v, want no-op", resp)
		}
	})
}
```

- [ ] **Step 2: Run the test before editing production.**

```bash
go test -run '^TestOpenAIResponsesInstructionsScope$' -count=1 .
```

Expected: `developer only block` and `developer only strip` fail because the plugin returns no termination or replacement body; the other cases pass. Keep this red-phase output.

### Task 2: Fix the one selector branch and run the focused tests

**Files:**
- Modify: `selectors_openai.go:370-374`
- Test: `selectors_openai_test.go`

**Interfaces:**
- Consumes: `roles.has(string) bool`, `appendStringSpan(*[]textSpan, gjson.Result, string, scopeSet)`.
- Produces: exactly one `instructions` span with `system` precedence or `developer` fallback.

- [ ] **Step 1: Replace only the existing `instructions` role gate.**

```go
if roles.has("system") {
	appendStringSpan(spans, root.Get("instructions"), "system", roles)
} else if roles.has("developer") {
	appendStringSpan(spans, root.Get("instructions"), "developer", roles)
}
```

- [ ] **Step 2: Run focused and full unit checks.**

```bash
go test -run '^TestOpenAIResponsesInstructionsScope$' -count=1 .
go test ./... -count=1
git diff --check
```

Expected: all pass. Review the diff so only the selected selector and test changed. Commit the focused behavior and tests using `git add selectors_openai.go selectors_openai_test.go` and `git commit -m "fix: inspect Responses instructions with developer scope"`.

### Task 3: Pin real-host behavior in integration

**Files:**
- Modify: `integration/http_test.go:373-385`

**Interfaces:**
- Consumes: existing `newMockUpstream(t)`, `startCPA(t, upstream.URL, true, config)`, `postJSON(t, url, body)`, `decodeCensorshipError(body)` and `upstream.arrivalCount()`.
- Produces: `TestHTTPResponsesDeveloperInstructionsBlock`, a real CPA registration and HTTP request regression.

- [ ] **Step 1: Add a test next to `TestHTTPResponsesBlockStringInput`.**

```go
func TestHTTPResponsesDeveloperInstructionsBlock(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "words:\n  block: [SECRET]\nscope:\n  roles: [developer]\n")
	status, _, body := postJSON(t, cpa.baseURL+"/v1/responses", []byte(`{"model":"censorship-integration-model","instructions":"SECRET","input":"safe"}`))
	response, err := decodeCensorshipError(body)
	if err != nil || status != 400 || response.Error.Code != "censorship_blocked" || response.Error.Term != "SECRET" || response.Error.Role != "developer" {
		t.Fatalf("status=%d body=%s error=%v", status, body, err)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("blocked Responses instructions reached upstream")
	}
}
```

- [ ] **Step 2: Run `make integration` with the Windows environment variables below; verify the new test, plugin load/registration, and existing HTTP/WebSocket tests.** Commit with `git add integration/http_test.go` and `git commit -m "test: cover developer Responses instructions through CPA"`.

### Task 4: Document and verify the patch

**Files:**
- Modify: `README.md:159-168`, add one sentence about `instructions` canonical role precedence.
- Modify: `RELEASE_NOTES.md:1`, add a v0.3.3 section, preserving existing release history.
- Modify: `main_test.go:1184-1217`, update the current-release notes assertion from v0.3.2 to v0.3.3 while retaining historical compatibility checks.
- Keep: `docs/superpowers/specs/2026-09-28-responses-instructions-role-design.md`, `docs/superpowers/plans/2026-09-28-responses-instructions-role.md`.

**Interfaces:**
- Consumes: tests above, `.github/workflows/build.yml` tag/release flow, existing version `v0.3.2`.
- Produces: documented v0.3.3 behavior and verified local build/test artifacts.

- [ ] **Step 1: Document only the verified behavior.** In `README.md` after the OpenAI Responses provider-path paragraph, state that `instructions` is selected as `system` if enabled, otherwise as `developer` if enabled. Prepend this minimal section to `RELEASE_NOTES.md`:

```markdown
# Censorship v0.3.3

## v0.3.3 fix

- OpenAI Responses `instructions` is inspected with `scope.roles: [developer]`; when `system` is also enabled, its existing `system` role takes precedence. This closes the developer-only block/strip gap without changing other request fields.

v0.3.3 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.
```

- [ ] **Step 2: Update the release-notes test that currently expects v0.3.2 at the top.** The current version is v0.3.3; keep v0.3.2, v0.3.1, and v0.3.0 compatibility sentences as historical assertions. The failing run of `go test ./... -count=1` before this edit reported `main_test.go:1192: RELEASE_NOTES.md does not begin with the v0.3.2 current-release section`. Make only these replacements in `TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions`:

```diff
- const currentHeader = "# Censorship v0.3.2\n\n## v0.3.2 fixes\n"
+ const currentHeader = "# Censorship v0.3.3\n\n## v0.3.3 fix\n"
- currentEnd := bytes.Index(raw, []byte("\n# Censorship v0.3.1\n"))
+ currentEnd := bytes.Index(raw, []byte("\n# Censorship v0.3.2\n"))
- "v0.3.2 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.",
+ "v0.3.3 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.",
+ "v0.3.2 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.",
```

Also adjust the two error strings naming the current release to v0.3.3. Run `go test -run '^TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions$' -count=1 .` to verify this version assertion passes; do not touch the test's old-spec supersession check.

- [ ] **Step 3: Run the full verification matrix.** This Windows shell currently uses Cygwin Make; pass these variables as Make *command-line variables* (not just shell exports) so Go, temporary files, and the CPA child process work:

```bash
VARS=(GOPATH='C:/Users/user/go' GOMODCACHE='C:/Users/user/go/pkg/mod' TMP='C:/Users/user/AppData/Local/Temp' TEMP='C:/Users/user/AppData/Local/Temp' GOCACHE='C:/Users/user/AppData/Local/go-build' USERPROFILE='C:/Users/user')
make test "${VARS[@]}"
make race "${VARS[@]}"
make vet "${VARS[@]}"
make integration "${VARS[@]}"
make build "${VARS[@]}"
make package "${VARS[@]}"
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
git diff --check
```

Expected: all return 0; integration logs include plugin loaded/registered and new HTTP case. Inspect `dist/` and `.integration/` before any cleanup; do not edit their generated contents. Record any failing command exactly instead of claiming success.

- [ ] **Step 4: Review only the current diff for regressions in selector role precedence and integration coverage.** Commit docs and the release-notes assertion with `git add README.md RELEASE_NOTES.md main_test.go docs/superpowers/specs/2026-09-28-responses-instructions-role-design.md docs/superpowers/plans/2026-09-28-responses-instructions-role.md` and `git commit -m "docs: describe Responses instructions scope fix"`.

### Task 5: Merge and publish a verified patch

**Files:** No further source edits; Git branch/tag/release only.

**Interfaces:** Consumes a fully verified topic branch and existing `.github/workflows/build.yml`; produces local `main`, remote `main`, tag `v0.3.3`, CI artifacts and GitHub release.

- [ ] **Step 1: Check local and remote refs and authentication without force operations.** Confirm `git status --short --branch`, `git fetch origin --tags`, `git log main..HEAD`, `git log HEAD..origin/main`, `git tag -l v0.3.3`, `git ls-remote --tags origin refs/tags/v0.3.3`, and `gh auth status`. If `main` or tag has diverged, stop and report before choosing a different patch version.
- [ ] **Step 2: Fast-forward the verified topic branch into local `main`; recheck clean status.** Use `git switch main` and `git merge --ff-only fix/responses-instructions-developer`. Create annotated `v0.3.3` only after merge and verification; do not move existing tags.
- [ ] **Step 3: Push `main`, then `v0.3.3` without force.** The tag triggers the seven-target build and published release. Query GitHub Actions using `gh run list --workflow Build --branch v0.3.3`, watch the tag run using its actual ID, and inspect all job conclusions. If it fails, diagnose and fix this plugin branch, then create a new patch tag rather than rewriting a published tag.
- [ ] **Step 4: Verify `gh release view v0.3.3 --json isDraft,assets,url` shows a published release, seven ZIPs, seven matching lowercase SHA-256 sidecars, and `checksums.txt`. Download into a new temporary directory, run each `sha256sum --check` and the aggregate `checksums.txt`, and report the final commit/tag, CI result, and release link. If release jobs remain pending, report that state, not completion.
