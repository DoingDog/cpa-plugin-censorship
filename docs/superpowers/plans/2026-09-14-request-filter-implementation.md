# 请求过滤功能实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 censorship plugin 增加按已认证 CPA caller credential 和客户端请求模型执行 include/exclude 审查的配置、共享 request gate、标准 panel 字段、测试、文档和 v0.3.0 发布。

**Architecture:** 在 immutable `configSnapshot` 中编译一个 `requestFilter`，用单个 `request_filter.go` 实现 full-string `*`/`?` glob、`caller_scope` 验证和处理 predicate。`interceptBeforeAuth` 在 JSON body 解析前调用该 gate，五种 `SourceFormat` 共享同一条路径，现有 provider selectors 和 `block -> strip -> obfs` 流程不变。

**Tech Stack:** Go 1.26、stdlib `crypto/sha256`、`encoding/hex`、`net/http`、`strings`、`unicode/utf8`，`gopkg.in/yaml.v3`，CLIProxyAPI v7.2.152 plugin API，现有 Make/GitHub Actions release pipeline。

**Spec:** `docs/superpowers/specs/2026-09-14-request-filter-design.md`

## Global Constraints

- 只修改 `cpa-plugin-censorship`；不修改 CLIProxyAPI core，不升级 pinned `github.com/router-for-me/CLIProxyAPI/v7 v7.2.152`。
- 保持 native ABI v1、RPC schema 5、`C.GoBytes` input copy、allocator ownership 和 pointer lifetime 不变。
- `filter_mode` 合法值为 `exclude`、`include`，默认 `exclude`。
- `filter_logic` 合法值为 `or`、`and`，默认 `or`。
- `filter` 是 strict Object，仅接受可选 `api-keys` 和 `models` string arrays；只要两个列表都为空或缺失，request gate 禁用，所有请求进入现有处理。
- 每个列表内部取 OR；两个非空维度按 `filter_logic` 取并集或交集；单一非空维度单独决定匹配结果。
- `exclude` 对匹配请求返回精确 no-op；`include` 只处理匹配请求。
- `*` 匹配零个或多个 Unicode scalar，`?` 匹配一个 Unicode scalar；full-string、case-sensitive，仅这两个字符特殊。
- model 只使用 `RequestedModel`；不回退到 `Model`，不从 request body 读取 model。
- API key 必须与 authenticated `Metadata["caller_scope"]` 绑定；不得直接信任、记录或回显 raw headers。
- Principal 与 literal credential 不同时，exact、wildcard 和 query-only credential patterns 均不匹配；RequestInterceptor 不可见 query。
- 只在 `Authorization`、`X-Goog-Api-Key`、`X-Api-Key` 中恢复 wildcard raw candidate；不增加 cache、ModelRouter 或 query 猜测。
- Panel 在 `ignore_case`、`words`、`scope`、`obfs` 后追加 `filter_mode`、`filter_logic`、`filter`；`words` 继续是 Object，不注册 global `mode`。
- 保持 `block -> strip -> obfs`、明确 provider text paths、no generic recursive walker、byte-preserving rewrite 和 last-known-good reconfigure 行为。
- 不手改 `.integration/` 或 `dist/` 生成内容。
- 行为改动必须先有 focused RED，再写最小 GREEN；每个 task 完成后运行所列 focused checks。
- 编码 worker 使用 Sonnet 1M/xhigh；计划和 review worker 使用 Opus 1M/xhigh。

## File Structure

- Create `request_filter.go`：filter types、compile、glob、caller scope、credential binding、match predicate。
- Create `request_filter_test.go`：glob、安全身份绑定、truth table、五格式 shared gate 与 exact no-op。
- Modify `config.go`：strict YAML parsing、snapshot defaults、filter pattern compile call。
- Modify `config_test.go`：配置 defaults、activation、strict invalid、whole-snapshot concurrency。
- Modify `main.go`：三个 appended `ConfigFields`，before-auth shared gate。
- Modify `main_test.go`：request helper、registration、lifecycle、文档 contract、v0.3.0 compatibility。
- Modify `benchmark_test.go`：disabled、exact 和 wildcard filter benchmark，不改变生产 contract。
- Modify `integration/http_test.go`：五种 HTTP format metadata/filter、strip、bypass parity、watcher reload。
- Modify `integration/websocket_test.go`：Responses WebSocket model turn filter match/miss。
- Modify `README.md`：配置、truth table、credential/model source、纯插件限制。
- Modify `RELEASE_NOTES.md`：新增 v0.3.0 section，保留所有历史 section。
- Do not modify `abi_cgo.go`、`selectors*.go`、`transform.go`、`matcher.go`、`go.mod`、`go.sum`、`Makefile`、`.github/workflows/build.yml` 或 package scripts，除非 focused RED 证明本计划中的既定 contract 无法通过；若出现该情况，先记录根因再做最小修复。

## Execution Waves

1. Task 1 -> Task 2 顺序执行，固定 parser、types、glob 和 predicate。
2. Task 2 GREEN 后，让两个 Sonnet 1M/xhigh worker 在同一 worktree 并行编辑互不重叠的文件：一个只做 Task 3 Steps 1-5 的 unit RED，另一个只做 Task 5 Steps 1-4 的 integration RED。两个 worker 都不得执行 `git add`、`git commit`、`git checkout` 或修改对方文件；父 session 独占 git index 和 HEAD。
3. 两组 RED 都有证据后，父 session 执行 Task 3 Step 6 的 shared gate，再运行 Task 3 unit GREEN 和 Task 5 integration GREEN。父 session先按 Task 3 的显式文件列表 commit，再按 Task 5 的显式文件列表 commit，避免并行 index/HEAD race。
4. Task 4 在 gate 和 integration commit 后执行。
5. Task 6 docs 与 Task 7 benchmark 可以由两个非提交 worker 并行编辑互不重叠的文件；父 session 汇合后顺序运行测试并按各自文件列表 commit。Task 7 不派发独立 simplifier。
6. Task 8 是唯一读取完整实现 diff 的 code review，之后顺序执行 Task 9 verification。
7. Task 10 merge/release 只能在所有本地验证和 review fixes 完成后执行。

---

### Task 1: Strict filter configuration and immutable snapshot

**Files:**
- Create: `request_filter.go`
- Modify: `config.go:14-45,61-78,117-235,295-351`
- Test: `config_test.go:16-65,177-220,222-298,300-362`

**Interfaces:**
- Produces:
  - `type filterMode string`
  - `const filterModeExclude filterMode = "exclude"`
  - `const filterModeInclude filterMode = "include"`
  - `type filterLogic string`
  - `const filterLogicOr filterLogic = "or"`
  - `const filterLogicAnd filterLogic = "and"`
  - `type compiledFilterPattern struct { Text string; Runes []rune; CallerScope string }`
  - `type requestFilter struct { Mode filterMode; Logic filterLogic; APIKeys []compiledFilterPattern; Models []compiledFilterPattern }`
  - `func (f requestFilter) enabled() bool`
  - `func compileRequestFilter(f *requestFilter)`
- Consumes: existing `validateMapping`, `stringScalar`, `compileSnapshot`, `configSnapshot` atomic publication.

- [ ] **Step 1: Add RED tests for defaults, activation, preservation, and valid parsing**

Add table-driven tests to `config_test.go` with these exact assertions:

```go
func TestParseConfigYAMLRequestFilter(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantMode   filterMode
		wantLogic  filterLogic
		wantAPI    []string
		wantModels []string
		wantActive bool
	}{
		{name: "missing", raw: "words: [x]\n", wantMode: filterModeExclude, wantLogic: filterLogicOr},
		{name: "empty object", raw: "filter: {}\n", wantMode: filterModeExclude, wantLogic: filterLogicOr},
		{name: "empty API array", raw: "filter: {api-keys: []}\n", wantMode: filterModeExclude, wantLogic: filterLogicOr},
		{name: "empty model array", raw: "filter: {models: []}\n", wantMode: filterModeExclude, wantLogic: filterLogicOr},
		{name: "empty arrays", raw: "filter: {api-keys: [], models: []}\n", wantMode: filterModeExclude, wantLogic: filterLogicOr},
		{name: "API only", raw: "mode: strip\nwords: [x]\nfilter: {api-keys: [sk-a, ' sk-b ', sk-a]}\n", wantMode: filterModeExclude, wantLogic: filterLogicOr, wantAPI: []string{"sk-a", " sk-b ", "sk-a"}, wantActive: true},
		{name: "Object words coexist", raw: "words: {block: [x]}\nfilter: {models: [model-a]}\n", wantMode: filterModeExclude, wantLogic: filterLogicOr, wantModels: []string{"model-a"}, wantActive: true},
		{name: "models only", raw: "filter_mode: include\nfilter: {models: ['gpt-*', '模型-?']}\n", wantMode: filterModeInclude, wantLogic: filterLogicOr, wantModels: []string{"gpt-*", "模型-?"}, wantActive: true},
		{name: "both and", raw: "filter_mode: include\nfilter_logic: and\nfilter: {api-keys: ['key-*'], models: ['model-?']}\n", wantMode: filterModeInclude, wantLogic: filterLogicAnd, wantAPI: []string{"key-*"}, wantModels: []string{"model-?"}, wantActive: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := mustConfig(t, test.raw)
			if cfg.Filter.Mode != test.wantMode || cfg.Filter.Logic != test.wantLogic || cfg.Filter.enabled() != test.wantActive {
				t.Fatalf("filter = %#v", cfg.Filter)
			}
			if got := filterPatternTexts(cfg.Filter.APIKeys); !reflect.DeepEqual(got, test.wantAPI) {
				t.Fatalf("api-keys = %#v, want %#v", got, test.wantAPI)
			}
			if got := filterPatternTexts(cfg.Filter.Models); !reflect.DeepEqual(got, test.wantModels) {
				t.Fatalf("models = %#v, want %#v", got, test.wantModels)
			}
		})
	}
}
```

Add this test-only helper next to `ruleTerms`:

```go
func filterPatternTexts(patterns []compiledFilterPattern) []string {
	if patterns == nil {
		return nil
	}
	out := make([]string, len(patterns))
	for i := range patterns {
		out[i] = patterns[i].Text
	}
	return out
}
```

- [ ] **Step 2: Add RED tests for every invalid shape without echoing pattern values**

Extend `TestParseConfigYAMLDefaultsAndValidation` or add `TestParseConfigYAMLRejectsInvalidRequestFilter` with this exact invalid matrix:

```go
invalid := []string{
	"filter_mode: null\n",
	"filter_mode: [include]\n",
	"filter_mode: INCLUDE\n",
	"filter_mode: include\nfilter_mode: exclude\n",
	"filter_logic: null\n",
	"filter_logic: [or]\n",
	"filter_logic: xor\n",
	"filter_logic: or\nfilter_logic: and\n",
	"filter: null\n",
	"filter: scalar\n",
	"filter: []\n",
	"filter: {unknown: []}\n",
	"filter:\n  api-keys: [a]\n  api-keys: [b]\n",
	"filter: {api-keys: null}\n",
	"filter: {api-keys: key}\n",
	"filter: {api-keys: {key: true}}\n",
	"filter: {api-keys: ['']}\n",
	"filter: {api-keys: [1]}\n",
	"filter: {models: null}\n",
	"filter: {models: model}\n",
	"filter: {models: ['']}\n",
	"filter: {models: [false]}\n",
}
```

For every row, require `parseConfigYAML` to return non-nil error. For `filter: {api-keys: [secret-sentinel, '']}`, also assert `err.Error()` contains `filter.api-keys` but does not contain `secret-sentinel`. Task 4 independently verifies that reconfigure logging preserves the same non-disclosure contract.

- [ ] **Step 3: Run the focused tests and confirm RED**

Run:

```bash
go test ./ -run 'TestParseConfigYAML(RequestFilter|RejectsInvalidRequestFilter|DefaultsAndValidation)|TestReconfigureStoresValidSnapshotAndKeepsLastKnownGoodOnError' -count=1
```

Expected: compile failure for missing filter types/fields, followed by behavior failures until parsing exists.

- [ ] **Step 4: Add the minimal filter data types and defaults**

Create `request_filter.go` with package declaration and these types/constants:

```go
package main

type filterMode string

const (
	filterModeExclude filterMode = "exclude"
	filterModeInclude filterMode = "include"
)

type filterLogic string

const (
	filterLogicOr  filterLogic = "or"
	filterLogicAnd filterLogic = "and"
)

type compiledFilterPattern struct {
	Text        string
	Runes       []rune
	CallerScope string
}

type requestFilter struct {
	Mode    filterMode
	Logic   filterLogic
	APIKeys []compiledFilterPattern
	Models  []compiledFilterPattern
}

func (f requestFilter) enabled() bool {
	return len(f.APIKeys) != 0 || len(f.Models) != 0
}
```

Add `Filter requestFilter` to `configSnapshot`. Initialize it in `defaultSnapshot`:

```go
Filter: requestFilter{
	Mode:  filterModeExclude,
	Logic: filterLogicOr,
},
```

- [ ] **Step 5: Parse the three top-level fields strictly**

Add these cases to the existing root switch in `parseConfigYAML`:

```go
case "filter_mode":
	text, err := stringScalar(value, "filter_mode")
	if err != nil {
		return nil, err
	}
	switch filterMode(text) {
	case filterModeExclude, filterModeInclude:
		cfg.Filter.Mode = filterMode(text)
	default:
		return nil, fmt.Errorf("invalid filter_mode")
	}
case "filter_logic":
	text, err := stringScalar(value, "filter_logic")
	if err != nil {
		return nil, err
	}
	switch filterLogic(text) {
	case filterLogicOr, filterLogicAnd:
		cfg.Filter.Logic = filterLogic(text)
	default:
		return nil, fmt.Errorf("invalid filter_logic")
	}
case "filter":
	if err := parseRequestFilter(value, &cfg.Filter); err != nil {
		return nil, err
	}
```

Implement strict nested parsing in `config.go`:

```go
func parseRequestFilter(node *yaml.Node, filter *requestFilter) error {
	if err := validateMapping(node, "filter"); err != nil {
		return err
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		switch key {
		case "api-keys":
			patterns, err := parseFilterPatterns(value, "filter.api-keys")
			if err != nil {
				return err
			}
			filter.APIKeys = patterns
		case "models":
			patterns, err := parseFilterPatterns(value, "filter.models")
			if err != nil {
				return err
			}
			filter.Models = patterns
		default:
			return fmt.Errorf("unknown filter key %q", key)
		}
	}
	return nil
}

func parseFilterPatterns(node *yaml.Node, name string) ([]compiledFilterPattern, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s must be a sequence", name)
	}
	if len(node.Content) == 0 {
		return nil, nil
	}
	patterns := make([]compiledFilterPattern, 0, len(node.Content))
	for _, item := range node.Content {
		text, err := stringScalar(item, name+" value")
		if err != nil {
			return nil, err
		}
		if text == "" {
			return nil, fmt.Errorf("%s value must not be empty", name)
		}
		patterns = append(patterns, compiledFilterPattern{Text: text})
	}
	return patterns, nil
}
```

Do not include the secret pattern text in an error. Make `compileRequestFilter(&cfg.Filter)` the first statement in `compileSnapshot`, before its `IgnoreCase` early return, so every valid snapshot receives identical filter compilation.

- [ ] **Step 6: Compile pattern metadata without changing user text**

In `request_filter.go`, implement the compile step using only stdlib:

```go
func compileRequestFilter(filter *requestFilter) {
	for i := range filter.APIKeys {
		pattern := &filter.APIKeys[i]
		pattern.Runes = nil
		pattern.CallerScope = ""
		if strings.ContainsAny(pattern.Text, "*?") {
			pattern.Runes = []rune(pattern.Text)
		}
	}
	for i := range filter.Models {
		pattern := &filter.Models[i]
		pattern.Runes = nil
		pattern.CallerScope = ""
		if strings.ContainsAny(pattern.Text, "*?") {
			pattern.Runes = []rune(pattern.Text)
		}
	}
}
```

Add `strings` import to `request_filter.go`. Task 2 extends the same function to precompute exact API-key scopes after it adds the real `callerScope` implementation; Task 1 remains independently compilable and testable.

- [ ] **Step 7: Run focused configuration tests and confirm GREEN**

```bash
go test ./ -run 'TestParseConfigYAML(RequestFilter|RejectsInvalidRequestFilter|DefaultsAndValidation)' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit the configuration contract**

```bash
git add config.go config_test.go request_filter.go
git commit -m "feat: parse request filter configuration"
```

Expected: the commit compiles and contains no lifecycle or runtime behavior that belongs to later tasks.

---

### Task 2: Full-string glob and authenticated caller matching

**Files:**
- Modify: `request_filter.go`
- Create: `request_filter_test.go`

**Interfaces:**
- Consumes: Task 1 `compiledFilterPattern`, `requestFilter` and wildcard `Runes`; this task fills exact API-key `CallerScope`.
- Produces:
  - `const callerScopeMetadataKey = "caller_scope"`
  - `func callerScope(value string) string`
  - `func callerScopeFromMetadata(metadata map[string]any) string`
  - `func matchFilterGlob(pattern, value []rune) bool`
  - `func scopeBoundHeaderValue(headers http.Header, name, scope string, bearer bool) string`
  - `func authenticatedCallerAPIKey(headers http.Header, scope string) string`
  - `func (f requestFilter) matchesAPIKey(headers http.Header, metadata map[string]any) bool`
  - `func (f requestFilter) matchesModel(model string) bool`
  - `func (f requestFilter) shouldProcess(request *pluginapi.RequestInterceptRequest) bool`

- [ ] **Step 1: Write RED glob tests with an independent table**

Create `request_filter_test.go` and add `TestMatchFilterGlob` with the following rows:

```go
{name: "exact", pattern: "abc", value: "abc", want: true}
{name: "exact mismatch", pattern: "abc", value: "ab", want: false}
{name: "prefix star", pattern: "ab*", value: "ab/甲", want: true}
{name: "suffix star", pattern: "*bc", value: "a/bc", want: true}
{name: "middle star empty", pattern: "a*c", value: "ac", want: true}
{name: "middle star", pattern: "a*c", value: "a/甲/c", want: true}
{name: "question Unicode scalar", pattern: "a?c", value: "a甲c", want: true}
{name: "question one only", pattern: "a?c", value: "a甲乙c", want: false}
{name: "anchored prefix", pattern: "ab*", value: "zab", want: false}
{name: "anchored suffix", pattern: "*bc", value: "bcz", want: false}
{name: "case sensitive", pattern: "Key-*", value: "key-a", want: false}
{name: "brackets literal", pattern: "m[0]", value: "m[0]", want: true}
{name: "brackets not class", pattern: "m[0]", value: "m0", want: false}
{name: "only star", pattern: "*", value: "anything", want: true}
{name: "star empty", pattern: "*", value: "", want: true}
```

Call `matchFilterGlob([]rune(test.pattern), []rune(test.value))` and assert the exact boolean.

- [ ] **Step 2: Write RED caller scope and credential-binding tests**

Add:

```go
func TestCallerScope(t *testing.T) {
	if got, want := callerScope("test-key"), "a420e246227b259b532446574f6fd7719cc2d7f551efd9944f93422b96811a56"; got != want {
		t.Fatalf("callerScope() = %q, want %q", got, want)
	}
	if callerScope(" \t\n") != "" {
		t.Fatal("blank caller scope was non-empty")
	}
}
```

The fixed value was independently calculated from the pinned domain separator and `test-key`; do not replace it with an expected value computed by `callerScope` itself.

Add table cases for `matchesAPIKey`:

- exact pattern `test-key`, matching metadata scope, no headers -> true.
- exact pattern `test-key`, missing/uppercase/non-hex/wrong scope -> false.
- exact pattern `" test-key "`, scope for `test-key` -> false.
- wildcard `test-*`, `Authorization: Bearer test-key`, matching scope -> true.
- wildcard `test-?ey`, lower-case `bearer`, matching scope -> true.
- wildcard with first forged Authorization value and a later correct value -> true.
- wildcard candidate under the non-canonical map key `authorization` -> true.
- wildcard candidate under mixed-case map keys `x-GoOg-aPi-KeY` and `X-aPi-kEy` -> true.
- wildcard candidate in canonical `X-Goog-Api-Key` -> true.
- wildcard candidate in canonical `X-Api-Key` -> true.
- matching raw credential only in `Cookie` or `Proxy-Authorization` -> false.
- matching raw header with scope for `account:42` -> false.
- matching wildcard header with no scope -> false.
- query-only exact represented by matching scope and no header -> true.
- Principal mismatch represented by scope for `account:42` and no header -> false.
- two wildcard patterns `[never-*, test-*]` with `test-key` -> true, proving list-internal OR.

- [ ] **Step 3: Write RED truth-table and short-circuit tests**

Add `TestRequestFilterShouldProcess` with explicit `requestFilter` values and these rows:

| Mode | Logic | API configured/match | Model configured/match | Want process |
| --- | --- | --- | --- | --- |
| include | or | absent | absent | true |
| exclude | and | absent | absent | true |
| include | or | true/true | absent | true |
| include | and | true/false | absent | false |
| exclude | or | true/true | absent | false |
| exclude | and | true/false | absent | true |
| include | or | true/false | true/true | true |
| include | or | true/false | true/false | false |
| include | and | true/true | true/true | true |
| include | and | true/true | true/false | false |
| exclude | or | true/true | true/false | false |
| exclude | or | true/false | true/false | true |
| exclude | and | true/true | true/false | true |
| exclude | and | true/true | true/true | false |

Add a request where `Model` and body contain a matching decoy but `RequestedModel` does not; require model dimension false. Add `RequestedModel == ""` with model pattern `*`; require false because missing model never matches even though `*` otherwise accepts zero Unicode scalars. Add an exact API-key pattern `" test-key "` with matching `callerScope("test-key")`: `include` must bypass (`shouldProcess == false`) and `exclude` must process (`shouldProcess == true`).

- [ ] **Step 4: Run matcher tests and confirm RED**

Run:

```bash
go test ./ -run 'Test(MatchFilterGlob|CallerScope|RequestFilter)' -count=1
```

Expected: compile or assertion failures because matcher and predicate functions are absent.

- [ ] **Step 5: Implement the greedy Unicode glob matcher**

Use the standard linear star-backtracking algorithm. The complete function contract is:

```go
func matchFilterGlob(pattern, value []rune) bool {
	patternIndex, valueIndex := 0, 0
	starIndex, retryValueIndex := -1, 0
	for valueIndex < len(value) {
		switch {
		case patternIndex < len(pattern) && (pattern[patternIndex] == '?' || pattern[patternIndex] == value[valueIndex]):
			patternIndex++
			valueIndex++
		case patternIndex < len(pattern) && pattern[patternIndex] == '*':
			starIndex = patternIndex
			patternIndex++
			retryValueIndex = valueIndex
		case starIndex >= 0:
			patternIndex = starIndex + 1
			retryValueIndex++
			valueIndex = retryValueIndex
		default:
			return false
		}
	}
	for patternIndex < len(pattern) && pattern[patternIndex] == '*' {
		patternIndex++
	}
	return patternIndex == len(pattern)
}
```

No regex, `path.Match`, character classes, escape handling or new dependency is allowed.

- [ ] **Step 6: Implement caller scope validation and raw credential binding**

Add imports `crypto/sha256`, `encoding/hex`, `net/http`, `strings`, and `github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi` to `request_filter.go`.

Implement:

```go
const (
	callerScopeMetadataKey = "caller_scope"
	callerScopeDomain      = "cli-proxy-api:caller-scope:v1\x00"
)

func callerScope(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(callerScopeDomain + value))
	return hex.EncodeToString(sum[:])
}

func callerScopeFromMetadata(metadata map[string]any) string {
	scope, _ := metadata[callerScopeMetadataKey].(string)
	if len(scope) != sha256.Size*2 {
		return ""
	}
	for i := range scope {
		if !('0' <= scope[i] && scope[i] <= '9') && !('a' <= scope[i] && scope[i] <= 'f') {
			return ""
		}
	}
	return scope
}
```

Extend Task 1's `compileRequestFilter` API-key loop so exact patterns receive a trusted comparison value while whitespace-bearing exact patterns remain impossible:

```go
if strings.ContainsAny(pattern.Text, "*?") {
	pattern.Runes = []rune(pattern.Text)
} else if pattern.Text == strings.TrimSpace(pattern.Text) {
	pattern.CallerScope = callerScope(pattern.Text)
}
```

Implement header extraction exactly once inside the API wildcard path:

```go
func scopeBoundHeaderValue(headers http.Header, name, scope string, bearer bool) string {
	for key, values := range headers {
		if !strings.EqualFold(key, name) {
			continue
		}
		for _, value := range values {
			if bearer {
				parts := strings.SplitN(value, " ", 2)
				if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
					value = parts[1]
				}
			}
			candidate := strings.TrimSpace(value)
			if candidate != "" && callerScope(candidate) == scope {
				return candidate
			}
		}
	}
	return ""
}

func authenticatedCallerAPIKey(headers http.Header, scope string) string {
	if scope == "" {
		return ""
	}
	if candidate := scopeBoundHeaderValue(headers, "Authorization", scope, true); candidate != "" {
		return candidate
	}
	if candidate := scopeBoundHeaderValue(headers, "X-Goog-Api-Key", scope, false); candidate != "" {
		return candidate
	}
	return scopeBoundHeaderValue(headers, "X-Api-Key", scope, false)
}
```

Every candidate is trimmed, empty candidates are rejected, and only a candidate whose recomputed scope equals authenticated metadata is returned to the private matcher. The value is never logged, cached or placed in a response.

- [ ] **Step 7: Implement exact/wildcard pattern matching and mode decision**

Implement model matching so the request value is converted to `[]rune` at most once:

```go
func (f requestFilter) matchesModel(value string) bool {
	if value == "" {
		return false
	}
	var valueRunes []rune
	for i := range f.Models {
		pattern := &f.Models[i]
		if pattern.Runes == nil {
			if pattern.Text == value {
				return true
			}
			continue
		}
		if valueRunes == nil {
			valueRunes = []rune(value)
		}
		if matchFilterGlob(pattern.Runes, valueRunes) {
			return true
		}
	}
	return false
}
```

Implement exact API-key comparison before any raw header access, then wildcard comparison against one authenticated candidate:

```go
func (f requestFilter) matchesAPIKey(headers http.Header, metadata map[string]any) bool {
	scope := callerScopeFromMetadata(metadata)
	if scope == "" {
		return false
	}
	hasWildcard := false
	for i := range f.APIKeys {
		pattern := &f.APIKeys[i]
		if pattern.Runes != nil {
			hasWildcard = true
			continue
		}
		if pattern.CallerScope != "" && pattern.CallerScope == scope {
			return true
		}
	}
	if !hasWildcard {
		return false
	}
	candidate := authenticatedCallerAPIKey(headers, scope)
	if candidate == "" {
		return false
	}
	valueRunes := []rune(candidate)
	for i := range f.APIKeys {
		pattern := &f.APIKeys[i]
		if pattern.Runes != nil && matchFilterGlob(pattern.Runes, valueRunes) {
			return true
		}
	}
	return false
}
```

Exact patterns with leading/trailing whitespace have empty `CallerScope` and nil `Runes`, so they are skipped and never match.

Implement final predicate:

```go
func (f requestFilter) shouldProcess(request *pluginapi.RequestInterceptRequest) bool {
	if !f.enabled() {
		return true
	}
	apiConfigured := len(f.APIKeys) != 0
	modelConfigured := len(f.Models) != 0
	var matched bool
	switch {
	case apiConfigured && modelConfigured && f.Logic == filterLogicAnd:
		matched = f.matchesModel(request.RequestedModel) && f.matchesAPIKey(request.Headers, request.Metadata)
	case apiConfigured && modelConfigured:
		matched = f.matchesModel(request.RequestedModel) || f.matchesAPIKey(request.Headers, request.Metadata)
	case apiConfigured:
		matched = f.matchesAPIKey(request.Headers, request.Metadata)
	default:
		matched = f.matchesModel(request.RequestedModel)
	}
	return matched == (f.Mode == filterModeInclude)
}
```

- [ ] **Step 8: Run Task 1 and Task 2 tests and confirm GREEN**

Run:

```bash
go test ./ -run 'TestParseConfigYAML(RequestFilter|RejectsInvalidRequestFilter|DefaultsAndValidation)|Test(MatchFilterGlob|CallerScope|RequestFilter)' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit matcher and predicate**

```bash
git add request_filter.go request_filter_test.go
git commit -m "feat: match authenticated request filters" -- request_filter.go request_filter_test.go
```

Expected: Task 1 remains a separate independently green commit.

---

### Task 3: Shared before-auth gate for all five formats

**Files:**
- Modify: `main.go:108-148`
- Modify: `main_test.go:195-249,354-397`
- Modify: `request_filter_test.go`

**Interfaces:**
- Consumes: `configSnapshot.Filter.shouldProcess(*pluginapi.RequestInterceptRequest)` from Task 2.
- Produces: `func callInterceptRequest(request pluginapi.RequestInterceptRequest) (pluginapi.RequestInterceptResponse, error)` as a test helper.
- Preserves: `transformRequest(body, sourceFormat, cfg)` signature and every selector.

- [ ] **Step 1: Add a narrow request-aware test helper**

Refactor only the existing test helper, preserving all call sites:

```go
func callIntercept(sourceFormat string, body []byte) (pluginapi.RequestInterceptResponse, error) {
	return callInterceptRequest(pluginapi.RequestInterceptRequest{
		RequestID:    "censorship-test",
		SourceFormat: sourceFormat,
		Body:         body,
	})
}

func callInterceptRequest(request pluginapi.RequestInterceptRequest) (pluginapi.RequestInterceptResponse, error) {
	raw, err := json.Marshal(request)
	if err != nil {
		return pluginapi.RequestInterceptResponse{}, err
	}
	envelopeBytes, err := handleMethod(pluginabi.MethodRequestInterceptBefore, raw)
	if err != nil {
		return pluginapi.RequestInterceptResponse{}, err
	}
	var env pluginabi.Envelope
	if err := json.Unmarshal(envelopeBytes, &env); err != nil {
		return pluginapi.RequestInterceptResponse{}, err
	}
	if !env.OK {
		return pluginapi.RequestInterceptResponse{}, fmt.Errorf("RPC %s: %s", env.Error.Code, env.Error.Message)
	}
	var resp pluginapi.RequestInterceptResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		return pluginapi.RequestInterceptResponse{}, err
	}
	return resp, nil
}
```

- [ ] **Step 2: Write RED tests for include/exclude and malformed-body bypass**

Add `TestRequestFilterGatesBeforeBodyValidation` to `request_filter_test.go`:

- Register include/models=`target-*`, words block=`SECRET`.
- Request `RequestedModel=other` and body `not-json` -> exact zero-value response.
- Request `RequestedModel=target-model` and body `not-json` -> existing terminated 400 with `censorship_invalid_request`.
- Register exclude/models=`target-*`.
- Request `RequestedModel=target-model` and duplicate-member body -> exact zero-value response.
- Request `RequestedModel=other` and duplicate-member body -> existing terminated 400.
- Register no censorship rules with include/api-keys=`test-*`; send malformed body, malformed metadata and a credential header -> exact zero-value response, proving `len(cfg.Rules) == 0` short-circuits before filter matching and body validation.

Compare the complete response with `pluginapi.RequestInterceptResponse{}` by `reflect.DeepEqual` for bypass cases.

- [ ] **Step 3: Write RED five-format table**

Add `TestRequestFilterGatesAllSupportedFormats` with this fixture table:

```go
formats := []struct {
	name   string
	format string
	body   []byte
}{
	{name: "openai", format: "openai", body: []byte(`{"model":"body-decoy","messages":[{"role":"user","content":"SECRET"}]}`)},
	{name: "openai-response", format: "openai-response", body: []byte(`{"model":"body-decoy","input":"SECRET"}`)},
	{name: "claude", format: "claude", body: []byte(`{"model":"body-decoy","messages":[{"role":"user","content":"SECRET"}]}`)},
	{name: "gemini", format: "gemini", body: []byte(`{"model":"body-decoy","contents":[{"role":"user","parts":[{"text":"SECRET"}]}]}`)},
	{name: "interactions", format: "interactions", body: []byte(`{"model":"body-decoy","input":"SECRET"}`)},
}
```

For each format under include/models=`target-*`:

- `RequestedModel=target-model`, `Model=ignored-current-model` -> existing `censorship_blocked` response.
- `RequestedModel=other`, `Model=target-model` -> exact zero-value response.

This proves all five formats use RPC metadata rather than body or `Model`.

- [ ] **Step 4: Write RED API-key gate test through the complete RPC path**

Register `filter_mode: include`, `filter.api-keys: [test-?ey]`, words block=`SECRET`. Send:

- matching `caller_scope`, `Authorization: Bearer test-key` -> block.
- matching header with missing scope -> no-op.
- forged scope for `account:42` -> no-op.
- exact pattern `test-key`, matching scope and no header -> block.
- exact pattern `" test-key "`, matching scope -> no-op.

- [ ] **Step 5: Run gate tests and confirm RED**

Run:

```bash
go test ./ -run 'TestRequestFilter(GatesBeforeBodyValidation|GatesAllSupportedFormats|APIKeyGate)' -count=1
```

Expected: requests that should bypass are still parsed/blocked because `interceptBeforeAuth` has not called the filter. Before Step 6, wait for the parallel Task 5 worker to finish Steps 1-5 and preserve its independent HTTP/WebSocket RED output.

- [ ] **Step 6: Add the one shared gate in `interceptBeforeAuth`**

Move the existing snapshot load below the known-format guard, then add the gate before `transformRequest`:

```go
if !knownSourceFormat(request.SourceFormat) {
	return okEnvelope(pluginapi.RequestInterceptResponse{})
}
cfg := loadedSnapshot()
if cfg == nil || len(cfg.Rules) == 0 || !cfg.Filter.shouldProcess(&request) {
	return okEnvelope(pluginapi.RequestInterceptResponse{})
}
result, err := transformRequest(request.Body, request.SourceFormat, cfg)
```

Do not move the outer RPC JSON decode, alter `transformRequest`, touch provider selectors or return the original body.

- [ ] **Step 7: Run focused gate and existing early-no-op tests**

Run:

```bash
go test ./ -run 'Test(RequestFilter|BeforeAuthEarlyNoOpsDoNotParseBody|BeforeAuthRejectsEnabledInvalidJSON|BeforeAuthRejectsDuplicateJSONMembers)' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit the shared gate from the parent session**

The parallel unit-test worker returns without touching git. After both Task 3 unit GREEN and Task 5 integration GREEN, the parent session runs:

```bash
git add main.go main_test.go request_filter_test.go
git commit -m "feat: gate censorship by request identity" -- main.go main_test.go request_filter_test.go
```

---

### Task 4: Panel metadata, lifecycle, and whole-snapshot concurrency

**Files:**
- Modify: `main.go:39-57`
- Modify: `main_test.go:79-152,425-504`
- Modify: `config_test.go:236-298`

**Interfaces:**
- Consumes: Task 1 config names and Task 2 behavior.
- Produces: exactly seven ordered `pluginapi.ConfigField` entries.
- Preserves: no custom panel capability, no ManagementAPI capability, existing atomic pointer publication.

- [ ] **Step 1: Make registration metadata tests RED**

Extend `TestRegistrationExposesEditableConfigFields` so `want` contains these appended entries after `obfs`:

```go
{
	name:        "filter_mode",
	typeName:    pluginapi.ConfigFieldTypeEnum,
	description: "Exclude matching requests from censorship, or include only matching requests (default exclude).",
	enumValues:  []string{"exclude", "include"},
},
{
	name:        "filter_logic",
	typeName:    pluginapi.ConfigFieldTypeEnum,
	description: "Combine non-empty filter api-keys and models with or or and (default or); one non-empty list is used alone.",
	enumValues:  []string{"or", "and"},
},
{
	name:        "filter",
	typeName:    pluginapi.ConfigFieldTypeObject,
	description: "Optional object with api-keys and models pattern arrays; * matches zero or more Unicode scalars and ? matches one. Empty arrays or an empty object disable request filtering. API-key patterns require authenticated caller_scope binding and are not masked by the standard panel.",
},
```

Add `enumValues []string` to the local expected struct. Compare `field.EnumValues` with `reflect.DeepEqual`, not only nil. Assert no field has `Name == "mode"` and `words` remains `ConfigFieldTypeObject`.

- [ ] **Step 2: Add lifecycle RED coverage**

Extend `TestReconfigureStoresValidSnapshotAndKeepsLastKnownGoodOnError`:

1. Register config A with include model `model-a`, block `alpha`.
2. Verify model A blocks and model B bypasses.
3. Call `handleMethod(MethodPluginRegister, ...)` with `filter: {api-keys: [secret-sentinel, '']}`; require a structured `plugin_error`, require the complete envelope bytes not to contain `secret-sentinel`, and verify config A remains installed.
4. Reconfigure config B with exclude model `model-a`, block `Beta`, `ignore_case: true`.
5. Verify model A bypasses and model B blocks `BETA`.
6. Reconfigure invalid `filter: {api-keys: [secret-sentinel, '']}`.
7. Verify B still applies, exactly one error log exists, and `fmt.Sprint(logs[0].Fields["error"])` contains `filter.api-keys` but does not contain `secret-sentinel`, the raw credential or `caller_scope`.

- [ ] **Step 3: Strengthen the existing concurrency test before implementation changes**

Update `TestConcurrentReconfigureObservesOnlyWholeSnapshot` to use:

```yaml
# A
filter_mode: include
filter_logic: and
filter:
  api-keys: [key-a]
  models: [model-a]
```

```yaml
# B
filter_mode: exclude
filter_logic: or
filter:
  api-keys: [key-b]
  models: [model-b]
```

Add `net/http` and `github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi` imports to `config_test.go`. Replace the worker's `callIntercept` call with:

```go
resp, err := callInterceptRequest(pluginapi.RequestInterceptRequest{
	RequestID:      "concurrent-filter-test",
	SourceFormat:   "openai",
	RequestedModel: "model-a",
	Headers:         http.Header{"Authorization": {"Bearer key-a"}},
	Metadata:        map[string]any{callerScopeMetadataKey: callerScope("key-a")},
	Body:            body,
})
```

A produces the existing strip `wantA`; B does not match either B pattern and produces the existing obfs `wantB`. Any mixed mode/pattern snapshot yields nil or another body and fails the existing two-result oracle.

- [ ] **Step 4: Run panel/lifecycle/concurrency tests and confirm RED**

Run:

```bash
go test ./ -run 'Test(RegistrationExposesEditableConfigFields|ReconfigureStoresValidSnapshotAndKeepsLastKnownGoodOnError|ConcurrentReconfigureObservesOnlyWholeSnapshot)' -count=1
```

Expected: registration field count/enum failures before metadata is added; lifecycle/concurrency failures if the runtime filter snapshot is incomplete.

- [ ] **Step 5: Append the three ConfigFields in production**

Add the exact three entries from Step 1 to `pluginRegistration().Metadata.ConfigFields` after `obfs`. Do not add `Default`, nested schema, custom panel, ManagementAPI or global `mode`.

- [ ] **Step 6: Run focused tests and race coverage**

Run:

```bash
go test ./ -run 'Test(Registration|Reconfigure|ConcurrentReconfigure)' -count=1
go test -race ./ -run 'TestConcurrentReconfigureObservesOnlyWholeSnapshot' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit panel and lifecycle behavior**

```bash
git add main.go main_test.go config_test.go
git commit -m "feat: expose request filters in plugin config"
```

---

### Task 5: Real CPA HTTP and Responses WebSocket integration

**Files:**
- Modify: `integration/http_test.go:130-214,216-309,351-391`
- Modify: `integration/websocket_test.go:23-41,316-383`

**Interfaces:**
- Consumes: existing `startCPA`, `postJSON`, `postChat`, `captureHTTP11Trace`, `newMockUpstream`, `responsesWSExchange`, `downstreamKey`, `modelName`.
- Produces: no new production interface and no second integration harness.

- [ ] **Step 1: Add a five-format HTTP integration table**

Add `TestHTTPRequestFilterMatchesAllSupportedFormats`. Use one subtest per route with the exact source bodies already accepted by current selectors:

- `/v1/chat/completions`: OpenAI `messages[0].content`.
- `/v1/responses`: Responses top-level `input`.
- `/v1/messages`: Claude user message plus `max_tokens: 16`.
- `/v1beta/models/censorship-integration-model:generateContent`: Gemini `contents[0].parts[0].text`.
- `/v1beta/interactions`: Interactions top-level `input`.

For every case, start CPA with:

```yaml
words:
  block: [SECRET]
filter_mode: include
filter_logic: and
filter:
  api-keys: [censorship-integration-?ey]
  models: [censorship-integration-*]
```

Send the request through `postJSON`; require HTTP 400, `censorship_blocked`, and `upstream.arrivalCount() == 0`. This proves the real host supplies matching `caller_scope`, raw Authorization candidate and `RequestedModel` for all five formats.

- [ ] **Step 2: Add strip and bypass parity integration cases**

Add one OpenAI Chat strip case with the same `and` filter and `words.strip: [SECRET]`; assert upstream message content is transformed and all non-target fields match a plugin-disabled baseline through `validateCapturedChatRequest`.

Add one OpenAI Chat bypass case using include/models=`never-*` and block=`SECRET`. Compare:

- enabled versus disabled upstream request bytes after existing route normalization helper;
- full HTTP/1.1 response trace by `reflect.DeepEqual`;
- arrival count exactly one on each upstream.

This is the host-level proof that an unprocessed request is not modified.

- [ ] **Step 3: Extend watcher integration for filter snapshot replacement**

Modify `TestWatcherReloadLinearizesAtObservedSnapshotB` without creating a second watcher harness:

- Snapshot A includes `filter_mode: include`, `filter.models: [censorship-integration-model]`, and the existing `alpha-only` rule.
- Snapshot B uses `filter_mode: exclude`, `filter.models: [never-match]`, and the existing `beta-only` rule. Since the requested model does not match, B processes and blocks `BETA-ONLY`.
- After observing B, write invalid `filter_logic: xor`; poll the CPA log until `censorship plugin reconfigure rejected` appears, then verify B still blocks `BETA-ONLY` and old A-only input reaches upstream.

- [ ] **Step 4: Add Responses WebSocket match and miss cases**

Change `TestResponsesWebSocketBlockReturnsStatus400ThenCloses` config to include/models=`censorship-integration-*`, preserving its existing status 400, peer-close and zero-upstream assertions.

Add `TestResponsesWebSocketRequestFilterBypassMatchesDisabledPlugin`:

- Payload contains `SECRET` in Responses input and `modelName` in top-level model.
- Enabled CPA uses include/models=`never-*` and words block=`SECRET`.
- Disabled CPA has no plugin.
- Compare complete `[]wsMessage` from `responsesWSExchange` with `reflect.DeepEqual`.
- Compare captured upstream request bodies with `bytes.Equal`.

- [ ] **Step 5: Run integration tests before adding the shared gate and confirm RED**

The existing integration runner has no test-name passthrough, so run its supported command directly after Task 2 and before Task 3 Step 6:

```bash
make integration
```

Expected: FAIL in the new HTTP bypass parity and Responses WebSocket include-miss tests because the parsed filter is not yet enforced; existing tests and filter-match block cases should pass. If the failure is unrelated, diagnose it before implementing the gate.

- [ ] **Step 6: Re-run integration after Task 3 Step 6 and confirm GREEN**

```bash
make integration
```

Expected: PASS. Any host-contract discrepancy must be diagnosed with `superpowers:systematic-debugging` before changing scope.

- [ ] **Step 7: Commit integration coverage from the parent session**

The parallel worker returns without touching git. After Task 3's path-limited commit, the parent session runs:

```bash
git add integration/http_test.go integration/websocket_test.go
git commit -m "test: cover request filters through CPA" -- integration/http_test.go integration/websocket_test.go
```

---

### Task 6: User documentation and v0.3.0 release notes

**Files:**
- Modify: `main_test.go:506-649`
- Modify: `README.md:1-178`
- Modify: `RELEASE_NOTES.md:1-160`

**Interfaces:**
- Consumes: all settled config/runtime behavior from Tasks 1 through 4.
- Produces: documentation strings enforced by `TestDocumentationListsConfigAndLimits` and `TestReleaseNotesCompatibilityTargetsCurrentVersion`.

- [ ] **Step 1: Make documentation contract tests RED**

Update the embedded `configExample` in `main_test.go` so these fields appear after `obfs`:

```yaml
      filter_mode: exclude
      filter_logic: or
      filter: {}
```

Add required tokens covering:

- standard ConfigFields now expose all seven field names in order;
- default exclude/or;
- empty filter or both empty lists disables request filtering and processes all requests;
- `api-keys` and `models` lists, list-internal OR, cross-dimension and/or truth table;
- full-string case-sensitive `*` and `?` Unicode scalar semantics;
- include/exclude process/bypass table;
- model uses `RequestedModel`, never `Model` or body model;
- `caller_scope` binding and Principal/literal credential mismatch;
- exact query-only conditional support and wildcard query limitation;
- management config readback and lack of secret masking;
- bypass returns no replacement body or header changes before JSON validation;
- v0.3.0 compatibility sentence.

Change `TestReleaseNotesCompatibilityTargetsCurrentVersion` to require:

```plaintext
v0.3.0 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.
```

Run:

```bash
go test ./ -run 'Test(DocumentationListsConfigAndLimits|ReleaseNotesCompatibilityTargetsCurrentVersion)' -count=1
```

Expected: FAIL because README and RELEASE_NOTES still describe v0.2.6 and four panel fields.

- [ ] **Step 2: Update README without deleting existing verified contracts**

Make these surgical changes:

1. Change current compatibility from v0.2.6 to v0.3.0 without changing CPA version, schema, host commit, ABI or glibc floor.
2. Change the ConfigFields sentence from four names to seven ordered names.
3. Append `filter_mode`, `filter_logic`, and `filter: {}` to the canonical YAML example after `obfs`.
4. Add the three config rows and compact truth tables from the spec.
5. Add a focused `Request filtering` subsection describing data sources, glob rules, empty-filter behavior, Principal limitation, header carriers, query limitation, no secret masking and gate-before-body-validation no-op.
6. Leave all provider text-path, matcher, ABI, integration and release-artifact sections unchanged except where the new filter behavior must be mentioned.

- [ ] **Step 3: Add a new v0.3.0 release section while preserving history**

At the top of `RELEASE_NOTES.md`:

```markdown
# Censorship v0.3.0

## v0.3.0 features

- Adds `filter_mode` with `exclude` default and `include` selection.
- Adds `filter_logic` with `or` default and `and` selection across non-empty `api-keys` and `models` lists.
- Adds full-string, case-sensitive `*` and `?` request patterns, bound to authenticated `caller_scope` for API keys and `RequestedModel` for models.
- Applies one shared filter gate to `openai`, `openai-response`, `claude`, `gemini`, and `interactions`; bypassed requests return no body or header modification before body validation.
- Appends the three settings to standard plugin `ConfigFields`; `words` remains Object-only and the panel still does not emit global `mode`.

v0.3.0 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.

## v0.2.6 fixes
```

Keep the existing v0.2.6 bullets and every older section after that heading. Expand the current Object configuration panel and accepted-limit prose in the shared body to match README; do not rewrite unrelated historical claims.

- [ ] **Step 4: Run documentation tests and confirm GREEN**

```bash
go test ./ -run 'Test(DocumentationListsConfigAndLimits|ReleaseNotesCompatibilityTargetsCurrentVersion)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit docs and release notes from the parent session**

The parallel documentation worker returns without touching git. After the parent session reruns Step 4 and the Task 7 benchmark check, it runs:

```bash
git add main_test.go README.md RELEASE_NOTES.md
git commit -m "docs: prepare censorship v0.3.0" -- main_test.go README.md RELEASE_NOTES.md
```

---

### Task 7: Performance evidence for compiled fast paths

**Files:**
- Modify: `benchmark_test.go`

**Interfaces:**
- Consumes: settled runtime implementation.
- Produces: benchmark evidence; no new public abstraction.

- [ ] **Step 1: Add representative filter benchmarks**

Add `BenchmarkRequestFilter` sub-benchmarks using a precompiled snapshot and one stable `pluginapi.RequestInterceptRequest`:

- `disabled` with both lists empty.
- `model-exact` with exact `RequestedModel`.
- `api-exact` with precomputed matching `caller_scope` and no headers.
- `api-wildcard` with matching scope and `Authorization: Bearer test-key`.
- `and-both` with matching wildcard API key and model.
- `or-model-decisive` with matching model and a configured wildcard API pattern, proving the cheap model branch returns before header scanning.
- `and-model-decisive` with nonmatching model and a configured wildcard API pattern, proving the cheap model branch returns before header scanning.

Use `b.ReportAllocs()` and assign the boolean result to a package-level sink. Do not set pass/fail thresholds before measuring.

- [ ] **Step 2: Run benchmark and record measured output in the task transcript**

```bash
go test ./ -run '^$' -bench '^BenchmarkRequestFilter$' -benchmem -count=5
```

Expected behavioral checks inside the benchmark must pass. Disabled and exact paths should avoid credential header scanning; wildcard cost is measured rather than guessed.

- [ ] **Step 3: Verify the measured fast paths without a second code-review dispatch**

The benchmark worker checks only its assertions and measurements, then returns without touching git. It must not launch a simplifier or rescan the implementation files that Task 8's single Opus reviewer will inspect. The parent session runs:

```bash
go test ./ -count=1
go test ./ -run '^$' -bench '^BenchmarkRequestFilter$' -benchmem -count=1
```

Expected: PASS. The disabled and exact cases do not allocate wildcard rune subjects; both model-decisive combined cases finish without credential header work. No threshold or speculative cache is added.

- [ ] **Step 4: Commit benchmark evidence from the parent session**

```bash
git add benchmark_test.go
git commit -m "perf: verify request filter fast paths" -- benchmark_test.go
```

---

### Task 8: Opus review and verified fixes

**Files:**
- Review: only implementation files committed after this plan's own commit
- Modify: only files containing confirmed findings

**Interfaces:**
- Consumes: complete implementation diff and design spec.
- Produces: one reviewed, corrected feature branch with no unverified finding applied.

- [ ] **Step 1: Run one Opus 1M/xhigh review context over the diff**

Resolve the implementation-only base before dispatch:

```bash
plan_commit="$(git log -1 --format=%H -- docs/superpowers/plans/2026-09-14-request-filter-implementation.md)"
test -n "$plan_commit"
git diff --name-only "$plan_commit"..HEAD
```

Pass `plan_commit..HEAD` to one reviewer. Do not use `735944b..HEAD`, because that would include and repeat-review this implementation plan. Because multiple agents must not repeat the same scan, the reviewer reads the implementation diff once and performs these lenses sequentially:

1. spec conformance and truth tables;
2. parser strictness and error leakage;
3. caller_scope security and forged-header resistance;
4. Unicode glob correctness and complexity;
5. no-op/body/header preservation;
6. concurrency and lifecycle;
7. panel field order and docs;
8. integration/release completeness;
9. simplification and performance.

The reviewer must attempt to refute each potential finding before retaining it and return file/line, concrete failure scenario, severity and minimal fix. It must not read old plan/spec files or CLIProxyAPI unrelated code.

- [ ] **Step 2: Reproduce every retained finding before editing**

For each finding:

- add or identify the smallest failing focused test;
- run it and capture RED;
- if the failure does not reproduce and source reasoning disproves it, record it as rejected and do not edit;
- if it reproduces, apply the minimal root-cause change and rerun GREEN.

Invoke `superpowers:receiving-code-review` before applying reviewer feedback.

- [ ] **Step 3: Run affected focused tests after fixes**

```bash
go test ./ -run 'Test(RequestFilter|ParseConfigYAMLRequestFilter|RegistrationExposesEditableConfigFields|Reconfigure|ConcurrentReconfigure)' -count=1
```

When a confirmed finding touches host behavior, run the supported full runner:

```bash
make integration
```

Expected: PASS; the runner has no test-name passthrough.

- [ ] **Step 4: Commit review fixes only when files changed**

```bash
git add request_filter.go request_filter_test.go config.go config_test.go main.go main_test.go benchmark_test.go integration/http_test.go integration/websocket_test.go README.md RELEASE_NOTES.md
git commit -m "fix: address v0.3.0 review findings"
```

Stage only confirmed-finding files. If review retains no finding, create no empty commit.

---

### Task 9: Full local verification and release package

**Files:**
- Verify only; generated `.integration/` and `dist/` remain unedited and uncommitted.

**Interfaces:**
- Consumes: final feature branch.
- Produces: fresh command evidence required before completion or merge.

- [ ] **Step 1: Invoke verification discipline**

Invoke `superpowers:verification-before-completion`. Do not claim completion from earlier focused runs.

- [ ] **Step 2: Confirm branch diff and clean tracked state**

```bash
git status --short --branch
git diff --check main...HEAD
git log --oneline --decorate main..HEAD
```

Expected: only intentional commits, no tracked or untracked source changes, no whitespace errors.

- [ ] **Step 3: Run the exact CI test gate**

Run each command and retain its exit status/output:

```bash
make test
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
make race
make vet
make integration
```

Expected: every command exits 0. Diagnose any failure with `superpowers:systematic-debugging`; do not skip or weaken tests.

- [ ] **Step 4: Build the host release-version package and aggregate manifest**

Start from generated output only, then exercise both Makefile package modes:

```bash
make clean
make package GOOS="$(go env GOHOSTOS)" GOARCH="$(go env GOHOSTARCH)" VERSION=v0.3.0
make package VERSION=v0.3.0
```

Expected: all commands exit 0. The first `make package` invokes `package-platform` and creates the host tuple ZIP plus lowercase `.zip.sha256`; the second packages discovered `dist/<goos>_<goarch>` libraries and writes `dist/checksums.txt`. Do not run redundant `make build` because `package-platform` already calls `build-platform`.

- [ ] **Step 5: Verify local package checksums and repository cleanliness**

Verify both the per-archive sidecar and aggregate file from inside `dist` so their basename entries resolve correctly:

```bash
host_os="$(go env GOHOSTOS)"
host_arch="$(go env GOHOSTARCH)"
(
	cd dist
	sha256sum --check "censorship_0.3.0_${host_os}_${host_arch}.zip.sha256"
	sha256sum --check checksums.txt
	test "$(wc -l < checksums.txt)" -eq 1
)
git status --short --branch
git diff --check main...HEAD
```

Expected: both checksum commands pass, the host-only aggregate has one line, generated files are ignored, and the source tree is clean.

- [ ] **Step 6: Invoke branch finishing workflow**

Invoke `superpowers:finishing-a-development-branch`. The user has already selected the outcome: merge into local `main`, then push and release. Do not ask for another choice.

---

### Task 10: Merge local main, push, tag, and verify GitHub release

**Files:**
- Git refs and GitHub release only; no source edits after verification unless a remote failure reveals a reproducible source defect.

**Interfaces:**
- Consumes: clean, fully verified `worktree-request-filter-v0.3.0` branch.
- Produces: local and remote `main` at the feature commit, tag `v0.3.0`, successful Build workflow, published release with 15 verified assets.

- [ ] **Step 1: Fetch and verify the integration base has not diverged unexpectedly**

```bash
git fetch origin --prune
git status --short --branch
git merge-base --is-ancestor origin/main HEAD
```

Expected: clean feature branch and `origin/main` is an ancestor. If remote main advanced, merge or rebase only through a non-interactive, conflict-safe path after running `superpowers:systematic-debugging` or `mattpocock-skills:resolving-merge-conflicts` as applicable, then rerun Task 9.

- [ ] **Step 2: Fast-forward the original local main checkout**

Verify the original checkout is on clean `main`:

```bash
git -C "C:/Users/user/Downloads/cpa-plugin-censorship" status --short --branch
```

Then fast-forward only:

```bash
git -C "C:/Users/user/Downloads/cpa-plugin-censorship" merge --ff-only worktree-request-filter-v0.3.0
```

Expected: local `main` points to the final feature commit with no merge conflict and no unrelated local changes.

- [ ] **Step 3: Push main and identify its Build workflow run**

```bash
git -C "C:/Users/user/Downloads/cpa-plugin-censorship" push origin main
```

Print the pushed commit SHA and current candidate runs:

```bash
main_sha="$(git -C "C:/Users/user/Downloads/cpa-plugin-censorship" rev-parse main)"
printf 'main_sha=%s\n' "$main_sha"
gh run list --repo DoingDog/cpa-plugin-censorship --workflow build.yml --branch main --event push --limit 10 --json databaseId,headSha,status,conclusion,url
```

Do not select a run by timing alone.

- [ ] **Step 4: Wait for main Build workflow success**

Poll only until the run for the exact pushed SHA is visible, then watch it:

```bash
main_sha="$(git -C "C:/Users/user/Downloads/cpa-plugin-censorship" rev-parse main)"
main_run_id=""
for attempt in $(seq 1 20); do
	main_run_id="$(gh run list --repo DoingDog/cpa-plugin-censorship --workflow build.yml --branch main --event push --limit 20 --json databaseId,headSha --jq "map(select(.headSha == \"$main_sha\"))[0].databaseId // empty")"
	if test -n "$main_run_id"; then break; fi
	sleep 15
done
test -n "$main_run_id"
gh run watch "$main_run_id" --repo DoingDog/cpa-plugin-censorship --exit-status
gh run view "$main_run_id" --repo DoingDog/cpa-plugin-censorship --json conclusion,jobs,url,headSha
```

Expected: conclusion `success`, matching SHA, `test` and all seven platform build/package jobs successful. A failed run must be inspected with `gh run view "$main_run_id" --repo DoingDog/cpa-plugin-censorship --log-failed`; fix only reproducible defects, commit on the feature branch, rerun Task 9, fast-forward/push main again, then watch the replacement run.

- [ ] **Step 5: Create and push v0.3.0 on the verified main commit**

Before creation:

```bash
git ls-remote --refs origin refs/tags/v0.3.0
git -C "C:/Users/user/Downloads/cpa-plugin-censorship" tag --list v0.3.0
```

Expected: both empty. Then:

```bash
git -C "C:/Users/user/Downloads/cpa-plugin-censorship" tag -a v0.3.0 -m "Censorship v0.3.0"
git -C "C:/Users/user/Downloads/cpa-plugin-censorship" push origin v0.3.0
```

Do not move or overwrite an existing tag.

- [ ] **Step 6: Wait for the tag Build/release workflow**

Poll for the tag push run whose `headSha` equals the peeled tag commit, then watch it:

```bash
tag_sha="$(git -C "C:/Users/user/Downloads/cpa-plugin-censorship" rev-parse 'v0.3.0^{}')"
tag_run_id=""
for attempt in $(seq 1 20); do
	tag_run_id="$(gh run list --repo DoingDog/cpa-plugin-censorship --workflow build.yml --branch v0.3.0 --event push --limit 20 --json databaseId,headSha --jq "map(select(.headSha == \"$tag_sha\"))[0].databaseId // empty")"
	if test -n "$tag_run_id"; then break; fi
	sleep 15
done
test -n "$tag_run_id"
gh run watch "$tag_run_id" --repo DoingDog/cpa-plugin-censorship --exit-status
gh run view "$tag_run_id" --repo DoingDog/cpa-plugin-censorship --json conclusion,jobs,url,headSha
```

Expected: `success`; matching SHA, `test`, all seven platform build/package jobs, and `release` succeeded. On failure, inspect only that run with `gh run view "$tag_run_id" --repo DoingDog/cpa-plugin-censorship --log-failed`.

- [ ] **Step 7: Verify the published release and exact asset set**

Run executable assertions rather than inspecting displayed JSON:

```bash
test "$(gh release view v0.3.0 --repo DoingDog/cpa-plugin-censorship --json isDraft --jq .isDraft)" = "false"
test "$(gh release view v0.3.0 --repo DoingDog/cpa-plugin-censorship --json tagName --jq .tagName)" = "v0.3.0"
test "$(gh release view v0.3.0 --repo DoingDog/cpa-plugin-censorship --json assets --jq '.assets | length')" -eq 15
expected_assets="$(printf '%s\n' \
	checksums.txt \
	censorship_0.3.0_darwin_amd64.zip \
	censorship_0.3.0_darwin_amd64.zip.sha256 \
	censorship_0.3.0_darwin_arm64.zip \
	censorship_0.3.0_darwin_arm64.zip.sha256 \
	censorship_0.3.0_freebsd_amd64.zip \
	censorship_0.3.0_freebsd_amd64.zip.sha256 \
	censorship_0.3.0_linux_amd64.zip \
	censorship_0.3.0_linux_amd64.zip.sha256 \
	censorship_0.3.0_linux_arm64.zip \
	censorship_0.3.0_linux_arm64.zip.sha256 \
	censorship_0.3.0_windows_amd64.zip \
	censorship_0.3.0_windows_amd64.zip.sha256 \
	censorship_0.3.0_windows_arm64.zip \
	censorship_0.3.0_windows_arm64.zip.sha256 | sort)"
actual_assets="$(gh release view v0.3.0 --repo DoingDog/cpa-plugin-censorship --json assets --jq '[.assets[].name] | sort | .[]')"
test "$actual_assets" = "$expected_assets"
release_body="$(gh release view v0.3.0 --repo DoingDog/cpa-plugin-censorship --json body --jq .body)"
committed_body="$(git show HEAD:RELEASE_NOTES.md)"
test "$release_body" = "$committed_body"
gh release view v0.3.0 --repo DoingDog/cpa-plugin-censorship --json url --jq .url
```

Expected: every assertion exits 0, proving non-draft state, exact tag, exact 15-name asset set and release body parity with the committed notes.

- [ ] **Step 8: Independently download and verify every checksum**

Create a unique temp directory outside the repo, download all assets, and run every sidecar plus aggregate checksum verification. On Git Bash:

```bash
verify_dir="$(mktemp -d -t censorship-v0.3.0-verify.XXXXXX)"
trap 'rm -rf "$verify_dir"' EXIT
gh release download v0.3.0 --repo DoingDog/cpa-plugin-censorship --dir "$verify_dir"
(
	cd "$verify_dir"
	for checksum in *.zip.sha256; do sha256sum --check "$checksum" || exit 1; done
	sha256sum --check checksums.txt
	test "$(wc -l < checksums.txt)" -eq 7
)
rm -rf "$verify_dir"
trap - EXIT
test ! -e "$verify_dir"
```

Expected: seven sidecars and seven aggregate entries verify successfully, `checksums.txt` has exactly seven lines, and the temporary download directory is removed even if verification fails.

- [ ] **Step 9: Final consistency check**

```bash
git -C "C:/Users/user/Downloads/cpa-plugin-censorship" status --short --branch
git -C "C:/Users/user/Downloads/cpa-plugin-censorship" rev-parse main
git -C "C:/Users/user/Downloads/cpa-plugin-censorship" rev-parse 'v0.3.0^{}'
git ls-remote origin refs/heads/main refs/tags/v0.3.0 'refs/tags/v0.3.0^{}'
```

Expected: local main is clean; local main, the peeled local tag, remote main and the peeled remote tag identify the same final commit. The annotated tag object's own SHA is expected to differ.

- [ ] **Step 10: Remove temporary task memory and reference clone**

After release verification only:

- remove `active-request-filter-task.md` from the Claude project memory directory;
- remove its one-line entry from `MEMORY.md`;
- remove `C:\Users\user\AppData\Local\Temp\cpa-plugin-model-mapper-ref-20260914-opus5`;
- preserve all durable project commits, source, test logs referenced in the final report, and the release verification result.

- [ ] **Step 11: Report concrete evidence**

Final report must include:

- final commit SHA and tag;
- local verification commands and their actual outcomes;
- main and tag workflow URLs/conclusions;
- release URL, asset count and checksum result;
- any intentionally unsupported behavior, specifically wildcard query credential and Principal-not-equal-literal-key no-match;
- no claim that skipped or failed checks passed.
