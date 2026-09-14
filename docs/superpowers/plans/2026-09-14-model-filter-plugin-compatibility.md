# Models Filter 与模型名称修改插件兼容 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `filter.models` 在 host callback 产生 nested execution 时按该 invocation 的 effective model 匹配，同时保持 outer invocation 的原始客户端模型语义。

**Architecture:** 保持 censorship 的 request interceptor 阶段、filter 布尔逻辑、glob 编译和自然语言 transform 不变。根据 host-owned `Metadata["source"]` 区分普通 outer invocation 与 `host.model.execute*` 产生的 nested callback invocation：outer 匹配 `RequestedModel`，nested callback 匹配当前 `Model`。filter gate 对每次 host invocation 独立计算，nested 结果不回溯撤销 outer 已完成的 transform。通过 unit tests 锁定 distinct-alias 阶段合同，并使用现有跨格式 gate 测试回归 body decoy、空模型、include/exclude 和 API key 组合；只读取这个明确的 host source 标记，不解析 body/URL/headers 或其他 Metadata 字段，不识别任何特定模型改名插件。

**Tech Stack:** Go 1.x、标准库 `testing`、固定 `github.com/router-for-me/CLIProxyAPI/v7 v7.2.152` plugin API、现有 integration harness。

**Spec:** `docs/superpowers/specs/2026-09-14-model-filter-plugin-compatibility-design.md`

## Global Constraints

- 只修改 CPA censorship plugin，不修改 CLIProxyAPI core。
- outer invocation 的 `filter.models` 匹配 `RequestInterceptRequest.RequestedModel`；仅当 `Metadata["source"] == "plugin_host_model_callback"` 时匹配当前 `RequestInterceptRequest.Model`。
- 不从 body、URL、headers 或除 `Metadata["source"]` 之外的 Metadata/provider-specific machine fields 推测模型。
- 不绑定 `cpa-plugin-model-mapper` 的 plugin ID、配置键、规则格式或响应内容。
- 保持 exact/glob、大小写敏感、完整字符串匹配、`include`/`exclude`、`and`/`or`、空 filter 和 API key 行为。
- 保持五种 HTTP source format、Responses WebSocket、自然语言 selectors、ABI v1 buffer ownership、`C.GoBytes` 和 pointer lifetime 契约。
- 使用 TDD：先写会失败的测试，再改最小生产代码，再运行聚焦和全量验证。
- 生产编码使用 Sonnet 1M xhigh；spec、plan、review 使用 Opus 1M xhigh。

---

## 文件地图

- Modify: `request_filter.go:257-298`，新增一个基于 host-owned callback source 的最小模型 subject 选择，并保留现有匹配算法。
- Modify: `request_filter_test.go:301-383,491-535`，将旧的 RequestedModel-only 测试扩展为 outer/nested callback 两阶段合同，并补充 source 缺失、未知、空 Model 和 body decoy 回归。
- Modify: `main_test.go`、`README.md`、`RELEASE_NOTES.md`，更新 models filter 的公开行为描述和文档一致性断言。
- Inspect: `integration/http_test.go`、`integration/websocket_test.go`、`integration/harness_test.go`；只有能观察 host-owned nested callback source 时才增加端到端回归，否则记录 host-level prerequisite，不伪造 CPA alias 语义。
- Do not modify: `main.go`、`config.go`、`transform.go`、`selectors.go`、CLIProxyAPI module cache 或 native ABI 实现。

## Task 1: Write the failing effective-model tests

**Files:**
- Modify: `request_filter_test.go:301-383,491-535`

**Interfaces:**
- Consumes: existing `requestFilter.shouldProcess(*pluginapi.RequestInterceptRequest) bool`.
- Produces: focused tests proving ordinary outer calls use `RequestedModel`, while only host callback nested calls use `Model`.

- [ ] **Step 1: Add a host callback source constant and stage-specific test cases**

In `request_filter_test.go`, define the test source value exactly as the pinned host contract:

```go
const pluginHostModelCallbackSource = "plugin_host_model_callback"
```

Keep the table-driven API/model expectations, but set ordinary requests with `RequestedModel: "target"` when `modelMatches` is true and `Model: "upstream-decoy"`. Add a nested case with `Metadata: map[string]any{"source": pluginHostModelCallbackSource}`, `RequestedModel: "client-a"`, and `Model: "upstream-b"`.

- [ ] **Step 2: Replace the old RequestedModel-only test with an outer/nested stage test**

Rename `TestRequestFilterShouldProcessUsesRequestedModel` to `TestRequestFilterShouldProcessUsesInvocationModelSubject` and use:

```go
func TestRequestFilterShouldProcessUsesInvocationModelSubject(t *testing.T) {
	filter := requestFilter{
		Mode:   filterModeInclude,
		Logic:  filterLogicOr,
		Models: []compiledFilterPattern{{Text: "upstream-b"}},
	}
	request := &pluginapi.RequestInterceptRequest{
		Model:          "upstream-b",
		RequestedModel: "client-a",
		Body:           []byte(`{"model":"body-decoy"}`),
	}
	if filter.shouldProcess(request) {
		t.Fatal("outer invocation matched a future Model instead of RequestedModel")
	}

	request.Metadata = map[string]any{"source": pluginHostModelCallbackSource}
	if !filter.shouldProcess(request) {
		t.Fatal("nested callback invocation did not match Model")
	}

	requestedModelFilter := requestFilter{
		Mode:   filterModeInclude,
		Logic:  filterLogicOr,
		Models: []compiledFilterPattern{{Text: "client-a"}},
	}
	if requestedModelFilter.shouldProcess(request) {
		t.Fatal("nested callback invocation matched RequestedModel instead of Model")
	}
}
```

This test must fail before the production change because the current code always reads `RequestedModel`.

- [ ] **Step 3: Replace the missing-requested-model test with source and empty-model tests**

Rename `TestRequestFilterShouldProcessRejectsMissingRequestedModel` to `TestRequestFilterShouldProcessRejectsUntrustedNestedModelFallback` and cover both unknown source and empty callback Model:

```go
func TestRequestFilterShouldProcessRejectsUntrustedNestedModelFallback(t *testing.T) {
	filter := requestFilter{Mode: filterModeInclude, Logic: filterLogicOr, Models: []compiledFilterPattern{{Text: "anything"}}}
	compileRequestFilter(&filter)
	request := &pluginapi.RequestInterceptRequest{
		Model: "anything", Body: []byte(`{"model":"anything"}`),
	}
	if filter.shouldProcess(request) {
		t.Fatal("outer invocation fell back from an empty RequestedModel")
	}

	request.RequestedModel = "other"
	request.Metadata = map[string]any{"source": "unknown"}
	if filter.shouldProcess(request) {
		t.Fatal("shouldProcess trusted an unknown source or body model")
	}

	request.Model = ""
	request.Metadata["source"] = pluginHostModelCallbackSource
	if filter.shouldProcess(request) {
		t.Fatal("shouldProcess matched an empty nested Model")
	}
	filter.Mode = filterModeExclude
	if !filter.shouldProcess(request) {
		t.Fatal("exclude filter did not process an empty nested Model")
	}
}
```

- [ ] **Step 4: Update the five-format gate fixture to preserve outer semantics**

Keep the body `model` value as `body-decoy`. For matching ordinary requests use `RequestedModel: "target-model"` and `Model: "upstream-decoy"`; for nonmatching ordinary requests use `RequestedModel: "other"` and `Model: "target-model"`. This proves the ordinary filter remains independent of current execution `Model` and body machine fields.

- [ ] **Step 5: Run the focused tests and verify red**

Run:

```bash
go test ./... -run 'TestRequestFilter(ShouldProcess|GatesAllSupportedFormats)'
```

Expected: FAIL at the exact callback-source assertion because production still reads `RequestedModel`; ordinary table rows and five-format outer fixtures remain green. The first `t.Fatal` ends that test, so the callback assertion that excludes an OR with `RequestedModel` becomes observable after the positive callback case is implemented. If a failure is unrelated to these fixtures, record the exact output and isolate the test name before continuing.

- [ ] **Step 6: Commit the red test change**

```bash
git add request_filter_test.go
 git commit -m "test: require models filter to use effective model"
```

The commit may contain failing tests because it is the explicit TDD red step; do not amend the earlier documentation commit.

## Task 2: Make the minimal stage-aware production change

**Files:**
- Modify: `request_filter.go:257-298`

**Interfaces:**
- Consumes: unchanged `requestFilter.shouldProcess(*pluginapi.RequestInterceptRequest) bool` and host-owned `RequestInterceptRequest.Metadata["source"]`.
- Produces: model-only and combined filters evaluated against `RequestedModel` for outer calls, or `Model` for the exact nested callback source.

- [ ] **Step 1: Add the host callback source constant and subject selector**

Add the smallest local helpers beside the existing filter constants:

```go
const modelExecutionSourceKey = "source"
const modelExecutionCallbackSource = "plugin_host_model_callback"

func (f requestFilter) modelSubject(request *pluginapi.RequestInterceptRequest) string {
	if source, _ := request.Metadata[modelExecutionSourceKey].(string); source == modelExecutionCallbackSource {
		return request.Model
	}
	return request.RequestedModel
}
```

The selector must read exactly one host-owned source key and must not inspect body, URL, headers, other Metadata keys, plugin IDs, or a fallback model field. Keep the existing empty-string behavior in `matchesModel`.

- [ ] **Step 2: Use the selector in all three model branches**

In `shouldProcess`, calculate the subject once after the enabled fast path:

```go
model := f.modelSubject(request)
```

Use `model` in the `and` branch, the `or` branch, and the model-only default branch. Leave API-key matching and boolean logic unchanged.

- [ ] **Step 3: Run the focused tests and verify green**

Run:

```bash
go test ./... -run 'TestRequestFilter(ShouldProcess|GatesAllSupportedFormats)'
```

Expected: PASS, including outer/nested invocation tests, unknown-source tests, empty-model tests, and every existing API/model combination.

- [ ] **Step 4: Run the complete unit suite**

Run:

```bash
go test ./...
```

Expected: PASS. This checks configuration parsing, all transformations, ABI-facing helpers, concurrency snapshots, and the updated filter contract together.

- [ ] **Step 5: Commit the production fix**

```bash
git add request_filter.go request_filter_test.go
 git commit -m "fix: match callback filters against effective model"
```

## Task 3: Update public contract documentation and integration boundaries

**Files:**
- Modify: `README.md:82-84`, `RELEASE_NOTES.md:5-11`, `main_test.go:650-767`
- Inspect: `integration/http_test.go`, `integration/websocket_test.go`, `integration/harness_test.go`
- Modify integration files only if a test can observe the host-owned nested callback source distinctly through the pinned CPA harness.

**Interfaces:**
- Consumes: the stage-aware `requestFilter` behavior from Task 2 and current plugin registration/docs.
- Produces: documentation that accurately separates outer `RequestedModel` matching from nested callback `Model` matching, without claiming CPA alias rewriting is visible before censorship.

- [ ] **Step 1: Update README and release notes**

Replace the old statement that models always match `RequestedModel` with text stating:

```text
Outer request-interceptor calls match `RequestedModel`. When CPA invokes a nested execution through the host model callback, the request carries the host-owned `Metadata["source"]` value `plugin_host_model_callback`; only that nested call matches `Model`. The plugin does not parse request-body model fields or implement CPA alias routing.
```

Keep the existing API-key, glob, no-body-mutation, and filter-mode statements. Update the five-format source summary so it does not contradict the nested contract:

```text
All five supported `SourceFormat` values use the same stage-aware request-filter sources: `Metadata["caller_scope"]` for API keys, `RequestedModel` for ordinary model checks, `Model` only when `Metadata["source"]` equals `plugin_host_model_callback`, and the documented wildcard credential carriers.
```

- [ ] **Step 2: Update `main_test.go` documentation assertions**

Replace the old monolithic sentence token with stable contract fragments and do not add a duplicate standalone callback token:

```go
"Outer request-interceptor calls match `RequestedModel`.",
"host-owned `Metadata[\"source\"]` value `plugin_host_model_callback`",
"only that nested call matches `Model`.",
"does not parse request-body model fields or implement CPA alias routing.",
"The filter gate is evaluated independently for each host invocation;",
"the nested call's result does not undo an outer transform that has already completed.",
```

Update the five-format required tokens to require `same stage-aware request-filter sources`, ordinary `RequestedModel`, and callback-only `Model`, while retaining the existing wildcard carrier assertions.

Rename the release compatibility test to `TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions` and require both exact compatibility statements:

```go
for _, sentence := range []string{
	"v0.3.1 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.",
	"v0.3.0 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.",
} {
	if !bytes.Contains(raw, []byte(sentence)) {
		t.Fatalf("RELEASE_NOTES.md does not contain compatibility sentence %q", sentence)
	}
}
```

Retain the existing machine-field and ABI assertions.

- [ ] **Step 3: Check integration provenance**

Confirm whether the existing harness loads a model router/executor and exposes:

```text
outer: RequestedModel = client-a
nested: Metadata["source"] = plugin_host_model_callback, Model = upstream-b
body.model = body-decoy
```

The current harness baseline has only the censorship plugin and equal model `name`/`alias`; it cannot prove this condition. Do not add a fake model-mapper or infer nested values from CPA alias configuration. Leave integration source unchanged if the values are not observable, and record the host-level prerequisite in the SDD ledger.

- [ ] **Step 4: Run available integration verification

Run the repository's smallest applicable command:

```bash
make integration
```

If required `CPA_INTEGRATION_BIN` or `CENSORSHIP_PLUGIN_DIR` inputs are unavailable, run the integration package's compile/test check that does not start CPA and report the exact skipped prerequisite. Do not replace a missing host binary with a different CPA version.

## Task 4: Full verification and review gate

**Files:**
- Inspect all changed files and git diff; no additional production files expected.

- [ ] **Step 1: Run formatting and static checks**

Run:

```bash
gofmt -w request_filter.go request_filter_test.go
go vet ./...
```

Expected: no vet diagnostics and no unrelated formatting changes. Inspect `git diff --check` after formatting.

- [ ] **Step 2: Run the project verification commands**

Run, recording each result:

```bash
make build
make test
make race
make vet
```

`make package` is run only after the version bump in Task 5 because package metadata embeds the release version. `make integration` is rerun if Task 3 was applicable.

- [ ] **Step 3: Perform an Opus review of the final diff**

Review only the changed diff against the spec. Check specifically that:

- all three model branches use one stage-aware subject, with only exact `plugin_host_model_callback` selecting `request.Model`;
- outer calls continue to match `RequestedModel`, with no fallback from unknown/empty source to body or other machine fields;
- API key and transform behavior is unchanged;
- tests prove outer/nested distinct aliases, unknown source, empty nested Model, and body decoy behavior;
- documentation describes the host callback contract without claiming CPA alias visibility before censorship;
- no CLIProxyAPI core or generated artifact was modified.

Fix any confirmed issue with a focused test-first change, rerun affected checks, and record the final review result.

- [ ] **Step 4: Confirm the working tree scope**

Run:

```bash
git status --short
 git diff --stat main...HEAD
```

Expected: only the new spec/plan and the directly related code/tests are present, plus any explicitly required release metadata in Task 5.

## Task 5: Version bump, package, merge, and publish

**Files:**
- Inspect version and release files through the repository's existing Makefile/release conventions before editing.
- Modify only the version source and generated release outputs required by the existing package workflow; never hand-edit generated `.integration/` or `dist/` artifacts.

- [ ] **Step 1: Determine the next patch version from the current release**

Read the repository's current version source and tags. Bump the patch version from `v0.3.0` to `v0.3.1` unless an existing local or remote tag already occupies that version; if occupied, use the next unused patch version and apply it consistently through the existing version mechanism.

- [ ] **Step 2: Run package generation and checksum validation**

Run:

```bash
make package
```

Verify release packages have matching lowercase SHA-256 files and aggregate `checksums.txt` entries. Do not edit generated outputs by hand.

- [ ] **Step 3: Commit the release changes**

```bash
git add -A
git commit -m "release: v0.3.1"
```

- [ ] **Step 4: Merge the feature branch into local main**

After all verification succeeds, switch to local `main` and merge the feature branch with a non-fast-forward merge only if the repository's existing history requires it; otherwise use the repository's normal fast-forward-compatible merge. Confirm `main` contains the implementation and release commits.

- [ ] **Step 5: Push main and create the release tag**

Push the merged `main` branch, create the matching annotated tag using the repository's existing tag convention, and push the tag. This is an outward-facing action authorized by the task request and must happen only after verification passes.

- [ ] **Step 6: Verify GitHub workflow and release**

Inspect the pushed tag's GitHub Actions workflow and release state using the repository's configured tooling. Confirm the workflow reaches a terminal success state, artifacts/checksums exist, and the release version matches the tag. Report any remote failure with its job name and log summary instead of claiming completion.
