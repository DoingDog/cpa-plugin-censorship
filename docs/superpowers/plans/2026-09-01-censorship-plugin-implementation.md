# CLIProxyAPI Censorship Plugin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从空仓库实现一个request-only的CLIProxyAPI动态插件，按YAML顺序在五种SourceFormat中执行可配置大小写语义的 `block`、`strip`或 `obfs`，支持非Home热更新，并发布七个平台artifact。

**Architecture:** CPA ABI层只把RPC分派给 `main.go`；每次BeforeAuth只load一次immutable `configSnapshot`，selector模块只返回原始JSON string token的span，matcher和transform模块在decoded text上执行规则并一次重建body。各协议selector使用独立文件和测试文件，以便实现与单文件review并行；插件不注册任何response-side capability。

**Tech Stack:** Go 1.26.0、CGO `-buildmode=c-shared`、CLIProxyAPI ABI/RPC schema v4、`github.com/tidwall/gjson@v1.18.0`、`gopkg.in/yaml.v3@v3.0.1`、Go标准库、GitHub Actions。

**Spec:** `docs/superpowers/specs/2026-09-01-censorship-plugin-design.md`

## Global Constraints

- CLIProxyAPI依赖固定为 `github.com/router-for-me/CLIProxyAPI/v7 v7.2.147-0.20260831020448-81e1b5374f99`，对应commit `81e1b5374f99c212f196f34956eeed964a46b8fa`。
- 所有生产代码使用 `package main`；不得修改CLIProxyAPI，不得增加单实现interface、factory、通用递归JSON walker、内容hash cache或conversation state。
- 唯一词表来源是YAML `words`；不得内置、下载或补全词条。
- `ignore_case`默认 `false`；显式 `true`采用 `unicode.SimpleFold`等价类，不采用full case folding，不执行Unicode normalization。
- 大小写敏感路径必须继续使用 `strings.Contains`和 `strings.ReplaceAll` fast path。
- block按rule-major、document-minor顺序选择首个命中，HTTP 400回显YAML原词和canonical role。
- strip和obfs按rule-major顺序处理每个node中全部左到右非重叠occurrence；后一条规则读取前一条规则的结果。
- obfs只允许U+200B或U+2060，每个match在实际原文首个Unicode scalar后插入一次，保留原文大小写。
- selector只进入规格第7节列出的leaf；JSON key、tool call、arguments、thinking、machine result、media/base64和unknown typed node始终排除。
- no-op不得重建或回传replacement body；changed body只重新编码已变化的JSON string token，span外bytes逐字节不变。
- BeforeAuth只load一次snapshot；AfterAuth固定no-op；registration只声明 `RequestInterceptor`，`Metadata.ConfigFields`为空。
- 相同body、snapshot和 `pluginVersion`必须产生相同返回类型、status、headers和body bytes；不要求幂等。
- 每个实现任务严格执行RED命令、确认预期失败、最小GREEN、当前全套测试、独立review、commit。
- 不允许两个并行实现agent编辑同一文件。共享接口任务完成后，协议selector、matcher、packager和integration harness可以并行；单文件review可以全并行，跨模块review在合并后执行。
- 所有命令从仓库根目录运行。commit不得使用 `--no-verify`。

## Frozen Internal Interfaces

后续任务必须使用以下名称和签名，不得自行改名：

```go
type mode string

type compiledRule struct {
	Term  string
	Runes []rune
}

type scopeSet map[string]struct{}

type configSnapshot struct {
	Mode       mode
	IgnoreCase bool
	Rules      []compiledRule
	Formats    scopeSet
	Roles      scopeSet
	ObfsChar   string
}

type textSpan struct {
	RawStart int
	RawEnd   int
	Text     string
	Role     string
	Changed  bool
}

type blockMatch struct {
	Term string
	Role string
}

type transformResult struct {
	Body    []byte
	Blocked *blockMatch
	Invalid bool
}

func parseConfigYAML(raw []byte) (*configSnapshot, error)
func installSnapshot(next *configSnapshot)
func loadedSnapshot() *configSnapshot
func transformRequest(body []byte, sourceFormat string, cfg *configSnapshot) (transformResult, error)
func selectTextSpans(body []byte, sourceFormat string, roles scopeSet) ([]textSpan, error)
func (s scopeSet) has(value string) bool
func appendStringSpan(spans *[]textSpan, value gjson.Result, role string, roles scopeSet)
func geminiTextPartAllowed(part gjson.Result) bool
func applyMode(spans []textSpan, cfg *configSnapshot) (*blockMatch, bool)
func rebuildBody(body []byte, spans []textSpan) ([]byte, error)
func containsRule(text string, rule compiledRule, ignoreCase bool) bool
func stripRule(text string, rule compiledRule, ignoreCase bool) (string, bool)
func obfuscateRule(text string, rule compiledRule, ignoreCase bool, char string) (string, bool)
```

`selectTextSpans`按 `RawStart`升序返回不重叠span。协议文件只能调用 `appendStringSpan`，不得调用matcher、mode engine或body重建函数。

## File Map

```plaintext
abi_cgo.go                         ABI v1 init/call/free/shutdown和host callback bridge
main.go                            pluginVersion、RPC envelope、registration、BeforeAuth/AfterAuth
main_test.go                       公开RPC dispatcher和capability测试
config.go                          strict YAML parser、immutable snapshot、atomic store
config_test.go                     defaults、validation、register/reconfigure和并发测试
matcher.go                         sensitive fast path、SimpleFold匹配和replacement
matcher_test.go                    occurrence、Unicode fold、cascade与确定性单元测试
transform.go                       mode engine、direct error body和一次body重建
transform_test.go                  block/strip/obfs、byte preservation与确定性
selectors.go                       JSON object validation、dispatcher、span排序/校验、共享predicate
selectors_openai.go                openai和openai-response selector
selectors_openai_test.go           两种OpenAI格式所有row与hard exclusions
selectors_claude.go                Claude selector
selectors_claude_test.go           Claude system/message/tool_result范围
selectors_gemini.go                Gemini selector与machine discriminator排除
selectors_gemini_test.go           Gemini两种system key、role和part排除
selectors_interactions.go          Interactions受限item/steps递归
selectors_interactions_test.go     Interactions全部shape、继承role和排除
fuzz_test.go                       independent oracle和protocol fuzz targets
benchmark_test.go                  资源矩阵与unit/RPC benchmark
integration/doc.go                 默认build中的空integration package
integration/harness_test.go        build tag integration，共享CPA process/mock upstream
integration/http_test.go           HTTP、SSE、watcher热更新与response透明性
integration/websocket_test.go      Responses WebSocket model-executed turn与block限制
integration/abi_benchmark_test.go  固定CPA pluginhost动态ABI benchmark
.github/scripts/integration-runner.go  固定SHA checkout/build/copy/test orchestration
.github/scripts/integration-runner_test.go runner path、SHA和extension单元测试
.github/scripts/package-release.go     七平台zip和checksum packager
.github/scripts/package-release_test.go packager与version normalization测试
.github/workflows/build.yml        test、七平台build、artifact和tag release
Makefile                           test/race/vet/integration/build/package targets
README.md                          YAML、行为、覆盖范围、限制、构建和发布
RELEASE_NOTES.md                   GitHub Release配置、行为、限制和artifact说明
.gitignore                         dist和integration临时目录
```

## Dependency Graph and Workflow Ownership

1. Task 1到Task 4串行，冻结RPC、config、span、dispatcher和mode接口；Task 3一次创建五个协议文件的可编译no-op collector，后续协议任务不再编辑共享 `selectors.go`。
2. Task 5到Task 7串行完成SimpleFold matcher、strip、obfs和raw rebuild。
3. Task 7后启动四条互不写同一文件的协议线：OpenAI线按Task 8 -> Task 9串行；Task 10 Claude、Task 11 Gemini、Task 12 Interactions各自并行。Task 11和Task 12共同使用Task 3已冻结的 `geminiTextPartAllowed`，不互相依赖。
4. Task 13依赖Task 2和Task 7，可与协议线并行实现atomic reconfigure，因为它只编辑ABI、config和main文件；合并前必须确认协议任务没有编辑这些文件。
5. Task 14在Task 8到Task 13全部完成后运行跨模块fuzz。Task 15 unit/RPC benchmark和Task 16 packager可在Task 14后并行。
6. Task 17依赖所有runtime模块并实现integration runner、HTTP/SSE/WebSocket测试和动态ABI benchmark。Task 18依赖Task 16与Task 17，写Makefile、workflow和release notes。Task 19依赖全部代码，写README、同步release notes并执行最终verification。
7. 每个task完成后先运行其定向测试，再运行 `go test ./...`，review通过后才commit。并行任务由各自agent commit；同一主分支实施时按依赖顺序执行，隔离worktree实施时按依赖顺序cherry-pick，禁止并行写共享worktree。

---

### Task 1: Go Module、ABI和Request-only Registration

**Files:**
- Create: `go.mod`
- Create: `go.sum`
- Create: `abi_cgo.go`
- Create: `main.go`
- Create: `main_test.go`
- Create: `.gitignore`

**Interfaces:**
- Consumes: CLIProxyAPI `pluginabi.ABIVersion == 1`、`pluginabi.SchemaVersion == 4`、`pluginapi.RequestInterceptRequest`和 `pluginapi.RequestInterceptResponse`。
- Produces: `handleMethod(method string, request []byte) ([]byte, error)`、`pluginRegistration() registration`、`interceptBeforeAuth([]byte)`、`interceptAfterAuth([]byte)`、`setHostCallback(hostCallback)`，以及测试helper `mustHandle`、`decodeEnvelope`、`decodeResult`和 `lifecycleJSON`。

- [ ] **Step 1: Create the module and write the failing registration test**

Create `go.mod`:

```go
module github.com/DoingDog/cpa-plugin-censorship

go 1.26.0

require github.com/router-for-me/CLIProxyAPI/v7 v7.2.147-0.20260831020448-81e1b5374f99
```

Create `main_test.go` with the public dispatcher assertion:

```go
package main

import (
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestRegistrationDeclaresOnlyRequestInterceptor(t *testing.T) {
	raw, err := handleMethod(pluginabi.MethodPluginRegister, []byte(`{"config_yaml":"","schema_version":4}`))
	if err != nil {
		t.Fatalf("handleMethod() error = %v", err)
	}
	var env pluginabi.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("register envelope = %s", raw)
	}
	var got struct {
		SchemaVersion uint32             `json:"schema_version"`
		Metadata      pluginapi.Metadata `json:"metadata"`
		Capabilities  map[string]bool    `json:"capabilities"`
	}
	if err := json.Unmarshal(env.Result, &got); err != nil {
		t.Fatalf("decode registration: %v", err)
	}
	if got.SchemaVersion != pluginabi.SchemaVersion || got.Metadata.Name != "censorship" || got.Metadata.Version != pluginVersion {
		t.Fatalf("registration = %#v", got)
	}
	if got.Metadata.Author == "" || got.Metadata.GitHubRepository == "" || got.Metadata.ConfigFields == nil || len(got.Metadata.ConfigFields) != 0 {
		t.Fatalf("metadata = %#v", got.Metadata)
	}
	if !got.Capabilities["request_interceptor"] {
		t.Fatalf("capabilities = %#v", got.Capabilities)
	}
	for _, name := range []string{"request_lifecycle_plugin", "response_interceptor", "response_stream_interceptor", "websocket_response_observer", "management_api", "model_router", "executor"} {
		if got.Capabilities[name] {
			t.Fatalf("capability %q unexpectedly enabled", name)
		}
	}
}

func mustHandle(t *testing.T, method string, request []byte) []byte {
	t.Helper()
	raw, err := handleMethod(method, request)
	if err != nil {
		t.Fatalf("handleMethod(%q): %v", method, err)
	}
	return raw
}

func decodeEnvelope(t *testing.T, raw []byte, env *pluginabi.Envelope) {
	t.Helper()
	if err := json.Unmarshal(raw, env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
}

func decodeResult[T any](t *testing.T, env pluginabi.Envelope) T {
	t.Helper()
	if !env.OK {
		t.Fatalf("RPC envelope error = %#v", env.Error)
	}
	var result T
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return result
}

func lifecycleJSON(t *testing.T, configYAML string) []byte {
	t.Helper()
	raw, err := json.Marshal(lifecycleRequest{
		ConfigYAML:    []byte(configYAML),
		SchemaVersion: pluginabi.SchemaVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
```

- [ ] **Step 2: Run the RED test**

Run: `go test ./... -run '^TestRegistrationDeclaresOnlyRequestInterceptor$'`

Expected: FAIL to compile because `handleMethod` and `pluginVersion` do not exist.

- [ ] **Step 3: Implement the minimal RPC and registration**

Create `main.go` with the link-injectable variable `var pluginVersion = "0.0.0-dev"`, `main()`, lifecycle/envelope dispatch, metadata `Author: "DoingDog"`, `GitHubRepository: "https://github.com/DoingDog/cpa-plugin-censorship"`, an empty non-nil `ConfigFields`, and these exact capability fields:

```go
type registrationCapabilities struct {
	RequestInterceptor        bool `json:"request_interceptor"`
	RequestLifecyclePlugin    bool `json:"request_lifecycle_plugin"`
	ResponseInterceptor       bool `json:"response_interceptor"`
	StreamChunkInterceptor    bool `json:"response_stream_interceptor"`
	WebSocketResponseObserver bool `json:"websocket_response_observer"`
	ManagementAPI             bool `json:"management_api"`
	ModelRouter               bool `json:"model_router"`
	Executor                  bool `json:"executor"`
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             "censorship",
			Version:          pluginVersion,
			Author:           "DoingDog",
			GitHubRepository: "https://github.com/DoingDog/cpa-plugin-censorship",
			ConfigFields:     []pluginapi.ConfigField{},
		},
		Capabilities: registrationCapabilities{RequestInterceptor: true},
	}
}
```

`handleMethod` must return `pluginabi.Envelope` for `plugin.register`, `request.intercept_before` and `request.intercept_after`; unknown methods return `ok:false` with code `unknown_method`. Both request methods initially return an empty `pluginapi.RequestInterceptResponse`.

Create `abi_cgo.go` with build tag `//go:build cgo`, the ABI structs from CPA `examples/plugin/request-lifecycle/go/main.go`, host ABI version/callback validation, exported `cliproxy_plugin_init`, `cliproxyPluginCall`, `cliproxyPluginFree`, `cliproxyPluginShutdown`, and the model-mapper pattern that installs a Go `hostCallback` through `setHostCallback`.

Create `.gitignore`:

```gitignore
dist/
.integration/
*.h
```

- [ ] **Step 4: Verify registration and ABI build**

Run:

```bash
go mod tidy
go test ./... -run '^TestRegistrationDeclaresOnlyRequestInterceptor$'
mkdir -p dist/test
go build -trimpath -buildmode=c-shared -o dist/test/censorship.dll .
```

Expected: all commands exit 0; the build creates `dist/test/censorship.dll` and `dist/test/censorship.h` on Windows.

- [ ] **Step 5: Review and commit**

Review only `abi_cgo.go`, `main.go`, `main_test.go`, `go.mod` and `.gitignore`; verify no response-side RPC method is dispatched.

```bash
git add go.mod go.sum abi_cgo.go main.go main_test.go .gitignore
git commit -m "feat: register request censorship plugin"
```

---

### Task 2: Strict YAML Config and Immutable Snapshot

**Files:**
- Create: `config.go`
- Create: `config_test.go`
- Modify: `main.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: `handleMethod` and lifecycle `config_yaml` bytes from Task 1.
- Produces: all frozen config types, `parseConfigYAML([]byte) (*configSnapshot, error)`, `defaultSnapshot() *configSnapshot`, `installSnapshot(*configSnapshot)` and `loadedSnapshot() *configSnapshot`.

- [ ] **Step 1: Write the failing config table test**

Create `config_test.go`:

```go
package main

import (
	"reflect"
	"sort"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

func TestParseConfigYAMLDefaultsAndValidation(t *testing.T) {
	defaults, err := parseConfigYAML(nil)
	if err != nil {
		t.Fatalf("parse defaults: %v", err)
	}
	if defaults.Mode != modeBlock || defaults.IgnoreCase || len(defaults.Rules) != 0 || defaults.ObfsChar != "​" {
		t.Fatalf("defaults = %#v", defaults)
	}
	if !reflect.DeepEqual(sortedKeys(defaults.Formats), []string{"claude", "gemini", "interactions", "openai", "openai-response"}) {
		t.Fatalf("formats = %v", sortedKeys(defaults.Formats))
	}
	if !reflect.DeepEqual(sortedKeys(defaults.Roles), []string{"developer", "system", "user"}) {
		t.Fatalf("roles = %v", sortedKeys(defaults.Roles))
	}

	invalid := []string{
		"[]\n",
		"scalar\n",
		"---\n{}\n---\n{}\n",
		"mode: nope\n",
		"mode: [block]\n",
		"mode: block\nmode: strip\n",
		"ignore_case: yes\n",
		"ignore_case: 'true'\n",
		"words: null\n",
		"words: ok\n",
		"words: [ok, '']\n",
		"words: [1]\n",
		"scope: []\n",
		"scope:\n  formats: openai\n",
		"scope:\n  formats: [OpenAI]\n",
		"scope:\n  formats: [openai]\n  formats: [claude]\n",
		"scope:\n  roles: {user: true}\n",
		"scope:\n  roles: [function]\n",
		"scope:\n  unknown: []\n",
		"obfs: []\n",
		"obfs:\n  char: [x]\n",
		"obfs:\n  char: x\n",
		"obfs:\n  char: '​'\n  char: '⁠'\n",
		"obfs:\n  unknown: x\n",
		"mode: obfs\nwords: [x]\n",
		"mode: obfs\nwords: ['a​b']\n",
		"word: [typo]\n",
	}
	for _, raw := range invalid {
		if _, err := parseConfigYAML([]byte(raw)); err == nil {
			t.Errorf("parseConfigYAML(%q) error = nil", raw)
		}
	}
}
```

Add the exact-order, empty-scope and duplicate-scope tests plus shared config helpers:

```go
func TestParseConfigPreservesRuleOrderWhitespaceAndDuplicates(t *testing.T) {
	cfg := mustConfig(t, "ignore_case: true\nwords: [' b ', a, a]\nscope:\n  formats: []\n  roles: []\n")
	got := []string{cfg.Rules[0].Term, cfg.Rules[1].Term, cfg.Rules[2].Term}
	if want := []string{" b ", "a", "a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rules = %#v, want %#v", got, want)
	}
	if !cfg.IgnoreCase || len(cfg.Formats) != 0 || len(cfg.Roles) != 0 {
		t.Fatalf("snapshot = %#v", cfg)
	}

	duplicates := mustConfig(t, "words: [x]\nscope:\n  formats: [openai, openai]\n  roles: [user, user]\n")
	if len(duplicates.Formats) != 1 || len(duplicates.Roles) != 1 {
		t.Fatalf("duplicate scopes = %#v %#v", duplicates.Formats, duplicates.Roles)
	}
}

func sortedKeys(set scopeSet) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mustConfig(t *testing.T, raw string) *configSnapshot {
	t.Helper()
	cfg, err := parseConfigYAML([]byte(raw))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return cfg
}

func TestRegisterRejectsInvalidConfigAndAcceptsHostOwnedKeys(t *testing.T) {
	raw := lifecycleJSON(t, "ignore_case: yes\nwords: [x]\n")
	var env pluginabi.Envelope
	decodeEnvelope(t, mustHandle(t, pluginabi.MethodPluginRegister, raw), &env)
	if env.OK || env.Error == nil {
		t.Fatalf("invalid register envelope = %#v", env)
	}
	valid := "enabled: true\npriority: 7\nstore:\n  version: 1.2.3\nwords: [x]\n"
	decodeEnvelope(t, mustHandle(t, pluginabi.MethodPluginRegister, lifecycleJSON(t, valid)), &env)
	if !env.OK {
		t.Fatalf("host-owned keys rejected: %#v", env)
	}
}
```

- [ ] **Step 2: Run the RED test**

Run: `go test ./... -run '^Test(ParseConfig|RegisterRejects)'`

Expected: FAIL to compile because `parseConfigYAML`, mode constants and snapshot types do not exist.

- [ ] **Step 3: Implement the strict YAML parser**

Add `gopkg.in/yaml.v3 v3.0.1` and implement `config.go` with:

```go
const (
	modeBlock mode = "block"
	modeStrip mode = "strip"
	modeObfs  mode = "obfs"
)

var activeConfig atomic.Pointer[configSnapshot]

func init() {
	activeConfig.Store(defaultSnapshot())
}

func installSnapshot(next *configSnapshot) {
	activeConfig.Store(next)
}

func loadedSnapshot() *configSnapshot {
	return activeConfig.Load()
}
```

Parse into `yaml.Node`, require zero or one document and a top-level mapping. Walk mappings explicitly, reject duplicate mapping keys and unknown plugin-owned keys, accept and ignore host-owned `enabled`, `priority` and `store`. Require `mode` and `obfs.char` to be string scalars when present, `ignore_case` to be `!!bool`, `words`/formats/roles to be sequences whose elements have tag `!!str`, and `scope`/`obfs` to be mappings with no unknown keys. Missing fields receive defaults; explicit empty sequences remain empty.

Compile every word exactly once:

```go
rules = append(rules, compiledRule{Term: node.Value, Runes: []rune(node.Value)})
```

Validate empty terms, allowed formats/roles, obfs char, at least two scalars per obfs term, and absence of the selected obfs char in every obfs term. Build fresh slices/maps for every successful parse and never mutate them after return.

Modify `plugin.register` to decode `lifecycleRequest{ConfigYAML []byte, SchemaVersion uint32}`, call `parseConfigYAML`, atomically install only after success, then return registration. An invalid first register returns `ok:false` and does not publish a candidate snapshot.

- [ ] **Step 4: Run config and full tests**

Run:

```bash
go mod tidy
go test ./... -run '^Test(ParseConfig|RegisterRejects)'
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Review and commit**

Review `config.go` alone for strict node kinds, no trimming/deduplication of words, explicit empty scope preservation and one atomic store after complete validation.

```bash
git add go.mod go.sum config.go config_test.go main.go
git commit -m "feat: parse censorship plugin config"
```

---

### Task 3: Dispatcher Early No-op and Invalid JSON Error

**Files:**
- Create: `transform.go`
- Create: `selectors.go`
- Create: `selectors_openai.go`
- Create: `selectors_claude.go`
- Create: `selectors_gemini.go`
- Create: `selectors_interactions.go`
- Modify: `main.go`
- Modify: `main_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: immutable snapshot from Task 2 and `RequestInterceptRequest` from Task 1.
- Produces: `textSpan`, `blockMatch`, `transformResult`, `transformRequest`, `selectTextSpans`, `appendStringSpan`, `applyMode`, `rebuildBody`, and deterministic direct response encoding.

- [ ] **Step 1: Write the failing early-return dispatcher test**

Add to `main_test.go`:

```go
func TestBeforeAuthEarlyNoOpsDoNotParseBody(t *testing.T) {
	cases := []struct {
		name   string
		format string
		yaml   string
	}{
		{name: "unknown format", format: "future", yaml: "words: [x]\n"},
		{name: "empty words", format: "openai", yaml: "words: []\n"},
		{name: "empty formats", format: "openai", yaml: "words: [x]\nscope:\n  formats: []\n"},
		{name: "format disabled", format: "openai", yaml: "words: [x]\nscope:\n  formats: [claude]\n"},
		{name: "empty roles", format: "openai", yaml: "words: [x]\nscope:\n  roles: []\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, tc.yaml)
			resp := interceptRPC(t, tc.format, []byte(`not-json`))
			if resp.Terminate || len(resp.Body) != 0 || len(resp.ResponseBody) != 0 {
				t.Fatalf("response = %#v", resp)
			}
		})
	}
}

func TestBeforeAuthRejectsEnabledInvalidJSON(t *testing.T) {
	registerConfig(t, "words: [x]\n")
	for _, body := range [][]byte{[]byte(`not-json`), []byte(`[]`), []byte(`null`), []byte(`"x"`)} {
		resp := interceptRPC(t, "openai", body)
		if !resp.Terminate || resp.StatusCode != 400 || resp.ResponseHeaders.Get("Content-Type") != "application/json" {
			t.Fatalf("response for %q = %#v", body, resp)
		}
		if gjson.GetBytes(resp.ResponseBody, "error.code").String() != "censorship_invalid_request" || bytes.Contains(resp.ResponseBody, body) {
			t.Fatalf("response body = %s", resp.ResponseBody)
		}
	}
}

func TestAfterAuthAlwaysNoOpsWithoutParsingRequest(t *testing.T) {
	raw := mustHandle(t, pluginabi.MethodRequestInterceptAfter, []byte(`not-json`))
	var env pluginabi.Envelope
	decodeEnvelope(t, raw, &env)
	resp := decodeResult[pluginapi.RequestInterceptResponse](t, env)
	if resp.Terminate || len(resp.Body) != 0 || len(resp.Headers) != 0 || len(resp.ResponseBody) != 0 {
		t.Fatalf("AfterAuth response = %#v", resp)
	}
}

func registerConfig(t *testing.T, configYAML string) {
	t.Helper()
	var env pluginabi.Envelope
	decodeEnvelope(t, mustHandle(t, pluginabi.MethodPluginRegister, lifecycleJSON(t, configYAML)), &env)
	if !env.OK {
		t.Fatalf("register config: %#v", env.Error)
	}
}

func callIntercept(sourceFormat string, body []byte) (pluginapi.RequestInterceptResponse, error) {
	raw, err := json.Marshal(pluginapi.RequestInterceptRequest{
		RequestID:    "censorship-test",
		SourceFormat: sourceFormat,
		Body:         body,
	})
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

func interceptRPC(t *testing.T, sourceFormat string, body []byte) pluginapi.RequestInterceptResponse {
	t.Helper()
	resp, err := callIntercept(sourceFormat, body)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

type rawReplacement struct {
	Before string
	After  string
}

func replaceRawTokens(t *testing.T, body []byte, replacements ...rawReplacement) []byte {
	t.Helper()
	out := bytes.Clone(body)
	for _, replacement := range replacements {
		before := []byte(replacement.Before)
		if count := bytes.Count(out, before); count != 1 {
			t.Fatalf("raw token %q occurs %d times", before, count)
		}
		out = bytes.Replace(out, before, []byte(replacement.After), 1)
	}
	return out
}
```

Add `bytes`、`fmt` and `github.com/tidwall/gjson` to `main_test.go` imports. Add the direct module dependency `github.com/tidwall/gjson v1.18.0` to `go.mod`; `go mod tidy` records its transitive checksums in `go.sum`. `callIntercept` is the non-fatal helper used by later concurrency tests; `interceptRPC` is its test-fatal wrapper.

- [ ] **Step 2: Run the RED test**

Run: `go test ./... -run '^Test(BeforeAuth(EarlyNoOpsDoNotParseBody|RejectsEnabledInvalidJSON)|AfterAuthAlwaysNoOpsWithoutParsingRequest)$'`

Expected: FAIL because BeforeAuth always returns no-op and does not reject enabled invalid JSON.

- [ ] **Step 3: Implement the priority and empty selector dispatcher**

In `transform.go`, implement the exact priority:

```go
func transformRequest(body []byte, sourceFormat string, cfg *configSnapshot) (transformResult, error) {
	if cfg == nil || len(cfg.Rules) == 0 || !cfg.Formats.has(sourceFormat) || len(cfg.Formats) == 0 || len(cfg.Roles) == 0 {
		return transformResult{}, nil
	}
	spans, err := selectTextSpans(body, sourceFormat, cfg.Roles)
	if errors.Is(err, errInvalidRequest) {
		return transformResult{Invalid: true}, nil
	}
	if err != nil {
		return transformResult{}, err
	}
	blocked, changed := applyMode(spans, cfg)
	if blocked != nil {
		return transformResult{Blocked: blocked}, nil
	}
	if !changed {
		return transformResult{}, nil
	}
	out, err := rebuildBody(body, spans)
	return transformResult{Body: out}, err
}
```

Unknown formats must return before this function by an exact known-format check, even if a misspelled format is accidentally present in a test snapshot. In `selectors.go`, validate with `gjson.ValidBytes`, trim only for top-level object detection, parse with `gjson.ParseBytes`, require `root.IsObject()`, then dispatch all five known formats through fixed function calls. Create each protocol file now with the final collector signature and an empty body so the package compiles:

```go
func collectOpenAI(root gjson.Result, roles scopeSet, spans *[]textSpan) {}
func collectOpenAIResponses(root gjson.Result, roles scopeSet, spans *[]textSpan) {}
func collectClaude(root gjson.Result, roles scopeSet, spans *[]textSpan) {}
func collectGemini(root gjson.Result, roles scopeSet, spans *[]textSpan) {}
func collectInteractions(root gjson.Result, roles scopeSet, spans *[]textSpan) {}
```

Place the two OpenAI collectors in `selectors_openai.go` and one collector in each other protocol file. Implement `geminiTextPartAllowed` once in `selectors.go` with the exact eight machine discriminator keys, `thought == true` and `thoughtSignature` presence rules, so Gemini and Interactions tasks can run in parallel without editing a shared file. These no-op collectors are temporary GREEN implementations for the current early-return slice; each protocol task replaces only its owned function body.

In `main.go`, BeforeAuth load `cfg := loadedSnapshot()` exactly once, call `transformRequest`, and return:

- empty `RequestInterceptResponse` for no-op;
- `Body: result.Body` for changes;
- a typed `censorship_invalid_request` termination for invalid JSON;
- a typed `censorship_blocked` termination for a future block result.

AfterAuth must not unmarshal or inspect its request and must always envelope an empty response.

Use structs, not maps, to encode direct errors in deterministic field order. Invalid error fields are `type=invalid_request_error`, `code=censorship_invalid_request`, `message=request body must be a JSON object`; never include input bytes.

- [ ] **Step 4: Run tests**

Run:

```bash
go mod tidy
go test ./... -run '^Test(BeforeAuth(EarlyNoOpsDoNotParseBody|RejectsEnabledInvalidJSON)|AfterAuthAlwaysNoOpsWithoutParsingRequest)$'
go test ./...
```

Expected: PASS, and `go.mod` contains direct `github.com/tidwall/gjson v1.18.0`.

- [ ] **Step 5: Review and commit**

Review `main.go`, `selectors.go` and `transform.go` independently, then review the call chain for exactly one snapshot load and no body parse on each early path.

```bash
git add go.mod go.sum main.go main_test.go selectors.go selectors_openai.go selectors_claude.go selectors_gemini.go selectors_interactions.go transform.go
git commit -m "feat: enforce request dispatcher boundaries"
```

---

### Task 4: OpenAI String Selector and Ordered Block

**Files:**
- Modify: `selectors_openai.go`
- Modify: `selectors.go`
- Create: `matcher.go`
- Modify: `transform.go`
- Create: `transform_test.go`

**Interfaces:**
- Consumes: `textSpan`, `appendStringSpan`, config scope and transform flow from Task 3.
- Produces: `collectOpenAI(root gjson.Result, roles scopeSet, spans *[]textSpan)` and block mode in `applyMode`.

- [ ] **Step 1: Write the failing ordered block test**

Add to `transform_test.go`:

```go
func TestOpenAIBlockUsesRuleOrderBeforeDocumentOrder(t *testing.T) {
	registerConfig(t, "mode: block\nwords: [ab, a]\n")
	body := []byte(`{"messages":[{"role":"user","content":"a"},{"role":"developer","content":"ab"}]}`)
	resp := interceptRPC(t, "openai", body)
	if !resp.Terminate || resp.StatusCode != 400 || len(resp.Body) != 0 {
		t.Fatalf("response = %#v", resp)
	}
	if string(resp.ResponseBody) != `{"error":{"type":"invalid_request_error","code":"censorship_blocked","message":"request blocked by censorship rule","term":"ab","role":"developer"}}` {
		t.Fatalf("response body = %s", resp.ResponseBody)
	}
}
```

Add document-order/escaping coverage and the shared selector-role assertion:

```go
func TestOpenAIBlockDocumentOrderAndErrorEscaping(t *testing.T) {
	registerConfig(t, "mode: block\nwords: [hit]\n")
	resp := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"system","content":"hit"},{"role":"user","content":"hit"}]}`))
	if gjson.GetBytes(resp.ResponseBody, "error.role").String() != "system" {
		t.Fatalf("response body = %s", resp.ResponseBody)
	}

	term := "quote \" and\nnewline\n"
	registerConfig(t, "mode: block\nwords:\n  - |\n    quote \" and\n    newline\n")
	body, err := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "user", "content": term}}})
	if err != nil {
		t.Fatal(err)
	}
	resp = interceptRPC(t, "openai", body)
	if !json.Valid(resp.ResponseBody) || gjson.GetBytes(resp.ResponseBody, "error.term").String() != term {
		t.Fatalf("response body = %s", resp.ResponseBody)
	}
}

func assertBlockedRole(t *testing.T, sourceFormat, body, wantRole string) {
	t.Helper()
	registerConfig(t, "mode: block\nwords: [SECRET]\nscope:\n  roles: [system, developer, user, assistant, tool]\n")
	resp := interceptRPC(t, sourceFormat, []byte(body))
	if !resp.Terminate || resp.StatusCode != 400 || gjson.GetBytes(resp.ResponseBody, "error.term").String() != "SECRET" || gjson.GetBytes(resp.ResponseBody, "error.role").String() != wantRole {
		t.Fatalf("format=%s role=%s response=%#v body=%s", sourceFormat, wantRole, resp, resp.ResponseBody)
	}
}
```

Add `encoding/json` to `transform_test.go` imports.

- [ ] **Step 2: Run the RED test**

Run: `go test ./... -run '^TestOpenAIBlockUsesRuleOrderBeforeDocumentOrder$'`

Expected: FAIL because the OpenAI collector returns no spans.

- [ ] **Step 3: Implement OpenAI string leaves and block**

`collectOpenAI` must iterate only `messages[]`, require an exact string role, accept `system`、`developer`、`user`、`assistant` string content and `tool` string content, and call `appendStringSpan`. At this task, array content is deliberately left untouched for Task 8.

`appendStringSpan` must require `value.Type == gjson.String`, the role to be enabled, and a valid raw range; it stores `value.Index`, `value.Index+len(value.Raw)`, `value.Str` and role. `selectTextSpans` sorts by `RawStart`, rejects overlap or out-of-body ranges, and returns `errInvalidSpan` for internal selector defects.

Implement block mode:

```go
for _, rule := range cfg.Rules {
	for i := range spans {
		if containsRule(spans[i].Text, rule, cfg.IgnoreCase) {
			return &blockMatch{Term: rule.Term, Role: spans[i].Role}, false
		}
	}
}
```

Create `matcher.go` with the temporary `containsRule` implementation. Until Task 5, it may use only `strings.Contains` and must return false if `ignoreCase` is true; Task 5 replaces that branch in the same file.

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./... -run '^TestOpenAIBlockUsesRuleOrderBeforeDocumentOrder$'
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Review and commit**

Review `selectors_openai.go` for no recursive scan and `transform.go` for rule-major ordering.

```bash
git add selectors.go selectors_openai.go matcher.go transform.go transform_test.go
git commit -m "feat: block ordered openai text"
```

---

### Task 5: Unicode Simple-fold Matcher

**Files:**
- Modify: `matcher.go`
- Create: `matcher_test.go`
- Modify: `main_test.go`

**Interfaces:**
- Consumes: `compiledRule{Term, Runes}` and `IgnoreCase` from Task 2.
- Produces: `containsRule`, `stripRule`, `obfuscateRule`, `findFoldMatches(text string, runes []rune) []textMatch` and `sameFoldRune(a, b rune) bool`.

- [ ] **Step 1: Write the failing case behavior test**

Create `matcher_test.go`:

```go
package main

import "testing"

func TestContainsRuleCaseModes(t *testing.T) {
	rule := compiledRule{Term: "Alpha", Runes: []rune("Alpha")}
	if containsRule("alpha", rule, false) {
		t.Fatal("default matching ignored case")
	}
	if !containsRule("xxaLPHAyy", rule, true) {
		t.Fatal("ignore_case did not match ASCII mixed case")
	}
	cases := []struct {
		text string
		term string
		want bool
	}{
		{text: "ς", term: "Σ", want: true},
		{text: "K", term: "K", want: true},
		{text: "STRASSE", term: "straße", want: false},
		{text: "é", term: "é", want: false},
	}
	for _, tc := range cases {
		r := compiledRule{Term: tc.term, Runes: []rune(tc.term)}
		if got := containsRule(tc.text, r, true); got != tc.want {
			t.Errorf("containsRule(%q, %q) = %t, want %t", tc.text, tc.term, got, tc.want)
		}
	}
}
```

Add the public block assertion to `main_test.go`:

```go
func TestBeforeAuthBlockIgnoreCaseReturnsYAMLTerm(t *testing.T) {
	registerConfig(t, "mode: block\nignore_case: true\nwords: [Alpha]\n")
	resp := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"user","content":"aLPHA"}]}`))
	if !resp.Terminate || resp.StatusCode != 400 || gjson.GetBytes(resp.ResponseBody, "error.term").String() != "Alpha" || gjson.GetBytes(resp.ResponseBody, "error.role").String() != "user" {
		t.Fatalf("response = %#v body = %s", resp, resp.ResponseBody)
	}
}
```

Do not add another config-type test：Task 2 already rejects `ignore_case: yes` and accepts `ignore_case: true`.

- [ ] **Step 2: Run the RED test**

Run: `go test ./... -run '^(TestContainsRuleCaseModes|TestBeforeAuthBlockIgnoreCaseReturnsYAMLTerm)$'`

Expected: FAIL because the current ignore-case branch returns false.

- [ ] **Step 3: Implement scalar-aligned SimpleFold matching**

Use:

```go
type textMatch struct {
	Start int
	End   int
}

func sameFoldRune(a, b rune) bool {
	if a == b {
		return true
	}
	for folded := unicode.SimpleFold(a); folded != a; folded = unicode.SimpleFold(folded) {
		if folded == b {
			return true
		}
	}
	return false
}
```

`findFoldMatches` starts only at UTF-8 rune boundaries, compares exactly `len(runes)` scalars, records original byte ranges, advances to `End` after a match and one original scalar after a miss. Empty rules are impossible by config validation but return no matches defensively. `containsRule` must call `strings.Contains` when `ignoreCase == false` and stop after the first fold match when true.

Implement `stripRule` and `obfuscateRule` signatures now. Their sensitive branches use `strings.Contains` followed by `strings.ReplaceAll`; their fold branches consume `findFoldMatches`, preserve original unmatched bytes, and return the unchanged input with `false` when there is no match. `obfuscateRule` writes the actual match's first scalar, `char`, then the actual match remainder.

- [ ] **Step 4: Run matcher and full tests**

Run:

```bash
go test ./... -run '^(TestContainsRuleCaseModes|TestBeforeAuthBlockIgnoreCaseReturnsYAMLTerm)$'
go test ./...
```

Expected: both commands PASS. Task 15 adds and runs the required benchmark matrix.

- [ ] **Step 5: Review and commit**

Review `matcher.go` alone for no lowercase copy, scalar-aligned start/end positions, non-overlap advancement and use of the sensitive stdlib branch.

```bash
git add matcher.go matcher_test.go main_test.go
git commit -m "feat: support unicode case-insensitive matching"
```

---

### Task 6: Strip All Occurrences and Ordered Cascade

**Files:**
- Modify: `transform.go`
- Modify: `transform_test.go`
- Modify: `matcher_test.go`

**Interfaces:**
- Consumes: `stripRule` from Task 5 and mutable `textSpan.Text`.
- Produces: strip branch in `applyMode`, including sensitive and ignore-case rule-major semantics.

- [ ] **Step 1: Write the failing all-node strip test**

Add:

```go
func TestStripRemovesAllOccurrencesAcrossAllNodes(t *testing.T) {
	cfg := mustConfig(t, "mode: strip\nwords: [bad]\n")
	body := []byte(`{"messages":[{"role":"user","content":"bad bad"},{"role":"user","content":"xbadx"},{"role":"user","content":"bad/bad"}]}`)
	got, err := transformRequest(body, "openai", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"messages":[{"role":"user","content":" "},{"role":"user","content":"xx"},{"role":"user","content":"/"}]}`
	if string(got.Body) != want {
		t.Fatalf("body = %s, want %s", got.Body, want)
	}
}

func TestStripOrderedCascadeAndFoldedOccurrences(t *testing.T) {
	cfg := mustConfig(t, "mode: strip\nignore_case: true\nwords: [AB, x]\n")
	body := []byte(`{"messages":[{"role":"user","content":"aBxABx"}]}`)
	got, _ := transformRequest(body, "openai", cfg)
	if string(got.Body) != `{"messages":[{"role":"user","content":""}]}` {
		t.Fatalf("body = %s", got.Body)
	}
}

func TestStripUsesLeftmostNonOverlappingOccurrences(t *testing.T) {
	cfg := mustConfig(t, "mode: strip\nwords: [aa]\n")
	body := []byte(`{"messages":[{"role":"user","content":"aaa"}]}`)
	got, err := transformRequest(body, "openai", cfg)
	if err != nil || string(got.Body) != `{"messages":[{"role":"user","content":"a"}]}` {
		t.Fatalf("body = %s, err = %v", got.Body, err)
	}
}
```

- [ ] **Step 2: Run the RED test**

Run: `go test ./... -run '^TestStrip(RemovesAllOccurrencesAcrossAllNodes|OrderedCascadeAndFoldedOccurrences|UsesLeftmostNonOverlappingOccurrences)$'`

Expected: FAIL because `applyMode` does not execute strip.

- [ ] **Step 3: Implement strip mode**

For each rule, visit every span, call `stripRule`, assign returned text only when changed, and set `span.Changed = true`. Do not stop after a node, occurrence or rule. Return `changed=true` if any span changed. No body rebuilding logic belongs in this loop.

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./... -run '^TestStrip'
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Review and commit**

Review the strip branch and both matcher paths for all non-overlapping occurrences and ordered cascade.

```bash
git add transform.go transform_test.go matcher_test.go
git commit -m "feat: strip all ordered matches"
```

---

### Task 7: Obfs, Determinism and One-allocation Body Rebuild

**Files:**
- Modify: `transform.go`
- Modify: `transform_test.go`
- Modify: `matcher_test.go`

**Interfaces:**
- Consumes: `obfuscateRule`, changed spans and original raw token ranges.
- Produces: obfs branch and `rebuildBody` that JSON-encodes only changed strings and allocates the final body once.

- [ ] **Step 1: Write the failing obfs and determinism tests**

Add:

```go
func TestObfsPreservesMatchedCaseAndInsertsOncePerOccurrence(t *testing.T) {
	cfg := mustConfig(t, "mode: obfs\nignore_case: true\nwords: [Alpha, 世界]\nobfs:\n  char: '⁠'\n")
	body := []byte(`{"messages":[{"role":"user","content":"ALPHA Alpha 世界世界"}]}`)
	got, err := transformRequest(body, "openai", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"messages":[{"role":"user","content":"A⁠LPHA A⁠lpha 世⁠界世⁠界"}]}`
	if string(got.Body) != want {
		t.Fatalf("body = %s, want %s", got.Body, want)
	}
}

func TestRebuildMatchesDecodedEscapesAndUsesEncodingJSONEscaping(t *testing.T) {
	cfg := mustConfig(t, "mode: obfs\nwords: ['<X>']\n")
	u := string([]byte{0x5c, 'u'})
	encoded := u + "003cX" + u + "003e" + u + "0026" + u + "2028" + u + "2029"
	body := []byte(`{"messages":[{"role":"user","content":"` + encoded + `"}],"raw":"` + encoded + ` excluded"}`)
	got, err := transformRequest(body, "openai", cfg)
	if err != nil {
		t.Fatal(err)
	}
	wantToken := u + "003c" + "​" + "X" + u + "003e" + u + "0026" + u + "2028" + u + "2029"
	want := []byte(`{"messages":[{"role":"user","content":"` + wantToken + `"}],"raw":"` + encoded + ` excluded"}`)
	if !bytes.Equal(got.Body, want) {
		t.Fatalf("body = %q, want %q", got.Body, want)
	}
}

func TestTransformIsByteDeterministic(t *testing.T) {
	cfg := mustConfig(t, "mode: obfs\nignore_case: true\nwords: [Alpha]\n")
	body := []byte(" { \"messages\" : [ { \"role\" : \"user\", \"content\" : \"ALPHA Alpha\" } ], \"n\":1e+03 } ")
	want := []byte(" { \"messages\" : [ { \"role\" : \"user\", \"content\" : \"A" + "​" + "LPHA A" + "​" + "lpha\" } ], \"n\":1e+03 } ")
	var first []byte
	var firstHash [32]byte
	for i := 0; i < 100; i++ {
		got, err := transformRequest(body, "openai", cfg)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(got.Body)
		if i == 0 {
			first = append([]byte(nil), got.Body...)
			firstHash = hash
			if !bytes.Equal(first, want) {
				t.Fatalf("span-external bytes changed: got %q, want %q", first, want)
			}
			continue
		}
		if !bytes.Equal(got.Body, first) || hash != firstHash {
			t.Fatalf("iteration %d was nondeterministic", i)
		}
	}

	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				got, err := transformRequest(body, "openai", cfg)
				if err != nil {
					errs <- err
					return
				}
				if !bytes.Equal(got.Body, first) || sha256.Sum256(got.Body) != firstHash {
					errs <- fmt.Errorf("concurrent transform was nondeterministic")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
```

Add `bytes`、`crypto/sha256`、`fmt` and `sync` to `transform_test.go` imports.

- [ ] **Step 2: Run the RED tests**

Run: `go test ./... -run '^Test(Obfs|Rebuild|TransformIsByteDeterministic)'`

Expected: FAIL because obfs and final rebuild are incomplete.

- [ ] **Step 3: Implement obfs and exact rebuild**

The obfs branch mirrors strip's rule-major/all-node loop and calls `obfuscateRule`. `rebuildBody` must:

1. JSON-marshal only `Changed` span text into replacement byte slices.
2. Compute exact final length with overflow checks.
3. Allocate `out := make([]byte, finalLen)` once.
4. Walk changed spans from last to first, copy original tails and replacements backward.
5. Copy the original prefix and verify source/destination cursors end at zero.
6. Return `nil` when no span changed.

Do not marshal the root object. Standard `encoding/json` string escaping is required, including `<`, `>`, `&`, U+2028 and U+2029 behavior.

- [ ] **Step 4: Run tests and race**

Run:

```bash
go test ./... -run '^Test(Obfs|Rebuild|TransformIsByteDeterministic)'
go test ./...
go test -race ./...
```

Expected: PASS.

- [ ] **Step 5: Review and commit**

Review `transform.go` and `matcher.go` independently, then check cursor arithmetic and unchanged body behavior across the two files.

```bash
git add transform.go transform_test.go matcher_test.go
git commit -m "feat: obfuscate deterministic request text"
```

### Task 8: Complete OpenAI Scope, Raw Spans and Hard Exclusions

**Files:**
- Modify: `selectors_openai.go`
- Create: `selectors_openai_test.go`

**Interfaces:**
- Consumes: shared span collector from Task 4 and body rebuild from Task 7.
- Produces: final `openai` selector for string content, typed text parts, optional assistant and optional string tool content.

- [ ] **Step 1: Write one failing public-dispatch fixture containing all OpenAI rows and exclusions**

In `selectors_openai_test.go`, call the public BeforeAuth RPC, not `collectOpenAI` directly:

```go
func TestOpenAISelectorChangesOnlyContractedStringTokens(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system, developer, user, assistant, tool]\n")
	base64Payload := strings.Repeat("SECRET", (20<<20)/len("SECRET")+1)[:20<<20]
	body := []byte(fmt.Sprintf(`{
  "SECRET-key":"unchanged",
  "messages":[
    {"role":"system","content":"SECRET system"},
    {"role":"developer","content":[{"type":"text","text":"SECRET developer"},{"type":"image_url","image_url":{"url":"SECRET"},"text":"SECRET excluded"}]},
    {"role":"user","content":[{"type":"text","text":"SECRET user"},{"type":"input_audio","data":"SECRET"}]},
    {"role":"assistant","content":"SECRET assistant","reasoning":"SECRET"},
    {"role":"tool","content":"SECRET tool"},
    {"role":"tool","content":[{"type":"text","text":"SECRET excluded tool array"}]},
    {"role":"unknown","content":"SECRET unknown"},
    {"content":"SECRET missing role"},
    {"role":"user","content":123}
  ],
  "tools":[{"type":"function","function":{"name":"SECRET","description":"SECRET","parameters":{"SECRET":"SECRET"}}}],
  "tool_calls":[{"function":{"name":"SECRET","arguments":"{\"text\":\"SECRET\"}"}}],
  "image":"data:image/png;base64,%s"
}`, base64Payload))
	resp := interceptRPC(t, "openai", body)
	if resp.Terminate {
		t.Fatalf("response = %#v", resp)
	}
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET system"`, After: `" system"`},
		rawReplacement{Before: `"SECRET developer"`, After: `" developer"`},
		rawReplacement{Before: `"SECRET user"`, After: `" user"`},
		rawReplacement{Before: `"SECRET assistant"`, After: `" assistant"`},
		rawReplacement{Before: `"SECRET tool"`, After: `" tool"`},
	)
	if !bytes.Equal(resp.Body, want) {
		t.Fatalf("body differs outside contracted string tokens")
	}
}
```

Add the default-role boundary test:

```go
func TestOpenAISelectorDefaultRolesExcludeAssistantAndTool(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\n")
	body := []byte(`{"messages":[{"role":"system","content":"SECRET s"},{"role":"developer","content":"SECRET d"},{"role":"user","content":"SECRET u"},{"role":"assistant","content":"SECRET a"},{"role":"tool","content":"SECRET t"}]}`)
	resp := interceptRPC(t, "openai", body)
	want := `{"messages":[{"role":"system","content":" s"},{"role":"developer","content":" d"},{"role":"user","content":" u"},{"role":"assistant","content":"SECRET a"},{"role":"tool","content":"SECRET t"}]}`
	if string(resp.Body) != want {
		t.Fatalf("body = %s, want %s", resp.Body, want)
	}
}

func TestOpenAISelectorCanonicalRoles(t *testing.T) {
	cases := []struct {
		name, body, role string
	}{
		{name: "system string", body: `{"messages":[{"role":"system","content":"SECRET"}]}`, role: "system"},
		{name: "developer string", body: `{"messages":[{"role":"developer","content":"SECRET"}]}`, role: "developer"},
		{name: "user string", body: `{"messages":[{"role":"user","content":"SECRET"}]}`, role: "user"},
		{name: "assistant string", body: `{"messages":[{"role":"assistant","content":"SECRET"}]}`, role: "assistant"},
		{name: "typed text", body: `{"messages":[{"role":"user","content":[{"type":"text","text":"SECRET"}]}]}`, role: "user"},
		{name: "tool string", body: `{"messages":[{"role":"tool","content":"SECRET"}]}`, role: "tool"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "openai", tc.body, tc.role) })
	}
}
```

- [ ] **Step 2: Run the RED test**

Run: `go test ./... -run '^TestOpenAISelectorChangesOnlyContractedStringTokens$'`

Expected: FAIL because typed text parts are not selected.

- [ ] **Step 3: Complete only the OpenAI selector**

For exact roles `system`、`developer`、`user`、`assistant`:

- append string `messages[i].content`;
- if content is an array, append only part `.text` where part `.type` is the exact string `text`;
- never descend into any other part.

For exact role `tool`, append content only when it is a JSON string. Unknown/missing/wrong-type role or content is a candidate-level no-op. Scope filtering remains inside `appendStringSpan`; selector discriminator comparisons never use `ignore_case`.

- [ ] **Step 4: Run tests and inspect allocation behavior with the large exclusion**

Run:

```bash
go test ./... -run '^TestOpenAISelector'
go test ./...
```

Expected: PASS without changing the base64 bytes.

- [ ] **Step 5: Review and commit**

Review only `selectors_openai.go` and `selectors_openai_test.go`; verify no call traverses `tools`, `tool_calls`, reasoning or role=tool arrays.

```bash
git add selectors_openai.go selectors_openai_test.go
git commit -m "feat: select scoped openai request text"
```

---

### Task 9: OpenAI Responses Selector

**Files:**
- Modify: `selectors_openai.go`
- Modify: `selectors_openai_test.go`

**Interfaces:**
- Consumes: `appendStringSpan` and role set.
- Produces: `collectOpenAIResponses(root gjson.Result, roles scopeSet, spans *[]textSpan)` for all `openai-response` rows.

- [ ] **Step 1: Write the failing table-driven public-dispatch test**

Use one test function with a top-level string case and an array case:

```go
func TestOpenAIResponsesSelectorRowsAndExclusions(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system, developer, user, assistant, tool]\n")
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "top-level strings",
			body: `{"instructions":"SECRET system","input":"SECRET user","metadata":{"note":"SECRET"}}`,
			want: `{"instructions":" system","input":" user","metadata":{"note":"SECRET"}}`,
		},
		{
			name: "message items",
			body: `{"input":[
				{"type":"message","role":"system","content":"SECRET s"},
				{"type":"","role":"user","content":"SECRET empty type"},
				{"role":"developer","content":[{"type":"input_text","text":"SECRET d"},{"type":"","text":"SECRET e"},{"text":"SECRET m"}]},
				{"type":"message","role":"user","content":[{"type":"input_text","text":"SECRET u"},{"type":"output_text","text":"SECRET wrong output"},{"type":"input_image","image_url":"SECRET"}]},
				{"type":"message","role":"assistant","content":[{"type":"output_text","text":"SECRET a"}]},
				{"type":"message","content":"SECRET no role"},
				{"type":"message","role":"tool","content":"SECRET bad role"},
				{"type":"message","role":"user","content":123},
				{"type":"function_call_output","role":"user","output":"SECRET result","content":"SECRET fake"},
				{"type":"custom_tool_call_output","role":"user","output":"SECRET custom"},
				{"type":"unknown","role":"user","content":"SECRET unknown"}
			],"tools":[{"name":"SECRET","description":"SECRET"}]}`,
			want: `{"input":[
				{"type":"message","role":"system","content":" s"},
				{"type":"","role":"user","content":" empty type"},
				{"role":"developer","content":[{"type":"input_text","text":" d"},{"type":"","text":" e"},{"text":" m"}]},
				{"type":"message","role":"user","content":[{"type":"input_text","text":" u"},{"type":"output_text","text":"SECRET wrong output"},{"type":"input_image","image_url":"SECRET"}]},
				{"type":"message","role":"assistant","content":[{"type":"output_text","text":" a"}]},
				{"type":"message","content":"SECRET no role"},
				{"type":"message","role":"tool","content":"SECRET bad role"},
				{"type":"message","role":"user","content":123},
				{"type":"function_call_output","role":"user","output":"SECRET result","content":"SECRET fake"},
				{"type":"custom_tool_call_output","role":"user","output":"SECRET custom"},
				{"type":"unknown","role":"user","content":"SECRET unknown"}
			],"tools":[{"name":"SECRET","description":"SECRET"}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := interceptRPC(t, "openai-response", []byte(tc.body))
			if resp.Terminate || string(resp.Body) != tc.want {
				t.Fatalf("body = %s, want %s, terminate = %t", resp.Body, tc.want, resp.Terminate)
			}
		})
	}
}

func TestOpenAIResponsesSelectorCanonicalRoles(t *testing.T) {
	cases := []struct {
		name, body, role string
	}{
		{name: "instructions", body: `{"instructions":"SECRET"}`, role: "system"},
		{name: "input string", body: `{"input":"SECRET"}`, role: "user"},
		{name: "message string", body: `{"input":[{"type":"message","role":"developer","content":"SECRET"}]}`, role: "developer"},
		{name: "input text", body: `{"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"SECRET"}]}]}`, role: "user"},
		{name: "output text", body: `{"input":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"SECRET"}]}]}`, role: "assistant"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "openai-response", tc.body, tc.role) })
	}
}
```

- [ ] **Step 2: Run the RED test**

Run: `go test ./... -run '^TestOpenAIResponsesSelectorRowsAndExclusions$'`

Expected: FAIL because the dispatcher has no Responses collector.

- [ ] **Step 3: Implement exact Responses discriminators**

Implement:

```go
func openAIResponsesMessageType(item gjson.Result) bool
func openAIResponsesRole(item gjson.Result) (string, bool)
func collectOpenAIResponses(root gjson.Result, roles scopeSet, spans *[]textSpan)
```

Message type is allowed only when missing, empty string or exact `message`. Role must be an exact string among system/developer/user/assistant. String content is eligible. Array part type missing/empty/`input_text` is eligible for all allowed roles; exact `output_text` is eligible only for assistant. All other item/part types are subtree no-op. Top-level instructions and string input are independent rows. Never select any tool/function output, even if `scope.roles` contains tool.

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./... -run '^TestOpenAIResponsesSelector'
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Review and commit**

Review only the Responses functions in `selectors_openai.go` plus their tests; verify output_text is assistant-only and missing item role is skipped.

```bash
git add selectors_openai.go selectors_openai_test.go
git commit -m "feat: select openai responses input text"
```

---

### Task 10: Claude Selector and Typed Tool Result

**Files:**
- Modify: `selectors_claude.go`
- Create: `selectors_claude_test.go`

**Interfaces:**
- Consumes: shared dispatcher and span append helper.
- Produces: `collectClaude(root gjson.Result, roles scopeSet, spans *[]textSpan)`.

- [ ] **Step 1: Write the failing Claude contract fixture**

```go
func TestClaudeSelectorRowsAndExclusions(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system, user, assistant, tool]\n")
	body := []byte(`{"system":[{"type":"text","text":"SECRET sys"},{"type":"thinking","thinking":"SECRET","text":"SECRET excluded"}],"messages":[
		{"role":"system","content":"SECRET message system"},
		{"role":"user","content":[
			{"type":"text","text":"SECRET user"},
			{"type":"tool_result","tool_use_id":"SECRET","content":[{"type":"text","text":"SECRET tool"},{"type":"image","source":{"data":"SECRET"}},{"type":"unknown","text":"SECRET unknown"}]},
			{"type":"tool_result","content":"SECRET string result"},
			{"type":"tool_result","content":{"text":"SECRET object result"}},
			{"type":"tool_use","name":"SECRET","input":{"text":"SECRET"}},
			{"type":"thinking","thinking":"SECRET"},
			{"type":"redacted_thinking","data":"SECRET"},
			{"type":"image","source":{"data":"SECRET"}}
		]},
		{"role":"assistant","content":"SECRET assistant"},
		{"role":"developer","content":"SECRET unsupported role"},
		{"role":"unknown","content":"SECRET unknown role"}
	]}`)
	resp := interceptRPC(t, "claude", body)
	if resp.Terminate {
		t.Fatalf("response = %#v", resp)
	}
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET sys"`, After: `" sys"`},
		rawReplacement{Before: `"SECRET message system"`, After: `" message system"`},
		rawReplacement{Before: `"SECRET user"`, After: `" user"`},
		rawReplacement{Before: `"SECRET tool"`, After: `" tool"`},
		rawReplacement{Before: `"SECRET assistant"`, After: `" assistant"`},
	)
	if !bytes.Equal(resp.Body, want) {
		t.Fatalf("body differs outside contracted Claude string tokens")
	}
}

func TestClaudeTopLevelSystemStringAndToolScope(t *testing.T) {
	cases := []struct {
		name, roles, want string
	}{
		{name: "tool enabled", roles: "[system, tool]", want: `{"system":" sys","messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"text","text":" tool"}]}]}]}`},
		{name: "tool omitted", roles: "[system]", want: `{"system":" sys","messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"text","text":"SECRET tool"}]}]}]}`},
	}
	body := []byte(`{"system":"SECRET sys","messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"text","text":"SECRET tool"}]}]}]}`)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: "+tc.roles+"\n")
			resp := interceptRPC(t, "claude", body)
			if string(resp.Body) != tc.want {
				t.Fatalf("body = %s, want %s", resp.Body, tc.want)
			}
		})
	}
}

func TestClaudeSelectorCanonicalRoles(t *testing.T) {
	cases := []struct {
		name, body, role string
	}{
		{name: "system string", body: `{"system":"SECRET"}`, role: "system"},
		{name: "system block", body: `{"system":[{"type":"text","text":"SECRET"}]}`, role: "system"},
		{name: "message string", body: `{"messages":[{"role":"assistant","content":"SECRET"}]}`, role: "assistant"},
		{name: "message block", body: `{"messages":[{"role":"user","content":[{"type":"text","text":"SECRET"}]}]}`, role: "user"},
		{name: "typed tool result", body: `{"messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"text","text":"SECRET"}]}]}]}`, role: "tool"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "claude", tc.body, tc.role) })
	}
}
```

- [ ] **Step 2: Run the RED test**

Run: `go test ./... -run '^TestClaudeSelectorRowsAndExclusions$'`

Expected: FAIL because `claude` has no collector.

- [ ] **Step 3: Implement the Claude selector**

Select exactly:

- top-level `system` string;
- top-level `system[]` block text only when `type == "text"`;
- message string content for exact roles system/user/assistant;
- message array block text only when `type == "text"`;
- under role=user, outer `type == "tool_result"`, content must be array, inner block must have exact `type == "text"`, then select inner `.text` as role=tool.

Do not recognize Claude developer, string/object tool_result content, tool_use, thinking, redacted_thinking, image or document source.

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./... -run '^TestClaude'
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Review and commit**

Review `selectors_claude.go` and its test in parallel with other protocol reviews; verify only the nested typed tool result path maps to tool.

```bash
git add selectors_claude.go selectors_claude_test.go
git commit -m "feat: select claude request text"
```

---

### Task 11: Gemini Selector and Machine Part Exclusions

**Files:**
- Modify: `selectors_gemini.go`
- Create: `selectors_gemini_test.go`

**Interfaces:**
- Consumes: shared span append helper and `geminiTextPartAllowed(part gjson.Result) bool` frozen in Task 3.
- Produces: final `collectGemini` implementation.

- [ ] **Step 1: Write the failing Gemini fixture**

```go
func TestGeminiSelectorRowsAndMachineExclusions(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system, user, assistant, tool]\n")
	body := []byte(`{
		"systemInstruction":{"parts":[
			{"text":"SECRET camel"},
			{"text":"SECRET functionCall","functionCall":{"name":"x"}},
			{"text":"SECRET functionResponse","functionResponse":{"response":{}}},
			{"text":"SECRET inlineData","inlineData":{"data":"SECRET"}},
			{"text":"SECRET inline_data","inline_data":{"data":"SECRET"}},
			{"text":"SECRET fileData","fileData":{"fileUri":"SECRET"}},
			{"text":"SECRET file_data","file_data":{"file_uri":"SECRET"}},
			{"text":"SECRET executableCode","executableCode":{"code":"SECRET"}},
			{"text":"SECRET codeExecutionResult","codeExecutionResult":{"output":"SECRET"}},
			{"text":"SECRET thought false","thought":false},
			{"text":"SECRET thought","thought":true}
		]},
		"system_instruction":{"parts":[{"text":"SECRET snake"},{"text":"SECRET signed","thoughtSignature":"sig"}]},
		"contents":[
			{"parts":[{"text":"SECRET inherited"}]},
			{"role":"user","parts":[{"text":"SECRET user"}]},
			{"role":"model","parts":[{"text":"SECRET model"}]},
			{"role":"assistant","parts":[{"text":"SECRET unknown role"}]}
		]
	}`)
	resp := interceptRPC(t, "gemini", body)
	if resp.Terminate {
		t.Fatalf("response = %#v", resp)
	}
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET camel"`, After: `" camel"`},
		rawReplacement{Before: `"SECRET thought false"`, After: `" thought false"`},
		rawReplacement{Before: `"SECRET snake"`, After: `" snake"`},
		rawReplacement{Before: `"SECRET inherited"`, After: `" inherited"`},
		rawReplacement{Before: `"SECRET user"`, After: `" user"`},
		rawReplacement{Before: `"SECRET model"`, After: `" model"`},
	)
	if !bytes.Equal(resp.Body, want) {
		t.Fatalf("body differs outside contracted Gemini string tokens")
	}
}

func TestGeminiSelectorCanonicalRoles(t *testing.T) {
	cases := []struct {
		name, body, role string
	}{
		{name: "camel system", body: `{"systemInstruction":{"parts":[{"text":"SECRET"}]}}`, role: "system"},
		{name: "snake system", body: `{"system_instruction":{"parts":[{"text":"SECRET"}]}}`, role: "system"},
		{name: "missing role", body: `{"contents":[{"parts":[{"text":"SECRET"}]}]}`, role: "user"},
		{name: "user role", body: `{"contents":[{"role":"user","parts":[{"text":"SECRET"}]}]}`, role: "user"},
		{name: "model role", body: `{"contents":[{"role":"model","parts":[{"text":"SECRET"}]}]}`, role: "assistant"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "gemini", tc.body, tc.role) })
	}
}
```

- [ ] **Step 2: Run the RED test**

Run: `go test ./... -run '^TestGeminiSelectorRowsAndMachineExclusions$'`

Expected: FAIL because `gemini` has no collector.

- [ ] **Step 3: Implement exact Gemini predicates**

Use Task 3's `geminiTextPartAllowed`, which rejects a part when any exact key exists among `functionCall`, `functionResponse`, `inlineData`, `inline_data`, `fileData`, `file_data`, `executableCode`, `codeExecutionResult`; when `thought` is JSON boolean true; or when `thoughtSignature` exists. Do not copy or redefine that predicate in this module.

Collect text in both top-level system instruction spellings. For contents, missing role and exact user map to user; exact model maps to assistant; every other present role, including assistant, skips the whole content. Only allowed part `.text` strings are selected.

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./... -run '^TestGeminiSelector'
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Review and commit**

Review `selectors_gemini.go` and its test; verify machine discriminator presence wins even when text is also present.

```bash
git add selectors_gemini.go selectors_gemini_test.go
git commit -m "feat: select gemini request text"
```

---

### Task 12: Interactions Restricted Recursive Selector

**Files:**
- Modify: `selectors_interactions.go`
- Create: `selectors_interactions_test.go`

**Interfaces:**
- Consumes: `appendStringSpan`, `geminiTextPartAllowed` only for `parts[]`.
- Produces: `collectInteractions` and `collectInteractionItem(item gjson.Result, inheritedRole string, roles scopeSet, spans *[]textSpan)`.

- [ ] **Step 1: Write the failing Interactions fixture**

```go
func TestInteractionsSelectorRowsRoleInheritanceAndExclusions(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system, user, assistant, tool]\n")
	body := []byte(`{
		"system_instruction":{"text":"SECRET sys text","parts":[{"type":"text","text":"SECRET sys part"},{"text":"SECRET sys missing"},{"type":"","text":"SECRET sys empty"},{"type":"text","text":"SECRET sys machine","inlineData":{"data":"SECRET"}},{"type":"image","text":"SECRET image"}]},
		"input":[
			"SECRET direct",
			{"type":"user_input","content":"SECRET content","parts":[{"text":"SECRET part"},{"text":"SECRET machine","inlineData":{"data":"SECRET"}}]},
			{"role":"model","content":[{"type":"text","text":"SECRET inherited assistant"}]},
			{"type":"model_output","role":"user","content":{"type":"text","text":"SECRET forced assistant"}},
			{"role":"user","steps":[{"content":"SECRET nested user"},{"role":"assistant","parts":[{"type":"","text":"SECRET nested assistant"}]}]},
			{"type":"thought","content":"SECRET thought"},
			{"type":"function_call","content":"SECRET call"},
			{"type":"function_result","content":"SECRET result"},
			{"type":"unknown","content":"SECRET unknown"},
			{"role":"tool","content":"SECRET bad role"},
			{"type":"user_input","content":123,"parts":[{"type":"text","text":123}]},
			{"steps":["SECRET string step"]}
		]
	}`)
	resp := interceptRPC(t, "interactions", body)
	if resp.Terminate {
		t.Fatalf("response = %#v", resp)
	}
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET sys text"`, After: `" sys text"`},
		rawReplacement{Before: `"SECRET sys part"`, After: `" sys part"`},
		rawReplacement{Before: `"SECRET sys missing"`, After: `" sys missing"`},
		rawReplacement{Before: `"SECRET sys empty"`, After: `" sys empty"`},
		rawReplacement{Before: `"SECRET direct"`, After: `" direct"`},
		rawReplacement{Before: `"SECRET content"`, After: `" content"`},
		rawReplacement{Before: `"SECRET part"`, After: `" part"`},
		rawReplacement{Before: `"SECRET inherited assistant"`, After: `" inherited assistant"`},
		rawReplacement{Before: `"SECRET forced assistant"`, After: `" forced assistant"`},
		rawReplacement{Before: `"SECRET nested user"`, After: `" nested user"`},
		rawReplacement{Before: `"SECRET nested assistant"`, After: `" nested assistant"`},
	)
	if !bytes.Equal(resp.Body, want) {
		t.Fatalf("body differs outside contracted Interactions string tokens")
	}
}

func TestInteractionsTopLevelStringRows(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system, user]\n")
	cases := []struct {
		body, want string
	}{
		{body: `{"system_instruction":"SECRET system"}`, want: `{"system_instruction":" system"}`},
		{body: `{"input":"SECRET user"}`, want: `{"input":" user"}`},
	}
	for _, tc := range cases {
		resp := interceptRPC(t, "interactions", []byte(tc.body))
		if string(resp.Body) != tc.want {
			t.Fatalf("body = %s, want %s", resp.Body, tc.want)
		}
	}
}

func TestInteractionsSelectorCanonicalRoles(t *testing.T) {
	cases := []struct {
		name, body, role string
	}{
		{name: "system string", body: `{"system_instruction":"SECRET"}`, role: "system"},
		{name: "system object", body: `{"system_instruction":{"text":"SECRET"}}`, role: "system"},
		{name: "system part", body: `{"system_instruction":{"parts":[{"type":"","text":"SECRET"}]}}`, role: "system"},
		{name: "input string", body: `{"input":"SECRET"}`, role: "user"},
		{name: "input array string", body: `{"input":["SECRET"]}`, role: "user"},
		{name: "item content", body: `{"input":[{"type":"user_input","content":"SECRET"}]}`, role: "user"},
		{name: "content array", body: `{"input":[{"role":"assistant","content":[{"type":"text","text":"SECRET"}]}]}`, role: "assistant"},
		{name: "content object model output", body: `{"input":[{"type":"model_output","content":{"type":"text","text":"SECRET"}}]}`, role: "assistant"},
		{name: "parts", body: `{"input":[{"role":"user","parts":[{"text":"SECRET"}]}]}`, role: "user"},
		{name: "nested steps", body: `{"input":[{"role":"assistant","steps":[{"content":"SECRET"}]}]}`, role: "assistant"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "interactions", tc.body, tc.role) })
	}
}
```

- [ ] **Step 2: Run the RED test**

Run: `go test ./... -run '^TestInteractionsSelectorRowsRoleInheritanceAndExclusions$'`

Expected: FAIL because `interactions` has no collector.

- [ ] **Step 3: Implement only the specified recursion**

`collectInteractionItem` requires an object. Resolve role from inherited role: missing inherits; exact user maps user; exact model/assistant maps assistant; wrong type or any other value rejects the subtree. Resolve type: missing/empty/user_input keeps role; model_output forces assistant; every other nonempty or wrong-type value rejects subtree.

Select:

- item content string;
- content object or content array part `.text` only for missing/empty/exact `text` type;
- item `parts[]` `.text` only for the same type rule and `geminiTextPartAllowed`;
- recurse only into object entries of `steps[]`, passing the resolved role.

Root input item inherited role is user. Direct string is accepted only as top-level input or an immediate top-level input array element, never inside steps. Do not recursively walk arbitrary object values.

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./... -run '^TestInteractions'
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Review and commit**

Review `selectors_interactions.go` separately for bounded recursion and exact discriminator handling; verify function_result and thought cannot reach content parsing.

```bash
git add selectors_interactions.go selectors_interactions_test.go
git commit -m "feat: select interactions request text"
```

### Task 13: Atomic Reconfigure and Last-known-good

**Files:**
- Modify: `config.go`
- Modify: `config_test.go`
- Modify: `main.go`
- Modify: `main_test.go`
- Modify: `abi_cgo.go`

**Interfaces:**
- Consumes: strict parser and atomic store from Task 2, host callback bridge from Task 1.
- Produces: `handlePluginRegister`, `handlePluginReconfigure`, `callHost`, best-effort `host.log`, and linearizable A/B snapshot behavior.

- [ ] **Step 1: Write failing reconfigure tests**

Add:

```go
func TestReconfigureStoresValidSnapshotAndKeepsLastKnownGoodOnError(t *testing.T) {
	registerConfig(t, "mode: block\nwords: [alpha]\n")
	if got := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"user","content":"alpha"}]}`)); !got.Terminate {
		t.Fatal("config A did not block alpha")
	}

	reconfigureConfig(t, "mode: block\nignore_case: true\nwords: [Beta]\n")
	if got := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"user","content":"bETA"}]}`)); !got.Terminate || !bytes.Contains(got.ResponseBody, []byte(`"term":"Beta"`)) {
		t.Fatalf("config B response = %#v", got)
	}

	var logs []hostLogRequest
	setHostCallbackForTest(func(method string, request []byte) ([]byte, error) {
		if method == pluginabi.MethodHostLog {
			var req hostLogRequest
			if err := json.Unmarshal(request, &req); err != nil {
				return nil, err
			}
			logs = append(logs, req)
		}
		return okEnvelope(struct{}{})
	})
	t.Cleanup(func() { setHostCallbackForTest(nil) })
	reconfigureConfig(t, "ignore_case: yes\nwords: [gamma]\n")
	if got := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"user","content":"BETA"}]}`)); !got.Terminate || !bytes.Contains(got.ResponseBody, []byte(`"term":"Beta"`)) {
		t.Fatalf("invalid B replaced last-known-good: %#v", got)
	}
	if len(logs) != 1 || logs[0].Level != "error" || logs[0].Message != "censorship plugin reconfigure rejected" || logs[0].Fields["error"] == "" {
		t.Fatalf("host logs = %#v", logs)
	}
}
```

Add the public lifecycle helper and concurrent whole-snapshot test:

```go
func reconfigureConfig(t *testing.T, configYAML string) {
	t.Helper()
	var env pluginabi.Envelope
	decodeEnvelope(t, mustHandle(t, pluginabi.MethodPluginReconfigure, lifecycleJSON(t, configYAML)), &env)
	if !env.OK {
		t.Fatalf("reconfigure: %#v", env.Error)
	}
}

func TestConcurrentReconfigureObservesOnlyWholeSnapshot(t *testing.T) {
	const configA = "mode: strip\nwords: [alpha]\nscope:\n  formats: [openai]\n  roles: [user]\n"
	const configB = "mode: obfs\nignore_case: true\nwords: [Beta]\nscope:\n  formats: [openai]\n  roles: [user]\nobfs:\n  char: '⁠'\n"
	body := []byte(`{"messages":[{"role":"user","content":"alpha BETA"}]}`)
	wantA := []byte(`{"messages":[{"role":"user","content":" BETA"}]}`)
	wantB := []byte(`{"messages":[{"role":"user","content":"alpha B⁠ETA"}]}`)
	registerConfig(t, configA)

	done := make(chan struct{})
	errs := make(chan error, 32)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				resp, err := callIntercept("openai", body)
				if err != nil {
					errs <- err
					return
				}
				if !bytes.Equal(resp.Body, wantA) && !bytes.Equal(resp.Body, wantB) {
					errs <- fmt.Errorf("mixed snapshot body: %s", resp.Body)
					return
				}
			}
		}()
	}
	for i := 0; i < 1000; i++ {
		if i%2 == 0 {
			reconfigureConfig(t, configB)
		} else {
			reconfigureConfig(t, configA)
		}
	}
	close(done)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
```

Add `fmt` and `sync` to `config_test.go` imports. The worker path uses non-fatal `callIntercept`; only the parent test goroutine calls `reconfigureConfig` and `t.Fatal`.

- [ ] **Step 2: Run the RED tests**

Run: `go test -race ./... -run '^Test(ReconfigureStores|ConcurrentReconfigure)'`

Expected: FAIL because reconfigure does not implement last-known-good logging and the test host callback helper is absent.

- [ ] **Step 3: Implement lifecycle behavior**

Define:

```go
type hostLogRequest struct {
	Level   string         `json:"level,omitempty"`
	Message string         `json:"message,omitempty"`
	Fields  map[string]any `json:"fields,omitempty"`
}
```

`plugin.register` parses and validates, then performs one `installSnapshot(candidate)`. `plugin.reconfigure` parses into a candidate without touching active state. On success it performs one store and returns current registration. On parse/validation failure it calls `host.log` with level error, fixed message above and field `error` containing `err.Error()`, ignores callback failure, leaves the pointer unchanged, and returns a successful envelope containing the same registration.

`callHost` JSON-marshals the payload, copies the callback pointer while holding `RLock`, releases the lock before invoking it, decodes a `pluginabi.Envelope`, and returns its result or error. `cliproxyPluginShutdown` clears only the host callback; it does not mutate the immutable snapshot while active requests may hold it.

- [ ] **Step 4: Run race and full tests**

Run:

```bash
go test -race ./... -run '^Test(ReconfigureStores|ConcurrentReconfigure)'
go test ./...
go test -race ./...
```

Expected: PASS.

- [ ] **Step 5: Review and commit**

Review config lifecycle and ABI callback files independently, then check store/log ordering: candidate validation, one store on success; log and no store on failure.

```bash
git add abi_cgo.go main.go main_test.go config.go config_test.go
git commit -m "feat: hot reload censorship rules atomically"
```

---

### Task 14: Differential Oracles and Fuzz Targets

**Files:**
- Create: `fuzz_test.go`

**Interfaces:**
- Consumes: matcher, mode engine and all selectors.
- Produces: `FuzzRuleEngineAgainstOracle`, `FuzzProtocolTransform` and independent reference helpers that do not call production matcher functions.

- [ ] **Step 1: Write the fuzz seeds and prove an injected mismatch is detected**

Create two fuzz targets. The independent fold oracle converts only the candidate window to `[]rune`, requires the same scalar count as the term, and calls `strings.EqualFold(string(window), term)`; it never calls `containsRule`, `findFoldMatches`, `stripRule` or `obfuscateRule`.

```go
func FuzzRuleEngineAgainstOracle(f *testing.F) {
	for _, seed := range []struct {
		text, terms string
		mode, fold  uint8
	}{
		{"ababa", "aba|ba", 0, 0},
		{"ALPHA Alpha", "Alpha", 1, 1},
		{"ςΣσ", "Σ", 2, 1},
		{"STRASSE/straße", "straße", 1, 1},
		{"世界世界", "世界", 2, 0},
	} {
		f.Add(seed.text, seed.terms, seed.mode, seed.fold)
	}
	f.Fuzz(func(t *testing.T, text, packed string, modeByte, foldByte uint8) {
		terms := boundedTerms(packed, 8, 32)
		if len(terms) == 0 || len(text) > 256 {
			t.Skip()
		}
		cfg := snapshotForFuzz(terms, modeByte%3, foldByte%2 == 1)
		got := applyRulesToTextsForTest([]string{text, "prefix " + text}, cfg)
		want := oracleApply([]string{text, "prefix " + text}, cfg)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})
}
```

Add the protocol fuzz target:

```go
func FuzzProtocolTransform(f *testing.F) {
	seeds := []struct {
		format string
		body   []byte
	}{
		{format: "openai", body: []byte(`{"messages":[{"role":"user","content":"SECRET"}],"tools":[{"description":"SECRET"}]}`)},
		{format: "openai-response", body: []byte(`{"instructions":"SECRET","input":[{"type":"function_call_output","output":"SECRET"}]}`)},
		{format: "claude", body: []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"SECRET"},{"type":"thinking","thinking":"SECRET"}]}]}`)},
		{format: "gemini", body: []byte(`{"contents":[{"role":"user","parts":[{"text":"SECRET"},{"text":"SECRET","inlineData":{"data":"SECRET"}}]}]}`)},
		{format: "interactions", body: interactionStepsSeed(64)},
		{format: "openai", body: []byte(`not-json`)},
	}
	for _, seed := range seeds {
		for mode := uint8(0); mode < 3; mode++ {
			f.Add(seed.format, seed.body, mode, false)
			f.Add(seed.format, seed.body, mode, true)
		}
	}
	f.Fuzz(func(t *testing.T, format string, body []byte, modeByte uint8, fold bool) {
		if len(format) > 32 || len(body) > 64<<10 {
			t.Skip()
		}
		modes := []string{"block", "strip", "obfs"}
		cfg := mustConfig(t, fmt.Sprintf("mode: %s\nignore_case: %t\nwords: [SECRET]\n", modes[modeByte%3], fold))
		beforeExcluded := oracleExcludedTokens(format, body)
		got, err := transformRequest(body, format, cfg)
		if err != nil {
			t.Fatal(err)
		}
		if got.Invalid {
			if validJSONObject(body) && knownFormat(format) {
				t.Fatalf("valid JSON object marked invalid: %s", body)
			}
			return
		}
		if got.Blocked != nil {
			if got.Blocked.Term != "SECRET" || !allowedCanonicalRole(got.Blocked.Role) {
				t.Fatalf("blocked = %#v", got.Blocked)
			}
			return
		}
		if len(got.Body) == 0 {
			return
		}
		if !json.Valid(got.Body) {
			t.Fatalf("changed output is invalid JSON: %s", got.Body)
		}
		if after := oracleExcludedTokens(format, got.Body); !reflect.DeepEqual(after, beforeExcluded) {
			t.Fatalf("excluded raw tokens changed: before=%q after=%q", beforeExcluded, after)
		}
	})
}

func interactionStepsSeed(depth int) []byte {
	node := `{"content":"SECRET"}`
	for i := 0; i < depth; i++ {
		node = `{"steps":[` + node + `]}`
	}
	return []byte(`{"input":[` + node + `]}`)
}
```

Task 14 defines independent `knownFormat(string) bool`、`validJSONObject([]byte) bool`、`allowedCanonicalRole(string) bool` and `oracleExcludedTokens(format string, body []byte) [][]byte` helpers in `fuzz_test.go`; none may call a production selector or matcher. The excluded-token oracle parses only the explicitly hard-excluded paths in each seed family and returns copied raw JSON tokens in document order.

Before implementing production fixes, temporarily change one oracle expected insertion offset in the working tree, run the seed corpus and observe at least one FAIL; restore the oracle immediately. This is a test-of-test step and must not be committed.

- [ ] **Step 2: Run fuzz seed corpus**

Run:

```bash
go test ./... -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=1x
go test ./... -run '^$' -fuzz '^FuzzProtocolTransform$' -fuzztime=1x
```

Expected: both seed corpora PASS after restoring the oracle.

- [ ] **Step 3: Complete independent oracle helpers**

Implement bounded term parsing that rejects empty terms and obfs-invalid seeds, rule-major block/strip/obfs logic over plain `[]string`, scalar-window case folding with `strings.EqualFold`, and an excluded-token extractor that records original raw tokens for hard-excluded fixture paths before and after transform. Do not import production helpers into oracle functions.

- [ ] **Step 4: Run timed fuzz and full tests**

Run:

```bash
go test ./... -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=30s
go test ./... -run '^$' -fuzz '^FuzzProtocolTransform$' -fuzztime=30s
go test ./...
```

Expected: no panic, mismatch or invalid changed JSON.

- [ ] **Step 5: Review and commit**

Review `fuzz_test.go` for oracle independence, bounded memory/nesting and coverage of both case modes.

```bash
git add fuzz_test.go
git commit -m "test: fuzz censorship transforms"
```

---

### Task 15: Unit and RPC Benchmarks

**Files:**
- Create: `benchmark_test.go`

**Interfaces:**
- Consumes: public transform and RPC functions.
- Produces: unit and in-process RPC benchmarks reporting `ns/op`、`B/op` and `allocs/op`. Dynamic ABI measurement belongs exclusively to Task 17.

- [ ] **Step 1: Write benchmark functions and first run**

Create `benchmark_test.go`:

```go
func BenchmarkTransformMatrix(b *testing.B) {
	bodySizes := []int{1 << 10, 1 << 20, 20 << 20}
	wordCounts := []int{0, 1, 32, 256, 1024}
	for _, size := range bodySizes {
		for _, words := range wordCounts {
			for _, fold := range []bool{false, true} {
				name := fmt.Sprintf("body=%d/words=%d/fold=%t", size, words, fold)
				b.Run(name, func(b *testing.B) {
					body, cfg := benchmarkFixture(size, words, fold)
					b.ReportAllocs()
					b.SetBytes(int64(len(body)))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := transformRequest(body, "openai", cfg); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		}
	}
}
```

Add named scenario, concurrent snapshot and in-process RPC benchmarks:

```go
func BenchmarkTransformScenarios(b *testing.B) {
	cases := []struct {
		name, yaml, text string
		nodes            int
		lastText         string
		excludedPosition string
	}{
		{name: "mode=block/match=none/nodes=1", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 1},
		{name: "mode=block/match=sparse/nodes=1000", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 1000, lastText: "SECRET"},
		{name: "mode=strip/match=dense", yaml: "mode: strip\nwords: [SECRET]\n", text: strings.Repeat("SECRET", 128), nodes: 1},
		{name: "mode=strip/match=overlap", yaml: "mode: strip\nwords: [aa]\n", text: "aaa", nodes: 1},
		{name: "mode=strip/match=cascade", yaml: "mode: strip\nwords: [AB, x]\n", text: "ABxABx", nodes: 1},
		{name: "mode=obfs/match=sparse", yaml: "mode: obfs\nwords: [SECRET]\n", text: "SECRET", nodes: 1},
		{name: "fold=ascii", yaml: "mode: strip\nignore_case: true\nwords: [Alpha]\n", text: "aLPHA", nodes: 1},
		{name: "fold=sigma", yaml: "mode: strip\nignore_case: true\nwords: [Σ]\n", text: "ςΣσ", nodes: 1},
		{name: "fold=kelvin", yaml: "mode: strip\nignore_case: true\nwords: [K]\n", text: "K", nodes: 1},
		{name: "fold=full-fold-miss", yaml: "mode: strip\nignore_case: true\nwords: [straße]\n", text: "STRASSE", nodes: 1},
		{name: "excluded=20MiB/before", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 1, excludedPosition: "before"},
		{name: "excluded=20MiB/middle", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 2, excludedPosition: "middle"},
		{name: "excluded=20MiB/after", yaml: "mode: block\nwords: [SECRET]\n", text: "plain", nodes: 1, excludedPosition: "after"},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			cfg := mustBenchmarkConfig(b, tc.yaml)
			body := benchmarkScenarioBody(tc.text, tc.lastText, tc.nodes, tc.excludedPosition)
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := transformRequest(body, "openai", cfg); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkConcurrentSnapshotSwap(b *testing.B) {
	old := loadedSnapshot()
	b.Cleanup(func() { installSnapshot(old) })
	const yamlA = "mode: strip\nwords: [alpha]\n"
	const yamlB = "mode: obfs\nignore_case: true\nwords: [Beta]\n"
	installSnapshot(mustBenchmarkConfig(b, yamlA))
	requestA, _ := json.Marshal(lifecycleRequest{ConfigYAML: []byte(yamlA), SchemaVersion: pluginabi.SchemaVersion})
	requestB, _ := json.Marshal(lifecycleRequest{ConfigYAML: []byte(yamlB), SchemaVersion: pluginabi.SchemaVersion})
	body := []byte(`{"messages":[{"role":"user","content":"alpha BETA"}]}`)
	stop := make(chan struct{})
	stopped := make(chan struct{})
	reconfigureErr := make(chan error, 1)
	go func() {
		defer close(stopped)
		for {
			select {
			case <-stop:
				return
			default:
				if _, err := handleMethod(pluginabi.MethodPluginReconfigure, requestA); err != nil {
					reconfigureErr <- err
					return
				}
				if _, err := handleMethod(pluginabi.MethodPluginReconfigure, requestB); err != nil {
					reconfigureErr <- err
					return
				}
			}
		}
	}()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := transformRequest(body, "openai", loadedSnapshot()); err != nil {
				b.Error(err)
			}
		}
	})
	close(stop)
	<-stopped
	select {
	case err := <-reconfigureErr:
		b.Fatal(err)
	default:
	}
}

func BenchmarkBeforeAuthRPCEnvelope(b *testing.B) {
	old := loadedSnapshot()
	b.Cleanup(func() { installSnapshot(old) })
	installSnapshot(mustBenchmarkConfig(b, "mode: block\nwords: [NEVER-MATCH]\n"))
	request, err := json.Marshal(pluginapi.RequestInterceptRequest{RequestID: "benchmark", SourceFormat: "openai", Body: benchmarkScenarioBody("plain", "", 1000, "")})
	if err != nil {
		b.Fatal(err)
	}
	b.Run("before_calls=1/after_calls=0", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(request)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := handleMethod(pluginabi.MethodRequestInterceptBefore, request); err != nil {
				b.Fatal(err)
			}
		}
	})
}
```

Add the fixture helpers:

```go
func mustBenchmarkConfig(b testing.TB, raw string) *configSnapshot {
	b.Helper()
	cfg, err := parseConfigYAML([]byte(raw))
	if err != nil {
		b.Fatal(err)
	}
	return cfg
}

func benchmarkFixture(size, wordCount int, fold bool) ([]byte, *configSnapshot) {
	rules := make([]compiledRule, wordCount)
	for i := range rules {
		term := fmt.Sprintf("term-%04d", i)
		rules[i] = compiledRule{Term: term, Runes: []rune(term)}
	}
	cfg := &configSnapshot{
		Mode:       modeBlock,
		IgnoreCase: fold,
		Rules:      rules,
		Formats:    scopeSet{"openai": {}},
		Roles:      scopeSet{"user": {}},
		ObfsChar:   "​",
	}
	const prefix = `{"messages":[{"role":"user","content":"`
	const suffix = `"}]}`
	if size < len(prefix)+len(suffix) {
		panic("benchmark body size too small")
	}
	body := []byte(prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix)
	return body, cfg
}

func benchmarkScenarioBody(text, lastText string, nodes int, excludedPosition string) []byte {
	if nodes < 1 {
		nodes = 1
	}
	messages := make([]string, 0, nodes+1)
	for i := 0; i < nodes; i++ {
		nodeText := text
		if lastText != "" && i == nodes-1 {
			nodeText = lastText
		}
		messages = append(messages, `{"role":"user","content":`+strconv.Quote(nodeText)+`}`)
	}
	if excludedPosition != "" {
		payload := strings.Repeat("SECRET", (20<<20)/len("SECRET")+1)[:20<<20]
		image := `{"role":"user","content":[{"type":"image_url","image_url":{"url":` + strconv.Quote(payload) + `}}]}`
		switch excludedPosition {
		case "before":
			messages = append([]string{image}, messages...)
		case "middle":
			at := len(messages) / 2
			messages = append(messages, "")
			copy(messages[at+1:], messages[at:])
			messages[at] = image
		case "after":
			messages = append(messages, image)
		default:
			panic("unknown excluded position")
		}
	}
	return []byte(`{"messages":[` + strings.Join(messages, ",") + `]}`)
}
```

The helpers import `strconv`; all fixture construction occurs before `ResetTimer`.

- [ ] **Step 2: Run unit benchmarks**

Run: `go test -run '^$' -bench . -benchmem -benchtime=1x`

Expected: every named benchmark executes once without panic; record the output in the task review notes, not in source code.

- [ ] **Step 3: Run the complete unit/RPC benchmark command**

Run: `go test -run '^$' -bench . -benchmem`

Expected: every unit and in-process RPC benchmark executes and reports allocations. No dynamic loader claim is made in this task.

- [ ] **Step 4: Review and commit**

Review `benchmark_test.go` for actual matrix coverage, no setup inside timed loops and the accurate `before_calls=1/after_calls=0` label on the in-process BeforeAuth RPC benchmark. Do not add Aho-Corasick without measured evidence and a separate approved change.

```bash
git add benchmark_test.go
git commit -m "perf: benchmark censorship request paths"
```

---

### Task 16: Release Packager

**Files:**
- Create: `.github/scripts/package-release.go`
- Create: `.github/scripts/package-release_test.go`

**Interfaces:**
- Consumes: platform libraries under `dist/<goos>_<goarch>/censorship.<ext>`.
- Produces: seven archive specs, `normalizeReleaseVersion`, zip creation, per-archive sha256 and aggregate checksums.

- [ ] **Step 1: Write failing packager tests**

Copy the model-mapper packager test structure and assert the exact censorship contract:

```go
func TestArtifactSpecs(t *testing.T) {
	got := artifactSpecs()
	want := []artifactSpec{
		{osName: "linux", arch: "amd64"},
		{osName: "linux", arch: "arm64"},
		{osName: "darwin", arch: "amd64"},
		{osName: "darwin", arch: "arm64"},
		{osName: "windows", arch: "amd64"},
		{osName: "windows", arch: "arm64"},
		{osName: "freebsd", arch: "amd64"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("artifactSpecs() = %#v, want %#v", got, want)
	}
}

func TestNormalizeReleaseVersionRemovesOneASCIILeadingV(t *testing.T) {
	for input, want := range map[string]string{"v1.2.3": "1.2.3", "1.2.3": "1.2.3", "vv1": "v1", "V1": "V1"} {
		if got := normalizeReleaseVersion(input); got != want {
			t.Errorf("normalizeReleaseVersion(%q) = %q, want %q", input, got, want)
		}
	}
}
```

Add the archive/checksum and version-source tests:

```go
func TestPackageLibraryAndChecksumContract(t *testing.T) {
	tmp := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.WriteFile("LICENSE", []byte("fixture license\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(tmp, "censorship.so")
	if err := os.WriteFile(library, []byte("fixture library"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(tmp, "censorship_1.2.3_linux_amd64.zip")
	checksum := archive + ".sha256"
	if err := packageLibrary(library, archive); err != nil {
		t.Fatal(err)
	}
	if err := writeChecksum(checksum, archive); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 2 || zr.File[0].Name != "censorship.so" || zr.File[1].Name != "LICENSE" {
		t.Fatalf("zip entries = %#v", zipNames(zr.File))
	}
	if zr.File[0].Mode().Perm() != 0o755 || strings.Contains(zr.File[0].Name, "/") {
		t.Fatalf("library header = name %q mode %v", zr.File[0].Name, zr.File[0].Mode())
	}
	line, err := os.ReadFile(checksum)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}  censorship_1\.2\.3_linux_amd64\.zip\n$`).Match(line) {
		t.Fatalf("checksum = %q", line)
	}
}

func TestResolveVersionSources(t *testing.T) {
	if got, err := resolveVersion("v1.2.3"); err != nil || got != "1.2.3" {
		t.Fatalf("flag version = %q, %v", got, err)
	}
	t.Setenv("VERSION", "v1.2.3")
	if got, err := resolveVersion(""); err != nil || got != "1.2.3" {
		t.Fatalf("env version = %q, %v", got, err)
	}
	t.Setenv("VERSION", "")
	repo := t.TempDir()
	runGitTest(t, repo, "init")
	runGitTest(t, repo, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "fixture")
	runGitTest(t, repo, "tag", "v1.2.3")
	old, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if got, err := resolveVersion(""); err != nil || got != "1.2.3" {
		t.Fatalf("tag version = %q, %v", got, err)
	}
}
```

Add these helpers in the same test file:

```go
func zipNames(files []*zip.File) []string {
	names := make([]string, len(files))
	for i, file := range files {
		names[i] = file.Name
	}
	return names
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
```

The script test imports `archive/zip`、`os`、`os/exec`、`path/filepath`、`reflect`、`regexp`、`strings` and `testing`.

- [ ] **Step 2: Run the RED tests**

Run: `go test .github/scripts/package-release.go .github/scripts/package-release_test.go`

Expected: FAIL because the packager files do not exist.

- [ ] **Step 3: Implement the minimal packager**

Adapt `DoingDog/cpa-plugin-model-mapper@41e5591` `.github/scripts/package-release.go` with `const pluginName = "censorship"`. Preserve its two modes:

- direct `-library/-archive/-checksum` packaging;
- `-version/-dist/-out` packaging of every existing supported artifact plus aggregate `checksums.txt`.

`normalizeReleaseVersion` trims surrounding whitespace and removes exactly one lowercase ASCII `v`. Zip only the library and optional repository-root `LICENSE`; set library mode 0755. All checksum filenames use archive basenames and two ASCII spaces.

Do not create a repository `LICENSE`; choosing a project license is outside this implementation request. The temporary fixture above still verifies that the packager includes an existing repository-root `LICENSE` when one is present.

- [ ] **Step 4: Run packager tests**

Run:

```bash
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Review and commit**

Review both script files for exact seven-platform order, zip root contents, version normalization and checksum formatting.

```bash
git add .github/scripts/package-release.go .github/scripts/package-release_test.go
git commit -m "build: package censorship releases"
```

### Task 17: Fixed-CPA Integration Runner, HTTP/SSE/Watcher and Responses WebSocket

**Files:**
- Create: `integration/doc.go`
- Create: `integration/harness_test.go`
- Create: `integration/http_test.go`
- Create: `integration/websocket_test.go`
- Create: `integration/abi_benchmark_test.go`
- Create: `.github/scripts/integration-runner.go`
- Create: `.github/scripts/integration-runner_test.go`

**Interfaces:**
- Consumes: built censorship dynamic library, fixed CPA source and its CLI `--config` mode.
- Produces: a cross-platform runner that verifies checkout SHA, black-box HTTP/SSE/Responses WebSocket behavior, watcher linearization and dynamic ABI benchmarks.

- [ ] **Step 1: Write the failing runner self-check and integration tests**

Create `integration/doc.go`:

```go
package censorshipintegration
```

All integration tests use:

```go
//go:build integration

package censorshipintegration
```

In `integration/harness_test.go`, define:

```go
const (
	cpaSHA       = "81e1b5374f99c212f196f34956eeed964a46b8fa"
	downstreamKey = "censorship-integration-key"
	modelName     = "censorship-integration-model"
)

type upstreamCapture struct {
	mu       sync.Mutex
	requests [][]byte
}

type cpaInstance struct {
	cmd      *exec.Cmd
	config   string
	baseURL  string
	wsURL    string
	waitDone chan error
	logPath  string
}
```

`newMockUpstream` records every request body. For non-stream requests it returns fixed status 200, headers `Content-Type: application/json`, `Date: Tue, 01 Sep 2026 00:00:00 GMT`, `X-Censorship-Fixture: fixed`, and a fixed OpenAI response. For stream requests it writes three fixed SSE events with a Flush after each and then `[DONE]`. It accepts `/chat/completions`、`/completions` and `/responses` paths.

`startCPA` reads `CPA_INTEGRATION_BIN`, uses a free loopback port, writes a minimal config with `openai-compatibility`, `api-keys`, plugins enabled/disabled, plugin dir from `CENSORSHIP_PLUGIN_DIR`, and the passed censorship YAML. It launches `<bin> --config config.yaml --no-browser`, waits by polling authenticated `/v1/models`, and registers cleanup that sends interrupt, then terminate, then kill with bounded waits.

Create `.github/scripts/integration-runner_test.go` against the pure runner helpers. Do not create `.github/scripts/integration-runner.go` during this RED step:

```go
package main

func TestRunnerPathsStayUnderIntegrationRoot(t *testing.T) {
	root := t.TempDir()
	paths, err := resolveRunnerPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths.checkout, paths.bin, paths.run} {
		rel, err := filepath.Rel(paths.integrationRoot, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("path %q escapes %q", path, paths.integrationRoot)
		}
	}
}

func TestPluginExtension(t *testing.T) {
	for goos, want := range map[string]string{"windows": ".dll", "darwin": ".dylib", "linux": ".so", "freebsd": ".so"} {
		got, err := pluginExtension(goos)
		if err != nil || got != want {
			t.Errorf("pluginExtension(%q) = %q, %v; want %q", goos, got, err, want)
		}
	}
	if _, err := pluginExtension("plan9"); err == nil {
		t.Fatal("unsupported GOOS accepted")
	}
}

func TestVerifyCheckoutRejectsWrongHEAD(t *testing.T) {
	dir := t.TempDir()
	runGitTest(t, dir, "init")
	runGitTest(t, dir, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "fixture")
	if err := verifyCheckout(dir, cpaSHA); err == nil {
		t.Fatal("wrong checkout HEAD accepted")
	}
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
```

The runner source therefore exposes package-private `resolveRunnerPaths(root string) (runnerPaths, error)`、`pluginExtension(goos string) (string, error)` and `verifyCheckout(path, wantSHA string) error`; tests import `os/exec`、`path/filepath`、`strings` and `testing`.

Create `integration/abi_benchmark_test.go`. The runner copies it inside the fixed CPA module so it can import `internal/config` and `internal/pluginhost`:

```go
//go:build integration

package censorshipintegration

func BenchmarkDynamicABIRequestInterceptors(b *testing.B) {
	pluginDir := os.Getenv("CENSORSHIP_PLUGIN_DIR")
	if pluginDir == "" {
		b.Fatal("CENSORSHIP_PLUGIN_DIR is required")
	}
	rawConfig := fmt.Sprintf("plugins:\n  enabled: true\n  dir: %q\n  configs:\n    censorship:\n      enabled: true\n      mode: strip\n      words: [BLOCKME]\n", pluginDir)
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(rawConfig), &cfg); err != nil {
		b.Fatal(err)
	}
	host := pluginhost.New()
	host.ApplyConfig(context.Background(), &cfg)
	b.Cleanup(host.ShutdownAll)
	if !host.PluginRegistered("censorship") {
		b.Fatal("censorship plugin was not registered")
	}

	loader := "unix"
	if runtime.GOOS == "windows" {
		loader = "windows"
	}
	for _, size := range []int{1 << 10, 1 << 20, 20 << 20} {
		body := dynamicABIBody(size)
		req := pluginapi.RequestInterceptRequest{RequestID: "abi-benchmark", SourceFormat: "openai", Body: body}
		cases := []struct {
			phase string
			call  func() pluginapi.RequestInterceptResponse
			check func(pluginapi.RequestInterceptResponse) bool
		}{
			{phase: "before", call: func() pluginapi.RequestInterceptResponse { return host.InterceptRequestBeforeAuth(context.Background(), req) }, check: func(resp pluginapi.RequestInterceptResponse) bool { return !bytes.Contains(resp.Body, []byte("BLOCKME")) }},
			{phase: "after", call: func() pluginapi.RequestInterceptResponse { return host.InterceptRequestAfterAuth(context.Background(), req) }, check: func(resp pluginapi.RequestInterceptResponse) bool { return bytes.Equal(resp.Body, body) }},
		}
		for _, tc := range cases {
			name := fmt.Sprintf("GOOS=%s/loader=%s/phase=%s/calls=1/body=%d", runtime.GOOS, loader, tc.phase, size)
			b.Run(name, func(b *testing.B) {
				if resp := tc.call(); !tc.check(resp) {
					b.Fatalf("%s oracle failed: body length %d", tc.phase, len(resp.Body))
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(body)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if resp := tc.call(); !tc.check(resp) {
						b.Fatalf("%s oracle failed", tc.phase)
					}
				}
			})
		}
	}
}

func dynamicABIBody(size int) []byte {
	const prefix = `{"messages":[{"role":"user","content":"`
	const suffix = ` BLOCKME"}]}`
	if size < len(prefix)+len(suffix) {
		panic("dynamic ABI body size too small")
	}
	return []byte(prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix)
}
```

The file imports `bytes`、`context`、`fmt`、`os`、`runtime`、`strings`、`testing`、CPA `internal/config`、CPA `internal/pluginhost`、`sdk/pluginapi` and `gopkg.in/yaml.v3`. Setup and the first oracle call stay outside timed loops.

- [ ] **Step 2: Add failing HTTP, hot reload and transparency tests**

`integration/http_test.go` must contain:

```go
func TestHTTPBlockIncludesTermAndRole(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nwords: [Alpha]\nignore_case: true\n")
	status, header, body := postJSON(t, cpa.baseURL+"/v1/chat/completions", []byte(`{"model":"censorship-integration-model","messages":[{"role":"user","content":"aLPHA"}]}`))
	if status != 400 || header.Get("Content-Type") != "application/json" || gjson.GetBytes(body, "error.term").String() != "Alpha" || gjson.GetBytes(body, "error.role").String() != "user" {
		t.Fatalf("status=%d header=%v body=%s", status, header, body)
	}
	if upstream.requestCount() != 0 {
		t.Fatal("blocked request reached upstream")
	}
}

func TestLegacyCompletionsPromptUsesConvertedUserRole(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nignore_case: true\nwords: [Alpha]\n")
	status, _, body := postJSON(t, cpa.baseURL+"/v1/completions", []byte(`{"model":"censorship-integration-model","prompt":"aLPHA legacy prompt"}`))
	if status != 400 || gjson.GetBytes(body, "error.term").String() != "Alpha" || gjson.GetBytes(body, "error.role").String() != "user" {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if upstream.requestCount() != 0 {
		t.Fatal("blocked legacy prompt reached upstream")
	}
}

func TestWatcherReloadLinearizesAtObservedSnapshotB(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nwords: [alpha-only]\n")
	writePluginConfig(t, cpa.config, "mode: block\nignore_case: true\nwords: [beta-only]\n")
	deadline := time.Now().Add(20 * time.Second)
	for {
		status, _, body := postChat(t, cpa, "BETA-ONLY")
		if status == 400 && gjson.GetBytes(body, "error.term").String() == "beta-only" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("snapshot B not observed, last status=%d body=%s", status, body)
		}
		time.Sleep(25 * time.Millisecond)
	}
	for i := 0; i < 50; i++ {
		status, _, body := postChat(t, cpa, "BETA-ONLY")
		if status != 400 || gjson.GetBytes(body, "error.term").String() != "beta-only" {
			t.Fatalf("post-linearization request %d saw non-B config: %d %s", i, status, body)
		}
	}
}
```

Add the output-trace test:

```go
type responseTrace struct {
	Status  int
	Headers http.Header
	Chunks  [][]byte
}

func TestHTTPAndSSEOutputTraceUnaffected(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("nonmatching/stream=%t", stream), func(t *testing.T) {
			disabledUpstream := newMockUpstream(t)
			enabledUpstream := newMockUpstream(t)
			disabled := startCPA(t, disabledUpstream.URL, false, "")
			enabled := startCPA(t, enabledUpstream.URL, true, "mode: strip\nwords: [NEVER-MATCH]\n")
			body := chatBody("plain input", stream)
			gotDisabled := captureHTTP11Trace(t, disabled.baseURL+"/v1/chat/completions", downstreamKey, body)
			gotEnabled := captureHTTP11Trace(t, enabled.baseURL+"/v1/chat/completions", downstreamKey, body)
			if !reflect.DeepEqual(gotEnabled, gotDisabled) {
				t.Fatalf("enabled trace = %#v, disabled trace = %#v", gotEnabled, gotDisabled)
			}
		})
	}

	for _, tc := range []struct {
		name, config, input, transformed string
	}{
		{name: "strip", config: "mode: strip\nwords: [SECRET]\n", input: "before SECRET after", transformed: "before  after"},
		{name: "obfs", config: "mode: obfs\nwords: [SECRET]\n", input: "before SECRET after", transformed: "before S​ECRET after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			disabledUpstream := newMockUpstream(t)
			enabledUpstream := newMockUpstream(t)
			disabled := startCPA(t, disabledUpstream.URL, false, "")
			enabled := startCPA(t, enabledUpstream.URL, true, tc.config)
			body := chatBody(tc.input, true)
			disabledTrace := captureHTTP11Trace(t, disabled.baseURL+"/v1/chat/completions", downstreamKey, body)
			enabledTrace := captureHTTP11Trace(t, enabled.baseURL+"/v1/chat/completions", downstreamKey, body)
			if !bytes.Contains(enabledUpstream.lastRequest(), []byte(tc.transformed)) {
				t.Fatalf("upstream body = %s", enabledUpstream.lastRequest())
			}
			if !reflect.DeepEqual(enabledTrace, disabledTrace) {
				t.Fatalf("enabled trace = %#v, disabled trace = %#v", enabledTrace, disabledTrace)
			}
		})
	}
}
```

`captureHTTP11Trace` opens a raw TCP connection, sends `Connection: close` and `Accept-Encoding: identity`, parses the status line and headers, then either reads the exact `Content-Length` payload as one chunk or parses each HTTP/1.1 chunk-size line and exact chunk payload until the zero chunk. It canonicalizes headers into cloned `http.Header` values but does not merge adjacent chunks; each parsed transfer chunk is one upstream flush observation. `chatBody(text string, stream bool)` JSON-marshals the fixed model, one user string message and stream flag. These two helpers live in `integration/harness_test.go`; they never use `resp.Body.Read` boundaries.

- [ ] **Step 3: Add failing Responses WebSocket tests**

`integration/websocket_test.go` imports `github.com/gorilla/websocket@v1.5.3` from the fixed CPA module after the runner copies the test into that module, and sends:

```go
func TestResponsesWebSocketModelTurnUsesResponsesSelector(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: strip\nignore_case: true\nwords: [Alpha]\n")
	conn := dialResponsesWebSocket(t, cpa.wsURL, downstreamKey)
	payload := []byte(`{"type":"response.create","model":"censorship-integration-model","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"aLPHA websocket"}]}]}`)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatal(err)
	}
	messages := readUntilCompleted(t, conn)
	if len(messages) == 0 {
		t.Fatal("no websocket completion messages")
	}
	captured := upstream.lastRequest()
	if bytes.Contains(captured, []byte("aLPHA")) || !bytes.Contains(captured, []byte(" websocket")) {
		t.Fatalf("upstream body = %s", captured)
	}
}
```

Add the block and output-message tests:

```go
type wsMessage struct {
	Opcode  int
	Payload []byte
}

func TestResponsesWebSocketBlockReturnsStatus400ThenCloses(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nwords: [SECRET]\n")
	conn := dialResponsesWebSocket(t, cpa.wsURL, downstreamKey)
	payload := []byte(`{"type":"response.create","model":"censorship-integration-model","input":"SECRET"}`)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatal(err)
	}
	opcode, event, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if opcode != websocket.TextMessage || gjson.GetBytes(event, "status").Int() != 400 {
		t.Fatalf("opcode=%d event=%s", opcode, event)
	}
	if gjson.GetBytes(event, "error.term").Exists() || gjson.GetBytes(event, "error.role").Exists() {
		t.Fatalf("WebSocket unexpectedly retained plugin direct body: %s", event)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("connection stayed open after terminal 400 event")
	}
	if upstream.requestCount() != 0 {
		t.Fatal("blocked WebSocket turn reached upstream")
	}
}

func TestResponsesWebSocketOutputMessagesUnaffected(t *testing.T) {
	payload := []byte(`{"type":"response.create","model":"censorship-integration-model","input":"plain input"}`)
	disabledUpstream := newMockUpstream(t)
	enabledUpstream := newMockUpstream(t)
	disabled := startCPA(t, disabledUpstream.URL, false, "")
	enabled := startCPA(t, enabledUpstream.URL, true, "mode: strip\nwords: [NEVER-MATCH]\n")
	gotDisabled := responsesWSExchange(t, disabled, payload)
	gotEnabled := responsesWSExchange(t, enabled, payload)
	if !reflect.DeepEqual(gotEnabled, gotDisabled) {
		t.Fatalf("enabled messages = %#v, disabled messages = %#v", gotEnabled, gotDisabled)
	}

	for _, config := range []string{
		"mode: strip\nwords: [SECRET]\n",
		"mode: obfs\nwords: [SECRET]\n",
	} {
		transformedUpstream := newMockUpstream(t)
		baselineUpstream := newMockUpstream(t)
		transformed := startCPA(t, transformedUpstream.URL, true, config)
		baseline := startCPA(t, baselineUpstream.URL, false, "")
		matching := []byte(`{"type":"response.create","model":"censorship-integration-model","input":"SECRET input"}`)
		gotTransformed := responsesWSExchange(t, transformed, matching)
		gotBaseline := responsesWSExchange(t, baseline, matching)
		if bytes.Contains(transformedUpstream.lastRequest(), []byte(`"SECRET input"`)) {
			t.Fatalf("input was not transformed: %s", transformedUpstream.lastRequest())
		}
		if !reflect.DeepEqual(gotTransformed, gotBaseline) {
			t.Fatalf("transformed messages = %#v, baseline messages = %#v", gotTransformed, gotBaseline)
		}
	}
}

func responsesWSExchange(t *testing.T, cpa *cpaInstance, payload []byte) []wsMessage {
	t.Helper()
	conn := dialResponsesWebSocket(t, cpa.wsURL, downstreamKey)
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatal(err)
	}
	return readUntilCompleted(t, conn)
}
```

`readUntilCompleted` returns `[]wsMessage` and copies each payload before the next read. It stops only after the fixed completion event, preserving opcode and message order.

Record prewarm and Realtime as explicit non-tests in the README task, not skipped tests.

- [ ] **Step 4: Run and observe RED before runner implementation**

Run:

```bash
go test .github/scripts/integration-runner_test.go
```

Expected: compile FAIL with undefined `cpaSHA`、`resolveRunnerPaths`、`pluginExtension` and `verifyCheckout`; the production runner source does not exist yet.

- [ ] **Step 5: Implement the integration runner**

Create `.github/scripts/integration-runner.go` in package `main` with:

```go
const cpaSHA = "81e1b5374f99c212f196f34956eeed964a46b8fa"
```

The runner must:

1. Resolve repository root and `.integration/{cpa,bin,run}`.
2. If checkout HEAD differs, remove only `.integration/cpa`, then run `git init`, `git remote add origin https://github.com/router-for-me/CLIProxyAPI`, `git fetch --depth=1 origin 81e1...`, `git checkout --detach FETCH_HEAD`.
3. Run `git rev-parse HEAD` and require the full exact SHA.
4. Build CPA with `go build -trimpath -o <bin> ./cmd/server` in the checkout.
5. Build this plugin with `CGO_ENABLED=1 go build -trimpath -buildmode=c-shared -o <run>/plugins/<goos>/<goarch>/censorship.<ext> .` and remove the generated header.
6. Copy every `integration/*.go` file into `<checkout>/integration/censorshipplugin`, preserving build tags.
7. Execute `go test -tags=integration -count=1 -v ./integration/censorshipplugin` inside the checkout with `CPA_INTEGRATION_BIN` and `CENSORSHIP_PLUGIN_DIR` environment variables.
8. When `BENCH=1`, append `-run ^$ -bench . -benchmem` and do not run behavior tests a second time.
9. Stream child stdout/stderr and propagate nonzero exit status.

Use `os/exec` argument arrays, not shell interpolation. Delete only paths under the resolved `.integration` root after checking with `filepath.Rel` that they do not escape it.

- [ ] **Step 6: Run integration and full tests**

Run:

```bash
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
go run ./.github/scripts/integration-runner.go
go test ./...
go test -race ./...
```

Then run the dynamic benchmark on Windows PowerShell:

```powershell
$env:BENCH = "1"; go run ./.github/scripts/integration-runner.go; Remove-Item Env:BENCH
```

On POSIX use `BENCH=1 go run ./.github/scripts/integration-runner.go`. Expected: runner unit tests, integration HTTP/SSE/watcher/WebSocket tests, normal/race tests and all six dynamic ABI cases PASS; benchmark names print GOOS, loader, phase and `calls=1`.

- [ ] **Step 7: Review and commit**

Review runner filesystem/process safety, harness cleanup, each protocol test, and dynamic ABI benchmark as separate files in parallel. Then run one cross-file review of env names and copied package paths.

```bash
git add integration .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
git commit -m "test: verify plugin against fixed cpa"
```

---

### Task 18: Makefile and Seven-platform GitHub CI/CD

**Files:**
- Create: `Makefile`
- Create: `.github/workflows/build.yml`
- Create: `RELEASE_NOTES.md`
- Modify: `.github/scripts/package-release_test.go`
- Modify: `.gitignore`

**Interfaces:**
- Consumes: tests, integration runner and packager.
- Produces: `test`, `race`, `vet`, `integration`, `build-platform`, host-specific `build`, `package-platform`, `package`, `clean` and tag release automation.

- [ ] **Step 1: Write a failing CLI-level build contract test**

Extend `.github/scripts/package-release_test.go` with a subprocess test that runs:

```go
func TestPackagerCLIVersionSourcesProduceSameBasename(t *testing.T) {
	script, err := filepath.Abs("package-release.go")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	dist := filepath.Join(tmp, "dist")
	if err := os.MkdirAll(filepath.Join(dist, "linux_amd64"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "linux_amd64", "censorship.so"), []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	tagRepo := filepath.Join(tmp, "tag-repo")
	if err := os.MkdirAll(tagRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, tagRepo, "init")
	runGitTest(t, tagRepo, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "fixture")
	runGitTest(t, tagRepo, "tag", "v1.2.3")

	cases := []struct {
		name, dir, envVersion string
		args                  []string
	}{
		{name: "flag", dir: ".", args: []string{"-version", "v1.2.3"}},
		{name: "environment", dir: ".", envVersion: "v1.2.3"},
		{name: "exact tag", dir: tagRepo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(tmp, strings.ReplaceAll(tc.name, " ", "-"))
			args := append([]string{"run", script}, tc.args...)
			args = append(args, "-dist", dist, "-out", out)
			cmd := exec.Command("go", args...)
			cmd.Dir = tc.dir
			cmd.Env = withEnvironment(os.Environ(), "VERSION", tc.envVersion)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("packager: %v\n%s", err, output)
			}
			if _, err := os.Stat(filepath.Join(out, "censorship_1.2.3_linux_amd64.zip")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func withEnvironment(base []string, key, value string) []string {
	out := make([]string, 0, len(base)+1)
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(name, key) {
			out = append(out, entry)
		}
	}
	return append(out, key+"="+value)
}
```

Add a static workflow parser test:

```go
type workflowFile struct {
	On   map[string]yaml.Node   `yaml:"on"`
	Env  map[string]string      `yaml:"env"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowPush struct {
	Branches []string `yaml:"branches"`
	Tags     []string `yaml:"tags"`
}

type workflowJob struct {
	Needs any    `yaml:"needs"`
	If    string `yaml:"if"`
	Strategy struct {
		Matrix struct {
			Include []workflowPlatform `yaml:"include"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
	Steps []workflowStep `yaml:"steps"`
}

type workflowPlatform struct {
	GOOS   string `yaml:"GOOS"`
	GOARCH string `yaml:"GOARCH"`
	Runner string `yaml:"runner"`
}

type workflowStep struct {
	ID   string         `yaml:"id"`
	Uses string         `yaml:"uses"`
	Run  string         `yaml:"run"`
	With map[string]any `yaml:"with"`
}

func TestBuildWorkflowContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "workflows", "build.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatalf("workflow YAML: %v", err)
	}
	for _, trigger := range []string{"pull_request", "push", "workflow_dispatch"} {
		if _, ok := workflow.On[trigger]; !ok {
			t.Fatalf("missing trigger %q", trigger)
		}
	}
	var push workflowPush
	if err := workflow.On["push"].Decode(&push); err != nil {
		t.Fatalf("decode push trigger: %v", err)
	}
	if !reflect.DeepEqual(push.Branches, []string{"main"}) || !reflect.DeepEqual(push.Tags, []string{"v*"}) {
		t.Fatalf("push trigger = %#v", push)
	}
	if workflow.Env["PLUGIN_NAME"] != "censorship" {
		t.Fatalf("workflow env = %#v", workflow.Env)
	}

	testRuns := nonemptyRuns(workflow.Jobs["test"].Steps)
	wantTestRuns := []string{
		"make test",
		"go test .github/scripts/package-release.go .github/scripts/package-release_test.go",
		"make race",
		"make vet",
		"make integration",
	}
	if !reflect.DeepEqual(testRuns, wantTestRuns) {
		t.Fatalf("test commands = %#v, want %#v", testRuns, wantTestRuns)
	}

	const buildCondition = "${{ github.event_name != 'pull_request' }}"
	const releaseCondition = "${{ startsWith(github.ref, 'refs/tags/v') }}"
	wantMatrix := []workflowPlatform{
		{GOOS: "linux", GOARCH: "amd64", Runner: "ubuntu-24.04"},
		{GOOS: "linux", GOARCH: "arm64", Runner: "ubuntu-24.04-arm"},
		{GOOS: "darwin", GOARCH: "amd64", Runner: "macos-15-intel"},
		{GOOS: "darwin", GOARCH: "arm64", Runner: "macos-15"},
		{GOOS: "windows", GOARCH: "amd64", Runner: "windows-2025"},
	}
	build := workflow.Jobs["build"]
	if !reflect.DeepEqual(build.Strategy.Matrix.Include, wantMatrix) || !reflect.DeepEqual(normalizeNeeds(build.Needs), []string{"test"}) || build.If != buildCondition {
		t.Fatalf("build job = %#v", build)
	}
	if !hasReleaseMetadata(build) ||
		!jobRunContains(build, "make package") ||
		!jobRunContains(build, `VERSION="${VERSION}"`) ||
		!jobRunContains(build, "GOOS=${{ matrix.GOOS }}") ||
		!jobRunContains(build, "GOARCH=${{ matrix.GOARCH }}") ||
		!jobUses(build, "actions/upload-artifact@v4") {
		t.Fatal("matrix package/upload contract is incomplete")
	}

	cross := map[string]struct {
		target, dir, library, archive string
	}{
		"build-windows-arm64": {target: "windows-arm64", dir: "windows_arm64", library: "censorship.dll", archive: "censorship_${VERSION}_windows_arm64.zip"},
		"build-freebsd-amd64": {target: "freebsd-amd64", dir: "freebsd_amd64", library: "censorship.so", archive: "censorship_${VERSION}_freebsd_amd64.zip"},
	}
	for id, tc := range cross {
		job := workflow.Jobs[id]
		libraryPath := "dist/" + tc.dir + "/" + tc.library
		if !reflect.DeepEqual(normalizeNeeds(job.Needs), []string{"test"}) ||
			job.If != buildCondition ||
			!hasReleaseMetadata(job) ||
			!jobUses(job, "go-cross/cgo-actions@v1") ||
			jobActionWith(job, "go-cross/cgo-actions@v1", "targets") != tc.target ||
			jobActionWith(job, "go-cross/cgo-actions@v1", "out-dir") != "dist/"+tc.dir ||
			jobActionWith(job, "go-cross/cgo-actions@v1", "output") != tc.library ||
			jobActionWith(job, "go-cross/cgo-actions@v1", "x-flags") != "main.pluginVersion=${{ steps.release_metadata.outputs.version }}" ||
			!jobRunContains(job, "go run ./.github/scripts/package-release.go") ||
			!jobRunContains(job, "-library \""+libraryPath+"\"") ||
			!jobRunContains(job, "-archive \"dist/"+tc.archive+"\"") ||
			!jobRunContains(job, "-checksum \"dist/"+tc.archive+".sha256\"") ||
			!jobUses(job, "actions/upload-artifact@v4") {
			t.Fatalf("cross job %s = %#v", id, job)
		}
	}

	release := workflow.Jobs["release"]
	wantNeeds := []string{"build", "build-freebsd-amd64", "build-windows-arm64"}
	gotNeeds := normalizeNeeds(release.Needs)
	sort.Strings(gotNeeds)
	if !reflect.DeepEqual(gotNeeds, wantNeeds) ||
		release.If != releaseCondition ||
		!jobUses(release, "actions/checkout@v5") ||
		jobActionWith(release, "actions/download-artifact@v4", "path") != "release" ||
		!jobActionWithBool(release, "actions/download-artifact@v4", "merge-multiple") ||
		!jobRunContains(release, "cat release/*.sha256 | sort > release/checksums.txt") ||
		!jobRunContains(release, "gh release upload") ||
		!jobRunContains(release, "--clobber") ||
		!jobRunContains(release, "gh release create") ||
		!jobRunContains(release, "--verify-tag") ||
		!jobRunContains(release, `gh release edit "$tag" --notes-file RELEASE_NOTES.md`) ||
		!jobRunContains(release, "--notes-file RELEASE_NOTES.md") {
		t.Fatalf("release job = %#v", release)
	}
}

func nonemptyRuns(steps []workflowStep) []string {
	var runs []string
	for _, step := range steps {
		if run := strings.TrimSpace(step.Run); run != "" {
			runs = append(runs, run)
		}
	}
	return runs
}

func normalizeNeeds(value any) []string {
	switch needs := value.(type) {
	case string:
		return []string{needs}
	case []any:
		out := make([]string, 0, len(needs))
		for _, item := range needs {
			need, ok := item.(string)
			if !ok {
				return []string{"<invalid>"}
			}
			out = append(out, need)
		}
		return out
	case nil:
		return nil
	default:
		return []string{"<invalid>"}
	}
}

func jobRunContains(job workflowJob, fragment string) bool {
	for _, step := range job.Steps {
		if strings.Contains(step.Run, fragment) {
			return true
		}
	}
	return false
}

func hasReleaseMetadata(job workflowJob) bool {
	for _, step := range job.Steps {
		if step.ID != "release_metadata" {
			continue
		}
		return strings.Contains(step.Run, `VERSION="${GITHUB_REF_NAME#v}"`) &&
			strings.Contains(step.Run, `VERSION="0.0.0-dev"`) &&
			strings.Contains(step.Run, `echo "VERSION=${VERSION}" >> "${GITHUB_ENV}"`) &&
			strings.Contains(step.Run, `echo "version=${VERSION}" >> "${GITHUB_OUTPUT}"`)
	}
	return false
}

func jobActionWith(job workflowJob, action, key string) string {
	for _, step := range job.Steps {
		if step.Uses == action {
			value, _ := step.With[key].(string)
			return value
		}
	}
	return ""
}

func jobActionWithBool(job workflowJob, action, key string) bool {
	for _, step := range job.Steps {
		if step.Uses == action {
			value, _ := step.With[key].(bool)
			return value
		}
	}
	return false
}

func jobUses(job workflowJob, action string) bool {
	for _, step := range job.Steps {
		if step.Uses == action {
			return true
		}
	}
	return false
}
```

The final script test imports must include `bytes`、`os`、`os/exec`、`path/filepath`、`reflect`、`sort`、`strings` and `gopkg.in/yaml.v3`. Keep the existing exact `artifactSpecs` assertion as the seven-platform tuple oracle; this test checks workflow wiring and commands.

- [ ] **Step 2: Run RED tests**

Run: `go test .github/scripts/package-release.go .github/scripts/package-release_test.go`

Expected: FAIL because `.github/workflows/build.yml` does not exist.

- [ ] **Step 3: Implement Makefile**

Use model-mapper's structure with `PLUGIN_NAME := censorship`. Required targets:

```makefile
test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

integration:
	$(GO) test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
	$(GO) run ./.github/scripts/integration-runner.go
```

`build-platform` requires GOOS/GOARCH, chooses `.dll/.dylib/.so`, sets `CGO_ENABLED=1`, optional `BUILD_CC`, `-trimpath -buildmode=c-shared`, and `-ldflags='-s -w -X main.pluginVersion=$(NORMALIZED_VERSION)'`. `NORMALIZED_VERSION` strips exactly one lowercase leading `v` and defaults to `0.0.0-dev` only when VERSION is empty.

`package-platform` calls the packager direct mode and writes `dist/censorship_<normalized>_<goos>_<goarch>.zip[.sha256]`. `package` without GOOS/GOARCH packages existing artifacts. `build` is documented in target output as host-specific and only builds the current GOOS/GOARCH; it is not a shared gate.

- [ ] **Step 4: Implement GitHub Actions**

Adapt the model-mapper workflow exactly with these changes:

- env plugin name `censorship`; every build job has `id: release_metadata`, removes one leading `v` with `VERSION="${GITHUB_REF_NAME#v}"` for a `v*` tag, otherwise uses `VERSION="0.0.0-dev"`, then writes `VERSION` to `GITHUB_ENV` and `version` to `GITHUB_OUTPUT`;
- test job runs `make test`, packager test, `make race`, `make vet`, `make integration` in that order;
- build jobs run only outside PRs and depend on test;
- main matrix: linux/amd64 Ubuntu 24.04, linux/arm64 Ubuntu 24.04 ARM, darwin/amd64 macOS 15 Intel, darwin/arm64 macOS 15, windows/amd64 Windows 2025 plus MSYS2 MinGW;
- separate `go-cross/cgo-actions@v1` jobs for windows/arm64 and freebsd/amd64, with exact `targets`、`out-dir`、library `output` and `x-flags: main.pluginVersion=${{ steps.release_metadata.outputs.version }}`; after the action builds the library, each job calls `package-release.go` direct mode with `-library`、`-archive` and `-checksum` instead of rebuilding through `make package`;
- every platform produces one zip and `.sha256` with normalized tag or `0.0.0-dev` version;
- release job depends on all three build job IDs, which represent all seven platform outputs, downloads them with `path: release` plus `merge-multiple: true`, and runs `cat release/*.sha256 | sort > release/checksums.txt`;
- when the tag release exists, run `gh release edit "$tag" --notes-file RELEASE_NOTES.md` before `gh release upload "$tag" ... --clobber`;
- when it does not exist, run `gh release create "$tag" ... --verify-tag --notes-file RELEASE_NOTES.md`.

Create `RELEASE_NOTES.md` before running workflow tests. It must contain the complete YAML configuration and strict types/defaults, ordered `block`/`strip`/`obfs` and `ignore_case` semantics, supported selector and hard-exclusion summary, all eleven accepted pure-plugin limits from spec section 13 as eleven separately numbered items, and `censorship_<version>_<goos>_<goarch>.zip` plus `.sha256` naming. Do not use a generated changelog or GitHub auto-notes because those would omit the operational limits.

Use `actions/checkout@v5`, `actions/setup-go@v6`, `actions/upload-artifact@v4`, `actions/download-artifact@v4`, least-privilege contents permissions, and exact triggers from the spec.

- [ ] **Step 5: Run build script verification**

Run:

```bash
make test
make race
make vet
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
make integration
make package VERSION=v0.0.0-dev GOOS=windows GOARCH=amd64
```

Expected: commands PASS on the current Windows/amd64 host; archive basename is `censorship_0.0.0-dev_windows_amd64.zip`, contains only `censorship.dll` because this repository has no `LICENSE`, and checksum format matches.

- [ ] **Step 6: Review and commit**

Review Makefile and workflow independently, then cross-check seven platform tuples, version normalization and release dependencies.

```bash
git add Makefile .github/workflows/build.yml .github/scripts/package-release_test.go RELEASE_NOTES.md .gitignore
git commit -m "ci: build censorship plugin releases"
```

---

### Task 19: README, Accepted Limits and Final Verification

**Files:**
- Create: `README.md`
- Modify: `RELEASE_NOTES.md`
- Modify: `main_test.go`

**Interfaces:**
- Consumes: final public behavior and build commands.
- Produces: user-facing configuration, supported selector table, limits, installation and release documentation.

- [ ] **Step 1: Write the failing documentation assertion before creating README**

Do not create `README.md` or edit `RELEASE_NOTES.md` yet. Add the test below first; the GREEN documents must contain these sections and exact facts:

1. `censorship` purpose and request-only boundary.
2. Installation paths and library names for Windows/Linux/macOS/FreeBSD.
3. Complete YAML example containing `enabled`, `mode`, `ignore_case`, `words`, `scope.formats`, `scope.roles`, `obfs.char`.
4. Config table, including strict types/defaults, YAML order and last-known-good behavior.
5. Mode semantics, block error example, non-overlap and ordered cascade.
6. `ignore_case` semantics with examples `Alpha/aLPHA` match, sigma/Kelvin simple-fold match, `straße/STRASSE` no match, no normalization, obfs preserves original case.
7. Five-format selector table and exact optional assistant/tool scope.
8. Hard exclusions.
9. NonHome watcher behavior and observable B-sentinel procedure.
10. Build/test/package commands.
11. All eleven accepted pure-plugin limits from spec section 13, including hook not raw ingress, preprocessing visibility, Responses WS subset, no prewarm, no Realtime/Live/sideband/DataChannel, no Alpha Search, WS block body loss, fail-open, Before/After RPC call multiplicity/cost, Home no watcher, and schema drift default-no-op.
12. Release artifact/checksum naming.

`RELEASE_NOTES.md` created in Task 18 must contain the same complete YAML example, config contract, hard exclusions and eleven limits. Add this documentation contract test in `main_test.go`:

```go
func TestDocumentationListsConfigAndLimits(t *testing.T) {
	const configExample = `plugins:
  enabled: true
  configs:
    censorship:
      enabled: true
      mode: block
      ignore_case: false
      words:
        - example
      scope:
        formats: [openai, openai-response, claude, gemini, interactions]
        roles: [system, developer, user]
      obfs:
        char: "​"`

	required := []string{
		configExample,
		"no custom panel, menu, or Management API",
		"request-only; never inspects or changes model output",
		"`enabled`: boolean; host-owned",
		"`mode`: string; default `block`",
		"`ignore_case`: boolean; default `false`",
		"`words`: sequence of strings; default `[]`",
		"words is the only term source; the plugin has no built-in terms",
		"`scope.formats`: sequence of strings; default all five formats",
		"`scope.roles`: sequence of strings; default `system`, `developer`, and `user`",
		"`obfs.char`: must be `U+200B` or `U+2060`; default `U+200B`",
		"block checks rules before document order and returns the YAML term with canonical role",
		"strip and obfs process all leftmost non-overlapping occurrences before the next rule",
		"obfs preserves original case and inserts after the first Unicode scalar",
		"unicode.SimpleFold",
		"Alpha/aLPHA",
		"Greek sigma",
		"Kelvin sign",
		"straße/STRASSE does not match",
		"no Unicode normalization",
		"assistant is inspected only when explicitly listed in scope.roles",
		"tool is inspected only for OpenAI string tool content and Claude typed text tool_result content",
		"valid non-Home YAML changes apply without restart after observing a snapshot-B sentinel",
		"invalid reconfiguration keeps the last-known-good snapshot",
		"Never changes tool calls, tool schemas, arguments, reasoning, thinking, other tool/function results, JSON keys, machine JSON, binary uploads, or image/audio/video/file base64.",
		"1. hook is not raw ingress; document order follows current execution-body spans",
		"2. preprocessing can observe uncensored input",
		"3. Responses WebSocket covers only model-executed turns",
		"4. `generate=false` prewarm bypasses the plugin",
		"5. `/v1/realtime`, Live, sideband, and DataChannel bypass the plugin",
		"6. Alpha Search bypasses the plugin",
		"7. WebSocket block events omit `term` and `role`",
		"8. RequestInterceptor failures are fail-open",
		"9. BeforeAuth runs once per handler execution; AfterAuth can run zero, one, or multiple times; every call carries the full body and incurs full-body RPC encoding/copy cost",
		"10. Home mode does not watch local YAML",
		"11. unknown SourceFormat and future content types are not inspected; review schema drift when upgrading CPA",
		"censorship_<version>_<goos>_<goarch>.zip",
		".zip.sha256",
		"64 lowercase hex characters, two spaces, and the archive basename",
	}
	for _, name := range []string{"README.md", "RELEASE_NOTES.md"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range required {
			if !bytes.Contains(raw, []byte(token)) {
				t.Errorf("%s missing %q", name, token)
			}
		}
	}
}
```

Add `os` to `main_test.go` imports; `bytes` is already added in Task 3. The deliberately exact prose makes omissions in either user-facing document fail publicly.

- [ ] **Step 2: Run the RED documentation test before completing README**

Run: `go test ./... -run '^TestDocumentationListsConfigAndLimits$'`

Expected: FAIL until every required token and section is present.

- [ ] **Step 3: Complete README, synchronize release notes and run documentation test**

Create `README.md` with the twelve sections above. Edit `RELEASE_NOTES.md` so both files contain the exact YAML block, config table phrases, hard-exclusion sentence and separately numbered limitation phrases required by the test. Then run: `go test ./... -run '^TestDocumentationListsConfigAndLimits$'`

Expected: PASS.

- [ ] **Step 4: Run fresh complete verification**

Run every command, read full output and stop on the first failure:

```bash
gofmt -w abi_cgo.go main.go config.go matcher.go transform.go selectors*.go *_test.go integration/*.go .github/scripts/*.go
git diff --check
go test ./...
go test -race ./...
go vet ./...
go test -run '^$' -bench . -benchmem
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
make integration
make package VERSION=v0.0.0-dev GOOS=windows GOARCH=amd64
```

Then inspect the current-platform zip and checksum with the packager test helpers. Run `git status --short`, require no generated `dist`, `.integration` or header files are tracked.

Seven non-host build jobs are verified by the workflow definition and must execute in GitHub Actions on main/tag; do not claim those cross-platform binaries were locally executed unless their actual jobs ran.

- [ ] **Step 5: Parallel single-file review and cross-module review**

Dispatch independent read-only reviews for:

- `abi_cgo.go` and `main.go`;
- `config.go`;
- `matcher.go`;
- `transform.go`;
- each `selectors_*.go` file separately;
- fuzz/benchmark files;
- integration runner and each integration test file;
- packager, Makefile, workflow and README.

Every finding must include a concrete input/state and wrong result. Verify findings adversarially before fixing. After fixes, rerun the complete verification block. Finish with one cross-module review against every spec completion criterion and one schema-drift comparison against the fixed CPA translators.

- [ ] **Step 6: Commit documentation and verified fixes**

```bash
git add README.md RELEASE_NOTES.md main_test.go
git commit -m "docs: document censorship plugin behavior"
```

If review required code fixes, each logically independent fix gets its own test-backed commit before this documentation commit.

- [ ] **Step 7: Final repository evidence**

Run:

```bash
git log --oneline --decorate -20
git status --short --branch
git diff HEAD~1..HEAD --check
```

Expected: clean working tree; implementation commits are small and ordered; final fresh verification outputs are available in the Workflow run transcript.
