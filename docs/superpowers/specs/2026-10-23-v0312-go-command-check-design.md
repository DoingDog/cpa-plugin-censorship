# v0.3.12 Go 命令覆盖断言设计

## 范围与现有语义

仅修改 `.github/scripts/package-release_test.go` 中的 `TestMakeBuildIgnoresTargetOverrides`，并新增本设计及对应实施计划。Makefile、产品代码、依赖、CPA、主审查文档及生成产物保持不变。

Makefile 使用 `GO ?= go`，允许环境和命令行提供 Go 命令文本。命令可以包含 executable 的绝对路径、shell 引号及合法的 `-C` 参数。未定义 `GO` 时默认使用 `go`；显式空值保持空值的语义。

现有测试连续匹配 `go run` 和带目标 tuple 的 `go build`。相同 executable 的绝对路径会触发构建断言失败，`go -C .` 会触发 helper 断言失败；实际 Make 输出仍保留正确目标和 helper。这是测试断言缺陷。

## 最小修正

在原测试内用 table/subtests 复用全部 Make 检查，覆盖继承环境（默认 CI 中 `GO` 未定义）、真实 Go executable 的绝对路径、`go -C .`。使用 `exec.LookPath` 查找已安装的 `go`，按 Make 使用的 shell 规则引用路径。各子测试串行执行，并用 `t.Setenv` 隔离覆盖值。

各子测试通过 `os.LookupEnv("GO")` 读取原始命令文本，仅在未定义时采用 `go`。helper 断言连续匹配该文本及 `run ./.github/scripts/package-release.go "$helper"`；构建断言连续匹配 `GOOS="linux" GOARCH="amd64"`、该文本及 `build -trimpath -buildmode=c-shared`。不解析命令、不强制 `GO=go`、不增加 helper 或依赖。

host tuple、raw `VERSION`、`vv` 归档路径和 aggregate 断言保持完整。

## 验收边界

- RED：先加入子测试并保留原硬编码断言，实际观察绝对路径、`-C` 被正确 Make 输出误判。
- GREEN：修正两处断言后，在 Windows 和 Debian WSL 通过完整 packager test、race、vet。
- 正例：外部 `GO` 未定义、绝对路径、`go -C .`、绝对路径加 `-C`、带引号的命令、`env` wrapper，均使用同一个真实 Go executable。
- 负例：仅变异捕获 stdout 中的实际库构建行，分别改变 `GOOS`、`GOARCH`、`-buildmode=c-shared`；另移除 packager 调用的 helper。真实测试必须分别失败，不提交变异。
- 测试源码 SHA-256、命令、退出码、RED/GREEN 及负例日志保存在本 worktree 已忽略的 `.local-verification/functional-audit-20261023/C/`。临时 clone、overlay 及数据按归属清理。

此次不改变插件行为或 fixture，无需重跑模型 integration/native；dry-run 检查不增加构建产物正确性的结论。
