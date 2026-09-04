# Censorship Performance Round 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:test-driven-development` for every production change. Execution is orchestrated by one Workflow. Steps use checkbox syntax for tracking.

**Goal:** 在不改变插件行为合同和 wire bytes 的前提下，使用 TDD 实施本轮十二项已确认性能优化，并留下可复现的 benchmark、fuzz、integration 和 dynamic ABI 证据。

**Architecture:** 保持现有 request-only 分层。`selectors*.go` 只验证 JSON 并收集 span，`config.go` 只生成 immutable snapshot 的当前模式派生数据，`matcher.go` 提供按证据启用的 matcher，`transform.go` 保持 rule-major 变换并只重编码 changed spans，RPC 和 CGO 层只减少可证明多余的编码与复制。当前主工作区始终只有一个 writer；独立 review 在全部 GREEN 后并行执行。

**Tech Stack:** Go 1.26.0、CGO `-buildmode=c-shared`、CLIProxyAPI ABI v1 / RPC schema v4、`github.com/tidwall/gjson@v1.18.0`、`gopkg.in/yaml.v3@v3.0.1`、Go 标准库、GitHub Actions。

**Spec:** `docs/superpowers/specs/2026-09-01-censorship-plugin-design.md`

## Context

当前基线为 `main@be27107180cafac98a71f238e501e0de26674a20`，工作树已核对为 `main...origin/main` 且无改动。`v0.1.2` 已完成上一轮 folded block 和 dense folded rewrite 优化，本计划只处理后续仓库级扫描确认的新热点：重复 JSON 校验重读 body、part object 重复扫描、禁用 role 的无效 traversal、folded rewrite 全量 miss、changed-span 中间 replacement、success envelope 二次 marshal、AfterAuth 插件侧输入复制、无条件派生数据、folded matcher root map、exact rewrite 重复扫描、exact block 的 `O(R×B)` 路径，以及 folded long-prefix rewrite。

## Global Constraints

- CLIProxyAPI 依赖固定为 `github.com/router-for-me/CLIProxyAPI/v7 v7.2.147-0.20260831020448-81e1b5374f99`，不得修改上游源码或依赖版本。
- 插件只注册 `RequestInterceptor`。`InterceptRequestBeforeAuth` 审查请求；`InterceptRequestAfterAuth` 固定 no-op。
- 永远不读取、修改或观察 response body、SSE chunk 或 server WebSocket event。
- 词表唯一来源是 YAML `words`。不得增加内置词表、fallback、在线下载或 runtime cache。
- `block`、`strip`、`obfs`、`ignore_case`、scope、canonical role、hard exclusions、invalid JSON 和 duplicate fail-closed 行为保持不变。
- `block` 继续按 YAML rule 顺序优先，再按 eligible span document order 选择 role。
- `strip` 和 `obfs` 必须保留 `for rule -> for span` 的 rule-major 循环。后一条规则读取前一条规则产生的当前文本。
- 每条 rewrite 处理全部左到右非重叠 occurrence。`obfs` 只在实际 match 的第一个 Unicode scalar 后插入配置字符。
- `ignore_case: true` 只使用 `unicode.SimpleFold` 等价类。不得使用 full case folding 或 Unicode normalization。
- exact matching 逐 byte 保持 `strings.Contains` / `strings.ReplaceAll` 语义，包括 invalid UTF-8。
- changed body 只重编码 changed JSON string token。span 外 member order、空白、number spelling、unknown fields、tool 和 media bytes 必须保持不变。
- 相同 body、immutable snapshot 和 `pluginVersion` 必须得到相同字节结果。不要求幂等。
- `Metadata.ConfigFields` 当前五个可视化字段必须保留，不得按旧设计文档回退为空。
- 不增加依赖、YAML 字段、threshold 配置、interface、factory 或通用 root-wide selector walker。
- 每项生产变更必须先观察正确 RED，提交 RED checkpoint，再写最小 GREEN，运行 focused test 和 `go test ./...`，提交 GREEN checkpoint。
- 不使用 `git commit --amend`、`--no-verify`、rebase、force push 或跳过 hook。
- 全程禁用 windows-mcp 截图和鼠标工具。
- 本轮不修改 `RELEASE_NOTES.md`，不改版本，不创建 tag，不 push，不发布 GitHub Release。

## Scope and Exclusions

本轮实现以下十二项：

1. duplicate walker 复用现有 gjson root。
2. Gemini/Interactions part object 单次扫描。
3. OpenAI、Responses、Claude、Gemini、Interactions 的安全 role gate。
4. folded strip/obfs 的 per-span combined total-miss preflight。
5. forward `bytes.Buffer` + 单一 `json.Encoder` rebuild。
6. success envelope 单次 `json.Marshal`。
7. AfterAuth 在 ABI method dispatch 后跳过 `C.GoBytes`。
8. 按 `Mode` 和 `IgnoreCase` 编译 snapshot 派生数据。
9. folded matcher ASCII root table。
10. exact rewrite 删除冗余 `Contains`，并预编译 exact obfs replacement。
11. adaptive byte Aho-Corasick exact block。
12. adaptive KMP folded strip/obfs。

明确排除：窄化 BeforeAuth wire struct、inactive snapshot 在 RPC decode 前早退、`unsafe.String` 或 borrowed gjson view、其他 ABI input borrow、trie 压缩、非 ASCII obfs `Grow` 调整、跨规则 combined rewrite、whole-body remarshal、CLIProxyAPI 修改和自动发布。

## File Map and Ownership

| Writer | Production write surface | Test and evidence surface |
|---|---|---|
| Baseline writer | 无 production 改动 | durable plan copy、`benchmark_test.go` 的稳定入口基准、baseline 输出 |
| Selector writer | `selectors.go`、五个 `selectors_*.go` | selector tests、duplicate/selector 部分 `fuzz_test.go` |
| Rebuild writer | 只改 `transform.go` 的 `rebuildBody` | `transform_test.go`、rebuild 部分 `fuzz_test.go` |
| Envelope writer | `main.go` 的 `okEnvelope` | `main_test.go` |
| ABI writer | `abi_cgo.go` | 新增 `abi_cgo_test.go`、`integration/abi_benchmark_test.go`、`README.md` 对应限制 |
| Compiler/matcher writer | `config.go`、`matcher.go`、`transform.go` 的 `applyMode` 和 matcher dispatch | `config_test.go`、`matcher_test.go`、matcher 部分 `fuzz_test.go`、后续所有 `benchmark_test.go` 改动 |
| Evidence writer | 无 production 改动 | 新 TDD evidence 文档 |

所有 writer 在同一主工作区串行运行。后一个 writer 必须读取并保留前一个 writer 的 GREEN，不得回写不属于自己的区域。`benchmark_test.go` 在 baseline commit 后只允许 compiler/matcher writer修改。

## Frozen Internal Shape

保持现有类型名，按最小 diff 增加派生字段：

```go
type compiledRule struct {
    Term             string
    Runes            []rune // 仅 ignore_case 路径使用，保存 canonical fold keys
    ExactReplacement string // 仅 exact obfs 使用
    FoldFailure      []int  // 仅 adaptive folded rewrite 使用
}

type configSnapshot struct {
    Mode              mode
    IgnoreCase        bool
    Rules             []compiledRule
    Formats           scopeSet
    Roles             scopeSet
    ObfsChar          string
    BlockMatcher      *foldMatcher // folded block 或 folded rewrite preflight
    ExactBlockMatcher *byteMatcher
}
```

新增的最小 production 接口：

```go
func compileSnapshot(cfg *configSnapshot) error
func scanTextPart(part gjson.Result, requireTextType bool) (gjson.Result, bool)
func canonicalJSONMemberName(key gjson.Result) (string, bool)
func shouldCopyPluginRequest(method string) bool
func useFoldRewritePreflight(ruleCount, textBytes int) bool
func useExactByteMatcher(ruleCount, totalTextBytes int) bool
func useFoldedKMP(textBytes, patternScalars int) bool
```

不得为测试增加 runtime instrumentation。forced matcher constructors 和 test-only legacy/oracle helpers放在 `_test.go`，生产接口不暴露配置开关。

## Workflow Phase DAG

```text
Baseline
  -> Selector lane: duplicate -> part scanner -> role gates
  -> Rebuild lane
  -> Envelope lane
  -> ABI lane
  -> Compiler/matcher lane:
       mode compiler -> ASCII root -> exact rewrite
       -> folded preflight -> exact byte AC -> folded KMP
  -> parallel read-only reviews
  -> verified remediation when needed
  -> final verification and evidence
```

主工作区 writer 必须按上图串行。只有全部 GREEN 后的只读 review 使用 `parallel()`。

---

### Task 0: Persist the Approved Plan and Capture Baselines

**Files:**
- Create: `docs/superpowers/plans/2026-09-04-censorship-performance-round-2.md`
- Modify: `benchmark_test.go`
- Modify: `integration/abi_benchmark_test.go`

**Interfaces:**
- Consumes: 当前稳定的 `transformRequest`、`selectTextSpans`、`applyMode`、`rebuildBody`、`okEnvelope` 和 dynamic ABI harness。
- Produces: 后续候选共用的稳定 benchmark names、test-only baseline helpers 和 `$env:TEMP` 中的 baseline raw output。

- [ ] **Step 1: Copy this approved plan verbatim into the durable plan path**

不得改动旧计划和旧 TDD evidence。

- [ ] **Step 2: Add layered benchmarks without production changes**

新增或扩展以下 benchmark，fixtures 全部在 timer 外构造，oracle 在 `ResetTimer` 前执行：

```text
BenchmarkDuplicateValidation
BenchmarkTextPartScanning
BenchmarkDisabledRoleSelectors
BenchmarkRebuildChangedSpans
BenchmarkSuccessEnvelope
BenchmarkExactRewriteStrategies
BenchmarkFoldRootTransitions
BenchmarkRewritePreflightStrategies
BenchmarkExactBlockStrategies
BenchmarkFoldedRewriteStrategies
```

Strategy benchmark 名称必须包含 `impl`、`mode`、`rules`、`text` 或 `body`、`pattern`、`match`、`set=calibration|holdout`。baseline helper 只存在于 `_test.go`。

`integration/abi_benchmark_test.go` 必须把 `bytes.Contains`、`bytes.Equal` 和完整 response oracle 移到计时区外。计时循环只调用 interceptor，并把 response 写入 package-level sink，避免 benchmark 测量整份响应扫描。

- [ ] **Step 3: Verify the benchmark harness**

Run:

```powershell
go test ./...
go test . -run '^$' -bench '^(BenchmarkDuplicateValidation|BenchmarkTextPartScanning|BenchmarkDisabledRoleSelectors|BenchmarkRebuildChangedSpans|BenchmarkSuccessEnvelope|BenchmarkExactRewriteStrategies|BenchmarkFoldRootTransitions|BenchmarkRewritePreflightStrategies|BenchmarkExactBlockStrategies|BenchmarkFoldedRewriteStrategies)$' -benchmem -count=1
```

Expected: tests and all benchmark smoke runs exit 0。每个 benchmark 在 timer 前验证结果，不能只消耗返回值。

- [ ] **Step 4: Capture the repeated baseline**

Run:

```powershell
go test . -run '^$' -bench '^(BenchmarkDuplicateValidation|BenchmarkTextPartScanning|BenchmarkDisabledRoleSelectors|BenchmarkRebuildChangedSpans|BenchmarkSuccessEnvelope|BenchmarkExactRewriteStrategies|BenchmarkFoldRootTransitions|BenchmarkRewritePreflightStrategies|BenchmarkExactBlockStrategies|BenchmarkFoldedRewriteStrategies)$' -benchmem -count=10 | Tee-Object "$env:TEMP\censorship-round2-baseline.txt"
```

记录 `go version`、`go env GOOS GOARCH` 和 CPU 型号。原始临时文件不提交。

- [ ] **Step 5: Commit the plan and benchmark harness separately**

```powershell
git add docs/superpowers/plans/2026-09-04-censorship-performance-round-2.md
git commit -m "docs: add performance round two plan"
git add benchmark_test.go integration/abi_benchmark_test.go
git commit -m "test: benchmark remaining request path hotspots"
```

Expected: `git status --short` 为空。

---

### Task 1: Reuse the Parsed Root for Duplicate Detection

**Files:**
- Modify: `selectors.go`
- Modify: `main_test.go`
- Modify: `fuzz_test.go`

**Interfaces:**
- Consumes: `gjson.ValidBytes` 和 `gjson.ParseBytes` 已产生的 root。
- Produces: `hasDuplicateJSONMembers(root gjson.Result) bool`。

- [ ] **Step 1: Write the RED tests and independent fuzz target**

新增：

```text
TestDuplicateMemberWalkerAcceptsParsedRoot
TestDuplicateMemberWalkerCanonicalizesMemberNames
TestDuplicateMemberWalkerChecksExcludedSubtrees
FuzzDuplicateWalkerAgainstOracle
```

固定 key cases：

```text
duplicate: member name x 与 member-name bytes U+005C + u0078
duplicate: U+005C + ud800 + U+005C + u0061 与 U+005C + ufffda
non-duplicate: U+005C + ud800 + U+005C + u0061 与 U+005C + ufffd
```

第二、三项的第一个 member name必须包含两个连续 JSON escapes。lone high surrogate 后必须紧接 U+005C + `u0061`，不能改成 literal `a`。另覆盖 sibling objects、array nesting、JSON-looking strings 和 `tools`/media excluded subtree 中的 duplicate。fuzz oracle 复用现有独立 `oracleHasDuplicateJSONMembers`，不得调用 production walker。

- [ ] **Step 2: Run and commit RED**

Run:

```powershell
go test . -run '^(TestDuplicateMemberWalkerAcceptsParsedRoot|TestDuplicateMemberWalkerCanonicalizesMemberNames|TestDuplicateMemberWalkerChecksExcludedSubtrees)$' -count=1
```

Expected: compile RED，原因是当前 `hasDuplicateJSONMembers` 接收 `[]byte`，测试传入 `gjson.Result`。

```powershell
git add selectors.go main_test.go fuzz_test.go
git commit -m "test: specify parsed-root duplicate detection"
```

该 RED commit 只能包含测试调用和 oracle/seed，不得先改 production signature。

- [ ] **Step 3: Implement the minimal root walker**

实现结构：

```go
func canonicalJSONMemberName(key gjson.Result) (string, bool) {
    if !strings.ContainsRune(key.Raw, '\\') && utf8.ValidString(key.Str) {
        return key.Str, true
    }
    var decoded string
    if err := json.Unmarshal([]byte(key.Raw), &decoded); err != nil {
        return "", false
    }
    return decoded, true
}
```

`hasDuplicateJSONMembers` 使用 `Result.ForEach`。每个 object 新建自己的 `map[string]struct{}`；array 递归 values；scalar 不读取 `Str`。canonicalization 失败返回 true，保持 fail-closed。不得使用 `Result.Map()`。`selectTextSpans` 继续先做 `gjson.ValidBytes` 和 root object check，然后把已解析 root 传给 walker。

- [ ] **Step 4: Verify GREEN and commit**

Run:

```powershell
go test . -run '^(TestBeforeAuthRejectsDuplicateJSONMembers|TestDuplicateJSONMembersInsideStringRemainOpaque|TestDuplicateMemberWalker.*)$' -count=1
go test . -run '^$' -fuzz '^FuzzDuplicateWalkerAgainstOracle$' -fuzztime=10s
go test ./...
```

```powershell
git add selectors.go main_test.go fuzz_test.go
git commit -m "perf: reuse parsed JSON for duplicate detection"
```

---

### Task 2: Scan Gemini and Interactions Text Parts Once

**Files:**
- Modify: `selectors.go`
- Modify: `selectors_gemini.go`
- Modify: `selectors_interactions.go`
- Modify: `selectors_gemini_test.go`
- Modify: `selectors_interactions_test.go`

**Interfaces:**
- Produces: `scanTextPart(part gjson.Result, requireTextType bool) (text gjson.Result, allowed bool)`。

- [ ] **Step 1: Write RED tests**

新增：

```text
TestScanTextPartPreservesResultIndex
TestScanTextPartTraversesWholeObjectBeforeDecision
TestScanTextPartPreservesProtocolRules
```

表格必须覆盖：text 在 exclusion 前后、missing/empty/`text`/invalid type、`thought:true` 与 false、machine key 值为 null、snake/camel machine keys、`extra_content.google.thought_signature`、普通 `extra_content`。返回的 `gjson.Result` 必须保留原始 `Raw`、`Str` 和 `Index`。

- [ ] **Step 2: Run and commit RED**

```powershell
go test . -run '^TestScanTextPart' -count=1
git add selectors_gemini_test.go selectors_interactions_test.go
git commit -m "test: specify single-pass text part scanning"
```

Expected: compile RED，`undefined: scanTextPart`。

- [ ] **Step 3: Implement the single outer ForEach**

helper 先拒绝非-object。一次 outer `ForEach` 同时记录 text result、type、`thought` 和所有 machine key presence；即使已经判定不允许，也继续扫描到 object 末尾。machine key 集合保持现有 camel/snake 全集。只对 `extra_content` value 进行一次受限 nested `google.thought_signature` existence 查询。

Gemini 调用 `scanTextPart(part, false)`。Interactions 调用 `scanTextPart(part, true)`。Interactions 只允许 missing type、空 string 或 `"text"`。调用者直接把返回的 text result 交给 `appendStringSpan`，不得再 `part.Get("text")`。

- [ ] **Step 4: Verify GREEN and commit**

```powershell
go test . -run '^(TestScanTextPart|TestGemini.*|TestInteractions.*)$' -count=1
go test ./...
git add selectors.go selectors_gemini.go selectors_interactions.go selectors_gemini_test.go selectors_interactions_test.go
git commit -m "perf: scan Gemini text parts once"
```

---

### Task 3: Gate Disabled Selector Roles Safely

**Files:**
- Modify: `selectors.go`
- Modify: `selectors_openai.go`
- Modify: `selectors_claude.go`
- Modify: `selectors_gemini.go`
- Modify: `selectors_interactions.go`
- Modify: corresponding selector tests

**Interfaces:**
- Produces: `selectorHasEnabledRole(sourceFormat string, roles scopeSet) bool` 和 protocol-local early gates。

- [ ] **Step 1: Write behavior and allocation RED**

新增：

```text
TestSelectorHasEnabledRoleByFormat
TestSelectorRoleGatesPreserveCanonicalOverrides
TestSelectorRoleGateRunsAfterGlobalValidation
TestDisabledSelectorRoleTraversalAllocationCeiling
```

必须固定以下例外：

- Responses 的 source item role 未启用时，只要 assistant 启用，仍扫描 `output_text` 和 `refusal`。
- Claude user 未启用但 tool 启用时，仍扫描 typed `tool_result.content[]` text。
- Gemini role disabled 时仍更新 `previousRole`；invalid role 仍推进 alternation。
- Interactions 当前 inherited role disabled 时仍递归 `steps`，child 可用 `role:"assistant"` 或 `type:"model_output"` 切换。
- 非空但不适用于格式的 role set 只能在完整 JSON 和 duplicate validation 后早退。
- 显式空 `roles` 仍保留既有的 parse-before no-op。

allocation test 对预解析 root 直接调用 collectors。每个 fixture 放置足够多的 escaped disabled text，使当前延迟 role filtering 超过每次 32 allocations；新上限固定为不超过 32。fixture、root、roles 和 sink 在 closure 外构造，不使用 wall-clock assertion。

- [ ] **Step 2: Run and commit RED**

```powershell
go test . -run '^(TestSelectorHasEnabledRoleByFormat|TestSelectorRoleGatesPreserveCanonicalOverrides|TestSelectorRoleGateRunsAfterGlobalValidation|TestDisabledSelectorRoleTraversalAllocationCeiling)$' -count=1
```

Expected: 首先因缺少 `selectorHasEnabledRole` compile fail。加入测试所需声明后，旧 traversal 必须继续因 allocation ceiling 失败。

```powershell
git add selectors.go selectors_*_test.go
git commit -m "test: specify safe selector role gates"
```

- [ ] **Step 3: Implement protocol-specific gates**

`selectorHasEnabledRole` 的格式映射：

```text
openai: system, developer, user, assistant, tool
openai-response: system, developer, user, assistant
claude: system, user, assistant, tool
gemini: system, user, assistant
interactions: system, user, assistant
```

具体 gate：

- OpenAI Chat 在解析 role 后，disabled role 不读取 content。
- Responses 分别 gate top-level instructions/input。item source role disabled 但 assistant enabled 时仍读取 content。
- Claude system disabled 时跳过 system content；user message 只有 user 和 tool 都 disabled 时才能跳过 content。
- Gemini 始终先更新 alternation，再按 effective role 决定是否读取 parts。
- Interactions 只跳过当前 item 的 content/parts，始终保留 `steps` 递归。

- [ ] **Step 4: Verify GREEN and commit**

```powershell
go test . -run '^(TestSelector.*Role|TestDisabledSelectorRoleTraversalAllocationCeiling|TestOpenAIResponsesAssistantOutputShapesUseAssistantScope|TestClaudeTopLevelSystemStringAndToolScope|TestGemini.*Role.*|TestInteractions.*Role.*|TestBeforeAuthRejectsDuplicateJSONMembers)$' -count=1
go test ./...
git add selectors.go selectors_openai.go selectors_claude.go selectors_gemini.go selectors_interactions.go selectors_*_test.go
git commit -m "perf: gate selector traversal by role"
```

---

### Task 4: Rebuild Changed JSON Spans Forward

**Files:**
- Modify: `transform.go`
- Modify: `transform_test.go`
- Modify: `fuzz_test.go`

**Interfaces:**
- Keeps: `rebuildBody(body []byte, spans []textSpan) ([]byte, error)`。

- [ ] **Step 1: Write RED tests and independent oracle fuzz**

新增：

```text
TestRebuildBodyValidatesBeforeWriting
TestRebuildBodyUsesEncoderDefaultEscaping
TestRebuildBodyAllocationCeiling
FuzzRebuildBodyAgainstMarshalOracle
```

allocation fixture 含 64 个 changed spans，fixture 和 spans 在 closure 外构造，上限为每次 8 allocations。当前 per-span `json.Marshal` 必须超限。fuzz oracle 在 `_test.go` 独立保留当前 per-span Marshal + backward-copy reference；比较 exact bytes 和 error class。

- [ ] **Step 2: Run and commit RED**

```powershell
go test . -run '^(TestRebuildBodyValidatesBeforeWriting|TestRebuildBodyUsesEncoderDefaultEscaping|TestRebuildBodyAllocationCeiling)$' -count=1
```

Expected: validation 和 escaping characterization 可以通过；allocation test 必须失败且值大于 8。

```powershell
git add transform_test.go fuzz_test.go
git commit -m "test: specify forward body rebuilding"
```

- [ ] **Step 3: Implement the forward encoder**

第一遍验证全部 changed spans 的 range、升序和不重叠。没有 changed span 返回 nil。验证完成后才创建一个 `bytes.Buffer` 和一个 `json.Encoder`。

预分配只使用安全下界：

```text
span-external body bytes + 每个 changed JSON string 的两个 quote bytes
```

不得无条件 `Grow(len(body))`。用 checked arithmetic 计算下界；overflow 返回 `errInvalidSpan`。从左向右写 span 外原始 bytes。每个 changed span 调用同一个 `encoder.Encode(span.Text)`，确认最后一个物理 byte 是 newline，然后只 `Truncate(Len()-1)`。不得 `TrimSpace`，不得调用 `SetEscapeHTML(false)`，不得重新 marshal root，最后直接返回 `buffer.Bytes()`。

- [ ] **Step 4: Verify GREEN and commit**

```powershell
go test . -run '^(TestRebuildBody|TestRebuildMatchesDecodedEscapesAndUsesEncodingJSONEscaping|TestTransformIsByteDeterministic)$' -count=1
go test . -run '^$' -fuzz '^FuzzRebuildBodyAgainstMarshalOracle$' -fuzztime=10s
go test ./...
git add transform.go transform_test.go fuzz_test.go
git commit -m "perf: rebuild changed JSON spans forward"
```

---

### Task 5: Marshal Successful RPC Envelopes Once

**Files:**
- Modify: `main.go`
- Modify: `main_test.go`

- [ ] **Step 1: Write exact-wire and allocation RED**

新增 `TestOKEnvelopeExactBytesAndAllocationCeiling`。固定以下 bytes：

```json
{"ok":true,"result":{"value":"x"}}
{"ok":true,"result":null}
```

再用包含 1 MiB `Body` 的 `pluginapi.RequestInterceptResponse` 执行 `testing.AllocsPerRun`，上限为每次 4 allocations。当前 double marshal 的已测基线高于该值。

- [ ] **Step 2: Run and commit RED**

```powershell
go test . -run '^TestOKEnvelopeExactBytesAndAllocationCeiling$' -count=1
git add main_test.go
git commit -m "test: pin success envelope encoding"
```

Expected: exact bytes 通过，allocation ceiling 失败。

- [ ] **Step 3: Implement one Marshal**

```go
type successEnvelope struct {
    OK     bool `json:"ok"`
    Result any  `json:"result"`
}

func okEnvelope(value any) ([]byte, error) {
    return json.Marshal(successEnvelope{OK: true, Result: value})
}
```

`Result` 不得使用 `omitempty`。字段顺序保持 `ok`、`result`。不得修改 `errorEnvelope`。

- [ ] **Step 4: Verify GREEN and commit**

```powershell
go test . -run '^(TestOKEnvelopeExactBytesAndAllocationCeiling|TestRegistrationDeclaresOnlyRequestInterceptor|TestAfterAuthAlwaysNoOpsWithoutParsingRequest)$' -count=1
go test ./...
git add main.go main_test.go
git commit -m "perf: marshal success envelopes once"
```

---

### Task 6: Skip the AfterAuth ABI Input Copy

**Files:**
- Modify: `abi_cgo.go`
- Create: `abi_cgo_test.go`
- Modify: `README.md`

**Interfaces:**
- Produces: `shouldCopyPluginRequest(method string) bool`。

- [ ] **Step 1: Write the copy-policy RED**

新增带 `//go:build cgo` 的 `TestShouldCopyPluginRequest`。AfterAuth 必须返回 false；BeforeAuth、register、reconfigure 和 unknown method 必须返回 true。保留现有 empty pointer/length behavior 的 characterization。

- [ ] **Step 2: Run and commit RED**

```powershell
go test . -run '^TestShouldCopyPluginRequest$' -count=1
git add abi_cgo_test.go
git commit -m "test: specify AfterAuth ABI copy policy"
```

Expected: compile RED，`undefined: shouldCopyPluginRequest`。

- [ ] **Step 3: Dispatch by method before copying**

`cliproxyPluginCall` 先执行一次 `methodName := C.GoString(method)`。只有 `shouldCopyPluginRequest(methodName)`、request 非 nil 且 length 大于零时才调用 `C.GoBytes`。随后把同一个 `methodName` 和 request bytes 传给 `handleMethod`。

不得改变 response C ownership、`C.malloc`、output copy、`free_buffer`、host callback 或其他 method 的 input copy。不得扩展为 borrowed Go slice。

README 的既有限制改为：host 仍为每次 AfterAuth 编码并携带 full body，但插件在识别固定 no-op method 后不再执行插件侧 C-to-Go input copy。

- [ ] **Step 4: Verify GREEN and commit**

```powershell
go test . -run '^(TestShouldCopyPluginRequest|TestAfterAuthAlwaysNoOpsWithoutParsingRequest|TestDocumentationListsConfigAndLimits)$' -count=1
go test ./...
make build VERSION=0.0.0-dev
git add abi_cgo.go abi_cgo_test.go README.md
git commit -m "perf: skip AfterAuth ABI request copies"
```

---

### Task 7: Compile Only Mode-Specific Snapshot Data

**Files:**
- Modify: `config.go`
- Modify: `config_test.go`
- Modify: `matcher.go`
- Modify: `matcher_test.go`
- Modify: `fuzz_test.go`
- Modify: `benchmark_test.go`

**Interfaces:**
- Produces: `compileSnapshot(cfg *configSnapshot) error` 和前述派生字段。

- [ ] **Step 1: Write the compiler RED**

新增：

```text
TestCompileSnapshotBuildsOnlyActiveDerivedData
TestCompileSnapshotValidatesObfsByRuneCount
TestSyntheticSnapshotsUseProductionCompiler
```

矩阵保证：

- exact block/strip 不创建 `Runes`、`BlockMatcher`、`FoldFailure` 或 `ExactReplacement`。
- exact obfs 只创建 `ExactReplacement`。
- folded block 创建 canonical `Runes` 和 `BlockMatcher`，不创建 `FoldFailure`。
- folded strip/obfs 为全部规则创建 canonical `Runes`；只有 rule count 至少进入 Task 10 固定 calibration grid 的最低候选值 8 时才构建 preflight `BlockMatcher`，Task 10 选定阈值后再把构建边界收紧到实测值；KMP failure 留给 Task 12 的 measured region。
- fuzz 和 benchmark snapshots 先构造 term-only rules，再调用 production compiler。

obfs scalar validation 用 `utf8.RuneCountInString`，不依赖预先生成的 rune slice。

- [ ] **Step 2: Run and commit RED**

```powershell
go test . -run '^(TestCompileSnapshot|TestSyntheticSnapshotsUseProductionCompiler|TestSnapshotForFuzz.*)$' -count=1
git add config_test.go matcher_test.go fuzz_test.go benchmark_test.go
git commit -m "test: specify mode-specific snapshot compilation"
```

Expected: compile RED，缺少 `compileSnapshot` 和派生字段。

- [ ] **Step 3: Implement parse, validate, then compile**

YAML `words` parse 阶段只保存 `Term`。完成所有字段解析和 obfs validation 后调用一次 `compileSnapshot`。folded `Runes` 保存 `foldClassRune` canonical keys；matcher 比较 source rune 时也转换成同一 key。folded rewrite 的 `BlockMatcher` 只在 rule count 至少为 8 时作为候选 4 的可用派生数据构建，Task 10 选定阈值后同步收紧这个边界。所有成功 snapshot 在 atomic store 前完成编译，之后保持 immutable。

- [ ] **Step 4: Verify GREEN and commit**

```powershell
go test . -run '^(TestCompileSnapshot|TestParseConfig|TestSnapshotForFuzz|TestFoldMatcher)' -count=1
go test ./...
git add config.go config_test.go matcher.go matcher_test.go fuzz_test.go benchmark_test.go
git commit -m "perf: compile only active matcher data"
```

---

### Task 8: Add an ASCII Root Table to the Fold Matcher

**Files:**
- Modify: `matcher.go`
- Modify: `matcher_test.go`
- Modify: `benchmark_test.go`

- [ ] **Step 1: Write root-transition RED**

新增：

```text
TestFoldMatcherASCIITransitionsSurviveClearedRootMap
TestFoldMatcherFailureToRootUsesASCIITable
```

测试直接要求 `foldMatcher.asciiRoot [utf8.RuneSelf]int`。构造 matcher 后清空 root sparse map，ASCII direct match 和 failure 回 root 后的 transition 仍须得到 rule-major oracle 结果。

- [ ] **Step 2: Run and commit RED**

```powershell
go test . -run '^(TestFoldMatcherASCIITransitionsSurviveClearedRootMap|TestFoldMatcherFailureToRootUsesASCIITable)$' -count=1
git add matcher_test.go
git commit -m "test: require ASCII fold root transitions"
```

Expected: compile RED，`foldMatcher` 无 `asciiRoot`。

- [ ] **Step 3: Implement one root transition helper**

0 表示无 child，真实 child index 从 1 开始。root ASCII 使用 table；root 非 ASCII 和全部 non-root 节点继续使用 sparse map。trie insertion、failure construction、runtime match 以及 failure chain 回 root 后的 retry 必须调用同一 transition helper。不得增加 non-root dense table。

- [ ] **Step 4: Verify GREEN and commit**

```powershell
go test . -run '^TestFoldMatcher' -count=1
go test ./...
go test . -run '^$' -bench '^BenchmarkFoldRootTransitions$' -benchmem -count=10
git add matcher.go matcher_test.go benchmark_test.go
git commit -m "perf: index ASCII fold root transitions"
```

Benchmark holdout 回退不得超过 5%。

---

### Task 9: Remove Exact Rewrite Double Scans

**Files:**
- Modify: `config.go`
- Modify: `matcher.go`
- Modify: `config_test.go`
- Modify: `matcher_test.go`
- Modify: `benchmark_test.go`

- [ ] **Step 1: Write exact rewrite RED**

新增：

```text
TestCompileSnapshotPrecomputesExactReplacement
TestRewriteExactReportsMatchesByLength
TestExactRewriteMissAllocatesNothing
TestExactObfuscationHitAllocationCeiling
```

新增 helper 的目标签名：

```go
func rewriteExact(text string, rule compiledRule, obfuscate bool) (string, bool)
```

exact obfs hit 的上限为一次 allocation；exact strip/obfs miss 为零 allocation。多字节首 scalar 和 invalid UTF-8 term 都要覆盖。

- [ ] **Step 2: Run and commit RED**

```powershell
go test . -run '^(TestCompileSnapshotPrecomputesExactReplacement|TestRewriteExactReportsMatchesByLength|TestExactRewriteMissAllocatesNothing|TestExactObfuscationHitAllocationCeiling)$' -count=1
git add config_test.go matcher_test.go
git commit -m "test: specify single-scan exact rewrites"
```

Expected: compile RED，缺少 `rewriteExact` 或 `ExactReplacement`。

- [ ] **Step 3: Implement one ReplaceAll call**

`compileSnapshot` 只为 exact obfs 预计算实际 term 首 scalar bytes + `ObfsChar` + remainder。`rewriteExact` 直接调用一次 `strings.ReplaceAll`。strip 以输出长度减少判断 matched；obfs 以输出长度增加判断 matched。不得使用 `next != text`，不得保留前置 `strings.Contains`。folded path 不读取 `ExactReplacement`。

- [ ] **Step 4: Verify GREEN and commit**

```powershell
go test . -run '^(TestCompileSnapshotPrecomputesExactReplacement|TestRewriteExact|TestExact|TestStripUsesLeftmostNonOverlappingOccurrences|TestObfsPreservesMatchedCaseAndInsertsOncePerOccurrence)$' -count=1
go test ./...
go test . -run '^$' -bench '^BenchmarkExactRewriteStrategies$' -benchmem -count=10
git add config.go config_test.go matcher.go matcher_test.go benchmark_test.go
git commit -m "perf: rewrite exact matches with one scan"
```

---

### Task 10: Skip Folded Rewrite Spans with No Initial Match

**Files:**
- Modify: `transform.go`
- Modify: `transform_test.go`
- Modify: `config.go`
- Modify: `benchmark_test.go`

**Interfaces:**
- Adds: `textSpan.SkipFoldRewrite bool` 和 `useFoldRewritePreflight`。

- [ ] **Step 1: Write folded-only preflight RED**

新增：

```text
TestFoldRewritePreflightMarksOnlyTotalMisses
TestFoldRewritePreflightPreservesRuleMajorCascade
TestFoldRewritePreflightKeepsSpansIsolated
```

关键 cascade：

```text
mode=strip, ignore_case=true, words=[X, ab], text=aXb, result=""
```

初始 combined scan 只证明该 span 是否完全没有任何 rule match。任一初始 hit 后，所有 rules 必须继续运行。exact rewrite 不走这个 preflight。

- [ ] **Step 2: Run and commit RED**

```powershell
go test . -run '^(TestFoldRewritePreflightMarksOnlyTotalMisses|TestFoldRewritePreflightPreservesRuleMajorCascade|TestFoldRewritePreflightKeepsSpansIsolated)$' -count=1
git add transform_test.go
git commit -m "test: specify folded rewrite miss preflight"
```

Expected: compile RED，缺少 `SkipFoldRewrite` 或 preflight helper。

- [ ] **Step 3: Benchmark and implement without changing loop order**

在 `{8,32,128}` rules 与 `{4 KiB,16 KiB,64 KiB}` text boundary grid 上选择 `useFoldRewritePreflight` 的最低连续获益区域。启用时先逐 span 调用 compiled `BlockMatcher.match`；total miss 设置 `SkipFoldRewrite=true`。随后必须保留：

```go
for _, rule := range cfg.Rules {
    for i := range spans {
        if spans[i].SkipFoldRewrite {
            continue
        }
        // existing stripRule/obfuscateRule on current span text
    }
}
```

不得改为 span-major，不得按 preflight 返回的 rule index 做 pruning。

- [ ] **Step 4: Apply the benchmark gate**

Run:

```powershell
go test . -run '^$' -bench '^BenchmarkRewritePreflightStrategies$' -benchmem -count=10 | Tee-Object "$env:TEMP\censorship-round2-preflight.txt"
```

目标 total-miss 区域至少改善 15%，benchstat interval 不跨零。initial-hit、cascade 和 dense holdout 回退不得超过 5%，allocations 不增加。未满足时停止本候选，不提交猜测 threshold。

- [ ] **Step 5: Verify GREEN and commit**

```powershell
go test . -run '^(TestFoldRewritePreflight|TestStripOrderedCascadeAndFoldedOccurrences|TestObfsPreservesMatchedCaseAndInsertsOncePerOccurrence)$' -count=1
go test ./...
git add config.go transform.go transform_test.go benchmark_test.go
git commit -m "perf: skip folded rewrite total misses"
```

---

### Task 11: Add Adaptive Byte Aho-Corasick for Exact Block

**Files:**
- Modify: `matcher.go`
- Modify: `config.go`
- Modify: `transform.go`
- Modify: `matcher_test.go`
- Modify: `fuzz_test.go`
- Modify: `benchmark_test.go`

**Interfaces:**
- Produces: `byteMatcher`、`newByteMatcher(rules []compiledRule, ruleOffset int)`、`useExactByteMatcher` 和 `ExactBlockMatcher`。

- [ ] **Step 1: Write forced matcher RED and independent fuzz**

新增：

```text
TestByteMatcherPreservesExactBlockOrder
TestByteMatcherDoesNotCrossSpans
TestByteMatcherMatchesInvalidUTF8ByByte
TestAdaptiveExactBlockBoundary
FuzzByteMatcherAgainstOrderedContains
```

forced tests 覆盖 lowest YAML index、同一 rule earliest role、prefix/suffix/failure、duplicate rules、overlap、NUL、invalid UTF-8 和 no-match。fuzz oracle 只用 rule-major `strings.Contains`，不得调用 production byte matcher。

- [ ] **Step 2: Run and commit RED**

```powershell
go test . -run '^(TestByteMatcher|TestAdaptiveExactBlockBoundary)$' -count=1
git add matcher_test.go fuzz_test.go
git commit -m "test: specify adaptive exact block matching"
```

Expected: compile RED，缺少 byte matcher 或 adaptive helper。

- [ ] **Step 3: Implement byte trie and adaptive dispatch**

`byteMatcherNode` 使用 non-root sparse `map[byte]int`、failure 和最低 global YAML index；root 使用 `[256]int`。逐 byte 扫描 string，不使用 rune range。每个 span 从 state 0 开始。

保留一个小的 stdlib prefix。adaptive path 先按 YAML 顺序对 prefix rules 执行 `strings.Contains`；prefix miss 后用 byte AC 扫描 tail rules。跨 spans 保留最低 rule index，出现相同最低 index 时保留最早 span role。exact matcher 不用于 folded block。

- [ ] **Step 4: Select thresholds from a fixed grid**

Calibration grid：

```text
minimum rules: 32, 64, 128, 256
minimum total selected text bytes: 4 KiB, 16 KiB, 64 KiB
stdlib prefix rules: 0, 4, 8, 16
```

Run:

```powershell
go test . -run '^$' -bench '^BenchmarkExactBlockStrategies$' -benchmem -count=10 | Tee-Object "$env:TEMP\censorship-round2-exact-block.txt"
```

选择满足以下条件的最低连续区域：adaptive median 至少改善 15%，interval 不跨零，allocations 不增加；small rules/body、rule0 early、middle hit holdout 回退不超过 5%。阈值是 package-private constants，不增加 YAML 配置。

- [ ] **Step 5: Verify GREEN and commit**

```powershell
go test . -run '^(TestByteMatcher|TestAdaptiveExactBlock|TestOpenAIBlockUsesRuleOrderBeforeDocumentOrder|TestOpenAIBlockDocumentOrderAndErrorEscaping)$' -count=1
go test . -run '^$' -fuzz '^FuzzByteMatcherAgainstOrderedContains$' -fuzztime=10s
go test ./...
git add matcher.go config.go transform.go matcher_test.go fuzz_test.go benchmark_test.go
git commit -m "perf: add adaptive byte matching for exact block"
```

---

### Task 12: Add Adaptive KMP for Folded Rewrites

**Files:**
- Modify: `config.go`
- Modify: `matcher.go`
- Modify: `matcher_test.go`
- Modify: `fuzz_test.go`
- Modify: `benchmark_test.go`

**Interfaces:**
- Produces: `FoldFailure`、`rewriteFoldedKMP` 和 `useFoldedKMP`。

- [ ] **Step 1: Write forced KMP RED and independent fuzz**

新增：

```text
TestCompileSnapshotBuildsKMPOnlyForFoldedRewrite
TestFoldKMPResetsAfterNonOverlappingMatch
TestFoldKMPPreservesSourceByteSpans
TestAdaptiveFoldKMPBoundary
FuzzFoldKMPAgainstOracle
```

固定 `aaa` / `aa` strip 得 `a`，证明 full match 后 state 重置为 0。source-span tests 覆盖 Sigma、Kelvin、多字节 source case 和 invalid UTF-8。fuzz expected 使用现有独立 `oracleStrip` / `oracleObfuscate`，不得调用 naive production rewrite 作为 oracle。

- [ ] **Step 2: Run and commit RED**

```powershell
go test . -run '^(TestCompileSnapshotBuildsKMPOnlyForFoldedRewrite|TestFoldKMP|TestAdaptiveFoldKMPBoundary)$' -count=1
git add config_test.go matcher_test.go fuzz_test.go
git commit -m "test: specify adaptive folded KMP rewrites"
```

Expected: compile RED，缺少 `FoldFailure`、`rewriteFoldedKMP` 或 adaptive helper。

- [ ] **Step 3: Implement canonical KMP**

pattern 使用 Task 7 的 canonical `Runes`。failure table 只为 folded strip/obfs 的可能获益 pattern 编译。source 使用 `utf8.DecodeRuneInString`，每个 rune 转为 `foldClassRune` key。full match end 后，用固定 pattern scalar count 次 `utf8.DecodeLastRuneInString` 得到 source byte start，不保存 occurrence list 或 offset ring。

strip 跳过实际 source bytes。obfs 保留实际 source match，并在其首 scalar 后插入字符。full match 后设置 `state=0`，禁止沿 failure link接受 overlapping match。首个 match 前不创建 builder；total miss 返回原 string 且零分配。

- [ ] **Step 4: Select thresholds from a fixed grid**

Calibration grid：

```text
minimum pattern scalars: 4, 8, 16
minimum text bytes: 4 KiB, 16 KiB, 64 KiB
mode: strip, obfs
match: none, sparse, dense, overlap
script: ASCII, Sigma, Kelvin, invalid UTF-8
```

Run:

```powershell
go test . -run '^$' -bench '^BenchmarkFoldedRewriteStrategies$' -benchmem -count=10 | Tee-Object "$env:TEMP\censorship-round2-fold-kmp.txt"
```

启用区域要求 median 至少改善 15%，interval 不跨零，allocations 不劣于 naive；small pattern/text、dense、Unicode 和 invalid UTF-8 holdout 回退不超过 5%。未达到 gate 时停止，不提交 always-off path 或猜测 threshold。

- [ ] **Step 5: Verify GREEN and commit**

```powershell
go test . -run '^(TestCompileSnapshotBuildsKMPOnlyForFoldedRewrite|TestFoldKMP|TestAdaptiveFoldKMP|TestFoldStripRuleProcessesAllNonOverlappingOccurrences|TestFoldObfuscateRulePreservesSourceCasePerOccurrence)$' -count=1
go test . -run '^$' -fuzz '^FuzzFoldKMPAgainstOracle$' -fuzztime=10s
go test ./...
git add config.go matcher.go config_test.go matcher_test.go fuzz_test.go benchmark_test.go
git commit -m "perf: use adaptive KMP for folded rewrites"
```

---

## Benchmark Gate

所有 strategy benchmarks 在同一个 Go test binary 中同时运行 baseline 和 candidate。不得比较来自不同源码、不同 fixture 或不同 oracle 的结果。

若 `benchstat` 已在 PATH，直接使用并记录其版本。若未安装，Workflow 先运行 `go list -m -json golang.org/x/perf@latest` 解析一次完整 pseudo-version，把该版本写入执行记录，再以 `go run golang.org/x/perf/cmd/benchstat@<已记录的完整版本>` 运行本轮全部比较。模块查询会产生网络读取时必须经过工具权限；权限被拒绝则保存十次 raw samples，把统计 gate 标记为 blocked，不得虚构结果或改用未冻结的 `@latest` 执行。

Promotion 条件：

- correctness tests 和 forced differential fuzz 全部 GREEN。
- 目标区域 median 至少改善 15%，统计 interval 不跨零。
- holdout 回退不超过 5%。
- allocations 不增加；明确 allocation 优化还必须达到对应 unit ceiling。
- 选择的阈值形成连续、可解释区域。

直接结构优化已经由前一轮仓库扫描确认，但本轮 benchmark 若与既有证据矛盾，Workflow 必须停止对应候选并报告，不得静默删除 scope、调宽测试或挑选单次最好结果。

## Workflow Script Structure

批准后调用一个 inline JavaScript Workflow。脚本以 pure-literal `meta` 开头，不先写入仓库：

```javascript
export const meta = {
  name: 'censorship-performance-round-2',
  description: 'Implement twelve confirmed censorship optimizations with TDD and measured gates',
  phases: [
    { title: 'Baseline' },
    { title: 'Selector lane' },
    { title: 'Independent lanes' },
    { title: 'Compiler and matcher lane' },
    { title: 'Read-only review' },
    { title: 'Remediation' },
    { title: 'Verification' },
  ],
}
```

执行拓扑：

1. Baseline writer，Sonnet/xhigh，完成 Task 0。
2. Selector writer，Sonnet/xhigh，串行完成 Tasks 1 到 3，每项独立 RED/GREEN pair。
3. Rebuild writer，Sonnet/xhigh，完成 Task 4。
4. Envelope writer，Sonnet/xhigh，完成 Task 5。
5. ABI writer，Sonnet/xhigh，完成 Task 6。
6. Compiler/matcher writer，Sonnet/xhigh，串行完成 Tasks 7 到 12，并独占后续 `benchmark_test.go`。
7. 四个 Opus/xhigh read-only reviewers 用一个 `parallel()` barrier 分别审查 selector/duplicate、transform/RPC/ABI、compiler/matcher/performance、TDD/spec completeness。
8. 一个 Opus/xhigh synthesizer 去重并验证 findings。reviewer 和 synthesizer 不得修改文件。
9. 存在 confirmed finding 时，只启动一个 Sonnet/xhigh remediation writer。每个 finding 仍建立新的 RED/GREEN commits，不 amend 旧 commits。
10. 一个 Sonnet/xhigh final verifier 运行完整验证，写 evidence 文档并提交。

每个 writer 返回结构化结果：completed task IDs、RED commit hashes、GREEN commit hashes、RED command 和失败摘要、GREEN commands、benchmark gate、dirty files 和 blockers。任一返回 null、缺 commit、工作区不净或 gate 未通过，Workflow 立即停止，不启动后续 writer。

总 agent 上限：baseline writer、五个 implementation writers、四个 reviewers、一个 synthesizer、最多一个 remediation writer、一个 final verifier，总数不超过十三。

## Workflow Failure Conditions

出现以下任一条件立即停止：

1. 起始 HEAD 不是 `be27107180cafac98a71f238e501e0de26674a20`，或工作树不干净。
2. RED 立即通过，或因 typo、fixture、依赖和环境错误而不是目标缺失失败。
3. hook 拒绝 checkpoint commit。不得跳 hook。
4. focused GREEN 或随后 `go test ./...` 失败。
5. benchmark 不在同一 binary 比较 forced baseline/candidate。
6. adaptive/preflight 候选不满足 15%/interval/5% gate。
7. 出现未授权依赖、YAML option、unsafe borrow、cache、CLIProxyAPI 修改或 response-side code。
8. writer 改动不属于自己的 production 区域，或留下 dirty tree。
9. fuzz oracle 调用 production matcher/selector helper。
10. `git diff --check` 失败。
11. Windows Go SDK 出现 `0xc0000006` 或 `No such device`。先用小型独立 Go command 确认环境故障；确认后记录 blocked，不修改业务代码、降低测试或循环重试掩盖。
12. benchstat 不可用且无法获得用户允许的工具读取。保存 raw samples，标记 gate blocked。

## Read-only Review Gate

四个 reviewers 只报告可复现问题：

- selector reviewer：member-name canonicalization、excluded subtree duplicate、part full traversal、role override、`Result.Index`。
- transform/RPC/ABI reviewer：rule-major、cascade、span isolation、Encoder bytes、success envelope bytes、C ownership、AfterAuth-only bypass。
- matcher/performance reviewer：mode-derived matrix、root transition、byte AC failure/output、KMP non-overlap、threshold 和 holdout evidence。
- completeness reviewer：十二项 scope、每项 RED/GREEN pair、independent oracles、明确 exclusions 和 no-release boundary。

Synthesizer 必须尝试反驳每个 finding，并分类为 confirmed correctness defect、confirmed evidence gap 或 out-of-scope suggestion。只有前两类进入 remediation。

## Final Verification

所有 GREEN 和 remediation 完成后，从仓库根目录依次运行：

```powershell
git status --short --branch
git log --oneline --decorate be27107..HEAD
git diff --check be27107..HEAD
git diff --stat be27107..HEAD
go test -count=1 ./...
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
go test -race -count=1 ./...
go vet ./...
make build VERSION=0.0.0-dev
```

逐个 fuzz，每个 30 秒，不并发争抢同一 corpus/cache：

```powershell
go test . -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=30s
go test . -run '^$' -fuzz '^FuzzProtocolTransform$' -fuzztime=30s
go test . -run '^$' -fuzz '^FuzzDuplicateWalkerAgainstOracle$' -fuzztime=30s
go test . -run '^$' -fuzz '^FuzzRebuildBodyAgainstMarshalOracle$' -fuzztime=30s
go test . -run '^$' -fuzz '^FuzzByteMatcherAgainstOrderedContains$' -fuzztime=30s
go test . -run '^$' -fuzz '^FuzzFoldKMPAgainstOracle$' -fuzztime=30s
```

Integration 和 dynamic ABI：

```powershell
make integration
$env:BENCH='1'; make integration
```

Dynamic ABI benchmark 必须确认 preflight oracle 在计时区外通过，timed loop 内无 full-body `bytes.Contains` 或 `bytes.Equal`。`ReportAllocs` 只解释 host Go runtime allocations，不声称覆盖 DLL 独立 Go runtime 或 C heap。

最后重跑全部 strategy benchmarks：

```powershell
go test . -run '^$' -bench '^(BenchmarkDuplicateValidation|BenchmarkTextPartScanning|BenchmarkDisabledRoleSelectors|BenchmarkRebuildChangedSpans|BenchmarkSuccessEnvelope|BenchmarkExactRewriteStrategies|BenchmarkFoldRootTransitions|BenchmarkRewritePreflightStrategies|BenchmarkExactBlockStrategies|BenchmarkFoldedRewriteStrategies)$' -benchmem -count=10 | Tee-Object "$env:TEMP\censorship-round2-candidate.txt"
```

用 baseline、candidate 和 forced strategy outputs 生成 benchstat。核对 `git diff be27107..HEAD -- RELEASE_NOTES.md go.mod go.sum` 为空，并确认没有 tag、push 或 release 操作。

## TDD Evidence

新建 `docs/superpowers/tdd/2026-09-04-censorship-performance-round-2.tdd.md`，不覆盖旧 evidence。记录：

- base commit、Go version、OS/arch、CPU。
- 十二项候选各自的 RED/GREEN commit hash。
- 每个 RED 命令及实际失败摘要。
- 每个 focused/full GREEN 命令和结果。
- allocation ceilings 与实际值。
- fuzz seed corpus、每个 30 秒结果和 oracle independence。
- forced benchmark matrix、raw output path、benchstat delta 和 holdout。
- preflight、byte AC、KMP 的实际 package-private thresholds。
- race、vet、c-shared、integration、dynamic ABI 结果。
- 任何 `0xc0000006`、device I/O 或 benchstat 阻塞，必须如实标记 blocked，不得写 PASS。
- 明确声明 round 2 尚未发布。

验证后提交：

```powershell
git add docs/superpowers/tdd/2026-09-04-censorship-performance-round-2.tdd.md
git commit -m "docs: record round two TDD and benchmark evidence"
git status --short
```

Expected: 最终工作树为空。若任何必要验证 blocked，Workflow 返回 blocked 状态并列出已完成部分，不声称全部完成。

## No-release Boundary

最终状态是已实现并经本地证据验证的未发布候选。不得修改 `pluginVersion` 默认值、`RELEASE_NOTES.md` 或 release workflow；不得创建或移动 tag；不得 push；不得创建 GitHub Release；不得提交 `dist`、`.integration` 或 benchmark 临时文件。只有后续用户明确要求发布时，才另开 release 任务。