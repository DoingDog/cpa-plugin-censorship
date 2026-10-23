# v0.3.12 Go 命令覆盖断言实施计划

依据：[Go 命令覆盖断言设计](../specs/2026-10-23-v0312-go-command-check-design.md)。本计划仅承接该设计中的测试修正，不修改主审查计划或发行记录。

## 文件与完成条件

独占文件为 `.github/scripts/package-release_test.go`、上述设计和本计划。完成时必须有文档前置提交、测试修正提交、Windows/WSL 验收证据、干净的 tracked 状态及临时目录清理记录。提交使用英语，不 amend、push、tag、合并或创建 PR。

## 执行步骤

1. 核对本 worktree 的 cwd、Git toplevel、GOMOD、干净状态及固定基线 `013e84afa1e7155567522e689ee1c13812dacd7b`，显式执行 `git -C <本 worktree> merge --ff-only <基线>`。仅读已完成诊断报告、目标完整函数、必要 helper 和 Makefile。
   - 核查：目录归属正确，测试文件未被其他任务修改；`.local-verification/functional-audit-20261023/C/` 已被忽略。
2. 核对设计与本计划的相对链接、修改范围及 `git diff --check`，仅提交两份文档。
   - 核查：提交中没有测试、产品或其他文档修改。
3. 在原测试内加入继承环境、带 shell 引号的真实 Go 绝对路径、`go -C .` 三个串行子测试，复用原有全部检查，暂时保留硬编码断言。
   - 核查：运行 focused 测试，继承默认场景通过，新增场景在正确 Make 输出上分别触发原构建/helper 断言。保存 RED 源码和日志。
4. 用 `os.LookupEnv` 读取原始 `GO`，仅未定义时默认 `go`；精确修改 helper/build 两处断言，构建检查包括目标 tuple 和 `-trimpath -buildmode=c-shared`。
   - 核查：focused GREEN 通过，gofmt 无额外改动，原 host/raw version/archive/aggregate 检查逐项保留。
5. Windows 使用当前 worktree；Debian WSL 在 OS 临时新目录本地 clone 固定基线，仅同步已核对的测试修改。
   - 核查：Windows 与 WSL 测试源码 SHA-256 一致，分别核对 GOMOD；脚本与临时源码/数据分目录，Go 使用现有安装，不安装或修复系统。
6. 各平台显式选择 packager 源文件集，执行完整测试、race、vet，并运行设计中的外部 `GO` focused 矩阵。
   - Windows 文件集：`.github/scripts/package-release.go`、`package-release_windows.go`、`package-release_test.go`、`package-release_windows_test.go`。
   - WSL 文件集：`.github/scripts/package-release.go`、`package-release_other.go`、`package-release_test.go`。
   - 命令形态：在各自仓库目录运行 `go test -count=1 <文件集>`、`go test -race -count=1 <文件集>`、`go vet <文件集>`；focused 命令增加 `-v -run '^TestMakeBuildIgnoresTargetOverrides$'`。
   - 核查：正例全部成功；各命令、环境、退出码及日志留在忽略的验证目录。后台任务等待完成通知，不轮询。
7. 在临时 overlay 中对捕获的 Make stdout 分别施加四种单项变异，运行真实断言。
   - 核查：仅实际 build 行的错误 OS/ARCH/buildmode 触发构建断言，移除 helper 触发 helper 断言；正确递归 Make tuple 保留，排除弱匹配通过。变异仅用于测试自身验证，不提交或冒充产品产物。
8. 自审、必要修正及复审后，运行 `git diff --check`，核对全部验收项，仅提交测试文件。
   - 核查：两个英语提交与文件 stat 清楚；tracked/untracked 状态干净，日志仍被忽略；清理本任务 OS 临时 clone、overlay 和数据，不删除缓存、其他 worktree 或其他任务内容。本 worktree 保留供主会话后续合并。

## 限制

只验证本次测试修正及 packager 回归；不重审全仓库，不运行模型 integration/native，不修改生成的 `.integration/` 或 `dist/`。显式空 `GO` 不转换成默认值，其原有失败可以保持。
