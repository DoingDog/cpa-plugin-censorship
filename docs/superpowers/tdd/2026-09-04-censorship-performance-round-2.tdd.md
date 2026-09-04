# censorship 性能优化 Round Two TDD 证据

## 来源与范围

- 设计来源：`docs/superpowers/specs/2026-09-01-censorship-plugin-design.md`
- 实施计划：`docs/superpowers/plans/2026-09-04-censorship-performance-round-2.md`
- 本轮基线：`be27107180cafac98a71f238e501e0de26674a20`
- 当前实现 HEAD：`9a8e0cb8546850cfcaa595417e0c5df263b21a97`
- 固定 CLIProxyAPI baseline：`81e1b5374f99c212f196f34956eeed964a46b8fa`
- 本轮只改插件、测试、基准和 evidence 文档，没有修改 CLIProxyAPI。
- 本轮没有修改 `RELEASE_NOTES.md`，没有修改版本，没有创建 tag、push 或 GitHub Release。

## 环境

| 项目 | 实际值 |
|---|---|
| Go | `go1.26.5 windows/amd64` |
| GOOS/GOARCH | `windows/amd64` |
| CPU | `AMD Ryzen 7 7840H with Radeon 780M Graphics` |
| CPA module | `github.com/router-for-me/CLIProxyAPI/v7 v7.2.147-0.20260831020448-81e1b5374f99` |
| gjson | `v1.18.0` |
| yaml.v3 | `v3.0.1` |
| benchstat | `golang.org/x/perf/cmd/benchstat@v0.0.0-20260825160852-19be9d8e6c70` |

## TDD 提交映射

每个 production 优化都先提交测试 RED checkpoint，再提交对应 GREEN checkpoint。表中的 RED 失败是目标接口缺失、旧实现违反 allocation assertion，或 benchmark assertion 暴露目标路径问题。

| Task | RED checkpoint | GREEN checkpoint | 保证 |
|---|---|---|---|
| 1. parsed-root duplicate walker | `36f1417` | `9776c02` | duplicate walker 接收已解析 gjson root，并保持 canonical member-name 和 fail-closed 行为 |
| 2. single-pass part scan | `3c4dfcd` | `375144e` | Gemini 和 Interactions part object 一次扫描，保留 `Raw`、`Str`、`Index` |
| 3. role gates | `d18a63e` | `149917f` | 五种格式安全跳过 disabled role，同时保留 canonical role override、alternation 和递归 |
| 4. forward rebuild | `b1dcbd3`，补充 `9a9256b` | `737a41c` | changed JSON string span 用一个 forward buffer 和一个 encoder 重建，先验证全部 span |
| 5. single-marshal envelope | `34bd425` | `99f1b64` | success envelope 保持 exact wire bytes，并只 marshal 一次 |
| 6. AfterAuth copy policy | `2defe7a` | `b21d5a3` | AfterAuth dispatch 后不执行插件侧 `C.GoBytes`，其他 method 仍复制输入 |
| 7. mode-specific snapshot data | `4c5c264` | `05c15c9` | 只为 active mode 和 case mode 编译必要派生数据 |
| 8. ASCII fold root table | `4496049` | `693ea94` | folded matcher 的 ASCII root transition 使用 dense table，failure retry 保持语义 |
| 9. exact rewrite single scan | `467c944` | `5a4ff52` | exact strip/obfs 删除冗余 `Contains`，exact obfs 使用预编译 replacement |
| 10. folded rewrite miss preflight | `67bdf64` | `4badacb` | per-span total miss 才跳过 rewrite，保留 rule-major cascade 和 span isolation |
| 11. adaptive exact byte matcher | `3eeb107`，校准 `a9b3895` | `e892a02` | exact block 在 measured region 使用 byte Aho-Corasick，保留 rule/document order |
| 12. adaptive folded KMP | `0467ca4`，benchmark `1f8b216`，fallback `3f8dd14` | `a2ae1d4` | folded strip/obfs 在 measured region 使用 KMP，保留 source byte span 和 non-overlap |

## 后续证据修复

| 提交 | 原因 | 结果 |
|---|---|---|
| `d3e62a3` | 加强 role-gate fixtures，修复 refusal、invalid Gemini role 和 `user_input` 的 test-of-test 覆盖 | 三个定向 mutation 均按预期使测试 RED |
| `3421c30` | 让 folded KMP fuzz 真实经过 adaptive dispatch，并增加 invalid-span oracle error class | dispatch mutation 和 error-class mutation 均按预期使测试 RED |
| `0516e69` | 为 exact block benchmark 固定 first、middle、last 位置 | 首位置回退能够被稳定重现 |
| `77190b0` | 使用修正后的位置 fixture 测量 exact block | 位置矩阵在同一 binary 中完成十轮对照 |
| `9a8e0cb` | 根据位置 benchmark 暴露的 early-hit 回退，改用 direct `strings.Contains` exact path | calibration 保持大幅收益，所有 exact block holdout 的统计 median 回退不超过百分之五 |

## RED 证据

### Success envelope wire bytes

`main_test.go` 新增真实 `pluginapi.RequestInterceptResponse` fixture，固定字段顺序、HTML escaping 和 base64 bytes。用源码 overlay 将 `okEnvelope` 临时改成 `json.Encoder` 加 `SetEscapeHTML(false)` 后，定向测试实际失败：

```text
okEnvelope() = {"ok":true,"result":{"Headers":{"X-Test":["<&","z"]},...}}
want       = {"ok":true,"result":{"Headers":{"X-Test":["\u003c\u0026","z"]},...}}
```

overlay 没有写回工作树，也没有产生 malformed `\u<\u&` 输出。它证明 exact wire test 能捕获 HTML escaping 回归。

### Exact block early-hit benchmark

修正位置 fixture 后，旧 exact adaptive dispatch 的十轮 benchstat 显示：

```text
256 rules, 16 KiB, first: baseline 8.066 ns/op, production 10.670 ns/op, +32.28%
256 rules, 16 KiB, middle: baseline 23.01 us/op, production 19.80 us/op, -13.97%
256 rules, 16 KiB, last: baseline 47.17 us/op, production 22.14 us/op, -53.07%
```

首位置回退超过计划的百分之五 holdout 上限，因此没有把旧实现直接当作最终结果。overlay 用 direct `strings.Contains` 替换 exact path 后，首位置约为 `7.9 ns/op`，与 baseline 对齐；该单变量结果随后由 `9a8e0cb` 实施。

### 其他 RED 形态

- parsed-root duplicate test 在实现前因旧签名接收 `[]byte` 而非 `gjson.Result` 编译失败。
- single-pass part scan 和 role-gate tests 在 helper、fixture 或 protocol-local gate 缺失时编译或行为失败。
- forward rebuild allocation ceiling 在旧的每 span marshal 实现上超过八次分配。
- success envelope allocation ceiling 在 double marshal 实现上超过四次分配。
- AfterAuth test 在 helper 缺失时编译失败。
- mode-specific compiler、ASCII root、exact rewrite、byte matcher 和 folded KMP 的 forced tests 在对应 production field 或 helper 缺失时编译失败。
- folded KMP、byte matcher、duplicate walker、rebuild fuzz 都使用独立 oracle，不调用待测 production matcher 生成 expected。

## GREEN 验证

以下命令在当前提交序列中实际通过：

```text
go test -count=1 ./...
ok   github.com/DoingDog/cpa-plugin-censorship 1.505s
?    github.com/DoingDog/cpa-plugin-censorship/integration [no test files]

go test -race -count=1 ./...
ok   github.com/DoingDog/cpa-plugin-censorship 12.253s
?    github.com/DoingDog/cpa-plugin-censorship/integration [no test files]

go vet ./...
```

`go test -cover ./...` 实际报告：

```text
coverage: 88.6% of statements
```

普通构建使用 `make build-platform GOOS=windows GOARCH=amd64 VERSION=0.0.0-dev`，并显式传入 Windows `TEMP`、`TMP`、`GOTMPDIR`、`GOPATH`、`GOMODCACHE`、`GOCACHE`、`USERPROFILE`、`LOCALAPPDATA` 和 `APPDATA`。构建生成：

```text
dist/windows_amd64/censorship.dll
dist/windows_amd64/censorship.h
```

脚本测试实际通过：

```text
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
ok   command-line-arguments 2.590s

go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
ok   command-line-arguments 0.949s
```

race 构建仍执行 exact-wire 和 correctness subtests。只有 allocation ceiling 在 race build 中跳过，因为 race instrumentation 会改变 `testing.AllocsPerRun` 的计数；普通构建中的 envelope ceiling、rebuild ceiling、disabled-role ceiling 和 exact-obfs ceiling 均通过。

## Fuzz 证据

六个 fuzz target 按串行顺序各运行约三十秒，全部退出码为零：

| Target | 实际结果 |
|---|---|
| `FuzzRuleEngineAgainstOracle` | PASS，约 `3,304,114` executions |
| `FuzzProtocolTransform` | PASS，约 `2,230,822` executions |
| `FuzzDuplicateWalkerAgainstOracle` | PASS，约 `3,318,147` executions |
| `FuzzRebuildBodyAgainstMarshalOracle` | PASS，约 `84,805` executions |
| `FuzzByteMatcherAgainstOrderedContains` | PASS，约 `3,679,882` executions |
| `FuzzFoldKMPAgainstOracle` | PASS，约 `157,449` executions |

每个 fuzz target 都先执行全部 seed corpus。rebuild fuzz 独立比较 exact bytes 和 error class；byte matcher 使用 rule-major `strings.Contains` oracle；KMP 使用独立 `oracleStrip` 和 `oracleObfuscate`。

## Benchmark 证据

### 实际 package-private threshold

```text
foldRewritePreflightMinRules     = 8
foldRewritePreflightMinTextBytes = 4 KiB
exactByteMatcherMinRules         = 256
exactByteMatcherMinTextBytes     = 16 KiB
exactByteMatcherPrefixRules      = 4
foldKMPMinPatternScalars         = 4
foldKMPMinTextBytes              = 4 KiB
```

### 原始输出和统计文件

原始文件位于 Windows `%TEMP%`：

```text
censorship-round2-baseline.txt
censorship-round2-exact-block-positions.txt
censorship-round2-exact-block-dispatch-final.txt
censorship-round2-candidate-final.txt
```

对应统计使用固定版本 `golang.org/x/perf/cmd/benchstat@v0.0.0-20260825160852-19be9d8e6c70`，结果文件包括：

```text
censorship-round2-exact-block-positions-benchstat.txt
censorship-round2-exact-block-dispatch-final-benchstat.txt
censorship-round2-candidate-final-benchstat.txt
censorship-round2-fold-kmp-final-benchstat.txt
```

所有 strategy benchmark 的 baseline、candidate 和 forced strategy 在同一个 Go test binary 内运行。fixtures、oracle 和 result check 在计时区外构造或执行；计时循环保留 package-level sink。

### Folded rewrite miss preflight

最终 candidate benchstat 的代表性结果：

| Case | baseline | preflight |
|---|---:|---:|
| 8 rules，4 KiB，total miss | `114.94 us/op` | `14.21 us/op` |
| 32 rules，16 KiB，total miss | `1807.08 us/op` | `59.46 us/op` |
| 128 rules，64 KiB，total miss | `28417.9 us/op` | `216.9 us/op` |

三个代表区域均超过百分之十五收益门槛。initial-hit、cascade、span isolation 和 exact mode 由 focused tests 覆盖；preflight 只标记 total miss span，后续 rule-major loop 未改变。

### Exact byte block matcher

最终 candidate benchstat 的 baseline 对 production 结果：

| Case | baseline | production | 结果 |
|---|---:|---:|---|
| 32 rules，4 KiB，last | `1.701 us/op` | `1.714 us/op` | 统计上无显著变化，回退约 `0.8%` |
| 128 rules，64 KiB，last | `106.19 us/op` | `99.24 us/op` | 约改善 `6.5%`，统计上无显著回退 |
| 256 rules，16 KiB，last | `46.77 us/op` | `19.71 us/op` | 改善 `54.46%`，`p=0.000` |
| 256 rules，64 KiB，last | `197.90 us/op` | `89.96 us/op` | 改善 `54.54%`，`p=0.000` |
| 256 rules，16 KiB，first | `7.638 ns/op` | `7.829 ns/op` | 统计上无显著变化，回退约 `2.5%` |
| 256 rules，16 KiB，middle | `22.98 us/op` | `19.91 us/op` | 约改善 `13.4%`，统计上无显著回退 |

两个 calibration 区域超过百分之十五收益门槛，四个 holdout 的 median 回退均未超过百分之五。所有 exact block strategy 的 `B/op` 和 `allocs/op` 都为零。

### Folded KMP

最终 candidate 的代表性 folded rewrite 结果：

| Case | baseline | production |
|---|---:|---:|
| strip，1 rule，64 KiB，4 scalars，none | `1053.1 us/op` | `258.0 us/op` |
| strip，1 rule，64 KiB，8 scalars，none | `1756.3 us/op` | `270.1 us/op` |
| strip，1 rule，64 KiB，16 scalars，none | `3307.2 us/op` | `251.8 us/op` |
| obfs，64 KiB，16 scalars，none | `3087.6 us/op` | `257.1 us/op` |
| strip，64 KiB，16 scalars，sparse | `3167.6 us/op` | `277.8 us/op` |
| obfs，64 KiB，16 scalars，sparse | `3208.2 us/op` | `293.4 us/op` |
| strip，64 KiB，dense | `228.9 us/op` | `222.8 us/op` |
| obfs，64 KiB，dense | `269.9 us/op` | `266.1 us/op` |
| strip，64 KiB，overlap | `220.0 us/op` | `224.5 us/op` |
| obfs，64 KiB，Sigma sparse | `5179.0 us/op` | `694.7 us/op` |
| strip，64 KiB，Kelvin sparse | `4218.5 us/op` | `332.1 us/op` |
| obfs，64 KiB，invalid UTF-8 sparse | `5628.0 us/op` | `1106.0 us/op` |

KMP calibration 区域满足收益门槛。dense 和 overlap holdout 的变化都在百分之五以内；Unicode、invalid UTF-8、sparse 和 no-match holdout 没有 correctness 回退，且大输入明显改善。`4095` bytes 和 `3` scalar small holdout 保持 naive/fallback 路径。

### 其他 benchmark

最终全套 candidate benchmark 共输出 `856` 行并退出码为零，覆盖：

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

success envelope benchmark 为约 `420.2 ns/op`、`312 B/op`、`3 allocs/op`。rebuild benchmark 为约 `3.129 us/op`、`4.129 KiB/op`、`3 allocs/op`。两者的 unit allocation ceiling 也通过。

## Dynamic ABI 和 integration

普通 dynamic integration：

```text
make integration
```

实际通过，覆盖：

- HTTP block 返回 term、role 和 status 400。
- duplicate JSON member 被拒绝。
- legacy `/v1/completions` prompt 使用转换后的 user role。
- watcher reload 在观察到 snapshot B 后使用新配置。
- HTTP 和 SSE output trace 在 nonmatching、strip、obfs 场景保持不变。
- Responses WebSocket 只审查 model-executed turn。
- Responses WebSocket block 返回 status 400 后关闭连接。
- Responses WebSocket output messages 在 strip、obfs 场景保持不变。

`BENCH=1 make integration` 同样通过，动态测试包报告约 `89.888s`。integration runner 使用固定 CPA commit，并验证 checkout clean 和 commit SHA。

现有 ABI unit test 验证 `shouldCopyPluginRequest`：AfterAuth 为 false，BeforeAuth、register、reconfigure 和 unknown method 为 true。dynamic integration 验证 DLL 的外部行为和 output 不变性，但现有 harness 不能直接观测 DLL 独立 Go runtime 内部是否调用了多少次 `C.GoBytes`，也不能直接证明每个 C buffer 的 allocator ownership 恰好释放一次。这两项记录为 blocked evidence，不添加 production instrumentation。

## 已知限制和未宣称事项

- benchmark threshold 是 package-private constants，不是 YAML 配置。
- Task 10、Task 11、Task 12 的 benchmark 使用固定 calibration 与 holdout fixtures，不等同于对全部可能输入分布做 exhaustive search。
- `ReportAllocs` 只表示 host Go runtime allocation，不覆盖 DLL 独立 Go runtime 或 C heap。
- race build 跳过 allocation ceiling，但 correctness、exact wire、fuzz 和完整 race execution 仍运行。
- 首次 `make build` 因递归 make 清空 Go 临时目录，尝试写 `C:\WINDOWS\go-build...` 并返回 access denied。使用显式 Windows 临时路径后 c-shared build 成功。该环境问题没有改动业务代码，也没有降低测试。
- `dist/` 和 `.integration/` 是 ignored 产物，没有提交。

## 最终 no-release 边界

最终状态是已实现并在本地验证的未发布候选。当前 Git 状态为：

```text
## main...origin/main [ahead 37]
```

当前 HEAD 没有 tag。`git diff be27107..HEAD -- RELEASE_NOTES.md go.mod go.sum` 为空。下一步只提交本 evidence 文档，不执行 tag、push、发布或提交 build artifact。
