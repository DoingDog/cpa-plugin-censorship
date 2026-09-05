# censorship audit hardening TDD evidence

日期：2026-09-05

范围：只审计和修改 `cpa-plugin-censorship` 插件及其测试、packaging 和文档。没有分析或修改 CPA 本体中与插件无关的源码。

## Slice 1：Interactions system text hard exclusion

### RED

回归测试在旧实现上确认：Interactions 的 `system_instruction` object-level direct `text` 路径绕过 `scanTextPart`，因此未知 `type: image` 和带 `inlineData` 的对象中的文本会被错误处理。

### GREEN

`system_instruction` 的 direct text 现在先通过 `scanTextPart(systemInstruction, true)`，只有 plain text object 才会建立 system span。`parts[]`、item 和 `steps` 的选择范围保持不变。

```text
go test . -run '^TestInteractions' -count=1
PASS
```

公开 `interceptRPC` 回归覆盖未知 image type、`inlineData` 和 plain text object。

## Slice 2：JSON nesting resource bound

### RED

```text
go test . -run '^(TestSelectorRejectsJSONBeyondConfiguredNestingDepth|TestJSONNestingScanIgnoresBracketsInsideStrings)$' -count=1
exit code 1
--- FAIL: TestSelectorRejectsJSONBeyondConfiguredNestingDepth
    error = <nil>, want request body must be a JSON object
```

旧 selector 对超过限制的深层 JSON 没有拒绝。

### GREEN

`jsonNestingWithin` 在 selector recursion 和 duplicate-member traversal 之前做线性扫描，只计算 JSON string 之外同时打开的 object/array 数量。上限为 1024，包含顶层 object；1025 层返回 `censorship_invalid_request`。字符串内的括号和 escaped quote 不增加深度。

```text
go test . -run '^(TestSelectorRejectsJSONBeyondConfiguredNestingDepth|TestJSONNestingScanIgnoresBracketsInsideStrings|TestDuplicate)' -count=1
PASS
```

## Slice 3：C ABI length safety

### RED

```text
go test . -run '^TestCheckedCIntLength$' -count=1
exit code 1
undefined: checkedCIntLength
```

这是 helper 的 RED 阶段。实现前没有 checked `uint64` 到 `C.int` 的边界函数。

### GREEN

`checkedCIntLength` 接受 0 和 `2147483647`，拒绝 `2147483648`。host callback 在长度检查前注册释放 buffer；过大的 response 返回 plugin error 且不调用 `C.GoBytes`。plugin request 在 `C.GoBytes` 前检查长度，response pointer 和 length 在确认非 nil 后立即清零。output copy 使用 `unsafe.Slice`，长度来自已分配 payload。

```text
go test . -run '^(TestCheckedCIntLength|TestShouldCopyPluginRequest)$' -count=1
PASS

go test -race . -run '^(TestCheckedCIntLength|TestShouldCopyPluginRequest)$' -count=1
PASS
go vet ./...
PASS
go build -trimpath -buildmode=c-shared -o dist/test/censorship.dll .
PASS
```

## Slice 4：release version validation

### RED

该 slice 的独立 RED 输出没有被保留下来，因此不把未记录的命令或失败输出写成事实。

### GREEN

release version 在移除一次前导小写 ASCII `v` 后，必须是安全 ASCII filename component：首字符为 ASCII letter 或 digit，后续字符只能是 ASCII letter、digit、`.`、`_`、`+` 或 `-`。aggregate mode、显式 `-version` direct mode、Makefile `package-platform` 和两个 cross workflow direct command 共用该校验；输出目录创建前拒绝非法值。

```text
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
PASS
go test -race .github/scripts/package-release.go .github/scripts/package-release_test.go
PASS
go vet ./...
PASS
```

## Slice 5：Responses WebSocket completion deadline

### RED

该 slice 的独立 RED 输出没有被保留下来，因此不把未记录的命令或失败输出写成事实。

### GREEN

测试 helper 为 completion read 设置 20 秒 deadline，并在收到 `response.completed` 后恢复无 deadline。新增 no-frame fixture 用短 deadline 验证超时和有限 cleanup；正常 completion/error message 顺序没有改变。

```text
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -count=1
PASS
go run ./.github/scripts/integration-runner.go
PASS: HTTP、SSE、watcher、Responses WebSocket 和 ABI checks
```

直接运行 integration-tagged narrow test 曾在编译阶段被仓库现有的 `abi_benchmark_test.go` import 问题阻止；固定 integration runner 是实际通过的集成入口。

## Slice 6：independent oracle

### RED

```text
go test . -run '^(TestProtocolOracleRequiresEarliestBlockRole|TestProtocolOracleRejectsOutsideSpanByteMutation|TestExactBlockAcrossSpans|TestFoldMatcherMatchesRuleMajorOracle)$' -count=1
exit code 1
--- FAIL: TestProtocolOracleRequiresEarliestBlockRole
    oracle accepted later eligible role
--- FAIL: TestProtocolOracleRejectsOutsideSpanByteMutation
    oracle accepted a number spelling mutation outside the eligible span
```

旧 protocol oracle 允许稍后命中的 role，也允许 selected span 之外的 JSON number spelling 改变。

### GREEN

protocol oracle 现在按 raw document order 选择第一个 eligible `SECRET` span，并要求 role 精确匹配；raw JSON test parser 记录 selected string-token byte ranges，prefix、gaps、suffix、unknown strings、hard-excluded strings、whitespace、number spelling 和 member order 在 selected span 之外必须逐 byte 保持。folded matcher oracle 使用 test-local `unicode.SimpleFold` cycle；exact multi-span expected 使用 rule-major/document-minor 和 test-local `strings.Contains`。这些 oracle 不调用 production selector、matcher、canonicalizer 或 rebuild helper。

```text
go test . -run '^(TestProtocolOracle|TestExactBlockAcrossSpans|TestFoldMatcher)' -count=1
PASS
go test . -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=1x
PASS
go test . -run '^$' -fuzz '^FuzzProtocolTransform$' -fuzztime=1x
PASS
```

## Slice 7：benchmark harness 和 exact rewrite candidate gate

### Harness GREEN

benchmark fixtures 在 timer 之前验证 `err`、`Invalid`、`Blocked` 和 expected Body 或 Envelope；计时循环把返回值写入 package-level sink。`BenchmarkConcurrentSnapshotSwap` 使用 goroutine-local result 和 `runtime.KeepAlive`，不让并发 goroutine 写共享非原子 result sink。JSON string fixture 使用 `json.Marshal`，与 production `encoding/json` 的 wire escaping 一致。

保留的 `BenchmarkExactRewriteStrategies` 是 baseline-only matrix，覆盖：

- `strip` 和 `obfs`；
- 32、128、256、1024 rules；
- 4 KiB、64 KiB、1 MiB text；
- `none`、`first`、`middle`、`last`、`sparse`、`dense`、`cascade`；
- test-local rule-major `strings.ReplaceAll` expected。

最终 baseline benchmark 命令：

```text
go env GOOS GOARCH GOVERSION
windows
amd64
go1.26.5

go test -run '^$' -bench '^Benchmark(TransformMatrix|TransformScenarios|BeforeAuthRPCEnvelope|ExactRewriteStrategies|JSONNestingDepth)$' -benchmem -count=1 .
PASS
```

该次运行的完整输出保存在本次工作 session 的 task output；以下是实际输出中的代表性样本，数值为该台 Windows amd64、Go 1.26.5 机器的测量值，不是代码中的硬阈值：

| Benchmark | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| `BenchmarkTransformMatrix/body=1024/words=0/fold=false` | 7.889 | 0 | 0 |
| `BenchmarkTransformMatrix/body=1048576/words=1024/fold=true` | 6,444,667 | 1,048,667 | 3 |
| `BenchmarkTransformScenarios/mode=block/match=sparse/nodes=1000` | 493,578 | 163,832 | 16 |
| `BenchmarkBeforeAuthRPCEnvelope/before_calls=1/after_calls=0` | 746,812 | 205,520 | 25 |
| `BenchmarkExactRewriteStrategies/impl=baseline/mode=strip/rules=128/text=65536/match=none` | 293,016 | 73,816 | 3 |
| `BenchmarkExactRewriteStrategies/impl=baseline/mode=obfs/rules=1024/text=1048576/match=dense` | 19,333,240 | 5,424,345 | 10 |

### Candidate RED and decision

候选实现 `135c58f` 为 case-sensitive `strip`/`obfs` rewrite 增加 `ExactRewriteMatcher`，在 128 rules、16 KiB text 起按 span 做 all-miss preflight。候选 holdout 命令如下：

```text
go test -run '^$' -bench '^BenchmarkExactRewriteCandidateHoldout$' -benchmem -benchtime=100ms -count=3 .
PASS
```

候选 benchmark 在计时前做 independent output assertion，比较同一配置下的 baseline 和 candidate，覆盖两种 mode、128/256/1024 rules、64 KiB/1 MiB text、all-miss 和多种 hit/cascade 场景。以每个 paired scenario 的三次样本 median 判定：

- all-miss 只有 6/12 个 holdout 场景达到至少约 20% speedup；另外 6 个未达到，最差改善为 4.8%；
- hit 场景有 44/72 个超过约 10% slowdown ceiling；最差 dense slowdown 为 221.6%；
- 多个 hit/dense 场景的 `B/op` 或 `allocs/op` 也高于 baseline。

候选没有达到既定保留门槛。`c27a9e2` 回退了候选 production code、preflight fields、candidate tests 和 candidate-only benchmark；保留可靠的 baseline harness。没有为了保留优化而放宽门槛。

## Integrated verification

截至文档提交前已实际通过：

```text
go test ./...
PASS
go test -race ./...
PASS
go vet ./...
PASS
git diff --check
PASS
```

早期 slice report 中记录的 README 文档 assertion 在文档同步后已修复；后续合并主工作树的完整测试通过，未把早期失败写成最终通过。

## Verification blockers

没有当前 blocker。一次直接运行 integration-tagged narrow test 在编译阶段被仓库现有 `abi_benchmark_test.go` imports 阻止，但固定 `integration-runner.go` 已通过完整集成检查；没有把该 narrow command 写成通过。
