# censorship 性能优化 TDD 证据

## 来源与范围

- 设计来源：`docs/superpowers/specs/2026-09-01-censorship-plugin-design.md`
- 本次范围：folded `block`、`strip`、`obfs` 的长输入吞吐和分配，以及测试 snapshot 的 matcher 复用。
- 未执行全仓库 review，也未修改 CLIProxyAPI。

## 用户旅程

- 作为 CLIProxyAPI 管理者，我希望大量词条和长请求仍能及时完成输入审查，避免规则数量与输入长度相乘造成延迟。
- 作为插件维护者，我希望 folded rewrite 处理高密度匹配时不保存全部 occurrence，避免峰值内存随匹配数量增长。
- 作为测试维护者，我希望手工构造的 fuzz snapshot 复用预编译 matcher，避免测试路径重复编译配置。

## RED 证据

性能改动前，提交 `df35482` 增加的目标测试无法编译，失败原因是生产接口尚未实现：

```text
undefined: newFoldMatcher
unknown field BlockMatcher in struct literal of type configSnapshot
undefined: stripFoldRule
undefined: obfuscateFoldRule
```

canonical key 回归测试在修复前实际失败：

```text
foldClassRune(K) = U+004B, foldClassRune(K) = U+006B; want equal keys
```

fuzz snapshot 回归测试在补字段前实际失败：

```text
snapshotForFuzz left BlockMatcher nil
```

## GREEN 证据

以下目标测试在对应修复后通过：

```text
go test -run '^(TestFold|TestSnapshotForFuzzPrecompilesFoldBlockMatcher)' .
ok   github.com/DoingDog/cpa-plugin-censorship  0.073s
```

最新普通验证通过：

```text
go test ./...
ok   github.com/DoingDog/cpa-plugin-censorship  2.018s
?    github.com/DoingDog/cpa-plugin-censorship/integration [no test files]

go vet ./...
```

seeded fuzz oracle 和 release packager tests 也在修复后通过。focused tests 覆盖 failure suffix、prefix、重复 folded rule、Kelvin sign、Sigma、跨 span、rule-major role、全部非重叠 occurrence 和 obfs source case。

## 实现结果

| 保证 | 测试或基准 | 结果 |
|---|---|---|
| folded `block` 每个 span 只扫描一次，并保留最低 YAML rule index | `matcher_test.go` folded matcher tests，`BenchmarkTransformMatrix` | PASS |
| `K`、`k` 和 `K` 使用同一 `SimpleFold` canonical key | `TestFoldClassRuneUnifiesKelvinSign`、rule-major oracle | PASS |
| failure suffix 不丢失较低 YAML rule index | `TestFoldMatcherIncludesFailureSuffixRules` | PASS |
| folded `strip` 不保存 occurrence 列表 | `TestFoldStripRuleProcessesAllNonOverlappingOccurrences`、dense benchmark | PASS |
| folded `obfs` 保留实际源文本并为每个 occurrence 插入一次字符 | `TestFoldObfuscateRulePreservesSourceCasePerOccurrence`、dense benchmark | PASS |
| fuzz snapshot 复用预编译 matcher | `TestSnapshotForFuzzPrecompilesFoldBlockMatcher` | PASS |

## 性能证据

在相同输入形状下，`1 MiB`、`1024` 条 folded rules 的 block 基准从约 `10.50 s` 降至约 `23.0 ms`。`20 MiB` 输入从约 `208.7 s` 降至约 `351 ms`。

dense folded rewrite 的最新单次 benchmark 为：

```text
BenchmarkFoldStripDense       12.8114 ms/op   1 allocs/op
BenchmarkFoldObfuscateDense  25.9077 ms/op   4 MiB allocated   1 allocs/op
```

obfs 的 builder 现在按非重叠 occurrence 的理论上限预留容量，不再因高密度匹配反复扩容和复制。

## 已知验证阻塞

- 最终 `go test -race ./...` 在项目外的最小 race package 中也于 `# runtime` 触发 Windows Go `unexpected signal during runtime execution`，错误码为 `0xc0000006`。因此当前环境没有提供最终 race PASS 证据。
- `go test -cover ./...` 读取 `F:\go-sdk\go1.26.5\src\runtime\coverage\coverage.go` 时报告设备 I/O 错误，并提示没有 `cover` tool。当前环境没有提供 coverage PASS 证据。
- `make build` 和 `make integration` 的 Go 子进程间歇触发同一 `0xc0000006`。直接 c-shared build 曾成功，但 Make 版本的最终验证仍需在稳定的 Go SDK 或本地磁盘环境中重跑。
