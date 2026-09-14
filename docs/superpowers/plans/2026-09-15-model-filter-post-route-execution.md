# Models Filter Post-Route Execution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 v0.3.1 的 outer-before 误拦截，让 non-stream `claude-opus -> kimi-k3` 在 `filter.models: [kimi-k3]`、`filter_mode: exclude` 时与 direct `kimi-k3` 一样绕过 censorship，并发布下一个未占用 patch version。

**Architecture:** 当 `filter.models` 为空时保持现有 BeforeAuth 处理；非空时 BeforeAuth 在 envelope decode 前 no-op，只有固定 CLIProxyAPI v7.2.152 提供有效 selected-auth marker 的 AfterAuth 才按 auth-selected attempt `request.Model` 执行完整 filter 与 `block -> strip -> obfs`。同一实例的 `plugin.reconfigure` 不允许在 model filter empty 与 non-empty phase class 之间切换。production code只依赖host导出的selected-auth metadata constants，不识别model-mapper；真实model-mapper v0.5.6仅作为pinned integration fixture。

**Tech Stack:** Go 1.26、CGO native ABI v1、CLIProxyAPI v7.2.152、model-mapper v0.5.6、Go testing、GitHub Actions。

**Spec:** `docs/superpowers/specs/2026-09-15-model-filter-post-route-execution-design.md`

## Global Constraints

- 只修改 censorship plugin repository，不修改 CLIProxyAPI core或model-mapper production source。
- production code不得识别model-mapper plugin ID、版本、priority、配置键、规则DSL或错误文本。
- model filter的subject是有效selected-auth AfterAuth收到的auth-selected attempt `RequestInterceptRequest.Model`，不从 `RequestedModel`、body、URL、headers、callback `source`或其他Metadata fallback。
- `selected_auth_id`与 `selected_auth_index` 使用pinned dependency中的 `executor.SelectedAuthMetadataKey`和 `executor.SelectedAuthIndexMetadataKey`，只接受非空、非纯空白string。
- 未配置 `filter.models` 时继续只在BeforeAuth处理；配置后BeforeAuth与无有效marker的AfterAuth必须在body处理前no-op。
- 保持API-key-only、exact/glob、大小写、完整字符串、`include`/`exclude`、`and`/`or`、五种SourceFormat、selectors和transform顺序。
- 同一已加载实例的reconfigure只能在相同model-filter phase class内更新；phase class、binary或enabled变化前需drain并restart，插件不实现跨invocation correlation。
- 保持ABI v1 ownership。AfterAuth开始读取input后必须恢复同步 `C.GoBytes`；不得保留host pointer。
- mapped nonexcluded在受支持callback路径中必须zero upstream。pinned host把nested terminal response降为outer HTTP 500是已知限制，不伪称400已保留。
- mapped `host.model.execute_stream`、opaque terminal Executor、AfterAuth之后的model rewrite和Antigravity credits fallback不在本release保证内。
- 不手改 `.integration/` 或 `dist/`；只由runner、Makefile和workflow生成。
- release需要七个平台zip、matching lowercase SHA-256 sidecars与aggregate `checksums.txt`。
- 行为修改严格使用TDD。production coding使用Sonnet 1M xhigh；spec、plan与review使用Opus 1M xhigh。Task 1至Task 5每个commit后立即完成独立spec/code-quality review并关闭finding，才能进入下一Task。

---

## File Map

- Modify: `.github/scripts/integration-runner.go`，构建pinned model-mapper fixture，并支持对prebuilt Windows DLL执行active-after ABI smoke。
- Modify: `.github/scripts/integration-runner_test.go`，锁定两个pinned revisions、runner paths、mode parsing、prebuilt library staging和test args。
- Modify: `.github/scripts/testdata/abi_benchmark_test.go`，让dynamic ABI oracle真正激活AfterAuth，并保留benchmark。
- Modify: `.github/workflows/build.yml`，在windows-amd64 artifact upload前执行prebuilt DLL active-after smoke。
- Modify: `integration/harness_test.go`，增加可配置provider models与model-mapper config的CPA fixture。
- Modify: `integration/http_test.go`，增加real mapper clean canary与direct/mapped excluded/nonexcluded矩阵。
- Modify: `main_test.go`，增加phase gate、marker、reconfigure与documentation tests，并提供按method调用request interceptor的helper。
- Modify: `request_filter_test.go`，把predicate合同改为始终按当前attempt `Model`。
- Modify: `abi_cgo_test.go`，先证明AfterAuth input必须复制并实际产生active response。
- Modify: `benchmark_test.go`，把model-bearing predicate fixture改为当前attempt `Model`，保留可比较benchmark名称。
- Modify: `main.go`，实现BeforeAuth/AfterAuth分流、selected-auth marker gate、共享transform和phase-class reconfigure拒绝。
- Modify: `request_filter.go`，删除callback-source selector，model branch直接使用 `request.Model`。
- Modify: `abi_cgo.go`，恢复AfterAuth同步 `C.GoBytes`。
- Modify: `README.md`、`RELEASE_NOTES.md`、旧spec、旧plan，替换错误合同并披露pinned host边界。
- Do not modify: `transform.go`、`selectors.go`、provider machine fields、CLIProxyAPI checkout内容、model-mapper checkout内容、generated artifacts。

## Task 1: Build the pinned real-mapper test infrastructure

**Files:**
- Modify: `.github/scripts/integration-runner.go`
- Modify: `.github/scripts/integration-runner_test.go`
- Modify: `integration/harness_test.go`
- Modify: `integration/http_test.go`

**Interfaces:**
- Produces: normal `make integration` builds CLIProxyAPI commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`, model-mapper commit `8fe4839dd2c39a4b0537447c4ac35a9f1d699bbf`, and current censorship source into one plugin directory.
- Produces: `startCPAWithOptions(t, cpaStartOptions)` can configure multiple provider models and an optional model-mapper block while existing `startCPA` call sites stay unchanged.

- [ ] **Step 1: Write runner and clean-canary tests first**

Add exact constants assertions:

```go
func TestPinnedModelMapperRevision(t *testing.T) {
	const want = "8fe4839dd2c39a4b0537447c4ac35a9f1d699bbf"
	if modelMapperSHA != want {
		t.Fatalf("modelMapperSHA = %q, want %q", modelMapperSHA, want)
	}
}
```

Extend `TestRunnerPathsStayUnderIntegrationRoot` to require `paths.mapperCheckout` under `.integration`. Add a test that creates a local clean Git fixture at `paths.mapperCheckout`, calls `prepareModelMapperCheckout(paths, head)`, and proves the existing checkout is reused. Add `TestPreparePluginPlatformDirRemovesStaleArtifacts`: prepopulate `pluginPlatformDir(paths)` with a fake native library, call `preparePluginPlatformDir(paths)`, and require the directory still exists but is empty. Keep `parseBenchmarkMode` and `integrationTestArgs` unchanged in this task.

In `integration/harness_test.go`, add:

```go
type cpaStartOptions struct {
	upstreamURL     string
	pluginsEnabled  bool
	censorshipYAML  string
	modelMapperYAML string
	providerModels  []string
}
```

Keep existing `startCPA(t, upstreamURL, pluginsEnabled, censorshipYAML)` as a wrapper using `providerModels: []string{modelName}`. `startCPAWithOptions` writes every `providerModels` entry with identical `name` and `alias`; it writes optional `model-mapper` config before `censorship`, so existing `writePluginConfig` can continue replacing only the censorship tail during watcher tests.

Add the clean fixture canary to `integration/http_test.go` with this exact setup:

```text
model-mapper priority: 999
openai_completions_rules: 'claude-opus=>kimi-k3;claude-control=>other-model'
provider models: kimi-k3, other-model
censorship priority: 800
words.block: blocked
filter_mode: exclude
filter.models: kimi-k3
```

Send clean content through both source models. Require 200, one upstream arrival per request, and captured body models `kimi-k3` then `other-model`. This canary will pass on v0.3.1 once the real mapper binary is present because clean text does not terminate outer processing.

- [ ] **Step 2: Run both RED checks**

Run independently:

```bash
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
go run ./.github/scripts/integration-runner.go
```

Expected: runner unit tests fail to compile only for missing `modelMapperSHA`, `mapperCheckout`, `prepareModelMapperCheckout`, `pluginPlatformDir` and `preparePluginPlatformDir`. The direct runner compiles the new integration helper/canary but fails the canary with model resolution failure because the current runner has not built model-mapper. Fix syntax or unrelated fixture errors until these are the observed causes.

- [ ] **Step 3: Implement the minimal runner infrastructure**

In `integration-runner.go`, add:

```go
const (
	cpaSHA            = "c76dfd4e0edabab9000628b1560ab8ab379eadb8"
	cpaRemote         = "https://github.com/router-for-me/CLIProxyAPI"
	modelMapperSHA    = "8fe4839dd2c39a4b0537447c4ac35a9f1d699bbf"
	modelMapperRemote = "https://github.com/DoingDog/cpa-plugin-model-mapper"
)
```

Add `mapperCheckout` to `runnerPaths`. Extract the existing git init/fetch/checkout sequence into `preparePinnedCheckout(integrationRoot, checkout, remote, revision string) error`; keep `prepareCheckout` responsible for removing generated CPA integration tests before calling it. Add `prepareModelMapperCheckout(paths, wantSHA string) error` and reuse `verifyCheckout`.

Use one `pluginPlatformDir(paths)` helper and one `preparePluginPlatformDir(paths) (string, error)` helper. Full and benchmark modes prepare both checkouts and build CPA, then call `preparePluginPlatformDir` exactly once before writing either native library. Build current censorship first and pinned model-mapper second; neither build helper may clear the directory itself. Build model-mapper with the same current GOOS/GOARCH and `CGO_ENABLED=1`:

```bash
go build -trimpath -buildmode=c-shared -o <plugin-platform-dir>/model-mapper.<ext> .
```

The command directory is `paths.mapperCheckout`. Remove generated `.h` through `removeContained`; never edit it.

- [ ] **Step 4: Verify infrastructure green**

Run:

```bash
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
make integration
```

Expected: runner tests pass; pinned mapper revision verifies clean; both clean mapped canaries reach the registered provider models; all existing integration tests remain green. If mapper setup fails, fix only fixture infrastructure before writing behavior RED.

- [ ] **Step 5: Commit Task 1**

```bash
git add .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go integration/harness_test.go integration/http_test.go
git commit -m "test: add pinned model mapper fixture"
```

## Task 2: Add the complete RED regression set

**Files:**
- Modify: `main_test.go`
- Modify: `request_filter_test.go`
- Modify: `abi_cgo_test.go`
- Modify: `.github/scripts/testdata/abi_benchmark_test.go`
- Modify: `integration/http_test.go`

**Interfaces:**
- Consumes: Task 1 real mapper and dynamic ABI infrastructure.
- Produces: focused failures for phase selection, model subject, reconfigure phase class, AfterAuth ABI input and the user's complete non-stream path.

- [ ] **Step 1: Add a method-aware unit helper**

Replace the internal implementation of `callInterceptRequest` with a wrapper over:

```go
func callInterceptRequestAt(method string, request pluginapi.RequestInterceptRequest) (pluginapi.RequestInterceptResponse, error)
```

The helper marshals `request`, calls `handleMethod(method, raw)`, decodes `pluginabi.Envelope`, rejects `OK=false`, and returns `RequestInterceptResponse`. Existing model-free callers continue to use the BeforeAuth wrapper. Update every existing interceptor test in `main_test.go` and `request_filter_test.go` whose config has non-empty `filter.models` to call AfterAuth with a valid selected-auth marker; this explicitly includes `TestReconfigureStoresValidSnapshotAndKeepsLastKnownGoodOnError`, `TestRequestFilterGatesBeforeBodyValidation` model-bearing rows, and `TestRequestFilterGatesAllSupportedFormats`. Those requests set `Model` to the tested model and `RequestedModel` to an opposite decoy. Do not move API-only, no-rules or empty-model rows.

Add a no-op assertion helper that requires the exact zero-value response with `reflect.DeepEqual`.

- [ ] **Step 2: Add phase decode-order and marker tests**

Add `TestModelFilterDefersBeforeAuthBeforeEnvelopeDecode`:

1. Register non-empty model filter, `words.block: [blocked]`, and use supported `SourceFormat: "openai"` for valid envelopes, so no-rules and unknown-format branches cannot satisfy the assertion.
2. Call `interceptBeforeAuth([]byte("not-json"))` directly.
3. Require a successful envelope containing an exact zero-value response.
4. Call BeforeAuth with a valid envelope whose body is malformed JSON and again require exact no-op.

Add `TestAfterAuthFastPathsBeforeEnvelopeDecode` for empty filter and API-key-only filter. Pass malformed non-empty raw envelope directly to `interceptAfterAuth`; require exact no-op without decode error.

Add `TestModelFilterProcessesOnlySelectedAuthAfter` with a table for marker metadata:

```text
missing, null, "", "   ", integer -> no-op
selected_auth_id="auth-1" -> active
selected_auth_index="index-1" -> active
```

Every marker row uses non-empty rules and supported `SourceFormat: "openai"`. Give at least the missing-marker and non-string-marker rows a malformed body and require exact no-op, proving the marker gate precedes body parsing. For active rows use `Model=target-model`, `RequestedModel=requested-decoy`, body model `body-decoy`, callback source `source-decoy`, and content `blocked`; require terminal 400. Add the inverse include filter with `Model=model-decoy`, `RequestedModel=target-model`; require no-op to prove no fallback.

Add `TestModelAndAPIKeyFiltersRunTogetherAfterAuth` with two active cases: exact `caller_scope` plus matching model under `filter_logic: and`, and wildcard Authorization header plus matching model under `filter_logic: or`. Each active AfterAuth request contains `blocked` and must terminate; the same combined config and request sent to BeforeAuth must be an exact no-op.

Migrate `TestRequestFilterGatesAllSupportedFormats` rather than adding a duplicate suite. Preserve its five existing `SourceFormat` values and provider-specific natural-language bodies, put the tested subject in `Model`, add a valid marker, and call AfterAuth. Each matching row must terminate, each nonmatching row must be exact no-op, and the same matching request sent to BeforeAuth must also be exact no-op.

- [ ] **Step 3: Replace the v0.3.1 predicate fixtures**

Delete the callback-source test constant and the two source-dependent tests. Rename the main contract test to `TestRequestFilterShouldProcessUsesAttemptModel`. Across all existing `request_filter_test.go` filter-gate and five-format fixtures, every model-bearing row sets `request.Model="target"` when matching and `RequestedModel="requested-decoy"`; nonmatching rows invert those values. Add exact assertions that unknown or callback `source` does not alter the result.

Keep `matchesModel` exact/glob tests unchanged.

- [ ] **Step 4: Add phase-class reconfigure tests**

Add `TestReconfigureRejectsModelFilterPhaseClassChanges` with two subtests:

- register non-empty model filter, attempt reconfigure to empty; require one error log whose public message is `censorship plugin reconfigure rejected`, whose error field is exactly `filter.models phase change requires restart`, and whose fields contain no YAML terms; then valid selected-auth AfterAuth still enforces the old snapshot;
- register empty model filter, attempt reconfigure to non-empty; require the same rejection and BeforeAuth still enforces the old snapshot.

Add `TestReconfigureAllowsSameModelFilterPhaseClass` for empty -> empty and non-empty -> non-empty. Prove new terms and mode take effect in their respective active phase.

- [ ] **Step 5: Tighten the active ABI oracle to RED**

Keep `TestShouldCopyPluginRequest` and change its AfterAuth expectation from false to true; every listed method must require copying. This gives a focused RED on the current exception and prevents a synchronous borrowed slice from satisfying behavior-only oracles.

Add `TestCliproxyPluginCallProcessesAfterAuthRequest`:

- register model include filter with strip term `BLOCKME`;
- marshal an AfterAuth request with valid selected-auth marker, `Model=target-model`, opposite `RequestedModel`, and non-empty OpenAI body;
- invoke exported `cliproxyPluginCall`;
- after receiving a non-empty response pointer, immediately `defer cliproxyPluginFree`, decode the C buffer, and require stripped response body;
- poison the original host request bytes after the call and require the response remains correct, documenting this assertion as response-buffer independence rather than proof of input copying.

In dynamic `abi_benchmark_test.go`, extract the existing host setup into a `testing.TB` helper and add `TestDynamicABIActiveAfter`. Configure a non-empty model filter, use matching `Model`, opposite `RequestedModel`, valid selected-auth marker and non-empty body, then require the stripped AfterAuth response and exact host-input immutability. Update benchmark cases under the same config: BeforeAuth expects exact no-op; AfterAuth carries a valid marker and expects strip.

- [ ] **Step 6: Add the real mapper behavior RED**

Extend the Task 1 canary test or add `TestHTTPMappedExcludedModelBypassesCensorship` in the same CPA process:

1. Direct `kimi-k3` plus `blocked`: 200, upstream delta 1, model `kimi-k3`, content unchanged.
2. Mapped `claude-opus` plus `blocked`: 200, upstream delta 1, model `kimi-k3`, content unchanged.
3. Mapped `claude-control` plus clean content: 200, upstream delta 1, model `other-model`.
4. Same `claude-control` route plus `blocked`: upstream delta 0; pinned outer status 500 and typed outer error code `internal_server_error`.

Add a separate mapper-disabled canary with clean content, where `claude-opus` is not registered as provider or alias. Require a non-success model-not-found response whose body identifies `claude-opus`, plus zero upstream arrivals; clean content prevents censorship block from impersonating model resolution failure.

- [ ] **Step 7: Run RED verification**

Run focused unit/ABI tests:

```bash
go test ./... -run 'Test(ModelFilter|RequestFilterShouldProcessUsesAttemptModel|ReconfigureRejectsModelFilterPhaseClassChanges|ReconfigureAllowsSameModelFilterPhaseClass|ShouldCopyPluginRequest|CliproxyPluginCallProcessesAfterAuthRequest)'
```

Expected failures:

- malformed raw BeforeAuth envelope returns decode error instead of no-op;
- active AfterAuth remains no-op;
- model predicate still follows `RequestedModel` without callback source;
- phase-class reconfigure is accepted;
- `shouldCopyPluginRequest` still exempts AfterAuth, and the exported AfterAuth ABI call cannot transform because it receives no copied input.

Run real integration:

```bash
make integration
```

Expected: clean mapper canaries and direct `kimi-k3` control pass; mapped `claude-opus` plus `blocked` returns 400 before mapper Executor, so the expected 200/upstream delta fails. The failure must not be model-not-found or mapper build/setup.

- [ ] **Step 8: Commit the RED tests**

```bash
git add main_test.go request_filter_test.go abi_cgo_test.go .github/scripts/testdata/abi_benchmark_test.go integration/http_test.go
git commit -m "test: reproduce post-route model filter failure"
```

## Task 3: Implement the minimal phase-aware production fix

**Files:**
- Modify: `main.go`
- Modify: `request_filter.go`
- Modify: `abi_cgo.go`
- Modify: `benchmark_test.go`

**Interfaces:**
- Produces: model-free filters run BeforeAuth; model-bearing filters run only at a selected-auth AfterAuth and use `request.Model`.
- Produces: reconfigure cannot switch phase class in one loaded instance.
- Produces: every ABI request method, including AfterAuth, owns a synchronous Go copy.

- [ ] **Step 1: Add the selected-auth marker helper**

Import the pinned exported constants:

```go
cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
```

Add one helper in `main.go`:

```go
func hasSelectedAuth(metadata map[string]any) bool {
	for _, key := range []string{
		cliproxyexecutor.SelectedAuthMetadataKey,
		cliproxyexecutor.SelectedAuthIndexMetadataKey,
	} {
		if value, ok := metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}
```

Do not read `source` or any mapper field.

- [ ] **Step 2: Split the request phase before decoding**

Implement:

```go
func interceptBeforeAuth(raw []byte) ([]byte, error) {
	cfg := loadedSnapshot()
	if cfg == nil || len(cfg.Rules) == 0 || len(cfg.Filter.Models) != 0 {
		return okEnvelope(pluginapi.RequestInterceptResponse{})
	}
	return interceptRequest(raw, cfg, false)
}

func interceptAfterAuth(raw []byte) ([]byte, error) {
	cfg := loadedSnapshot()
	if cfg == nil || len(cfg.Rules) == 0 || len(cfg.Filter.Models) == 0 {
		return okEnvelope(pluginapi.RequestInterceptResponse{})
	}
	return interceptRequest(raw, cfg, true)
}
```

Move the existing decode, known-format check, filter gate, transform and response construction into:

```go
func interceptRequest(raw []byte, cfg *configSnapshot, requireSelectedAuth bool) ([]byte, error)
```

After decoding, if `requireSelectedAuth && !hasSelectedAuth(request.Metadata)`, return exact no-op before `shouldProcess` or `transformRequest`. Do not change transform result handling.

- [ ] **Step 3: Make model matching use only the attempt Model**

In `request_filter.go`, delete `modelExecutionSourceKey`, `modelExecutionCallbackSource` and `modelSubject`. Keep the existing one-time calculation only when model patterns exist:

```go
if modelConfigured {
	model = request.Model
}
```

Leave all switch branches and API-key matching untouched. In `benchmark_test.go`, set `Model` on every model-bearing `BenchmarkRequestFilter` and `BenchmarkRequestFilterGlobAdversarial` fixture, retaining an opposite `RequestedModel` decoy where useful. Keep benchmark names unchanged so v0.3.1 comparisons remain addressable.

- [ ] **Step 4: Reject reconfigure phase-class changes**

After parsing a reconfigure candidate but before `installSnapshot`, compare:

```go
oldHasModels := current != nil && len(current.Filter.Models) != 0
newHasModels := len(candidate.Filter.Models) != 0
```

If they differ, log the same public reconfigure rejection event used for parse failures, with only:

```go
map[string]any{"error": "filter.models phase change requires restart"}
```

Return the normal registration envelope and retain the current snapshot. Factor a small `logReconfigureRejected(errorText string)` helper only if it removes the now duplicated host-log construction; do not refactor other lifecycle code.

- [ ] **Step 5: Restore AfterAuth ABI input ownership**

Keep the existing `shouldCopyPluginRequest` gate in `cliproxyPluginCall`, but make it return true for AfterAuth as well as every other method, so the existing guarded call always reaches `copyPluginRequest`. Keep its validation and `C.GoBytes` path. Do not use a borrowed slice in `cliproxyPluginCall`, cache input, or retain pointers.

- [ ] **Step 6: Verify GREEN in increasing scope**

Run:

```bash
go test ./... -run 'Test(ModelFilter|RequestFilterShouldProcessUsesAttemptModel|ReconfigureRejectsModelFilterPhaseClassChanges|ReconfigureAllowsSameModelFilterPhaseClass|ShouldCopyPluginRequest|CliproxyPluginCallProcessesAfterAuthRequest)'
go test ./...
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
make integration
```

Expected: all pass. The real mapper test must show both direct and mapped excluded requests reach upstream as `kimi-k3`; mapped nonexcluded clean reaches `other-model`; blocked mapped nonexcluded reaches no upstream and returns the documented pinned-host 500.

- [ ] **Step 7: Commit production GREEN**

```bash
git add main.go request_filter.go abi_cgo.go benchmark_test.go
git commit -m "fix: defer model filters until auth selection"
```

## Task 4: Gate the packaged Windows DLL with the active-after ABI smoke

**Files:**
- Modify: `.github/scripts/integration-runner.go`
- Modify: `.github/scripts/integration-runner_test.go`
- Modify: `.github/workflows/build.yml`
- Modify: `main_test.go`

**Interfaces:**
- Consumes: Task 2 `TestDynamicABIActiveAfter` oracle.
- Produces: `go run ./.github/scripts/integration-runner.go -abi-smoke <library>` stages the supplied current-platform censorship library and runs only `TestDynamicABIActiveAfter`.
- Produces: windows-amd64 build job cannot upload or release a DLL that fails AfterAuth input-copy and buffer-ownership execution.

- [ ] **Step 1: Write runner-mode and workflow tests first**

Replace `parseBenchmarkMode` tests with `parseRunnerOptions(args []string, benchEnv string)` cases for no arguments, `-bench-abi`, `-abi-smoke <path>`, missing path, mixed CLI modes and unknown arguments. Also require empty argv plus `benchEnv="1"` to select benchmark mode, while explicit ABI smoke plus `benchEnv="1"` selects only ABI smoke. Compare successful results to `runnerOptions{benchmark: true}` or `runnerOptions{abiSmokeLibrary: path}`; the initial compile failure proves the production type and parser are missing.

Change `integrationTestArgs` tests to pass `runnerOptions` and require:

```text
full       -> go test ... ./integration/censorshipplugin
benchmark  -> -run ^$ -bench ^BenchmarkDynamicABIRequestInterceptors$ -benchmem
abi smoke  -> -run ^TestDynamicABIActiveAfter$
```

Add a staging test that prepopulates the platform directory with a stale plugin, writes a fake source library, calls `stagePluginLibrary`, and asserts the stale file is gone, exact source bytes exist under `run/plugins/<GOOS>/<GOARCH>/censorship.<ext>`, and the source is unchanged.

In an existing documentation/workflow test section of `main_test.go`, isolate the YAML text from the `build:` job heading up to but excluding `build-windows-arm64:`. Within that substring, require the following exact step block exactly once:

```yaml
      - name: Verify Windows amd64 active-after ABI
        if: "${{ matrix.GOOS == 'windows' && matrix.GOARCH == 'amd64' }}"
        shell: msys2 {0}
        run: go run ./.github/scripts/integration-runner.go -abi-smoke dist/windows_amd64/censorship.dll
```

Assert the position of `Build and package on Windows` is before this block and this block is before the first `actions/upload-artifact@v4` in the same job substring. This prevents a comment, another job, a missing guard, or a post-upload smoke from satisfying the test.

Run the runner and workflow tests. Expected RED: missing `runnerOptions`, `parseRunnerOptions` and `stagePluginLibrary`, plus the absent ordered workflow block.

- [ ] **Step 2: Implement prebuilt ABI-smoke runner mode**

Have `run` call `parseRunnerOptions(os.Args[1:], os.Getenv("BENCH"))`. Accept only `[]`, `[-bench-abi]`, or `[-abi-smoke, <non-empty path>]`. `benchEnv="1"` selects benchmark only for empty argv; an explicit `-abi-smoke` mode wins and leaves `benchmark=false`.

Full and benchmark modes keep Task 1 behavior. ABI-smoke mode prepares and builds pinned CPA, stages only the supplied censorship library, copies integration tests and runs `TestDynamicABIActiveAfter`; it does not prepare model-mapper or rebuild censorship.

Use the existing platform directory helper from Task 1. `stagePluginLibrary(paths, source)` follows the runner's existing `os.ReadFile`/`os.WriteFile` copy pattern: read the caller-supplied source first, call `preparePluginPlatformDir` to remove and recreate the contained platform directory, and write identical bytes to `censorship.<ext>`. `integrationTestArgs(options)` selects the exact argument lists above, and `runIntegrationTests` receives the same options.

- [ ] **Step 3: Add the Windows gate**

Immediately after `Build and package on Windows` and before `actions/upload-artifact`, add a step guarded to windows amd64:

```yaml
      - name: Verify Windows amd64 active-after ABI
        if: "${{ matrix.GOOS == 'windows' && matrix.GOARCH == 'amd64' }}"
        shell: msys2 {0}
        run: go run ./.github/scripts/integration-runner.go -abi-smoke dist/windows_amd64/censorship.dll
```

The release job already depends on the complete `build` matrix, so no extra dependency edge is needed. Windows arm64 remains build-only.

- [ ] **Step 4: Verify locally available parts**

On Windows, build a current DLL and run the exact smoke:

```bash
make build VERSION=0.3.2
go run ./.github/scripts/integration-runner.go -abi-smoke dist/windows_amd64/censorship.dll
```

If the next patch changes at release preflight, rerun with the actual version in Task 7. Also run runner tests, workflow fragment tests and `git diff --check`.

- [ ] **Step 5: Commit Task 4**

```bash
git add .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go .github/workflows/build.yml main_test.go
git commit -m "ci: execute after-auth Windows ABI smoke"
```

## Task 5: Replace the v0.3.1 public contract

**Files:**
- Modify: `README.md`
- Modify: `RELEASE_NOTES.md`
- Modify: `main_test.go`
- Modify: `docs/superpowers/specs/2026-09-14-model-filter-plugin-compatibility-design.md`
- Modify: `docs/superpowers/plans/2026-09-14-model-filter-plugin-compatibility.md`

**Interfaces:**
- Produces: public docs match the implemented selected-auth attempt contract and preserve v0.3.1 as historical release information.

- [ ] **Step 1: Write documentation assertions first**

Update `main_test.go` required README tokens to assert separately:

```text
filter.models is empty
filter.models is non-empty
RequestAfterAuthInterceptor
selected_auth_id
selected_auth_index
request.Model
non-stream host.model.execute
HTTP 500
filter.models phase change requires restart
in-flight requests
C.GoBytes
```

Add README/current-release negative assertions for the obsolete claims that censorship only runs before authentication, ordinary models use `RequestedModel`, callback `source` selects `Model`, every valid reconfigure applies without restart, or AfterAuth avoids reading/C-to-Go input copy. Scope these negative checks to README and the new current-release section so preserved v0.3.1 history may still describe what that release did.

Update the current/historical release test to require the new patch compatibility sentence and retain exact v0.3.1 and v0.3.0 historical sentences. Add assertions that both old design and old plan begin with a visible superseded note pointing to the new spec.

Run focused documentation tests and confirm RED against current docs.

- [ ] **Step 2: Mark old execution documents superseded**

Add at the top of both 2026-09-14 documents, after the title:

```markdown
> Superseded by `docs/superpowers/specs/2026-09-15-model-filter-post-route-execution-design.md`. Do not execute the v0.3.1 outer-`RequestedModel` plan.
```

Do not rewrite their historical body.

- [ ] **Step 3: Update README current behavior**

Update every current-contract location, not only the stage paragraph: the README opening, Request filtering/stage source section, Configuration reload section, Build/ABI ownership section, limitations #9 and #13, and current compatibility-version paragraph. Replace them with the exact operational contract:

- model-free filters process BeforeAuth;
- model-bearing filters skip BeforeAuth and process only selected-auth AfterAuth;
- AfterAuth model subject is `request.Model` regardless of callback `source`;
- no body model parsing or alias routing in censorship;
- model+API combinations move together, with wildcard credentials observing headers visible at AfterAuth;
- same-instance config-only reconfigure cannot change model-filter phase class; phase class, binary or enabled changes require draining in-flight requests and restart;
- AfterAuth restores synchronous `C.GoBytes` and incurs input-sized copy;
- supported mapped scope is non-stream `host.model.execute` returning to ordinary AuthManager;
- mapped nonexcluded block remains zero-upstream but surfaces as HTTP 500 under pinned host;
- final wire model may differ after Executor-side payload rewrites;
- mapped stream, opaque terminal Executor and Antigravity fallback are excluded.

Keep existing provider selectors, machine-field exclusions and five-format table unchanged except for stage source wording.

- [ ] **Step 4: Add release notes for the next patch**

Prepend a new section using provisional v0.3.2, subject to fresh tag preflight:

```markdown
# Censorship v0.3.2

## v0.3.2 fixes
```

Document the user reproduction and implementation stage. State the pinned host limitation without describing 500 as desired. Preserve v0.3.1 and v0.3.0 sections verbatim as history except for a one-line pointer that v0.3.1 behavior is superseded.

Use compatibility sentence:

```text
v0.3.2 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.
```

- [ ] **Step 5: Run documentation and full tests GREEN**

```bash
go test ./... -run 'TestREADME|TestReleaseNotes|TestDocumentation|TestWorkflow'
go test ./...
git diff --check
```

Expected: all pass and no current README sentence contradicts selected-auth processing.

- [ ] **Step 6: Commit Task 5**

```bash
git add README.md RELEASE_NOTES.md main_test.go docs/superpowers/specs/2026-09-14-model-filter-plugin-compatibility-design.md docs/superpowers/plans/2026-09-14-model-filter-plugin-compatibility.md
git commit -m "docs: correct post-route filter contract"
```

## Task 6: Full verification, performance comparison and review

**Files:**
- Inspect: all files changed since `v0.3.1`
- Modify only for confirmed review findings through focused TDD fix rounds.

- [ ] **Step 1: Format and static checks**

```bash
gofmt -w main.go request_filter.go abi_cgo.go main_test.go request_filter_test.go abi_cgo_test.go .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go .github/scripts/testdata/abi_benchmark_test.go integration/harness_test.go integration/http_test.go
git diff --check
go vet ./...
```

Expected: no diagnostics and no unrelated formatting changes.

- [ ] **Step 2: Run all project gates**

Set a writable Windows `GOTMPDIR` when needed, then run independent commands in parallel where safe:

```bash
make build VERSION=0.3.2
make test
make race
make vet
make integration
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
```

Also run the exact prebuilt DLL smoke generated by `make build`.

- [ ] **Step 3: Measure affected paths**

Create one revision-neutral benchmark harness under this plan's git-ignored SDD workspace, then use `git archive` to extract clean `v0.3.1` and reviewed `HEAD` trees under the same workspace. Copy the identical root `post_route_benchmark_test.go` into both trees before compiling either. It defines unique `BenchmarkPostRoute*` names and these exact common workloads:

- `requestFilter.shouldProcess` for API exact, API wildcard, model exact, model+API `or`, and model+API `and`;
- a 1 MiB valid OpenAI envelope sent through both `handleMethod(request.intercept_before)` and `handleMethod(request.intercept_after)`;
- model-bearing rows use `Model: "target-model"`, opposite `RequestedModel`, `source: "plugin_host_model_callback"` for v0.3.1 compatibility, and non-empty `selected_auth_id`;
- config uses include model `target-model` and a `NEVER-MATCH` block term, so both revisions return the same successful no-op while v0.3.1 scans BeforeAuth and the fixed revision scans AfterAuth.

Create one identical revision-neutral integration benchmark fixture and overwrite only the two extracted trees' `.github/scripts/testdata/abi_benchmark_test.go`, never the tracked worktree copy. It dynamically loads each revision's own censorship library, uses the same marker/model/decoy/no-match request, checks exact input immutability and successful no-op, and benchmarks BeforeAuth and AfterAuth at 1 KiB, 1 MiB and 20 MiB. Run each extracted tree's runner once to prepare its pinned CPA/plugin fixture, then invoke the generated CPA integration package directly with that tree's `CENSORSHIP_PLUGIN_DIR`.

For both root and dynamic suites, use:

```bash
go test -run '^$' -bench '^BenchmarkPostRoute' -benchmem -count=5 -benchtime=1s
```

Run and save raw output in A/B order and then B/A order. Verify the identical harness compiles on both revisions before collecting results. Compare matching workload `B/op` and `allocs/op` first; timing may be reported only with order sensitivity and variance. Interpret before/after timing as intentionally different phase work, not equal-operation throughput: v0.3.1 scans model-bearing BeforeAuth and no-ops AfterAuth, while the fixed branch does the reverse.

Acceptance:

- API-only Go filter path remains zero-allocation where baseline was zero-allocation;
- model-bearing BeforeAuth has no input-sized allocation and does not decode envelope/body;
- active AfterAuth reports input-sized allocation expected from `C.GoBytes`;
- no performance claim uses noisy timing alone;
- no attempt removes `C.GoBytes` without a proven ownership-safe benchmark candidate.

- [ ] **Step 4: Run the whole-branch Opus review**

Confirm the SDD ledger records an approved independent spec/code-quality review for each Task 1 through Task 5, including scoped re-review after every fix. Do not defer a missing task review to this step.

Then run a whole-branch review for `v0.3.1..HEAD` with separate lenses:

- user reproduction and control-flow reachability;
- phase gate and reconfigure safety;
- API/filter/selector regressions;
- ABI ownership and Windows workflow;
- real mapper integration false-green resistance;
- docs, supported scope and known limitations;
- packaging and release readiness.

No reviewer may treat the pinned host 500 limitation or excluded mapped stream as a missing censorship-only implementation after the spec has accurately disclosed them.

For every confirmed whole-branch finding, write or tighten a focused RED test when behavior changes, apply one minimal fix, and rerun affected gates. Any fix changes the reviewed HEAD and invalidates every whole-branch lens, so after the final fix rerun the complete full-gate set and all whole-branch lenses against the same new HEAD. Repeat until that exact HEAD has zero open findings; do not substitute a scoped re-review for the final complete pass.

- [ ] **Step 5: Final verification after review fixes**

Rerun formatting, `git diff --check`, build, unit, race, vet, full pinned integration, active Windows DLL smoke and packager tests. Confirm `git status --short` contains no generated or uncommitted files, then record that these gate results and every zero-finding whole-branch review refer to the same HEAD.

- [ ] **Step 6: Mark Task 6 complete only after zero open findings**

Record exact command outcomes, benchmark allocation results, review findings and any adjudicated limitations in this plan's SDD ledger. Do not claim the user scenario fixed before the real mapper integration passes.

## Task 7: Package, merge, publish and verify the next patch

**Files:**
- Generated only through documented commands: `dist/**`
- No manual generated artifact edits.

- [ ] **Step 1: Refresh refs and perform version preflight**

Run from the reviewed clean feature worktree:

```bash
git fetch origin main --tags --prune
git status --short
git merge-base --is-ancestor origin/main HEAD
export RELEASE_TAG=v0.3.2
export RELEASE_VERSION="${RELEASE_TAG#v}"
if git show-ref --verify --quiet "refs/tags/${RELEASE_TAG}" || git ls-remote --exit-code --tags origin "refs/tags/${RELEASE_TAG}" >/dev/null 2>&1; then
  printf '%s\n' "${RELEASE_TAG} is already occupied" >&2
  exit 1
fi
```

`git status --short` must be empty and `origin/main` must be an ancestor of the reviewed HEAD. If either condition fails, stop the release sequence, integrate the new `origin/main` according to repository convention, resolve in this feature branch, and rerun all Task 6 gates plus the complete whole-branch review. If `v0.3.2` is occupied, select and recheck the next unused patch, update README assertions and release notes, and rerun Task 5 tests and Task 6. During that rerun, substitute the selected `RELEASE_VERSION` for every provisional `VERSION=0.3.2` verification command.

Persist the selected actual tag outside git so later shell invocations cannot fall back to the provisional value:

```bash
SDD_WORKSPACE="$(bash 'C:/Users/user/.claude/plugins/cache/claude-plugins-official/superpowers/6.3.0/skills/subagent-driven-development/scripts/sdd-workspace' 'docs/superpowers/plans/2026-09-15-model-filter-post-route-execution.md')"
printf '%s\n' "${RELEASE_TAG}" > "${SDD_WORKSPACE}/release-tag"
```

- [ ] **Step 2: Build and package with the actual version**

```bash
SDD_WORKSPACE="$(bash 'C:/Users/user/.claude/plugins/cache/claude-plugins-official/superpowers/6.3.0/skills/subagent-driven-development/scripts/sdd-workspace' 'docs/superpowers/plans/2026-09-15-model-filter-post-route-execution.md')"
export RELEASE_TAG="$(tr -d '\r\n' < "${SDD_WORKSPACE}/release-tag")"
[[ "${RELEASE_TAG}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { printf '%s\n' 'release tag required' >&2; exit 2; }
export RELEASE_VERSION="${RELEASE_TAG#v}"
make clean
make package-platform GOOS=windows GOARCH=amd64 VERSION="${RELEASE_VERSION}"
make package VERSION="${RELEASE_VERSION}"
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
go run ./.github/scripts/integration-runner.go -abi-smoke dist/windows_amd64/censorship.dll
```

The first command after `clean` creates the host-platform DLL, zip and sidecar; aggregate `make package` then regenerates the zip and writes `checksums.txt` from the artifact present under `dist/`. Verify the local zip contains only the expected library and LICENSE. Recompute SHA-256, require lowercase sidecar text, and require the exact aggregate `checksums.txt` entry. Reject any local output containing `0.0.0-dev` or a version other than `${RELEASE_VERSION}`.

- [ ] **Step 3: Integrate and push main**

Resolve the original checkout path from `git worktree list`, then fast-forward its checked-out local `main` to the reviewed feature HEAD; do not synthesize an empty release commit. Confirm local `main`, feature HEAD and the packaged commit are the same SHA. Run `make test`, `git diff --check` and clean status against that SHA, then push `main`. The user's original task explicitly authorizes this outward action after all gates pass.

- [ ] **Step 4: Verify the main Build before tagging**

Find the push workflow by branch ref and exact SHA, not by latest-run ordering. Wait until it is `completed` with conclusion `success`. Require the test job to pass unit, packager, race, vet and real pinned mapper integration; require the windows-amd64 job to execute `Verify Windows amd64 active-after ABI` before upload; require all seven platform build jobs to succeed. The tag must not be created while this main workflow is pending or failed.

- [ ] **Step 5: Recheck, create and verify the tag release**

Reload and validate `RELEASE_TAG` from the SDD `release-tag` file using the same sequence as Step 2. Repeat the exact local and live remote tag-absence checks immediately before creation. Create an annotated `${RELEASE_TAG}` at the verified main SHA and push only that tag. Find the tag workflow by tag ref and exact SHA, wait for `completed`/`success`, and require the release job to succeed.

If any main job fails, fix through the relevant TDD/review task and return to Step 1 before creating a tag. If any tag job fails after tag publication, inspect and fix through the same gates, publish a new commit, and use the next unused patch tag rather than moving the existing tag.

- [ ] **Step 6: Verify GitHub release assets and checksums**

Require a non-draft, non-prerelease release whose target commit equals the tag. Require 15 assets:

```text
7 platform zip files
7 matching .zip.sha256 files
checksums.txt
```

Platforms: `darwin_amd64`, `darwin_arm64`, `freebsd_amd64`, `linux_amd64`, `linux_arm64`, `windows_amd64`, `windows_arm64`.

Every filename uses the release version, every sidecar hash is lowercase, aggregate `checksums.txt` contains all seven zips exactly once, and downloaded asset hashes match sidecars and aggregate values.

- [ ] **Step 7: Close recovery state**

Update the SDD ledger with final commit, main/tag refs, workflow URLs, release URL and checksum verification. Remove the temporary `active-request-filter-task.md` memory and its `MEMORY.md` index only after remote release verification succeeds. Leave host-managed worktree cleanup to the worktree tool lifecycle unless the user explicitly requests removal.
