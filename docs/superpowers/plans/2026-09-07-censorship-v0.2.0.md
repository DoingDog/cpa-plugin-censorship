# Censorship v0.2.0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Release cpa-plugin-censorship v0.2.0 with canonical per-action Object configuration, legacy handwritten-YAML compatibility, current CPA SDK support, corrected provider selectors, verified performance improvements, icon metadata, and reliable build/release tooling.

**Architecture:** Parse both YAML forms into one immutable flattened `Rules` slice ordered block, strip, obfs, with `BlockEnd` and `StripEnd` boundaries. Keep provider selectors as explicit natural-language allowlists, then apply block to original spans before ordered strip and obfs rewrites. Upgrade the SDK and integration host together, and verify the real plugin boundary without modifying CPA production source.

**Tech Stack:** Go 1.26, CGO shared-library ABI v1, CLIProxyAPI/v7 v7.2.152, gjson, yaml.v3, Go `testing`, Make, GitHub Actions, GitHub CLI.

**Spec:** `docs/superpowers/specs/2026-09-07-censorship-v0.2.0-design.md`

## Global Constraints

- The first implementation change atomically upgrades `github.com/router-for-me/CLIProxyAPI/v7` to `v7.2.152` and the integration host to commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`; RPC schema is 5 and native ABI remains 1.
- `words` Object accepts any subset of `block`, `strip`, and `obfs`; `{}` means no rules; every key appears at most once; unknown and duplicate keys reject the candidate.
- Object-form rules execute `block -> strip -> obfs`; block sees original selected text; each bucket preserves list order and duplicates.
- The visual panel exposes only Object `words`; `mode` and list-form `words` remain only in handwritten YAML parsing, compatibility documentation, and parser tests.
- `pluginVersion` stays `0.0.0-dev` in source; tagged builds inject `0.2.0` through existing ldflags.
- Plugin metadata uses `pluginapi.Metadata.Logo` with `https://raw.githubusercontent.com/DoingDog/cpa-plugin-censorship/main/logo.png`; do not change or package `logo.png`.
- Do not inspect or modify CPA production source unrelated to plugin SDK, plugin loading, request interception, or the generated integration test seam.
- Do not add recursive provider string walking, term normalization, deduplication, an ABI bump, a runtime dependency, or unrelated refactoring.
- Coding workers use Sonnet xhigh. Plan and final review workers use Opus xhigh.
- Parallel workers receive disjoint file sets in isolated worktrees. No two workers read or edit the same file, and no finding receives duplicate review.
- The coordinator alone updates `docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md`; parallel workers return literal RED/GREEN/REFACTOR evidence instead of editing that shared file.
- Every production change follows RED -> GREEN -> REFACTOR and records the literal command, exit code, decisive output, and changed behavior in the TDD record.
- The user has authorized uninterrupted execution, local merge, tag creation, GitHub push, workflow observation, and release publication without intermediate approval stops.

## Execution Topology

1. Run Tasks 1 through 5 serially because each establishes interfaces used later.
2. After Task 5, run Tasks 6, 7, 8, and 9 concurrently in four isolated worktrees; cherry-pick their commits in numeric order.
3. Run Task 10 after that barrier to record the four workers' evidence and test the combined selector behavior.
4. Run Task 11 serially because it establishes the benchmark/integration seam.
5. Run Tasks 12, 13, and 14 concurrently in isolated worktrees; their file sets are disjoint. Cherry-pick in numeric order.
6. Run Tasks 15 through 18 serially.
7. Before starting implementation, invoke `superpowers:test-driven-development`, then `superpowers:subagent-driven-development`. Before each concurrent wave, invoke `superpowers:dispatching-parallel-agents`.

## Accepted Finding Coverage

| Findings | Owning task |
|---|---|
| `CFG-1`, `CFG-2` | Task 3 |
| `MAT-1`, `MAT-2`, `MAT-3` | Task 4 |
| `CORE-1`, `CORE-2`, `CORE-3` | Task 5 |
| `OA-1`, `OA-2`, `OA-3`, `OA-4` | Task 6 |
| `CL-1`, `CL-2` | Task 7 |
| `GM-1`, `GM-2`, `GM-3` | Task 8 |
| `GI-1` | Task 9 |
| `INT-1`, `INT-11` | Task 11 |
| `INT-2` | Task 2 |
| `INT-3` through `INT-10` | Task 12 |
| `REL-1` | Task 13 |
| `REL-2`, `REL-4` through `REL-8` | Task 14 |
| `REL-3`, `DOC-1` | Task 15 |

---

### Task 1: Establish the TDD Record and Baseline

**Files:**
- Create: `docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md`

**Interfaces:**
- Consumes: the approved spec and observed baseline commands.
- Produces: the sole chronological evidence log used by every later task.

- [ ] **Step 1: Create the evidence record with the observed baseline**

```markdown
# Censorship v0.2.0 TDD Record

## Baseline, 2026-09-07

- `make integration`: PASS; the pinned CPA HTTP, SSE, reload, Responses WebSocket, and ABI scenarios completed.
- `go vet ./...`: PASS.
- `make test`: FAIL only at `TestDocumentationListsConfigAndLimits`; `RELEASE_NOTES.md` lacks the exact expected configuration block.
- `make race`: FAIL only at the same documentation assertion.
- Baseline benchmark output: `%TEMP%/censorship-v020-before-bench.txt`.

Every task below records its literal RED, GREEN, and REFACTOR command, exit code, and decisive output.
```

- [ ] **Step 2: Verify the file has no unrecorded success claim**

Run:

```powershell
Select-String -Path 'docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md' -Pattern 'PASS|FAIL|Baseline'
```

Expected: exactly the four baseline results above plus the heading.

- [ ] **Step 3: Commit the baseline record**

```bash
git add docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md
git commit -m "test: record v0.2.0 baseline"
```

---

### Task 2: Upgrade CPA SDK and Integration Host Atomically

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `.github/scripts/integration-runner.go`
- Modify: `.github/scripts/integration-runner_test.go`
- Modify: `main_test.go`
- Modify: `README.md`
- Modify: `docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md`

**Interfaces:**
- Consumes: `pluginabi.SchemaVersion`, the runner's CPA pin constant, and existing integration checkout behavior.
- Produces: SDK v7.2.152, schema 5 registration, and exact host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8` for all later integration work.

- [ ] **Step 1: Read only the dependency and pin call sites owned by this task**

Read `go.mod`, the CPA pin declaration and its tests, the registration schema assertion in `main_test.go`, and the one README line naming the integration commit. Do not inspect unrelated CPA source.

- [ ] **Step 2: Write failing dependency and host-pin tests**

Add or update assertions equivalent to:

```go
func TestSupportedPluginSchema(t *testing.T) {
	if got, want := pluginabi.SchemaVersion, uint32(5); got != want {
		t.Fatalf("plugin schema = %d, want %d", got, want)
	}
}

func TestPinnedCPARevision(t *testing.T) {
	const want = "c76dfd4e0edabab9000628b1560ab8ab379eadb8"
	if cpaSHA != want {
		t.Fatalf("cpaSHA = %q, want %q", cpaSHA, want)
	}
}
```

- [ ] **Step 3: Run RED checks**

Run:

```powershell
go test . -run '^TestSupportedPluginSchema$' -count=1
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -run '^TestPinnedCPARevision$' -count=1
```

Expected: schema test reports 4 instead of 5; pin test reports `81e1b5374f99c212f196f34956eeed964a46b8fa` instead of the selected commit.

- [ ] **Step 4: Upgrade the module and generate exact sums**

Run:

```powershell
go get github.com/router-for-me/CLIProxyAPI/v7@v7.2.152
go mod download github.com/gorilla/websocket@v1.5.3
go mod tidy
```

Verify `go.mod` selects `v7.2.152`; verify `go.sum` contains module hash `h1:FkvGzpOCvuDGswaOyoVfbY5Ua7OlP/wMXw3agiNMUQI=` and go.mod hash `h1:lTHwMAGajc1wKGQiRtDvYbwV0FWsM7sy+N0ZU5/gxJQ=`.

- [ ] **Step 5: Update the runner and README pin in the same worktree**

```go
const cpaSHA = "c76dfd4e0edabab9000628b1560ab8ab379eadb8"
```

Change only the corresponding README compatibility line to the same full commit.

- [ ] **Step 6: Run GREEN checks**

Run:

```powershell
go test . -run '^TestSupportedPluginSchema$' -count=1
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -run '^TestPinnedCPARevision$' -count=1
go list -m -json github.com/router-for-me/CLIProxyAPI/v7
go mod verify
```

Expected: tests pass; module JSON reports `v7.2.152`; `go mod verify` reports all modules verified.

- [ ] **Step 7: Run the existing focused dependency-compatible suite**

```powershell
go test . -run '^(TestPluginRegistration|TestHandlePluginRegister|TestHandlePluginReconfigure)' -count=1
go vet ./...
```

Expected: PASS without an ABI-version change.

- [ ] **Step 8: Append literal RED/GREEN/REFACTOR evidence and commit atomically**

```bash
git add go.mod go.sum .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go main_test.go README.md docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md
git commit -m "build: upgrade CPA SDK to v7.2.152"
```

---

### Task 3: Compile Strict Object and Legacy YAML into One Snapshot

**Files:**
- Modify: `config.go`
- Modify: `config_test.go`
- Modify: `main.go`
- Modify: `main_test.go`
- Modify: `docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md`

**Interfaces:**
- Consumes: existing `parseConfigYAML`, `configSnapshot`, `rule`, lifecycle registration, and `atomic.Pointer` publication.
- Produces: `configSnapshot.Rules []rule`, `BlockEnd int`, `StripEnd int`, a block-prefix matcher, and a rewrite-suffix folded preflight matcher.

- [ ] **Step 1: Read the complete four-file lifecycle/config flow and every caller of fields being replaced**

Trace `parseConfigYAML` through registration, reconfiguration, snapshot installation, and tests. List every `Mode`, `Rules`, block matcher, rewrite preflight, replacement, format, and role field reference before editing.

- [ ] **Step 2: Add table-driven RED parser tests for accepted Object shapes**

Use cases equivalent to:

```go
{
	name: "empty object",
	yaml: "words: {}\n",
	wantRules: nil, wantBlockEnd: 0, wantStripEnd: 0,
},
{
	name: "two buckets ignore key order",
	yaml: "words:\n  obfs: [gamma]\n  block: [alpha, alpha]\n",
	wantTerms: []string{"alpha", "alpha", "gamma"},
	wantBlockEnd: 2, wantStripEnd: 2,
},
{
	name: "all buckets",
	yaml: "words:\n  strip: [beta]\n  obfs: [gamma]\n  block: [alpha]\n",
	wantTerms: []string{"alpha", "beta", "gamma"},
	wantBlockEnd: 1, wantStripEnd: 2,
},
```

Cover every zero/one/two/three-bucket subset, empty buckets, whitespace-only terms, duplicates within/across buckets, every mapping-key permutation, and a valid but behaviorally ignored top-level `mode`.

- [ ] **Step 3: Add RED rejection tests**

Use raw YAML strings to prove duplicate keys are visible to the node parser:

```go
bad := []string{
	"words: null\n",
	"words: scalar\n",
	"words: {unknown: [x]}\n",
	"words:\n  block: [x]\n  block: [y]\n",
	"words: {block: null}\n",
	"words: {strip: [1]}\n",
	"words: {obfs: ['']}\n",
}
```

Assert initial registration publishes nothing and invalid reconfiguration preserves the exact prior snapshot pointer.

- [ ] **Step 4: Add RED legacy-order and obfuscation-scope tests**

Prove a sequence appearing before `mode` uses the later mode, mapping form ignores valid mode, invalid supplied mode rejects both forms, and final `obfs.char` validation applies only to effective obfs terms.

- [ ] **Step 5: Run RED tests**

```powershell
go test . -run '^(TestParseConfig.*Words|Test.*Mapping|Test.*Legacy|Test.*Reconfigure)' -count=1
```

Expected: Object cases fail with the current sequence-only parser; old sequence cases continue to pass.

- [ ] **Step 6: Add parser-local representation and snapshot cut points**

Implement the minimum parser staging shape:

```go
type parsedWords struct {
	legacy bool
	list   []string
	block  []string
	strip  []string
	obfs   []string
}

type configSnapshot struct {
	Rules    []rule
	BlockEnd int
	StripEnd int
	// retain existing scope, obfs, matcher, and precomputed fields
	RewriteMatcher *foldMatcher
}
```

Parse the entire root first. For Object nodes, accept only unique string keys `block`, `strip`, and `obfs`, each with a sequence of non-empty string scalars. For a sequence, retain ordered terms and route them after final `mode` is known.

- [ ] **Step 7: Flatten and validate after all root keys are known**

```go
terms := make([]string, 0, len(words.block)+len(words.strip)+len(words.obfs))
terms = append(terms, words.block...)
blockEnd := len(terms)
terms = append(terms, words.strip...)
stripEnd := len(terms)
terms = append(terms, words.obfs...)
```

For legacy input, place the list into exactly one range. Apply two-rune and marker exclusion only to `terms[stripEnd:]`. Compile all owned matcher/replacement data before the one atomic store.

- [ ] **Step 8: Change visual metadata and icon under RED tests**

Make registration assertions require exactly these panel fields and no `mode`:

```go
ConfigFields: []pluginapi.ConfigField{
	{Name: "ignore_case", Type: pluginapi.ConfigFieldTypeBoolean, Description: "..."},
	{Name: "words", Type: pluginapi.ConfigFieldTypeObject, Description: "Optional block, strip, and obfs arrays; an empty object has no rules."},
	{Name: "scope", Type: pluginapi.ConfigFieldTypeObject, Description: "..."},
	{Name: "obfs", Type: pluginapi.ConfigFieldTypeObject, Description: "..."},
}
```

Set:

```go
Logo: "https://raw.githubusercontent.com/DoingDog/cpa-plugin-censorship/main/logo.png",
```

Temporarily set and restore `pluginVersion` in the release-metadata test; keep source default `0.0.0-dev`.

- [ ] **Step 9: Strengthen snapshot concurrency cleanup and coverage**

Register worker shutdown with `defer` before any fatal assertion. Alternate snapshots whose rules, cut points, formats, and roles all differ; assert readers observe only complete A or complete B.

- [ ] **Step 10: Run GREEN and race checks**

```powershell
go test . -run '^(TestParseConfig|TestRegistration|TestHandlePlugin|TestConcurrentSnapshot)' -count=1
go test -race . -run '^(TestConcurrentSnapshot|TestHandlePluginReconfigure)' -count=1
```

Expected: every strict Object, legacy, metadata, lifecycle, and atomic-snapshot case passes.

- [ ] **Step 11: Remove only parser scaffolding made unused by the change**

Run `gofmt` on the four files, then:

```powershell
go test . -run '^(TestParseConfig|TestRegistration|TestHandlePlugin|TestConcurrentSnapshot)' -count=1
go vet ./...
```

- [ ] **Step 12: Append evidence and commit**

```bash
git add config.go config_test.go main.go main_test.go docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md
git commit -m "feat: add per-action censorship rules"
```

---

### Task 4: Execute Mixed Rules with Existing Matchers

**Files:**
- Modify: `transform.go`
- Modify: `transform_test.go`
- Modify: `matcher.go` only if compilation requires range-aware constructor inputs; do not redesign its nodes or KMP.
- Modify: `matcher_test.go`
- Modify: `fuzz_test.go`
- Modify: `benchmark_test.go`
- Modify: `docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md`

**Interfaces:**
- Consumes: `Rules`, `BlockEnd`, `StripEnd`, block-prefix matchers, rewrite-suffix preflight matcher, current rule-major transforms.
- Produces: fixed block-original -> strip -> obfs execution with phase-specific adaptive thresholds.

- [ ] **Step 1: Read all transform callers and matcher index assumptions**

Trace `transformRequest`, `applyMode`, exact/folded block functions, rewrite preflight, strip/obfs helpers, oracle helpers, and adaptive threshold calls. Record which matcher returns global versus local rule indexes.

- [ ] **Step 2: Add RED mixed-execution tests**

Use a mapping where strip could remove a block term and require block to win. Add no-block cases that prove strip precedes obfs, mapping key order is irrelevant, and range-local duplicate/cascade behavior remains observable.

```go
// Original text contains "blocked" and strip would remove it.
// Expected: Blocked != nil; Body remains unrewritten.

// Original "aXb", strip ["X"], obfs ["ab"].
// Expected selected text: "a​b".
```

Run exact and `ignore_case: true` variants, including Sigma and Kelvin equivalence.

- [ ] **Step 3: Add RED boundary and strategy tests**

Prove strip/obfs-only terms never block, block indexes cannot exceed `BlockEnd`, rewrite preflight still detects suffix hits, and unrelated bucket counts do not choose the wrong adaptive path.

- [ ] **Step 4: Extend the independent fuzz oracle before production edits**

Model the normalized snapshot directly:

```go
func oracleApply(spans []textSpan, cfg *configSnapshot) oracleResult {
	if hit := oracleBlock(spans, cfg.Rules[:cfg.BlockEnd]); hit != nil {
		return oracleResult{blocked: hit}
	}
	spans = oracleRewrite(spans, cfg.Rules[cfg.BlockEnd:cfg.StripEnd], modeStrip)
	spans = oracleRewrite(spans, cfg.Rules[cfg.StripEnd:], modeObfs)
	return oracleResult{spans: spans}
}
```

Seed empty Objects, every bucket subset, legacy arrays, overlaps, cross-phase cascades, invalid matcher bytes, and same/cross-bucket duplicates.

- [ ] **Step 5: Run RED tests and short fuzz checks**

```powershell
go test . -run '^(Test.*Mixed|Test.*Block.*Boundary|Test.*CrossPhase|Test.*Legacy)' -count=1
go test . -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=10s
```

Expected: mixed tests fail at global-mode dispatch; legacy tests remain green.

- [ ] **Step 6: Replace global mode dispatch with range dispatch**

Implement the minimum shape:

```go
blockRules := cfg.Rules[:cfg.BlockEnd]
stripRules := cfg.Rules[cfg.BlockEnd:cfg.StripEnd]
obfsRules := cfg.Rules[cfg.StripEnd:]

if hit := matchBlock(spans, blockRules, cfg); hit != nil {
	return applyResult{Blocked: hit}
}
for _, r := range stripRules {
	spans = stripRuleFromSpans(spans, r, cfg.IgnoreCase)
}
for _, r := range obfsRules {
	spans = obfuscateRuleInSpans(spans, r, cfg)
}
```

Adapt this snippet to existing helper names; do not combine ordered rewrite passes into one automaton.

- [ ] **Step 7: Bound matcher inputs and threshold counts**

Use `cfg.BlockEnd` for byte/folded block strategy selection. Use `len(cfg.Rules)-cfg.BlockEnd` for folded rewrite preflight. The rewrite matcher is boolean-only and must never produce a block decision.

- [ ] **Step 8: Run GREEN unit, race, fuzz, and focused benchmark checks**

```powershell
go test . -run '^(Test.*Mixed|Test.*Block|Test.*Rewrite|Test.*Legacy|Test.*Matcher)' -count=1
go test -race . -run '^(Test.*Mixed|TestConcurrentSnapshot)' -count=1
go test . -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=30s
go test . -run '^$' -bench 'Mixed|FoldedRewriteStrategies|ExactBlock' -benchmem -count=3
```

Expected: tests and fuzz pass; phase-specific paths retain or improve the corresponding baseline without adding allocations to zero-rule paths.

- [ ] **Step 9: REFACTOR only duplicated range calculations introduced here**

Keep range expressions in one `applyMode`/`applyRules` site; do not add mode-tagged matcher nodes or three snapshot slices. Run `gofmt` and the focused suite again.

- [ ] **Step 10: Append evidence and commit**

```bash
git add transform.go transform_test.go matcher.go matcher_test.go fuzz_test.go benchmark_test.go docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md
git commit -m "feat: execute mixed censorship modes"
```

---

### Task 5: Harden Shared JSON and Span Selection

**Files:**
- Modify: `selectors.go`
- Modify: `selectors_role_gate_test.go`
- Modify: `benchmark_test.go`
- Modify: `docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md`

**Interfaces:**
- Consumes: shared JSON validation, `appendStringSpan`, `scanTextPart`, role-format gate.
- Produces: pre-recursion depth rejection, UTF-8 validation, zero empty spans, ProtoJSON-null-safe text parts, and `tool` eligibility for `openai-response`.

- [ ] **Step 1: Trace every caller of the shared helpers**

Read `selectors.go` and use reference search for `appendStringSpan`, `scanTextPart`, JSON nesting validation, and `selectorHasEnabledRole`. Do not edit provider files in this task.

- [ ] **Step 2: Add RED subprocess depth test**

Create a test mode triggered by an environment variable. The parent starts the test binary with a valid JSON object nested far beyond 1024 levels and requires a prompt `errInvalidRequest` exit rather than recursive validation stack growth.

```go
if os.Getenv("CENSORSHIP_DEEP_JSON_CHILD") == "1" {
	_, err := selectTextSpans(deepBody, "openai", defaultRoles())
	if !errors.Is(err, errInvalidRequest) { os.Exit(2) }
	os.Exit(0)
}
```

- [ ] **Step 3: Add RED UTF-8 and empty-span tests**

Assert invalid UTF-8 in a JSON key and in a selected value returns `errInvalidRequest`. Build a wide request with thousands of `"content":""` fields and assert zero spans; keep an escaped non-empty string as a positive control.

- [ ] **Step 4: Add RED null-discriminator and Responses-tool gate tests**

Assert `functionCall:null`, reversed key order, and `function_call:null` remain selectable text; non-null machine/signature values and `thought:true` remain excluded. Assert `selectorHasEnabledRole("openai-response", rolesWithTool)` returns true.

- [ ] **Step 5: Run RED checks**

```powershell
go test . -run '^(TestJSONDepthGuardPrecedesParser|TestInvalidUTF8|TestEmptyStringSpans|TestScanTextPartNull|TestSelectorHasEnabledRole)' -count=1
```

Expected: each newly added case fails for its current behavior.

- [ ] **Step 6: Reorder and tighten the shared boundary**

Use this ordering before provider traversal:

```go
if !jsonNestingWithin(body, maxJSONNesting) ||
	!utf8.Valid(body) ||
	!gjson.ValidBytes(body) {
	return nil, errInvalidRequest
}
root := gjson.ParseBytes(body)
if !root.IsObject() {
	return nil, errInvalidRequest
}
```

- [ ] **Step 7: Skip decoded empties and treat JSON null as absent**

```go
if value.Type != gjson.String || value.Str == "" {
	return spans
}

func presentNonNull(v gjson.Result) bool {
	return v.Exists() && v.Type != gjson.Null
}
```

Reuse a local expression instead of adding `presentNonNull` if it is used only once. Add `tool` to the existing `openai-response` role gate without broadening any other format.

- [ ] **Step 8: Run GREEN and benchmark checks**

```powershell
go test . -run '^(TestJSON|TestInvalidUTF8|TestEmptyStringSpans|TestScanTextPart|TestSelector)' -count=1
go test -race . -run '^(TestJSONDepthGuardPrecedesParser|TestEmptyStringSpans)' -count=1
go test . -run '^$' -bench 'Empty|DisabledRoleSelectors|TextPartScanning' -benchmem -count=3
```

Expected: all tests pass; empty-string case allocates no spans and performs no span sort.

- [ ] **Step 9: Append evidence and commit**

```bash
git add selectors.go selectors_role_gate_test.go benchmark_test.go docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md
git commit -m "fix: harden request text selection"
```

---

### Task 6: Cover Current OpenAI Request Text

**Parallel wave:** Run concurrently with Tasks 7, 8, and 9 in an isolated worktree based on Task 5.

**Files:**
- Modify: `selectors_openai.go`
- Modify: `selectors_openai_test.go`
- Modify: `selectors_role_gate_test.go`

**Interfaces:**
- Consumes: Task 5's `tool` role gate, null-safe shared helper, and span API.
- Produces: explicit OpenAI Chat tool/refusal/function paths and Responses tool-result paths.

- [ ] **Step 1: Read only the OpenAI selector, its tests, and Task 5's shared role-gate test file**

Trace current Chat and Responses discriminators. Do not read another provider selector.

- [ ] **Step 2: Add RED Chat cases**

Add role-on and role-off cases for tool text arrays, assistant top-level refusal, assistant refusal parts, and legacy function-message string content. Assert IDs, names, tool calls, and non-text content stay byte-identical.

- [ ] **Step 3: Add RED Responses result table**

Use exact rows for:

```go
[]string{
	"function_call_output",
	"custom_tool_call_output",
	"local_shell_call_output",
	"shell_call_output",
	"apply_patch_call_output",
	"mcp_call",
	"program_result",
}
```

Cover string `output`, valid `input_text` arrays where supported, shell stdout/stderr, MCP output/error, and program result. Add negative fields containing the same term in IDs, arguments, commands, URLs, files, screenshots, status, image/file parts, and unknown item types.

- [ ] **Step 4: Run RED checks**

```powershell
go test . -run '^(TestOpenAI.*Tool|TestOpenAI.*Refusal|TestOpenAI.*Function|TestOpenAIResponses.*Output|TestOpenAIRoleGate)' -count=1
```

Expected: current exclusions cause each new natural-language path to remain unchanged.

- [ ] **Step 5: Implement explicit Chat allowlists**

Normalize role `function` to canonical `tool`. Let tool arrays reach existing `type:"text"` traversal. For assistant messages, append top-level `refusal` and `type:"refusal"` part values only.

- [ ] **Step 6: Implement exact Responses result dispatch**

Use a `switch item.Get("type").String()` and append only fields named by the spec. Structured outputs select only `type:"input_text"` text. No default recursive traversal.

- [ ] **Step 7: Run GREEN and full OpenAI checks**

```powershell
go test . -run '^(TestOpenAI|TestSelector.*OpenAI)' -count=1
go test -race . -run '^(TestOpenAI|TestSelector.*OpenAI)' -count=1
```

Expected: all new and existing OpenAI tests pass.

- [ ] **Step 8: REFACTOR only repeated output-string selection inside this file**

Use one small file-local helper only if three or more branches have identical string-or-`input_text` logic. Run `gofmt` and the focused tests again.

- [ ] **Step 9: Commit and return TDD evidence to the coordinator**

```bash
git add selectors_openai.go selectors_openai_test.go selectors_role_gate_test.go
git commit -m "fix: cover OpenAI tool and refusal text"
```

Return the commit hash and literal RED/GREEN/REFACTOR evidence. Do not edit the shared TDD record.

---

### Task 7: Cover Claude Tool, Search, and Text-Document Results

**Parallel wave:** Run concurrently with Tasks 6, 8, and 9 in an isolated worktree based on Task 5.

**Files:**
- Modify: `selectors_claude.go`
- Modify: `selectors_claude_test.go`

**Interfaces:**
- Consumes: shared `appendStringSpan`, `scanTextPart`, and canonical `user`/`tool` role gates.
- Produces: explicit Claude string tool results, search prose, and plain-text document selection.

- [ ] **Step 1: Read only the Claude selector and its tests**

Identify direct-user versus nested-tool containers and every machine field that existing tests preserve.

- [ ] **Step 2: Add RED cases for all six enclosing forms**

Cover string `tool_result.content`, direct/nested `search_result.content[].text`, and direct/nested `document.source.data` where `source.type == "text"`. Repeat with the relevant role disabled.

- [ ] **Step 3: Add byte-preservation controls**

Place the term in search source/title/citation data, document media type, URL/file/base64 source, `tool_use_id`, `is_error`, tool input, thinking/redacted-thinking, and non-text blocks. Require unchanged bytes.

- [ ] **Step 4: Run RED checks**

```powershell
go test . -run '^TestClaude' -count=1
```

Expected: the new natural-language forms remain unselected while existing cases pass.

- [ ] **Step 5: Implement narrow enclosing-role traversal**

Before the existing array guard, append string tool-result content. Add file-local helpers that receive the enclosing canonical role and inspect only:

```go
case "search_result":
	// content[] entries whose text is a JSON string
case "document":
	// source.type == "text" and source.data is a JSON string
```

Do not recurse through arbitrary strings.

- [ ] **Step 6: Run GREEN and race checks**

```powershell
go test . -run '^TestClaude' -count=1
go test -race . -run '^TestClaude' -count=1
```

- [ ] **Step 7: Commit and return TDD evidence**

```bash
git add selectors_claude.go selectors_claude_test.go
git commit -m "fix: cover Claude result text"
```

Return the commit hash and literal evidence. Do not edit the shared TDD record.

---

### Task 8: Correct Gemini Omitted Roles and Traversal Cost

**Parallel wave:** Run concurrently with Tasks 6, 7, and 9 in an isolated worktree based on Task 5.

**Files:**
- Modify: `selectors_gemini.go`
- Modify: `selectors_gemini_test.go`

**Interfaces:**
- Consumes: Task 5's ProtoJSON-null-safe `scanTextPart`.
- Produces: missing/null/empty role equivalence and early return when neither user nor assistant can be selected.

- [ ] **Step 1: Read only the Gemini selector and its tests**

Trace role alternation and identify the point after system instruction but before `contents` traversal.

- [ ] **Step 2: Add RED role-equivalence cases**

Test first/only and post-explicit-user turns with missing role, `"role":null`, and `"role":""`. Require identical canonical-role results for all three omitted forms; retain skip-plus-advance for other nonempty roles.

- [ ] **Step 3: Reverse stale null-discriminator expectations**

Require both key orders of `functionCall:null` and snake-case `function_call:null` text to be selectable. Preserve non-null function call/response, executable code, media, thought, and signature exclusions.

- [ ] **Step 4: Add a system-only traversal benchmark and role-set tests**

Use a large `contents` array and role sets empty, system-only, and tool-only. Assert output spans stay the same and benchmark the early return.

- [ ] **Step 5: Run RED checks**

```powershell
go test . -run '^(TestGemini.*Role|TestGemini.*Null|TestGemini.*Disabled)' -count=1
go test . -run '^$' -bench '^BenchmarkGeminiSystemOnly' -benchmem -count=3
```

- [ ] **Step 6: Implement omitted-role and early-return logic**

```go
role := content.Get("role")
omitted := !role.Exists() || role.Type == gjson.Null || role.String() == ""
if omitted {
	// existing alternation branch
}

if !roles.has("user") && !roles.has("assistant") {
	return spans
}
```

Place the return after system selection and before reading `contents`.

- [ ] **Step 7: Run GREEN, race, and benchmark checks**

```powershell
go test . -run '^(TestGemini|TestScanTextPart)' -count=1
go test -race . -run '^TestGemini' -count=1
go test . -run '^$' -bench '^BenchmarkGeminiSystemOnly' -benchmem -count=5
```

Expected: tests pass and the disabled-content path no longer scales with turn count.

- [ ] **Step 8: Commit and return TDD evidence**

```bash
git add selectors_gemini.go selectors_gemini_test.go
git commit -m "fix: handle omitted Gemini roles"
```

Return commit hash and literal evidence. Do not edit the shared TDD record.

---

### Task 9: Cover Direct Gemini Interactions TextContent

**Parallel wave:** Run concurrently with Tasks 6, 7, and 8 in an isolated worktree based on Task 5.

**Files:**
- Modify: `selectors_interactions.go`
- Modify: `selectors_interactions_test.go`

**Interfaces:**
- Consumes: shared `scanTextPart`, user role gate, and existing `collectInteractionItem` step handling.
- Produces: exact top-level direct Object/array `type:"text"` handling without broad recursion.

- [ ] **Step 1: Read only the Interactions selector and its tests**

Trace top-level `input` dispatch separately from step-item dispatch.

- [ ] **Step 2: Add RED direct-content cases**

Cover a direct `{"type":"text","text":"SECRET"}` Object and an array containing direct TextContent. Require only `text` to change under `user`; require unchanged bytes when `user` is disabled.

- [ ] **Step 3: Add mixed-content exclusions**

Mix direct text with image/media content and with tool/thought/control steps. Require only direct natural-language text to be selected. Retain `model_output` under `assistant` only.

- [ ] **Step 4: Run RED checks**

```powershell
go test . -run '^TestInteractions' -count=1
```

Expected: direct TextContent cases produce no span under current dispatch.

- [ ] **Step 5: Implement exact top-level dispatch**

```go
if item.Get("type").String() == "text" {
	if scanTextPart(item, true) {
		spans = appendStringSpan(spans, item.Get("text"), "user", roles)
	}
	continue
}
spans = collectInteractionItem(spans, item, roles)
```

Apply the same decision to direct Object and array elements; do not alter nested step rules.

- [ ] **Step 6: Run GREEN and race checks**

```powershell
go test . -run '^TestInteractions' -count=1
go test -race . -run '^TestInteractions' -count=1
```

- [ ] **Step 7: Commit and return TDD evidence**

```bash
git add selectors_interactions.go selectors_interactions_test.go
git commit -m "fix: select direct Interactions text"
```

Return commit hash and literal evidence. Do not edit the shared TDD record.

---

### Task 10: Integrate the Provider Wave and Record Its Evidence

**Files:**
- Modify: `docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md`

**Interfaces:**
- Consumes: isolated commits and literal evidence from Tasks 6 through 9.
- Produces: one integrated selector state and a complete provider TDD record.

- [ ] **Step 1: Cherry-pick provider commits in numeric order**

```bash
git cherry-pick "$(git log --all --format=%H --grep='^fix: cover OpenAI tool and refusal text$' -1)"
git cherry-pick "$(git log --all --format=%H --grep='^fix: cover Claude result text$' -1)"
git cherry-pick "$(git log --all --format=%H --grep='^fix: handle omitted Gemini roles$' -1)"
git cherry-pick "$(git log --all --format=%H --grep='^fix: select direct Interactions text$' -1)"
```

Confirm each resolved hash equals the corresponding worker result before executing it. Resolve no conflict by discarding behavior; if a conflict appears outside `selectors_role_gate_test.go`, stop and diagnose the unexpected overlap.

- [ ] **Step 2: Append each worker's literal evidence under separate headings**

Record Task 6, 7, 8, and 9 RED/GREEN/REFACTOR commands, exit codes, and decisive lines exactly as returned. Do not summarize a failure as a pass.

- [ ] **Step 3: Run the combined selector suite**

```powershell
go test . -run '^(TestSelector|TestOpenAI|TestClaude|TestGemini|TestInteractions|TestScanTextPart|TestInvalidUTF8|TestEmptyString)' -count=1
go test -race . -run '^(TestOpenAI|TestClaude|TestGemini|TestInteractions)' -count=1
```

Expected: PASS with no provider cross-regression.

- [ ] **Step 4: Record the combined result and commit the TDD log**

```bash
git add docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md
git commit -m "test: record provider selector TDD"
```

---

### Task 11: Repair the Tagged Integration and ABI Benchmark Seam

**Files:**
- Delete: `integration/abi_benchmark_test.go`
- Create: `.github/scripts/testdata/abi_benchmark_test.go`
- Modify: `.github/scripts/integration-runner.go`
- Modify: `.github/scripts/integration-runner_test.go`
- Modify: `abi_cgo.go`
- Modify: `abi_cgo_test.go`
- Modify: `integration/doc.go`
- Modify: `docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md`

**Interfaces:**
- Consumes: host-module benchmark source, synchronous ABI call contract, `handleMethod`, host-owned input pointer, plugin-owned output buffer.
- Produces: buildable tagged plugin tests, runner-hosted ABI benchmark, and a definitive safe no-copy gate result.

- [ ] **Step 1: Read only ABI files, the benchmark, integration package declaration, and runner copy/benchmark functions**

Trace every `handleMethod` branch and every function receiving or retaining the raw request slice. Write the call graph into the TDD record before considering `unsafe.Slice`.

- [ ] **Step 2: Add RED tagged-build and benchmark-placement tests**

Require the integration package to compile under:

```powershell
go test -mod=readonly -tags=integration ./integration -run '^TestReadUntilCompletedReturnsOnDeadline$' -count=1
```

Expected RED: foreign CPA `internal` import blocks package setup. Add runner tests requiring the benchmark fixture to be copied into the generated CPA-module test directory and invoked there.

- [ ] **Step 3: Relocate the benchmark with Git history preserved**

```bash
git mv integration/abi_benchmark_test.go .github/scripts/testdata/abi_benchmark_test.go
```

Teach the existing integration runner to copy this fixture into `.integration/cpa/integration/censorshipplugin` beside generated integration tests and run `BenchmarkDynamicABIRequestInterceptors` from that CPA module.

- [ ] **Step 4: Run GREEN tagged-build and runner tests**

```powershell
go test -mod=readonly -tags=integration ./integration -run '^TestReadUntilCompletedReturnsOnDeadline$' -count=1
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -run 'BenchmarkPlacement|CopyIntegration' -count=1
```

- [ ] **Step 5: Write RED ABI borrowing tests before production change**

Cover zero length, one byte, architecture maximum-int overflow rejection, matching/nonmatching output equality, and no post-return dependency. The lifetime test allocates host-like C memory, calls synchronously, poisons it immediately after return, and confirms all returned bytes remain plugin-owned.

Define the planned conversion contract:

```go
func borrowedRequest(ptr unsafe.Pointer, n C.size_t) ([]byte, error)
```

Expected RED: helper is absent and the existing path allocates through `C.GoBytes`.

- [ ] **Step 6: Prove the synchronous non-retention call graph**

For register, reconfigure, before-auth, after-auth, and unknown methods, record every raw-slice use. The proof passes only if no path stores a slice, aliases it into a surviving string, closes over it, starts asynchronous work with it, or returns it as output without copying.

- [ ] **Step 7: Run the repaired host benchmark before changing the copy**

```powershell
go run ./.github/scripts/integration-runner.go -bench-abi
```

Record matching and nonmatching 1 KiB, 1 MiB, and 20 MiB `ns/op`, `B/op`, and allocations.

- [ ] **Step 8: Apply the no-copy implementation only after Steps 5 through 7 pass**

```go
func borrowedRequest(ptr unsafe.Pointer, n C.size_t) ([]byte, error) {
	if uint64(n) > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("request length exceeds Go int")
	}
	if n == 0 {
		return nil, nil
	}
	if ptr == nil {
		return nil, fmt.Errorf("request pointer is nil for non-zero length")
	}
	return unsafe.Slice((*byte)(ptr), int(n)), nil
}
```

Keep the actual C type used by the existing signature. Add one comment stating that the host owns the input and guarantees it only for this synchronous call. Ensure response bytes are encoded/copied into plugin-owned `malloc` memory before return. Preserve the existing AfterAuth no-input-copy fast path.

- [ ] **Step 9: Run GREEN safety, race, ABI, and benchmark checks**

```powershell
go test . -run '^(TestBorrowedRequest|TestChecked.*Length|Test.*ABI|TestHandleMethod.*Ownership)' -count=1
go test -race . -run '^(TestBorrowedRequest|Test.*ABI)' -count=1
go run ./.github/scripts/integration-runner.go -bench-abi
```

Expected: exact output equality; no retained input; input-sized plugin allocation disappears; repeated medians improve enough to justify the unsafe view. If any gate fails, revert only the production borrowing edit, retain the repaired benchmark/tests, and record the exact blocker without claiming no-copy performance.

- [ ] **Step 10: REFACTOR only the old length/copy helper made obsolete**

Remove a helper only if all callers moved to the bounded borrowing path. Run `gofmt`, unit tests, and benchmark once more.

- [ ] **Step 11: Append evidence and commit**

```bash
git add abi_cgo.go abi_cgo_test.go integration/doc.go .github/scripts/testdata/abi_benchmark_test.go .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md
git add -u integration/abi_benchmark_test.go
git commit -m "perf: remove verified ABI input copy"
```

If the gate fails, use commit message `test: repair ABI benchmark coverage` and ensure release notes later state that the copy remains.

---

### Task 12: Make Integration Paths, Readiness, I/O, Reload, and Responses Reliable

**Parallel wave:** Run concurrently with Tasks 13 and 14 in an isolated worktree based on Task 11.

**Files:**
- Modify: `integration/harness_test.go`
- Modify: `integration/http_test.go`
- Modify: `integration/websocket_test.go`

**Interfaces:**
- Consumes: selected CPA commit, external CPA process, plugin directory, test upstream, HTTP/WebSocket helpers.
- Produces: absolute/provenanced startup, health-only readiness, bounded I/O, exact replacement checks, and HTTP Responses coverage.

- [ ] **Step 1: Read only the three integration files and trace startup/teardown/request helpers**

List each helper's caller before choosing one root-cause edit point.

- [ ] **Step 2: Add RED path and provenance tests**

Build fixtures for equivalent absolute/relative binary and plugin paths, wrong revision, and missing VCS revision. Require rejection before process start for invalid provenance and identical resolved paths for valid inputs.

- [ ] **Step 3: Add RED readiness and timeout fixtures**

Use local fixtures that serve authenticated `/v1/models` without the former log line, accept HTTP/TCP/WebSocket connections without responding, and send a terminal 400 WebSocket event while keeping the connection open.

- [ ] **Step 4: Add RED reload and HTTP Responses cases**

First observe snapshot A blocking `alpha-only`; after B is observed blocking `beta-only`, require A to reach upstream. Add `/v1/responses` block and transform cases for string input and structured `input_text`.

- [ ] **Step 5: Run RED integration helper tests**

```powershell
go test -mod=readonly -tags=integration ./integration -run '^(TestResolve|TestRevision|TestReadiness|Test.*Timeout|TestWatcherReload|TestHTTPResponses)' -count=1
```

Expected: each new defect reproducer fails for the specified reason, not package setup.

- [ ] **Step 6: Resolve paths and verify build provenance before temporary directories**

Use `exec.LookPath`, `filepath.Abs`, and `debug/buildinfo.ReadFile`. Require `vcs.revision == "c76dfd4e0edabab9000628b1560ab8ab379eadb8"`; include expected and actual values in errors.

- [ ] **Step 7: Replace log-coupled readiness with public health**

Return on the first authenticated `200` from `/v1/models`. Keep plugin behavior checks in individual tests.

- [ ] **Step 8: Add operation-scoped I/O bounds**

Use one harness-owned `http.Client{Timeout: 5 * time.Second}`. Call `SetDeadline(time.Now().Add(5*time.Second))` immediately after raw TCP/WebSocket connection establishment. Before post-400 read, reset the deadline; reject `net.Error` timeouts and accept only peer close/EOF.

- [ ] **Step 9: Remove unsupported Windows interrupt wait**

```go
if runtime.GOOS == "windows" {
	terminateCPA(t, cmd)
	return
}
```

Retain interrupt plus grace period on non-Windows.

- [ ] **Step 10: Run GREEN tagged tests**

```powershell
go test -mod=readonly -tags=integration ./integration -count=1
go test -race -mod=readonly -tags=integration ./integration -run '^(TestResolve|TestRevision|TestReadiness|Test.*Timeout)' -count=1
```

- [ ] **Step 11: Commit and return TDD evidence**

```bash
git add integration/harness_test.go integration/http_test.go integration/websocket_test.go
git commit -m "test: harden CPA integration coverage"
```

Return commit hash and literal evidence; do not edit the shared TDD record.

---

### Task 13: Reuse the Integration Checkout

**Parallel wave:** Run concurrently with Tasks 12 and 14 in an isolated worktree based on Task 11.

**Files:**
- Modify: `.github/scripts/integration-runner.go`
- Modify: `.github/scripts/integration-runner_test.go`

**Interfaces:**
- Consumes: existing containment-checked removal, checkout verification, generated `integration/censorshipplugin` directory.
- Produces: second-run reuse while unrelated checkout dirt still invalidates cache.

- [ ] **Step 1: Read only runner preparation/copy code and its tests**

Confirm removal occurs only under the checkout root and before strict verification.

- [ ] **Step 2: Add RED reuse and dirt-rejection tests**

Create a clean temporary checkout, add only runner-owned generated files, and require reuse without fetch. Add an unrelated untracked file and require rejection/removal.

- [ ] **Step 3: Run RED checks**

```powershell
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -run '^(TestPrepareCheckoutReusesGeneratedDirectory|TestPrepareCheckoutRejectsUnrelatedDirt)$' -count=1
```

Expected: generated files currently make reuse fail.

- [ ] **Step 4: Remove only runner-owned output before verification**

Call the existing containment-checked removal for `<checkout>/integration/censorshipplugin`, then run unchanged strict checkout verification. Do not ignore arbitrary untracked files.

- [ ] **Step 5: Run GREEN and repeat-run checks**

```powershell
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -count=1
go run ./.github/scripts/integration-runner.go
go run ./.github/scripts/integration-runner.go
```

Expected: tests pass; the second invocation does not fetch the selected CPA commit.

- [ ] **Step 6: Commit and return TDD evidence**

```bash
git add .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
git commit -m "perf: reuse CPA integration checkout"
```

Return commit hash and literal evidence; do not edit the shared TDD record.

---

### Task 14: Make Packaging and Workflow Deterministic and Cheap

**Parallel wave:** Run concurrently with Tasks 12 and 13 in an isolated worktree based on Task 11.

**Files:**
- Modify: `.github/scripts/package-release.go`
- Modify: `.github/scripts/package-release_test.go`
- Modify: `.github/workflows/build.yml`
- Modify: `Makefile`

**Interfaces:**
- Consumes: version normalization, archive/checksum paths, `sha256File`, aggregate packaging, upload-artifact steps.
- Produces: nonempty safe versions, noncolliding paths, one ZIP hash pass, pinned cross action, and strict no-recompression uploads.

- [ ] **Step 1: Read only packager, workflow, Makefile, and their contract tests**

Trace direct package mode, aggregate mode, every path open, and all three artifact upload steps.

- [ ] **Step 2: Add RED version and collision tests**

Require `v`, empty normalized version, unsafe filename components, and all equal/alias pairs among library/archive/checksum to fail before file contents or timestamps change.

- [ ] **Step 3: Add RED one-pass hash instrumentation**

Use the smallest existing test seam to count `sha256File` calls. Aggregate N ZIPs must hash exactly N times, while each per-archive line exactly matches its line in `checksums.txt`.

- [ ] **Step 4: Add RED workflow-contract assertions**

Require both cross actions to use:

```yaml
uses: go-cross/cgo-actions@d0b8f2f2d67923ce9a42d92a7ef0ed1ebd905f0a # v1
```

Require all three upload steps to contain:

```yaml
compression-level: 0
if-no-files-found: error
```

- [ ] **Step 5: Run RED checks**

```powershell
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -run 'Version|Collision|Checksum|Workflow' -count=1
```

Expected: new safety, call-count, and workflow assertions fail.

- [ ] **Step 6: Reject empty normalized versions in Make and Go before output**

Keep one lowercase leading-`v` normalization. In Go, return an error before opening paths when the result is empty. In Make, add a precondition in the existing version-validation path rather than duplicating validation per target.

- [ ] **Step 7: Reject path collisions before opening outputs**

Canonicalize absolute clean paths. On Windows compare case-insensitively; where files exist, use `os.SameFile` to detect aliases. Reject library/archive, library/checksum, and archive/checksum equality.

- [ ] **Step 8: Retain first-pass checksum lines**

Change the existing checksum write path to return the formatted digest line, collect lines in deterministic artifact order, and write `checksums.txt` from those lines without reopening ZIPs.

```go
line, err := writeChecksum(archive, checksum)
if err != nil { return err }
lines = append(lines, line)
```

- [ ] **Step 9: Update workflow inputs and action pin**

Apply the exact SHA and both upload inputs to all three artifact groups. Keep release ZIP Deflate compression unchanged.

- [ ] **Step 10: Run GREEN, race, vet, and package smoke checks**

```powershell
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1
go test -race .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1
go vet .github/scripts/package-release.go
make package VERSION=v0.2.0 GOOS=windows GOARCH=amd64
```

Expected: tests pass; smoke package creates a safe versioned ZIP and matching checksum.

- [ ] **Step 11: Commit and return TDD evidence**

```bash
git add .github/scripts/package-release.go .github/scripts/package-release_test.go .github/workflows/build.yml Makefile
git commit -m "build: harden release packaging"
```

Return commit hash and literal evidence; do not edit the shared TDD record.

---

### Task 15: Integrate Tooling Wave and Publish Accurate Documentation

**Files:**
- Modify: `README.md`
- Modify: `RELEASE_NOTES.md`
- Modify: `main_test.go`
- Modify: `docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md`

**Interfaces:**
- Consumes: Task 12/13/14 commits and evidence, final Object/selector/ABI behavior, selected CPA pin.
- Produces: integrated tooling, resolved baseline documentation assertion, and v0.2.0 release notes containing only verified claims.

- [ ] **Step 1: Cherry-pick tooling commits in numeric order**

```bash
git cherry-pick "$(git log --all --format=%H --grep='^test: harden CPA integration coverage$' -1)"
git cherry-pick "$(git log --all --format=%H --grep='^perf: reuse CPA integration checkout$' -1)"
git cherry-pick "$(git log --all --format=%H --grep='^build: harden release packaging$' -1)"
```

Confirm each resolved hash equals the corresponding worker result before executing it.

- [ ] **Step 2: Append each tooling worker's literal evidence**

Create separate TDD headings for Tasks 12, 13, and 14, then record the combined tagged-integration and package results.

- [ ] **Step 3: Replace the stale documentation fixture with canonical Object content**

The primary example must be:

```yaml
plugins:
  enabled: true
  configs:
    censorship:
      enabled: true
      ignore_case: false
      words:
        block: [example]
        strip: []
        obfs: []
      scope:
        formats: [openai, openai-response, claude, gemini, interactions]
        roles: [system, developer, user]
      obfs:
        char: "​"
```

Update the exact `main_test.go` documentation assertion to require this canonical block in both README and release notes. Do not put `mode` in the primary or panel example.

- [ ] **Step 4: Add a separate handwritten-YAML compatibility subsection**

Document legacy list plus global `mode`, state that the panel neither renders nor emits it, and state Object mode is behaviorally ignored but a supplied value remains syntax-validated.

- [ ] **Step 5: Document final behavior and compatibility**

Cover fixed block -> strip -> obfs order, bucket subsets and strict keys, scoped obfs validation, every corrected provider path, unchanged machine exclusions, icon URL, CLIProxyAPI v7.2.152/schema 5, exact host commit, native ABI v1, glibc 2.34+ Linux requirement, accurate LICENSE inclusion, integration changes, and measured ABI outcome.

- [ ] **Step 6: Write v0.2.0 release notes without unmeasured claims**

If Task 11 passed all gates, report measured no-copy benchmark medians and allocation reduction. If it retained `C.GoBytes`, name the exact failed gate and omit a no-copy performance claim.

- [ ] **Step 7: Run the formerly failing RED-to-GREEN checks**

```powershell
go test . -run '^TestDocumentationListsConfigAndLimits$' -count=1
go test -race . -run '^TestDocumentationListsConfigAndLimits$' -count=1
```

Expected: PASS, directly resolving the baseline failure.

- [ ] **Step 8: Run docs/config contract checks**

```powershell
go test . -run '^(TestDocumentation|TestRegistration|TestParseConfig)' -count=1
git diff --check
```

- [ ] **Step 9: Append evidence and commit**

```bash
git add README.md RELEASE_NOTES.md main_test.go docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md
git commit -m "docs: prepare v0.2.0 release"
```

---

### Task 16: Run Complete Local Verification

**Files:**
- Modify: `docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md`
- Modify only when a verification failure proves a defect: the owning source/test file from Tasks 2 through 15.

**Interfaces:**
- Consumes: complete implementation branch.
- Produces: fresh local evidence for every completion criterion and a clean diff.

- [ ] **Step 1: Invoke `superpowers:verification-before-completion`**

Follow the skill before making any completion statement or release commit.

- [ ] **Step 2: Run formatting and generated-state checks**

```powershell
gofmt -w *.go integration/*.go .github/scripts/*.go .github/scripts/testdata/*.go
git diff --check
git status --short
```

Expected: no formatting error; only intentional tracked changes before committing any formatting result.

- [ ] **Step 3: Run unit, script, race, and vet suites from a fresh command**

```powershell
make test
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -count=1
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1
make race
make vet
```

Expected: every command exits 0.

- [ ] **Step 4: Run tagged integration package and real CPA integration twice**

```powershell
go test -mod=readonly -tags=integration ./integration -count=1
make integration
make integration
```

Expected: all behavior passes; second runner invocation reuses checkout and does not fetch the same host again.

- [ ] **Step 5: Run fuzz smoke coverage**

```powershell
go test . -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=30s
go test . -run '^$' -fuzz '^FuzzProtocolTransform$' -fuzztime=30s
go test . -run '^$' -fuzz '^FuzzRebuildBodyAgainstMarshalOracle$' -fuzztime=30s
```

Expected: no mismatch or crash.

- [ ] **Step 6: Record focused post-change benchmarks**

```powershell
go test . -run '^$' -bench 'TransformMatrix|Mixed|Fold|ExactBlock|Empty|GeminiSystemOnly' -benchmem -count=5 | Tee-Object -FilePath "$env:TEMP\censorship-v020-after-bench.txt"
go run ./.github/scripts/integration-runner.go -bench-abi
```

Compare like-for-like medians with `%TEMP%/censorship-v020-before-bench.txt`. Record regressions as failures unless caused by intentionally added mixed work and offset by a no-rule fast path.

- [ ] **Step 7: Build and inspect a local v0.2.0 package**

```powershell
make package VERSION=v0.2.0 GOOS=windows GOARCH=amd64
Get-FileHash -Algorithm SHA256 dist/censorship_0.2.0_windows_amd64.zip
```

Require the `.zip.sha256` line to equal the calculated digest and the ZIP to contain the plugin library plus `LICENSE`.

- [ ] **Step 8: Verify dependency, metadata, and source constraints**

```powershell
go list -m github.com/router-for-me/CLIProxyAPI/v7
git grep -n '81e1b537\|v7.2.147\|ConfigField.*mode\|C.GoBytes'
git grep -n 'https://raw.githubusercontent.com/DoingDog/cpa-plugin-censorship/main/logo.png'
```

Expected: selected module is v7.2.152; old pin and panel mode are absent; `C.GoBytes` is absent only if Task 11's gate passed; exact icon URL appears in metadata/docs/tests.

- [ ] **Step 9: Append every literal result and commit verification evidence**

```bash
git add docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md
git add -u
git commit -m "test: verify censorship v0.2.0"
```

Do not commit `dist/` or temporary benchmark files.

---

### Task 17: Run One Partitioned Opus Review and Fix Confirmed Findings

**Files:**
- Review partition A: `config.go`, `config_test.go`, `main.go`, `main_test.go`, `matcher.go`, `matcher_test.go`, `transform.go`, `transform_test.go`, `fuzz_test.go`, `benchmark_test.go`, `abi_cgo.go`, `abi_cgo_test.go`
- Review partition B: `selectors.go`, `selectors_role_gate_test.go`, all four provider selector/test pairs
- Review partition C: `integration/*.go`, `.github/scripts/testdata/abi_benchmark_test.go`
- Review partition D: `.github/scripts/integration-runner*.go`, `.github/scripts/package-release*.go`, `.github/workflows/build.yml`, `Makefile`, `go.mod`, `go.sum`, `README.md`, `RELEASE_NOTES.md`, the new spec/plan/TDD files
- Modify: only files implicated by confirmed findings
- Modify: `docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md`

**Interfaces:**
- Consumes: the verified branch diff from `v0.1.4..HEAD` and the approved spec.
- Produces: one non-overlapping Opus xhigh review pass, Sonnet xhigh fixes for confirmed findings, and fresh post-review verification.

- [ ] **Step 1: Invoke `superpowers:requesting-code-review`**

Use the skill once for this completion review. Do not start a second reviewer over any file.

- [ ] **Step 2: Dispatch four Opus xhigh reviewers in parallel with exact disjoint partitions**

Each reviewer checks correctness, spec compliance, performance regressions, test validity, and accidental CPA/model damage only within its assigned files. Each finding must include concrete input/state and wrong outcome. Reviewers do not read another partition.

- [ ] **Step 3: Apply evidence gating once**

For each reported finding, the coordinator reproduces it with the smallest owned test or proves it from the changed control flow. Mark it `confirmed` or `rejected` in the TDD record. Do not dispatch independent re-reviewers for the same finding.

- [ ] **Step 4: Invoke `superpowers:receiving-code-review` before edits**

Use the skill to validate technically nontrivial review feedback. Reject speculative cleanup and every item outside the spec.

- [ ] **Step 5: Dispatch Sonnet xhigh fix workers only for confirmed, disjoint groups**

Each worker writes a failing regression test, runs RED, makes the minimum fix, runs GREEN, and returns a commit plus literal evidence. Use isolated worktrees if two confirmed groups are independent; never assign the same file twice.

- [ ] **Step 6: Cherry-pick fixes and record evidence**

Cherry-pick in partition order A, B, C, D. Append each RED/GREEN result and rejected-finding rationale to the TDD record.

- [ ] **Step 7: Repeat complete verification after review fixes**

```powershell
make test
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -count=1
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1
make race
make vet
go test -mod=readonly -tags=integration ./integration -count=1
make integration
git diff --check
git status --short
```

Expected: every command exits 0 and the worktree contains only the TDD review evidence if it is not yet committed.

- [ ] **Step 8: Commit review evidence**

```bash
git add docs/superpowers/tdd/2026-09-07-censorship-v0.2.0.tdd.md
git commit -m "test: record final v0.2.0 review"
```

If no finding survives, this commit records the four partition outcomes and zero confirmed findings.

---

### Task 18: Merge, Push, Tag, Build, and Publish v0.2.0

**Files:**
- No source edits unless pre-tag main CI exposes a reproducible defect.
- Git refs: local `main`, `origin/main`, annotated tag `v0.2.0`.
- GitHub: Build workflow and v0.2.0 release.

**Interfaces:**
- Consumes: clean reviewed feature branch and all fresh verification evidence.
- Produces: merged `main`, immutable v0.2.0 tag, successful release workflow, seven platform archives, seven checksum files, and aggregate `checksums.txt`.

- [ ] **Step 1: Invoke `superpowers:finishing-a-development-branch`**

Use its merge path without asking for an execution choice because the user already selected local merge plus push/release.

- [ ] **Step 2: Verify branch and release preconditions**

```powershell
git status --short --branch
git log --oneline --decorate -10
git tag --list v0.2.0
gh auth status
```

Require a clean feature branch, no existing `v0.2.0` tag, and authenticated GitHub CLI.

- [ ] **Step 3: Run the final pre-merge gate once more**

```powershell
make test
make race
make vet
make integration
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1
```

Do not proceed if any command fails.

- [ ] **Step 4: Merge into local main with an explicit merge commit**

```bash
git switch main
git pull --ff-only origin main
git merge --no-ff feat/v0.2.0 -m "merge: release censorship v0.2.0"
```

- [ ] **Step 5: Verify the exact merged tree before publishing**

```powershell
make test
make vet
git status --short --branch
git diff origin/main...HEAD --check
```

Expected: tests pass and main is clean/ahead only by the reviewed merge.

- [ ] **Step 6: Push main and wait for its Build workflow before tagging**

```bash
git push origin main
```

Then:

```powershell
$run = gh run list --workflow Build --branch main --event push --limit 1 --json databaseId --jq '.[0].databaseId'
gh run watch $run --exit-status
```

If main CI exposes a reproducible defect, create a new corrective commit on main, rerun local verification, push normally, and watch the new main run. Never force-push or amend a published commit.

- [ ] **Step 7: Create and push the annotated release tag only after main CI passes**

```bash
git tag -a v0.2.0 -m "censorship v0.2.0"
git push origin v0.2.0
```

This ordering prevents moving a published release tag to repair an ordinary build defect.

- [ ] **Step 8: Watch the tag-triggered Build workflow**

```powershell
$tagRun = gh run list --workflow Build --branch v0.2.0 --event push --limit 1 --json databaseId --jq '.[0].databaseId'
gh run watch $tagRun --exit-status
```

For a transient infrastructure failure, use `gh run rerun $tagRun --failed` and watch the same immutable tag run. Do not move or recreate the pushed tag.

- [ ] **Step 9: Verify release assets and checksums**

```powershell
gh release view v0.2.0 --json tagName,targetCommitish,name,isDraft,isPrerelease,assets
```

Require seven `censorship_0.2.0_<goos>_<goarch>.zip` files, seven matching `.zip.sha256` files, and `checksums.txt`. Download to a temporary directory and validate every listed SHA-256 digest.

- [ ] **Step 10: Verify remote refs and final cleanliness**

```powershell
git fetch origin --tags
git status --short --branch
git rev-parse main
git rev-parse origin/main
git rev-parse v0.2.0^{}
gh run list --workflow Build --limit 5
gh release view v0.2.0
```

Expected: local/remote main and dereferenced tag point to the reviewed release commit, the worktree is clean, the tag workflow succeeded, and the release is published with complete assets.
