# CPA censorship plugin v0.2.1 Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Functional tracks may run in parallel only when their file lists are disjoint. Benchmark tracks run serially so CPU and memory contention cannot invalidate measurements.

**Goal:** Fix all thirteen confirmed audit defects, evaluate six runtime performance hypotheses, retain only benchmark-proven safe improvements, and publish v0.2.1.

**Architecture:** Preserve the request-only plugin and native ABI v1. Make narrowly scoped selector, ABI, packaging, and integration-harness corrections at their existing boundaries. Evaluate performance with benchmark-only candidates first; move a candidate into production only after two ten-sample `benchstat` comparisons and correctness holdouts pass.

**Tech Stack:** Go 1.26, CGO/native ABI v1, GNU Make, GitHub Actions, `testing`, `go test -race`, Go fuzzing, `golang.org/x/perf/cmd/benchstat`, CLIProxyAPI v7.2.152.

**Spec:** `docs/superpowers/specs/2026-09-09-censorship-v0.2.1-audit-design.md`

## Global Constraints

- Modify this repository only; do not patch CLIProxyAPI core.
- Preserve ABI v1 ownership, pointer lifetimes, synchronous calls, and `C.GoBytes` unless Task 9 clears every benchmark and correctness gate.
- Preserve exact configured terms, strict Object `words`, handwritten-YAML legacy sequence/global `mode`, and `block` -> `strip` -> `obfs` order.
- Use only documented natural-language selector paths. Do not add recursive string traversal.
- Keep plugin capabilities request-only. Responses, SSE chunks, and WebSocket output remain host pass-through.
- Do not hand-edit `.integration/` or `dist/`.
- Add no repository dependency. `benchstat.exe` is installed under the user Go bin only.
- Every behavior fix starts with a focused failing test. Every performance production change requires repeatable benchmark evidence.
- Parallel workers must not stage or commit. The parent stages exact disjoint file groups and commits after reviewing results.

## File map and ownership

- `selectors.go`: shared Gemini/Interactions Part validation and JSON validity/duplicate detection.
- `selectors_claude.go`: Claude message, search-result, document, and tool-result path selection.
- `selectors_gemini_test.go`, `selectors_interactions_test.go`, `selectors_claude_test.go`: provider regression and selector benchmark coverage.
- `abi_cgo.go`, `abi_cgo_test.go`: native input and callback buffer invariants.
- `.github/scripts/testdata/abi_benchmark_test.go`: end-to-end ABI benchmark validity.
- `Makefile`: safe VERSION data flow and one-time normalization.
- `.github/scripts/package-release.go`, `.github/scripts/package-release_test.go`: direct packaging collision safety and deterministic ZIPs.
- `integration/harness_test.go`, `integration/http_test.go`, `integration/websocket_test.go`: upstream arrival accounting and exact HTTP trace/assertion evidence.
- `transform.go`, `benchmark_test.go`: exact rewrite total-miss experiment.
- `matcher.go`, `matcher_test.go`, `config.go`, `config_test.go`: folded KMP and exact block threshold experiments.
- `docs/superpowers/tdd/2026-09-09-censorship-v0.2.1-audit.tdd.md`: parent-written red/green and benchmark decision log.
- `README.md`, `RELEASE_NOTES.md`: final v0.2.1 user-facing behavior and release notes.

## Execution schedule

1. Parent creates the TDD evidence log.
2. Tasks 1 through 4 run in parallel with Sonnet xhigh; their file sets are disjoint.
3. Parent reviews, runs focused tests, and commits each functional track by exact paths.
4. Tasks 5 through 9 run serially with Sonnet xhigh. No other CPU-heavy command runs during benchmark sampling.
5. Parent records every keep/revert decision and commits only retained production changes plus useful benchmark coverage.
6. Opus xhigh review checks the complete diff once; Sonnet xhigh applies confirmed findings through TDD.
7. Parent executes Task 10 verification and Task 11 release.

---

### Task 0: Create the evidence log

**Files:**
- Create: `docs/superpowers/tdd/2026-09-09-censorship-v0.2.1-audit.tdd.md`

**Interfaces:**
- Consumes: clean branch `feat/v0.2.1`, spec commit `49ea829`.
- Produces: one append-only human-readable record for each red/green cycle and benchmark decision.

- [ ] **Step 1: Write the fixed evidence header**

```markdown
# CPA censorship plugin v0.2.1 TDD and benchmark evidence

Baseline: `v0.2.0` (`1085eee`)
Branch: `feat/v0.2.1`
Toolchain: Go 1.26.5 windows/amd64, CGO enabled, GCC, GNU Make 4.4.1

## Baseline verification

- `make test`: pass
- `make race`: pass
- `make vet`: pass
- `make integration`: pass
- Six fuzz targets at 30 seconds each: pass

## Functional red/green cycles

## Performance experiments

## Final verification
```

- [ ] **Step 2: Confirm the new file is the only uncommitted file**

Run:

```powershell
git status --short
```

Expected: only the new TDD log is untracked.

---

### Task 1: Correct Gemini null Parts and Claude custom-content documents

**Files:**
- Modify: `selectors.go:125-160`
- Modify: `selectors_gemini_test.go:180-202`
- Modify: `selectors_interactions_test.go:212-272`
- Modify: `selectors_claude.go:95-124`
- Modify: `selectors_claude_test.go:78-156`

**Interfaces:**
- Consumes: `scanTextPart(part gjson.Result, requireTextType bool) (gjson.Result, bool)` and `appendClaudeResultText(*[]textSpan, gjson.Result, string, scopeSet)`.
- Produces: null-as-unset Part filtering and exact `source.type == "content"` text selection without changing signatures.

- [ ] **Step 1: Expand the Gemini null-discriminator test table**

Replace the four-case table in `TestGeminiNullMachineDiscriminatorsRemainSelectable` with all keys currently listed in `scanTextPart`:

```go
keys := []string{
    "functionCall", "function_call", "functionResponse", "function_response",
    "inlineData", "inline_data", "fileData", "file_data",
    "executableCode", "executable_code", "codeExecutionResult", "code_execution_result",
    "thoughtSignature", "thought_signature",
}
for _, key := range keys {
    t.Run(key+" null", func(t *testing.T) {
        part := gjson.Parse(`{"text":"SECRET","` + key + `":null}`)
        text, allowed := scanTextPart(part, false)
        if !allowed || text.Str != "SECRET" {
            t.Fatalf("scanTextPart() = %q, %t", text.Str, allowed)
        }
    })
    t.Run(key+" object", func(t *testing.T) {
        _, allowed := scanTextPart(gjson.Parse(`{"text":"SECRET","`+key+`":{}}`), false)
        if allowed {
            t.Fatal("non-null machine member remained selectable")
        }
    })
}
```

Add explicit `extra_content.google.thought_signature` null and non-null rows. Update `TestScanTextPartPreservesProtocolRules` so every null machine key expects `allowed: true`, while non-null siblings expect false.

- [ ] **Step 2: Run the null tests and record RED**

Run:

```powershell
go test . -run '^(TestGeminiNullMachineDiscriminatorsRemainSelectable|TestScanTextPartPreservesProtocolRules)$' -count=1
```

Expected: FAIL for the previously unconditional machine keys and null nested thought signature.

- [ ] **Step 3: Make machine presence depend on non-null values**

In `scanTextPart`, use the same rule for every machine discriminator:

```go
case "functionCall", "function_call", "functionResponse", "function_response",
    "inlineData", "inline_data", "fileData", "file_data",
    "executableCode", "executable_code", "codeExecutionResult", "code_execution_result",
    "thoughtSignature", "thought_signature":
    machinePart = machinePart || value.Type != gjson.Null
case "extra_content":
    signature := value.Get("google.thought_signature")
    machinePart = machinePart || signature.Type != gjson.Null
```

Do not change `thought: true`, `requireTextType`, or key decoding behavior.

- [ ] **Step 4: Add Claude custom-content document rows**

Add direct user and nested tool rows to `TestClaudeResultText`, each containing two typed text parts and one image part. Run each custom-content shape once with its role enabled and once with that role disabled; the disabled result must leave both text parts unchanged. Extend `TestClaudeResultTextPreservesMachineFields` with title, citation, URL, `cache_control`, and image decoys. Expected replacements include only the two `source.content[].text` values, in document order.

Representative direct input:

```json
{"messages":[{"role":"user","content":[{"type":"document","title":"SECRET title","source":{"type":"content","content":[{"type":"text","text":"SECRET first"},{"type":"image","source":{"data":"SECRET image"}},{"type":"text","text":"SECRET second"}]}}]}]}
```

- [ ] **Step 5: Run the Claude tests and record RED**

Run:

```powershell
go test . -run '^(TestClaudeResultText|TestClaudeResultTextPreservesMachineFields)$' -count=1
```

Expected: FAIL because both custom-content text values remain unchanged.

- [ ] **Step 6: Add the exact custom-content branch**

In `appendClaudeResultText`, retain the existing `source.type == "text"` behavior and add:

```go
case "content":
    content := source.Get("content")
    if !content.IsArray() {
        return
    }
    content.ForEach(func(_, part gjson.Result) bool {
        if part.IsObject() && part.Get("type").Str == "text" {
            appendStringSpan(spans, part.Get("text"), role, roles)
        }
        return true
    })
```

Implement this with a switch on `source.Get("type").Str`; do not inspect any other source field.

- [ ] **Step 7: Run selector GREEN and adjacent tests**

Run:

```powershell
go test . -run '^(TestGemini|TestInteractions|TestClaude|TestScanTextPart)' -count=1
go test . -run '^FuzzProtocolTransform$' -count=1
```

Expected: PASS.

- [ ] **Step 8: Return evidence to the parent**

Return exact RED failure lines, GREEN command output, and changed file list. Do not stage or commit.

---

### Task 2: Enforce native ABI buffer invariants

**Files:**
- Modify: `abi_cgo.go:75-87,100-121,126-160`
- Modify: `abi_cgo_test.go:15-171`

**Interfaces:**
- Consumes: ABI `ptr + len` pairs and synchronous host callback.
- Produces: `copyPluginRequest(ptr unsafe.Pointer, length uint64) ([]byte, error)` and `copyHostResponse(ptr unsafe.Pointer, length uint64) ([]byte, error)`; exported C ABI signatures stay unchanged.

- [ ] **Step 1: Add request descriptor and copy tests**

Keep the existing `borrowedRequest` nil/nonzero case and add `TestCopyPluginRequest`:

```go
func TestCopyPluginRequest(t *testing.T) {
    got, err := copyPluginRequest(nil, 0)
    if err != nil || got != nil {
        t.Fatalf("empty request = %v, %v", got, err)
    }
    if _, err := copyPluginRequest(nil, 1); err == nil {
        t.Fatal("nil request with nonzero length succeeded")
    }
    input := []byte{0x7f}
    got, err = copyPluginRequest(unsafe.Pointer(&input[0]), 1)
    if err != nil || !bytes.Equal(got, input) {
        t.Fatalf("copied request = %x, %v", got, err)
    }
    input[0] = 0
    if got[0] != 0x7f {
        t.Fatal("copied request aliases caller memory")
    }
}
```

Add a table test for `validPluginRequest(ptr unsafe.Pointer, length uint64) bool` with nil/zero, nil/one, and nonnil/one. The production entrypoint must call this helper before `shouldCopyPluginRequest`, so malformed after-auth descriptors are also rejected.

- [ ] **Step 2: Add host response copy tests**

Test a Go helper directly:

```go
func TestCopyHostResponseRejectsNilNonzeroLength(t *testing.T) {
    if _, err := copyHostResponse(nil, 1); err == nil {
        t.Fatal("nil host response with nonzero length succeeded")
    }
    got, err := copyHostResponse(nil, 0)
    if err != nil || got != nil {
        t.Fatalf("empty response = %v, %v", got, err)
    }
    input := []byte{0x7f}
    got, err = copyHostResponse(unsafe.Pointer(&input[0]), 1)
    if err != nil || !bytes.Equal(got, input) {
        t.Fatalf("copied response = %x, %v", got, err)
    }
}
```

- [ ] **Step 3: Run ABI tests and record RED**

Run:

```powershell
go test . -run '^(TestValidPluginRequest|TestCopyPluginRequest|TestCopyHostResponse)' -count=1
```

Expected: FAIL to compile because the production-path helpers do not exist.

- [ ] **Step 4: Implement and use the invariant helpers**

```go
func validPluginRequest(ptr unsafe.Pointer, length uint64) bool {
    return ptr != nil || length == 0
}

func copyPluginRequest(ptr unsafe.Pointer, length uint64) ([]byte, error) {
    if _, err := borrowedRequest(ptr, length); err != nil {
        return nil, err
    }
    if length == 0 {
        return nil, nil
    }
    requestLen, ok := checkedCIntLength(length)
    if !ok {
        return nil, fmt.Errorf("request too large: %d", length)
    }
    return C.GoBytes(ptr, C.int(requestLen)), nil
}

func copyHostResponse(ptr unsafe.Pointer, length uint64) ([]byte, error) {
    if length == 0 {
        return nil, nil
    }
    if ptr == nil {
        return nil, fmt.Errorf("host callback response pointer is nil with length %d", length)
    }
    responseLen, ok := checkedCIntLength(length)
    if !ok {
        return nil, fmt.Errorf("host callback response too large: %d", length)
    }
    return C.GoBytes(ptr, C.int(responseLen)), nil
}
```

Call `validPluginRequest(unsafe.Pointer(request), uint64(requestLen))` immediately after method validation and output clearing. In the copying branch, call `copyPluginRequest` and return ABI status 1 on its error. This ensures tests exercise the exact helper used by production while `C.GoBytes` remains in the entrypoint path. Replace the callback's combined nil/zero branch and inline copy with `copyHostResponse`; preserve the existing deferred `cliproxy_host_free` for every non-nil pointer, including zero-length and callback-error buffers.

- [ ] **Step 5: Run ABI GREEN**

Run:

```powershell
go test . -run '^(TestCheckedCIntLength|TestShouldCopyPluginRequest|TestBorrowedRequest|TestValidPluginRequest|TestCopyPluginRequest|TestCopyHostResponse|TestBorrowedABI)' -count=1
go test -race . -run '^(TestValidPluginRequest|TestCopyPluginRequest|TestCopyHostResponse|TestBorrowedABI)' -count=1
```

Expected: PASS.

- [ ] **Step 6: Return evidence to the parent**

Return exact RED/GREEN evidence and changed files. Do not stage or commit.

---

### Task 3: Make VERSION handling safe and packaging deterministic

**Files:**
- Modify: `Makefile:1-64`
- Modify: `.github/scripts/package-release.go:1-387`
- Modify: `.github/scripts/package-release_test.go:36-591`
- Modify only if contract assertions require it: `.github/workflows/build.yml:66-86`

**Interfaces:**
- Consumes: raw `VERSION` from environment, Make command-line assignment, flag, or tag.
- Produces: one normalized version, safe direct package paths, deterministic archive bytes.

- [ ] **Step 1: Add VERSION injection and one-normalization tests**

In `TestMakeVersionValidationContract`, invoke Make from a temporary directory with the repository Makefile by absolute `-f` path. Pass these values as single `exec.Command` arguments:

```go
[]struct {
    value    string
    sentinel string
}{
    {value: `v1";touch VERSION_INJECTION_SENTINEL;version="1`, sentinel: "VERSION_INJECTION_SENTINEL"},
    {value: `$(shell touch VERSION_MAKE_SENTINEL)`, sentinel: "VERSION_MAKE_SENTINEL"},
    {value: "v1\nprintf injected", sentinel: ""},
    {value: "v1 whitespace", sentinel: ""},
    {value: "v1/path", sentinel: ""},
}
```

Pass each table value as one argument formed by `"VERSION=" + tc.value` to `exec.Command`; require nonzero exit and absence of each non-empty sentinel. The rows explicitly cover quote/semicolon shell syntax, Make `$(shell ...)` syntax, newline, whitespace, and slash. Extend `TestMakeBuildIgnoresTargetOverrides` to require direct package commands to read `PACKAGER_VERSION` from the environment rather than embed a Make expansion, and add a `VERSION=vv` dry run whose final artifact version is `v`.

- [ ] **Step 2: Run VERSION tests and record RED**

Run:

```powershell
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^(TestMakeVersionValidationContract|TestMakeBuildIgnoresTargetOverrides)$' -count=1
```

Expected: FAIL because shell injection executes and `vv` is normalized twice before the Go packager.

- [ ] **Step 3: Pass VERSION as data, not recipe source**

At the top of `Makefile`, derive simple variables from unexpanded input and export only simple values:

```make
RAW_VERSION := $(value VERSION)
PACKAGER_VERSION := $(if $(strip $(RAW_VERSION)),$(strip $(RAW_VERSION)),0.0.0-dev)
NORMALIZED_VERSION := $(patsubst v%,%,$(PACKAGER_VERSION))
export PACKAGER_VERSION NORMALIZED_VERSION
```

Change validation to read the environment only:

```make
validate-version:
	@version="$$NORMALIZED_VERSION"; case "$$version" in ""|[!A-Za-z0-9]*|*[!A-Za-z0-9._+-]*) echo "VERSION must normalize to a safe non-empty release version"; exit 2;; esac
```

Use `$$NORMALIZED_VERSION` inside build linker flags. Use `$$PACKAGER_VERSION` for every `package-release.go -version` argument. In recursive Make calls, pass `VERSION="$${PACKAGER_VERSION}"`; never interpolate `$(VERSION)` or `$(NORMALIZED_VERSION)` into shell source.

- [ ] **Step 4: Add direct-package input/output collision tests**

Extend the existing collision table with:

- checksum equals repository `LICENSE`;
- archive equals repository `LICENSE`;
- checksum is a dangling symlink to the absent archive;
- archive and checksum are absent case-only siblings under the same existing directory.

For each case, snapshot library, LICENSE, archive, checksum, link targets, and timestamps before running. Require an error and `assertFilesUnchanged` afterward. Skip only the dangling-symlink row if `os.Symlink` returns a platform permission error.

- [ ] **Step 5: Run path tests and record RED**

Run:

```powershell
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^(TestPackagerRejectsPathCollisionsBeforeChangingFiles|TestValidateDirectPackagePaths)' -count=1
```

Expected: FAIL on LICENSE, dangling link, and case-only absent output rows.

- [ ] **Step 6: Reject writable symlink leaves and all aliases before writing**

Add:

```go
func rejectOutputSymlink(path, name string) error {
    info, err := os.Lstat(path)
    if os.IsNotExist(err) {
        return nil
    }
    if err != nil {
        return fmt.Errorf("inspect %s %s: %w", name, filepath.ToSlash(path), err)
    }
    if info.Mode()&os.ModeSymlink != 0 {
        return fmt.Errorf("%s must not be a symlink: %s", name, filepath.ToSlash(path))
    }
    return nil
}
```

Call it for archive and checksum before canonicalization. If `LICENSE` exists, canonicalize it and compare it with both writable outputs using `pathsAlias`. Treat equal-fold suffix components as aliases regardless of `runtime.GOOS`; remove the now-unused runtime import. Keep `os.SameFile` for existing ancestors and hardlinks.

- [ ] **Step 7: Add deterministic archive tests**

Package byte-identical, same-named library and LICENSE files twice after setting source mtimes several seconds apart. Require equal ZIP bytes and checksum lines. Inspect every ZIP entry and require the same fixed UTC `Modified` time.

- [ ] **Step 8: Run determinism test and record RED**

Run:

```powershell
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^TestPackageLibraryIsDeterministicAcrossSourceMtimes$' -count=1
```

Expected: FAIL because current headers copy source mtimes.

- [ ] **Step 9: Normalize both ZIP header timestamps**

Add `time` to imports and one package value:

```go
var zipModifiedTime = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
```

After each `zip.FileInfoHeader`, set `header.Modified = zipModifiedTime` before `CreateHeader`. Preserve existing entry names, Deflate method, library mode `0755`, and optional LICENSE behavior.

- [ ] **Step 10: Run release GREEN**

Run:

```powershell
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1
make validate-version VERSION=vv
make -n package VERSION=vv
```

Expected: PASS; the dry run carries raw `vv` to Go and uses normalized `v` in artifact names without executing payloads.

- [ ] **Step 11: Return evidence to the parent**

Return RED/GREEN output, exact path cases, deterministic checksum evidence, and changed files. Do not stage or commit.

---

### Task 4: Make integration evidence exact

**Files:**
- Modify: `integration/harness_test.go:347-447,677-796`
- Modify: `integration/http_test.go:15-175`
- Modify: `integration/websocket_test.go:202-227` only for blocked arrival assertions

**Interfaces:**
- Consumes: `upstreamCapture`, `captureHTTP11Trace`, raw HTTP/1.1 responses.
- Produces: independent arrival/completed counters and `responseTrace.Trailers` plus surplus-byte rejection.

- [ ] **Step 1: Add arrival and truncated-body self-test**

Add `arrivals int` under the existing mutex, `arrive()`, and `arrivalCount()`. Add a harness test that sends a raw HTTP request declaring a larger Content-Length than the bytes sent, closes the write side, and waits for the handler. Require `arrivalCount() == 1` and `requestCount() == 0`.

- [ ] **Step 2: Run the arrival test and record RED**

Run:

```powershell
go test -tags=integration ./integration -run '^TestUpstreamCaptureCountsArrivalBeforeTruncatedBody$' -count=1
```

Expected: FAIL to compile before arrival tracking exists.

- [ ] **Step 3: Record handler arrival before body reads**

Call `capture.arrive()` immediately after method/path acceptance and before `io.ReadAll`. Keep `record(body)` after successful read. Replace every blocked-request assertion in `http_test.go` and `websocket_test.go` with `arrivalCount() != 0`; retain `requestCount()` for successful-request arithmetic.

- [ ] **Step 4: Strengthen Chat Completions field assertions**

In the strip/obfs cases, add a decoy JSON field containing `tc.transformed`. Decode the captured request and require:

```go
if got := gjson.GetBytes(captured, "messages.0.content").String(); got != tc.transformed {
    t.Fatalf("upstream message content = %q, want %q; body = %s", got, tc.transformed, captured)
}
```

Require the target field not to equal `tc.input`. Do not use whole-body `bytes.Contains` as the success condition.

- [ ] **Step 5: Run the precise assertion test and record RED/GREEN transition**

First run the decoy test against a temporary deliberately wrong target expectation and record that it fails; restore the correct expectation, then run:

```powershell
go test -tags=integration ./integration -run '^TestHTTPAndSSEOutputTraceUnaffected$' -count=1
```

Expected final result: PASS.

- [ ] **Step 6: Extract raw response parsing into an error-returning helper**

Keep `captureHTTP11Trace(t, ...) responseTrace` as the test-facing wrapper. Move parsing after request write into:

```go
func readHTTP11Trace(reader *bufio.Reader) (responseTrace, error)
```

Change chunk parsing to:

```go
func readHTTP11Chunks(reader *bufio.Reader) ([][]byte, http.Header, error)
```

Add `Trailers http.Header` to `responseTrace`. After fixed-length bytes or the terminal chunk plus trailers, read one byte; require `io.EOF`, otherwise return an error for surplus bytes or a non-EOF read failure.

- [ ] **Step 7: Add table tests for trailers and surplus bytes**

Feed `bufio.NewReader(strings.NewReader(raw))` with:

- valid fixed-length body followed by EOF;
- fixed-length body followed by `X`;
- valid chunked body with `Trailer: X-Test` and terminal `X-Test: value`;
- terminal chunk/trailers followed by `X`.

Require exact payload chunks and trailer preservation for valid cases; require error for both surplus cases.

- [ ] **Step 8: Run trace GREEN**

Run:

```powershell
go test -tags=integration ./integration -run '^(TestReadHTTP11Trace|TestUpstreamCaptureCountsArrivalBeforeTruncatedBody)$' -count=1
```

Expected: PASS.

- [ ] **Step 9: Return evidence to the parent**

Return RED/GREEN evidence and changed files. Do not stage or commit.

---

### Task 5: Integrate and commit functional tracks

**Files:**
- Modify: `docs/superpowers/tdd/2026-09-09-censorship-v0.2.1-audit.tdd.md`

**Interfaces:**
- Consumes: results from Tasks 1 through 4.
- Produces: four reviewed commits and clean functional baseline.

- [ ] **Step 1: Review disjoint diffs**

Run:

```powershell
git diff --check
git diff -- selectors.go selectors_claude.go selectors_claude_test.go selectors_gemini_test.go selectors_interactions_test.go
git diff -- abi_cgo.go abi_cgo_test.go
git diff -- Makefile .github/scripts/package-release.go .github/scripts/package-release_test.go .github/workflows/build.yml
git diff -- integration/harness_test.go integration/http_test.go integration/websocket_test.go
```

Reject unrelated formatting or files.

- [ ] **Step 2: Run combined functional checks**

```powershell
gofmt -w selectors.go selectors_claude.go selectors_claude_test.go selectors_gemini_test.go selectors_interactions_test.go abi_cgo.go abi_cgo_test.go .github/scripts/package-release.go .github/scripts/package-release_test.go integration/harness_test.go integration/http_test.go integration/websocket_test.go
go test ./... -count=1
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1
go test -tags=integration ./integration -count=1
```

Expected: PASS.

- [ ] **Step 3: Append exact evidence to the TDD log**

Record each finding ID under its domain with RED command/failure, production change, GREEN command/result, and files.

- [ ] **Step 4: Commit selector corrections**

```powershell
git add -- selectors.go selectors_claude.go selectors_claude_test.go selectors_gemini_test.go selectors_interactions_test.go
git commit -m "fix: cover documented provider text fields"
```

- [ ] **Step 5: Commit ABI corrections**

```powershell
git add -- abi_cgo.go abi_cgo_test.go
git commit -m "fix: validate native ABI buffers"
```

- [ ] **Step 6: Commit release corrections**

```powershell
git add -- Makefile .github/scripts/package-release.go .github/scripts/package-release_test.go .github/workflows/build.yml
git commit -m "fix: secure deterministic release packaging"
```

Do not stage `.github/workflows/build.yml` if unchanged.

- [ ] **Step 7: Commit integration corrections and TDD evidence**

```powershell
git add -- integration/harness_test.go integration/http_test.go integration/websocket_test.go docs/superpowers/tdd/2026-09-09-censorship-v0.2.1-audit.tdd.md
git commit -m "test: make integration traces exact"
```

- [ ] **Step 8: Confirm clean functional baseline**

```powershell
git status --short --branch
```

Expected: clean `feat/v0.2.1`.

---

### Task 6: Evaluate exact rewrite total-miss preflight

**Files:**
- Modify: `benchmark_test.go:507-576` for benchmark-only strategies
- Modify only if benchmark wins: `config.go:31-44,295-351`
- Modify only if benchmark wins: `transform.go:182-253`
- Modify only if benchmark wins: `config_test.go:298-351`

**Interfaces:**
- Consumes: `byteMatcher`, exact rewrite rules, ordered strip/obfs loops.
- Produces only on a win: `configSnapshot.ExactRewriteMatcher *byteMatcher` and a total-miss preflight that falls back to unchanged loops on any hit.

- [ ] **Step 1: Add benchmark-only baseline and candidate strategies**

Extend `runBenchmarkExactRewriteStrategies` with 20 MiB/1,024-rule total miss, first, last, dense, duplicate, cross-span, and cascade cases plus serial and `RunParallel` variants. Construct the matcher outside the timed loop and route both strategies through this helper:

```go
func benchmarkExactRewriteStrategy(text string, cfg *configSnapshot, matcher *byteMatcher) (string, bool) {
    spans := [...]textSpan{{Text: text, Role: "user"}}
    if matcher != nil {
        if _, matched := matcher.match(text); !matched {
            return text, false
        }
    }
    _, changed := applyMode(spans[:], cfg)
    return spans[0].Text, changed
}
```

For `impl=preflight`, build `newByteMatcher(cfg.Rules[cfg.BlockEnd:], cfg.BlockEnd)` once before `b.ResetTimer`; for `impl=baseline`, pass nil. Name the implementation immediately below the benchmark name, for example `BenchmarkExactRewritePreflight/impl=baseline/...` and `.../impl=preflight/...`, so exact `-bench` filters and `benchstat -col /impl` work.

- [ ] **Step 2: Prove benchmark strategy equivalence**

Add `TestBenchmarkExactRewritePreflightMatchesBaseline` over duplicate terms, multiple spans, none/first/middle/last/dense hits, strip, obfs, and cascades. For multi-span cases, use the same matcher once per span and compare every final `Text`, every `Changed` bit, the block result, and aggregate changed flag with unmodified `applyMode`.

Run:

```powershell
go test . -run '^TestBenchmarkExactRewritePreflightMatchesBaseline$' -count=1
```

Expected: PASS before any production edit.

- [ ] **Step 3: Capture two quiet, opposite-order benchmark series**

With no other CPU-heavy process running, collect baseline then candidate in the first file and candidate then baseline in the second. Each implementation receives 10 process-level samples at each fixed `GOMAXPROCS` value:

```powershell
$forward = Join-Path $env:TEMP 'cpa-v021-exact-rewrite-forward.txt'
$reverse = Join-Path $env:TEMP 'cpa-v021-exact-rewrite-reverse.txt'
Remove-Item $forward,$reverse -ErrorAction SilentlyContinue
1..10 | ForEach-Object {
  go test . -run '^$' -bench '^BenchmarkExactRewritePreflight/impl=baseline/' -benchtime=500ms -benchmem -cpu=1,16 | Add-Content $forward
  go test . -run '^$' -bench '^BenchmarkExactRewritePreflight/impl=preflight/' -benchtime=500ms -benchmem -cpu=1,16 | Add-Content $forward
}
1..10 | ForEach-Object {
  go test . -run '^$' -bench '^BenchmarkExactRewritePreflight/impl=preflight/' -benchtime=500ms -benchmem -cpu=1,16 | Add-Content $reverse
  go test . -run '^$' -bench '^BenchmarkExactRewritePreflight/impl=baseline/' -benchtime=500ms -benchmem -cpu=1,16 | Add-Content $reverse
}
$benchstat = Join-Path (go env GOPATH) 'bin\benchstat.exe'
& $benchstat -col /impl $forward
& $benchstat -col /impl $reverse
```

- [ ] **Step 4: Apply or reject**

Keep only if both files show significant total-miss serial and parallel improvement, no allocation increase, and every hit/cascade holdout stays within 5%. On a win, add `ExactRewriteMatcher *byteMatcher` to `configSnapshot`, compile it from `rewriteRules` only when `!cfg.IgnoreCase && len(rewriteRules) >= exactByteMatcherMinRules`, and add `SkipExactRewrite bool` to `textSpan`. Before the current rule-major loops:

```go
if !cfg.IgnoreCase && cfg.ExactRewriteMatcher != nil {
    for i := range spans {
        spans[i].SkipExactRewrite = false
        if useExactByteMatcher(len(cfg.Rules)-blockEnd, len(spans[i].Text)) {
            _, matched := cfg.ExactRewriteMatcher.match(spans[i].Text)
            spans[i].SkipExactRewrite = !matched
        }
    }
}
```

Skip a span in both exact strip and obfs loops only when `SkipExactRewrite` is true. On a loss, keep only concise benchmark coverage and do not alter production files.

- [ ] **Step 5: Verify a retained candidate**

```powershell
go test . -run '^(TestBenchmarkExactRewritePreflightMatchesBaseline|TestMixed|TestStrip|TestObfs|TestCompileSnapshot)' -count=1
go test -race . -run '^(TestTransformIsByteDeterministic|TestConcurrentReconfigure)' -count=1
```

- [ ] **Step 6: Record and commit**

Append full `benchstat` tables and the keep/revert reason to the TDD log. Stage only `benchmark_test.go`, the log, and winning production/test files. Commit:

```powershell
git commit -m "perf: skip exact rewrite work on total misses"
```

If production loses, use `test: measure long exact rewrite workloads`.

---

### Task 7: Evaluate selector validation and duplicate-name allocation

**Files:**
- Modify: `selectors_gemini_test.go` or `selectors_interactions_test.go` for benchmark helpers
- Modify only if validation fusion wins: `selectors.go:20-49,75-82`
- Modify only if inline names win: `selectors.go:162-197`

**Interfaces:**
- Consumes: `selectTextSpans`, `jsonNestingWithin`, `canonicalJSONMemberName`.
- Produces zero, one, or two independent selector optimizations; each decision is measured separately.

- [ ] **Step 1: Add fused-validation benchmark candidate**

Add a benchmark-only candidate with the current state machine and one UTF-8 branch:

```go
func benchmarkJSONNestingAndUTF8Within(body []byte, maxDepth int) bool {
    depth, inString, escaped := 0, false, false
    for i := 0; i < len(body); {
        b := body[i]
        if b >= utf8.RuneSelf {
            _, size := utf8.DecodeRune(body[i:])
            if size == 1 {
                return false
            }
            if inString && escaped {
                escaped = false
            }
            i += size
            continue
        }
        i++
        if inString {
            switch {
            case escaped:
                escaped = false
            case b == '\\':
                escaped = true
            case b == '"':
                inString = false
            }
            continue
        }
        switch b {
        case '"':
            inString = true
        case '{', '[':
            depth++
            if depth > maxDepth {
                return false
            }
        case '}', ']':
            depth--
        }
    }
    return depth == 0 && !inString
}
```

`BenchmarkSelectorValidation` compares `jsonNestingWithin(body, maxJSONNestingDepth) && utf8.Valid(body)` as `impl=baseline` with the candidate as `impl=fused` on 20 MiB ASCII, 20 MiB valid multibyte, invalid tail, depth boundary, escaped strings, and 1 KiB bodies, each serial and `RunParallel`.

- [ ] **Step 2: Add differential correctness test**

Use valid/malformed corpora plus deterministic single-byte mutations. Require the candidate result to equal `jsonNestingWithin(body, maxJSONNestingDepth) && utf8.Valid(body)` for every input. Run:

```powershell
go test . -run '^TestJSONNestingAndUTF8WithinMatchesCurrentValidation$' -count=1
```

Expected: PASS before production changes.

- [ ] **Step 3: Measure fused validation twice**

Use the same opposite-order, ten-process harness from Task 6, changing the benchmark filters to:

```powershell
-bench '^BenchmarkSelectorValidation/impl=baseline/'
-bench '^BenchmarkSelectorValidation/impl=fused/'
```

Write `cpa-v021-selector-validation-forward.txt` and `cpa-v021-selector-validation-reverse.txt`, then run `benchstat -col /impl` on each file separately. Keep only if both long ASCII and multibyte serial/parallel cases improve significantly, allocations do not increase, and 1 KiB cases remain within 5%. If retained, move the candidate body into `jsonNestingWithin`, remove the separate `!utf8.Valid(body)` condition from `selectTextSpans`, and keep the differential test against a test-local copy of the old behavior.

- [ ] **Step 4: Add inline-name benchmark candidate**

Create benchmark-only `benchmarkHasDuplicateJSONMembersInline` using this object-local insertion logic and the same array/object recursion as production:

```go
var inline [4]string
inlineCount := 0
var overflow map[string]struct{}
// For each canonical name:
if overflow != nil {
    if _, exists := overflow[name]; exists {
        duplicate = true
        return false
    }
    overflow[name] = struct{}{}
} else {
    for _, previous := range inline[:inlineCount] {
        if previous == name {
            duplicate = true
            return false
        }
    }
    if inlineCount < len(inline) {
        inline[inlineCount] = name
        inlineCount++
    } else {
        overflow = make(map[string]struct{}, inlineCount+1)
        for _, previous := range inline {
            overflow[previous] = struct{}{}
        }
        overflow[name] = struct{}{}
    }
}
```

`BenchmarkDuplicateJSONMembers` compares production as `impl=map` with this candidate as `impl=inline` over 100,000 two-field objects, four-field objects, wide objects, nested tool schemas, escaped-equivalent duplicates, and late duplicates, serial and parallel.

- [ ] **Step 5: Add duplicate-walker differential test**

Require identical results for all existing duplicate fixtures, max-depth fixtures, escaped keys, arrays, early callback termination, and 10,000 deterministic generated object/array inputs.

Run:

```powershell
go test . -run '^TestInlineDuplicateWalkerMatchesMapWalker$' -count=1
```

Expected: PASS before production changes.

- [ ] **Step 6: Measure inline names twice**

Use the Task 6 harness with `BenchmarkDuplicateJSONMembers/impl=map/` and `/impl=inline/`, writing separate forward and reverse files with 10 process samples per implementation. Run `benchstat -col /impl` on each file. Keep only if many-small-object ns/op and allocs/op improve significantly, B/op does not increase, and small/wide holdouts remain within 5%.

If retained, replace only the object-local `seen` map in `hasDuplicateJSONMembers` with the measured `[4]string` logic. Preserve recursive calls, canonical key decoding, and immediate false callback termination.

- [ ] **Step 7: Run selector gates**

```powershell
go test . -run '^(TestJSON|TestInlineDuplicate|TestGemini|TestInteractions|TestClaude|TestDuplicate)' -count=1
go test -race . -run '^(TestJSON|TestInlineDuplicate|TestDuplicate)' -count=1
go test . -run '^FuzzDuplicateWalkerAgainstOracle$' -count=1
```

- [ ] **Step 8: Record and commit each independent decision**

Append both `benchstat` tables. Commit retained winners separately; if both lose, commit only benchmark coverage if it remains useful and concise. Never keep benchmark-only candidate implementations that duplicate production code without future value.

---

### Task 8: Evaluate folded KMP dispatch and exact block threshold

**Files:**
- Modify: `matcher_test.go:141-200,496-547`
- Modify only if KMP wins: `matcher.go:325-381`
- Modify only if threshold wins: `transform.go:19-39`
- Modify only if threshold wins: `config.go:295-351`
- Modify only if threshold wins: `config_test.go:298-351`

**Interfaces:**
- Consumes: `rewriteFoldedKMP`, `newByteMatcher`, `useExactByteMatcher`, `compileSnapshot`.
- Produces zero, one, or two independent matcher optimizations.

- [ ] **Step 1: Add KMP hybrid benchmark and oracle test**

Extend `benchmarkFoldedRewriteInput` with `match == "hybrid"`, producing two adjacent matches followed by a near-match-heavy suffix:

```go
case "hybrid":
    prefix := sourceTerm + sourceTerm
    unit := sourceTerm[:len(sourceTerm)-len(sourceSuffix)] + "q"
    return prefix + repeatBytes(unit, textBytes-len(prefix)), term
```

Add a test-only copy of `rewriteFoldedKMP` named `benchmarkRewriteFoldedKMPContinuous` with exactly the current lines 353-356 omitted. Benchmark current `rewriteFoldedKMP` as `impl=fallback` and the copy as `impl=continuous` at 64 KiB and 1 MiB for strip/obfs, plus dense, sparse, miss, Unicode SimpleFold, invalid UTF-8, and overlap holdouts, serial and `RunParallel`. `TestBenchmarkRewriteFoldedKMPContinuousMatchesOracle` compares candidate bytes and match flags with `rewriteFolded` for every benchmark case and 10,000 deterministic generated inputs.

- [ ] **Step 2: Capture and compare KMP series**

Use the Task 6 opposite-order harness with filters:

```powershell
-bench '^BenchmarkFoldedKMPHybrid/impl=fallback/'
-bench '^BenchmarkFoldedKMPHybrid/impl=continuous/'
```

Collect 10 process samples per implementation in forward and reverse files at `-cpu=1,16`, then run `benchstat -col /impl` on each. Keep removal of production lines 353-356 only if both hybrid sizes and parallel cases significantly improve, allocations stay unchanged, and no holdout regresses by more than 5%. If retained, delete the test-only copy and benchmark production under `impl=continuous`; if rejected, delete the candidate copy after recording results.

- [ ] **Step 3: Add exact-threshold benchmark matrix**

Extend `runBenchmarkExactBlockStrategies` to rule counts 128, 192, and 255; 64 KiB and 1 MiB; and none/first/middle/last hits. For no-hit cases, require `ruleIndex == -1 && !matched`; for hit cases, require the exact ordered rule index. Compare `benchmarkExactBlockStrategy(..., -1, nil)` as `impl=ordered` with `exactByteMatcherPrefixRules` ordered checks plus prebuilt `newByteMatcher(rules[exactByteMatcherPrefixRules:], exactByteMatcherPrefixRules)` as `impl=matcher`, serial and `RunParallel`.

Add `var benchmarkByteMatcherSink *byteMatcher` beside the existing benchmark sinks and `BenchmarkExactBlockMatcherBuild` for 128/192/255/256 rules:

```go
b.ReportAllocs()
b.ResetTimer()
for i := 0; i < b.N; i++ {
    benchmarkByteMatcherSink = newByteMatcher(
        rules[exactByteMatcherPrefixRules:],
        exactByteMatcherPrefixRules,
    )
}
```

Use B/op and allocs/op as the snapshot construction-memory evidence; report matcher node count as `nodes/op` outside the timer if the existing benchmark sink exposes it without changing production.

- [ ] **Step 4: Capture and compare threshold series**

Use the Task 6 harness with `BenchmarkExactBlockThreshold/impl=ordered/` and `/impl=matcher/`, plus a separate 10-process `BenchmarkExactBlockMatcherBuild` file. Run `benchstat -col /impl` on forward and reverse request files and normal `benchstat` on build samples. Select the lowest measured threshold whose 1 MiB no/late-hit serial and parallel cases significantly improve while construction B/op, allocations, all 64 KiB cases, and early hits regress by no more than 5%.

If no threshold satisfies every gate, keep `exactByteMatcherMinRules = 256`. If one wins, set the constant to that measured value and update `TestCompileSnapshotBuildsOnlyActiveDerivedData` plus active-derived-data tests to cover `threshold-1` and `threshold` exactly.

- [ ] **Step 5: Run matcher gates**

```powershell
go test . -run '^(TestFold|TestAdaptive|TestByteMatcher|TestCompileSnapshot)' -count=1
go test -race . -run '^(TestFold|TestAdaptive|TestByteMatcher|TestConcurrentReconfigure)' -count=1
go test . -run '^(FuzzFoldKMPAgainstOracle|FuzzByteMatcherAgainstOrderedContains)$' -count=1
```

- [ ] **Step 6: Record and commit independent decisions**

Append tables and thresholds to the TDD log. Commit each retained winner separately. Revert losing production edits to the current `HEAD` while preserving functional fixes.

---

### Task 9: Measure native input copy and retain `C.GoBytes` unless every gate passes

**Files:**
- Modify: `.github/scripts/testdata/abi_benchmark_test.go:48-106`
- Modify: `abi_cgo_test.go` for plugin-local copy/borrow benchmarks
- Modify production only on a complete win: `abi_cgo.go:126-145`

**Interfaces:**
- Consumes: `borrowedRequest`, current `C.GoBytes`, dynamic ABI host benchmark.
- Produces: corrected measurement harness. Production remains copied unless plugin-local and end-to-end proof plus pointer-liveness safety all pass.

- [ ] **Step 1: Correct the dynamic benchmark harness**

Create one immutable request before `b.ResetTimer`, reuse it by value, stop the timer, and validate the final response plus unchanged request:

```go
request := pluginapi.RequestInterceptRequest{
    RequestID: "abi-benchmark", SourceFormat: "openai", Body: bytes.Clone(input),
}
response := tc.call(request)
if err := validateDynamicABIResponse(tc.phase, input, request.Body, response, tc.expected); err != nil {
    b.Fatal(err)
}
b.ReportAllocs()
b.SetBytes(int64(len(input)))
b.ResetTimer()
for i := 0; i < b.N; i++ {
    response = tc.call(request)
}
b.StopTimer()
if err := validateDynamicABIResponse(tc.phase, input, request.Body, response, tc.expected); err != nil {
    b.Fatal(err)
}
```

Add `/serial` and `/parallel` subbenchmarks. `RunParallel` shares the same immutable request, keeps one local final response per worker, and validates it once after that worker's `pb.Next()` loop; do not write a shared sink from workers. Keep the comment that `ReportAllocs` sees host Go runtime allocations only.

- [ ] **Step 2: Add plugin-local copy versus borrow benchmarks**

Add `BenchmarkPluginRequestOwnership` in `abi_cgo_test.go`. For 1 KiB, 1 MiB, and 20 MiB stable byte slices, benchmark production `copyPluginRequest` as `impl=copy` and `borrowedRequest` as `impl=borrow`, serial and `RunParallel`. Construct input before timing, use a worker-local result, and after timing verify length, first byte, and last byte. This directly measures the `C.GoBytes` path without replacing it.

Run correctness first:

```powershell
go test . -run '^(TestCopyPluginRequest|TestBorrowedRequest|TestBorrowedABI)' -count=1
go test -race -gcflags=all=-d=checkptr=2 . -run '^(TestCopyPluginRequest|TestBorrowedRequest|TestBorrowedABI)' -count=1
```

- [ ] **Step 3: Capture ownership and end-to-end baselines**

Use the Task 6 opposite-order harness for `BenchmarkPluginRequestOwnership/impl=copy/` and `/impl=borrow/`, with 10 samples per implementation in each order at `-cpu=1,16`; compare each file with `benchstat -col /impl`.

Then collect the corrected current end-to-end benchmark without other load:

```powershell
$baseline = Join-Path $env:TEMP 'cpa-v021-abi-end-to-end-baseline.txt'
Remove-Item $baseline -ErrorAction SilentlyContinue
1..10 | ForEach-Object {
  go run ./.github/scripts/integration-runner.go -bench-abi | Add-Content $baseline
}
& (Join-Path (go env GOPATH) 'bin\benchstat.exe') $baseline
```

The end-to-end file is the copied production baseline. A before/after comparison is required only if Step 5 is allowed to build a borrowed production candidate.

- [ ] **Step 4: Apply the correctness blocker before any production experiment**

The pinned Windows host report found request liveness unresolved at the native call. Inspect only the already identified `loader_windows.go` call site or use its saved evidence; do not scan unrelated CPA source. If the host does not prove the backing Go request remains live for the complete synchronous native call, mark the borrowed production candidate rejected and do not edit `abi_cgo.go`.

- [ ] **Step 5: If and only if liveness is proven, run a temporary candidate build**

Replace only the before-auth `C.GoBytes` copy with validated `borrowedRequest`, run strict checkptr, race, unit, and dynamic integration tests, then collect two ten-process end-to-end candidate series. Keep only if it removes approximately one input-sized plugin allocation and significantly improves both 1 MiB and 20 MiB serial/parallel throughput with identical response bytes.

- [ ] **Step 6: Record the decision**

Expected default from current evidence: reject production borrowing and retain `C.GoBytes`; keep the corrected benchmark harness if it improves measurement validity. Append microbenchmark and end-to-end results plus the liveness gate to the TDD log.

- [ ] **Step 7: Commit measurement correction**

```powershell
git add -- .github/scripts/testdata/abi_benchmark_test.go abi_cgo_test.go docs/superpowers/tdd/2026-09-09-censorship-v0.2.1-audit.tdd.md
git commit -m "test: isolate native ABI benchmark costs"
```

Add `abi_cgo.go` only if every gate unexpectedly passes.

---

### Task 10: Review, documentation, and full verification

**Files:**
- Modify: `README.md`
- Modify: `RELEASE_NOTES.md`
- Modify: `docs/superpowers/tdd/2026-09-09-censorship-v0.2.1-audit.tdd.md`
- Review: every file changed since `1085eee`

**Interfaces:**
- Consumes: all functional commits and benchmark decisions.
- Produces: release-ready branch with one reviewed diff and complete verification evidence.

- [ ] **Step 1: Update release documentation surgically**

Document v0.2.1 fixes: Gemini null union semantics, Claude custom-content documents, ABI descriptor validation, safe one-time VERSION normalization, packaging alias protection and deterministic archives, and exact integration evidence. List only benchmark winners as performance improvements; list rejected experiments only in the TDD report.

- [ ] **Step 2: Run one Opus xhigh complete-diff review**

Review `1085eee..HEAD` plus the working tree once for correctness, ABI compatibility, provider path scope, packaging safety, benchmark validity, and plan coverage. Do not duplicate the original repository scan. Verify each finding with a concrete failure scenario; apply only confirmed findings through a focused test-first correction.

- [ ] **Step 3: Run code simplification on changed code**

Use `superpowers:requesting-code-review`, then `code-simplifier` or `simplify` only on changed files. Preserve behavior and remove only complexity introduced in this task.

- [ ] **Step 4: Run formatting and static checks**

```powershell
gofmt -w (git diff --name-only 1085eee -- '*.go')
git diff --check
make vet
```

Expected: no diff-check or vet output.

- [ ] **Step 5: Run all repository gates serially**

```powershell
make test
make race
make integration
make build VERSION=v0.2.1
make package VERSION=v0.2.1
```

Expected: all commands exit 0. Build/package may create ignored generated artifacts only.

- [ ] **Step 6: Verify package contents and checksums**

Run the package script tests, then independently hash each local ZIP:

```powershell
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1
$zips = Get-ChildItem -Path dist -Filter '*.zip' -File
foreach ($zip in $zips) {
  $line = (Get-Content -Raw ($zip.FullName + '.sha256')).Trim()
  $want = (Get-FileHash -Algorithm SHA256 $zip.FullName).Hash.ToLowerInvariant()
  $parts = $line -split '  ', 2
  if ($parts.Count -ne 2 -or $parts[0] -cne $want -or $parts[1] -cne $zip.Name) {
    throw "checksum mismatch for $($zip.Name): $line"
  }
}
$aggregate = Join-Path 'dist' 'checksums.txt'
if (Test-Path $aggregate) {
  $lines = @(Get-Content $aggregate | Where-Object { $_ -ne '' })
  if ($lines.Count -ne $zips.Count -or (@($lines | Sort-Object -Unique)).Count -ne $lines.Count) {
    throw 'checksums.txt does not contain exactly one line per ZIP'
  }
}
```

Expected: lowercase SHA-256 and exact basename match for every ZIP; aggregate lines are unique and complete when present.

- [ ] **Step 7: Run active fuzzing again**

Launch these six independent commands in parallel tool calls, one process per target:

```powershell
go test . -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=30s -parallel=1
go test . -run '^$' -fuzz '^FuzzFoldKMPAgainstOracle$' -fuzztime=30s -parallel=1
go test . -run '^$' -fuzz '^FuzzDuplicateWalkerAgainstOracle$' -fuzztime=30s -parallel=1
go test . -run '^$' -fuzz '^FuzzProtocolTransform$' -fuzztime=30s -parallel=1
go test . -run '^$' -fuzz '^FuzzRebuildBodyAgainstMarshalOracle$' -fuzztime=30s -parallel=1
go test . -run '^$' -fuzz '^FuzzByteMatcherAgainstOrderedContains$' -fuzztime=30s -parallel=1
```

Require all six to exit 0. If a crash corpus appears, stop release, add it as a regression test, and fix the root cause through TDD.

- [ ] **Step 8: Re-run retained benchmark holdouts**

For every retained production optimization, run its two ten-sample series once more from the final branch. Revert any winner whose final results no longer satisfy its keep criteria.

- [ ] **Step 9: Append final evidence and commit**

Record command, exit code, benchmark tables, fuzz duration, package hashes, and generated-artifact status. Commit docs and any review correction:

```powershell
git add -- README.md RELEASE_NOTES.md docs/superpowers/tdd/2026-09-09-censorship-v0.2.1-audit.tdd.md
git commit -m "docs: prepare v0.2.1 release"
```

- [ ] **Step 10: Confirm branch state**

```powershell
git status --short --branch
git log --oneline --decorate 1085eee..HEAD
```

Expected: clean branch and only task-related commits.

---

### Task 11: Merge, tag, push, and verify GitHub release

**Files:**
- No source edits after the verified release commit.

**Interfaces:**
- Consumes: clean verified `feat/v0.2.1`.
- Produces: local `main` containing all commits, annotated `v0.2.1`, pushed workflow run, and published release assets.

- [ ] **Step 1: Invoke branch-finishing and verification skills**

Use `superpowers:verification-before-completion`, then `superpowers:finishing-a-development-branch`. Because the user already selected local-main merge and release, do not ask for another integration choice.

- [ ] **Step 2: Fast-forward or no-ff merge into local main**

```powershell
git switch main
git pull --ff-only origin main
git merge --no-ff feat/v0.2.1 -m "merge: release censorship v0.2.1"
```

If `origin/main` moved, stop and resolve based on the actual diff; do not force or discard either side.

- [ ] **Step 3: Verify merged main before tagging**

Run the smallest complete release set again on `main`:

```powershell
make test
make race
make vet
make integration
make package VERSION=v0.2.1
git status --short --branch
```

Expected: all pass; only ignored artifacts exist.

- [ ] **Step 4: Create the annotated tag**

```powershell
git tag -a v0.2.1 -m "censorship v0.2.1"
git show --no-patch --decorate v0.2.1
```

Require tag target to equal merged `main` HEAD.

- [ ] **Step 5: Push main and tag**

```powershell
git push origin main
git push origin v0.2.1
```

Never force-push and never bypass hooks.

- [ ] **Step 6: Verify workflow and release**

Use authenticated `gh` commands:

```powershell
$runs = gh run list --workflow Build --branch v0.2.1 --limit 5 --json databaseId,headBranch,event,status,conclusion,url | ConvertFrom-Json
$run = $runs | Where-Object { $_.headBranch -eq 'v0.2.1' -and $_.event -eq 'push' } | Select-Object -First 1
if ($null -eq $run) { throw 'tag-triggered v0.2.1 Build run was not found' }
gh run watch $run.databaseId --exit-status
$release = gh release view v0.2.1 --json tagName,isDraft,isPrerelease,url,assets | ConvertFrom-Json
$release | ConvertTo-Json -Depth 8
```

Wait for the tag-triggered Build workflow to complete. Require all build legs and release job to succeed. Download or inspect asset metadata and require seven platform ZIPs, seven lowercase `.zip.sha256` files, and `checksums.txt` with one line per ZIP.

- [ ] **Step 7: Report completion from evidence**

Report merged commit, tag object and target, workflow URL/status, release URL, functional fixes, retained/rejected performance experiments, verification commands, and asset/checksum counts. Do not claim success for any skipped or failed step.
