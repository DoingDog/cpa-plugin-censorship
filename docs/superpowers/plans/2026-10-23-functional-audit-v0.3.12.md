# v0.3.12 功能修复实施计划

日期：2026-10-23。依据：[设计规格](../specs/2026-10-23-functional-audit-v0.3.12-design.md)。基线 `23fe07f1c4ba2ec23879ec2addc8015b4d7acc6d`，当前 v0.3.11，目标 v0.3.12。固定 CPA v7.2.152 / `c76dfd4e0edabab9000628b1560ab8ab379eadb8`、schema 5、ABI v1。

## 目标与执行依赖

首次无效注册后，operator 修正为合法空或非空 models 配置即可恢复实际拦截。已有有效配置继续遵守原 phase 保护。行为以规格 L1 至 L8 为准，生产修改限于初始化状态与 phase guard。

已完成的两次独立验证包含真实 DLL、固定 `Host.ApplyConfig`、watcher HTTP 复现。本稿未把其余全面扫描记为完成，也未把未证实候选列为修复。已报告通过的基线 test/race/vet/integration/build/package 与正式 DLL 注册不必在文档阶段重跑；实施和最终合并后仍须执行下列回归。

执行顺序：完成规格与本计划 -> 收齐相关完整性核验并补充同一文档 -> 独立 worktree 完成 U1 及其他最终确认的互不冲突修复 -> 主会话逐项审查并合并 -> 独立集成发布 worktree 完成 U2 -> 主会话合并 U2 并执行 U3。

### 完整性核对清单

- 收齐初始全面审查的全部分工与报告，逐项核对配置/lifecycle、RPC/ABI、规则引擎、五种 provider selectors/transform、filter/phase、integration、packager/CI/发行说明的覆盖；缺项继续交由对应审查者补查。不以本 lifecycle 规格代替整轮扫描结论。
- 收回尚无成功 snapshot 时再次收到无效 reconfigure 的独立核验，核对实际 envelope、host active 状态与后续合法恢复。该结论可能影响同一 lifecycle 函数，正式编码派发前统一其规格与文件责任；本稿不预设 error/OK 响应或新增修复。
- 其他报告仅在独立复现、反证与插件侧可执行修复均成立后，增补本规格和本计划；新增项写明 caller、测试归属、最小差异与验收，不另建重复规格，不加入网络安全扫描、CPA core 修改或无关重构。

### 工作区责任

每项编码任务由一个独立 worktree 的执行者连续完成 TDD、实现、测试、规格自审、修正、复审及英语提交，达到完成条件后主动回报。主会话仅在任务完成且确认修改位置无冲突后合并；不能接收未完成的实现或测试步骤。

U1 负责 `config.go`、`main.go`、`main_test.go` 的 lifecycle 区域及新增 `integration/lifecycle_test.go`。后续 provider 任务使用各自已有行为测试位置，避免同时修改这些文件；确需同文件时按依赖顺序执行。发行文档与 `main_test.go` 的版本 guard 留给代码合并后的 U2，不能与 U1 并行修改。前置依赖满足且文件不冲突的修复并行执行；新增独立任务在前置依赖满足后立即派发，现有 workflow 无空闲时新增 workflow。

各执行者在自己的 `.local-verification/functional-audit-20261023/<task>/` 保存复用脚本、命令、退出码和日志，使用本次预先加入的精确 ignore 规则，不并行修改 `.gitignore`。一次性临时文件使用 OS 临时目录。所有 shell/Go 命令显式核对自己的绝对 worktree 路径；路径消失或隔离工具无法访问时报告实际限制，不切回原仓库或修改其他 worktree。不轮询后台 workflow/subagent 输出，等待完成通知。

## U1. 生命周期恢复与真实调用回归

依赖：规格、自审及上述相关完整性核对完成。文件：`config.go`、`main.go`、`main_test.go`，新增 `integration/lifecycle_test.go`。行为范围：L1 至 L8。

### 公开 RPC 的 TDD

1. 在 `main_test.go` 的 lifecycle 测试处复用 `lifecycleJSON`、`mustHandle`、`decodeEnvelope`、`decodeResult`、`callInterceptRequestAt` 和 selected-auth helpers。每个 case 保存并恢复全局 snapshot/callback，明确从未安装状态开始，不依赖测试顺序，不对这些全局状态测试使用 `t.Parallel`。
2. 先加入 `TestInitialRegisterFailureRecoversOnReconfigure`：空词 register 失败后，合法非空 models reconfigure 应在 selected-auth after 拦截 `BLOCKME`。在当前实现运行 focused 命令取得 RED，确认实际失败是成功响应后仍未拦截；同时加入空 models 恢复及重复合法非空 models 对照。
3. 完成下节 native/watcher 测试的 RED 后，才删除启动预装默认 snapshot 的 `init()`，保留 `defaultSnapshot()`；仅在 current 非 nil 时限制 phase 切换。保留原 parser、atomic store、已有有效配置的拒绝分支、nil gate 和同步 `C.GoBytes`。
4. 运行同一 focused 命令取得 GREEN，再补齐 L3 至 L7 的边界与原有 lifecycle 回归。无效候选保留 last-good 使用既有测试；nil gate 对 malformed raw envelope 要求精确空 RPC response，并检查输入未变。
5. 成功安装零规则配置后，分别尝试两个方向的 phase 变化并检查原拒绝日志；随后在原 phase 加入规则，以公开请求实际拦截证明 phase 未切换，不能只断言内部指针或布尔值。

建议 focused 测试名为 `TestInitialRegisterFailureRecoversOnReconfigure`、`TestUninitializedInterceptorsNoOp`、`TestReconfigureKeepsPhaseForEmptyRules`。执行者记录实际采用名称及命令，例如 `go -C "$W" test -count=1 -run '^TestInitialRegisterFailureRecoversOnReconfigure$' .`；`$W` 必须是自己的已核对绝对路径。RED/GREEN 均保留真实退出码与失败断言，不把计划中的输出写成已发生结果。

### Native 与 watcher HTTP 的 TDD

1. 在专属 `integration/lifecycle_test.go` 复用 [harness](../../../integration/harness_test.go) 的 `startCPA`、`writePluginConfig`、`postChat`、upstream capture 与清理。native case 直接使用固定 `pluginhost.New` / `Host.ApplyConfig`；[既有 `withDynamicABIHost`](../../../.github/scripts/testdata/abi_benchmark_test.go) 要求首次成功注册，仅继续用于成功路径与 active-after 对照。
2. native 每个初始状态由测试父进程启动独立子进程，使用绝对测试程序路径和有界退出等待。一个子进程内依次保留同一 host、DLL 路径与实例，执行失败 -> 合法恢复 -> 重复合法配置。其他子进程覆盖首次合法注册、已有有效 snapshot 的双向拒绝、零规则 phase 保护、同 phase 更新与无效 candidate 保留。`ShutdownAll` 仅做资源清理，不能用来重置 DLL 静态 snapshot。
3. HTTP 对空与非空 models 分别启动独立 CPA 进程。初始配置含空词，确认首次 register 错误日志；clean canary 必须能正常到达 mock upstream。保持进程、DLL 与模型配置不变，使用 `writePluginConfig` 写入合法规则，并以有截止时间的 watcher 观察确认新规则生效。
4. 就绪探针与 clean canary 不计入最终请求基线。确认新配置生效后记录 upstream arrival/request 基线，再发送 `BLOCKME`，要求 canonical HTTP 400 `censorship_blocked` 且两种计数增量均为零。模型 case 用 harness 的可执行 `censorship-integration-model`，不引入无效 model/provider 对照。
5. 在公开 RPC 第 3 步修改生产代码前，让新增 native/watcher 恢复测试先得到 RED；修复后相同场景得到 GREEN。保存实际失败日志，不能在修复后用推断补写 RED。

[runner](../../../.github/scripts/integration-runner.go) 已自动复制 `integration/*.go` 与 ABI fixture，无须新增通用 runner。准备顺序必须保持：清理 runner 自己的旧生成夹具 -> 核对固定 CPA checkout -> 构建干净 CPA -> 构建当前 worktree 插件及固定 model-mapper -> 复制源夹具 -> 运行测试。CPA 必须满足已有 `vcs.revision` 与 `vcs.modified=false` 检查，不手改 `.integration/`。

完成条件：RPC、native、watcher 均有有效 RED/GREEN；L1 至 L8 逐项对应测试与输出；`make test`、`make race`、`make vet`、`make integration`、`make build` 通过。自审实际 diff 后完成必要修正和受影响回归，`git diff --check` 通过，只提交相关代码与测试。向主会话主动返回 commit、stat、测试证据和剩余限制。

## U2. 顺序集成、英语发行说明与正式包

依赖：U1 及其他最终确认修复均完成、经审查且已合并；不存在同文件并行修改。独立集成发布 worktree 从合并结果开始，连续完成本节全部工作。

修改文件：`README.md`、`RELEASE_NOTES.md`、`main_test.go` 的文档/当前发行版本断言。执行者核查生产代码、CI 与 packager 的合并状态，不为本任务顺手修改它们。

1. 英语 README 说明首次无效注册后的合法恢复与已有 phase 限制；更新当前版本示例至 v0.3.12。RELEASE_NOTES 新增实际修复及固定兼容版本，保留 v0.3.11 等历史段落与 glibc 2.34+ 要求。bare `pluginVersion` 保持 `0.0.0-dev`。
2. `main_test.go` 的 current-release guard 改为 v0.3.12，新边界是 v0.3.11 历史标题；把 v0.3.11 compatibility 句加入历史断言，保留其余历史检查。先记录旧 guard 对新增发行段的 RED，再修改 guard 并取得 GREEN。
3. 从最终集成源码完成下表验证。项目根 suite 不包含隐藏 `.github/scripts` 的 packager tests，必须独立执行。

| 检查 | 实际命令与要求 |
|---|---|
| 根目录完整回归 | `make -C "$W" test`、`make -C "$W" race`、`make -C "$W" vet` |
| Pinned integration 与构建 | `make -C "$W" integration`、`make -C "$W" build VERSION=v0.3.12`，包括全部 HTTP/SSE/WS/native 及 `TestDynamicABIActiveAfter` |
| 独立 Windows packager | `go -C "$W" test -count=1 .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go .github/scripts/package-release_windows_test.go`；对同一文件集合另行运行 `go test -race -count=1` 与 `go vet`，均显式保留 `-C "$W"` |
| 六个有界 fuzz 目标 | `FuzzRuleEngineAgainstOracle`、`FuzzFoldKMPAgainstOracle`、`FuzzDuplicateWalkerAgainstOracle`、`FuzzProtocolTransform`、`FuzzRebuildBodyAgainstMarshalOracle`、`FuzzByteMatcherAgainstOrderedContains`，逐个使用 `go -C "$W" test -run '^$' -fuzz '^<target>$' -fuzztime=3000x -parallel=2 .`，仅验证功能 |
| Selected-platform 正式包 | `make -C "$W" package GOOS=windows GOARCH=amd64 VERSION=v0.3.12`，保存该次 ZIP 与 checksum 供比较 |
| Aggregate 正式包 | `make -C "$W" package VERSION=v0.3.12`，核对本 worktree 已构建平台的 aggregate；本机结果不代表七平台构建 |

正式包验收复用可靠 ZIP parser，检查 CRC、精确成员 DLL 与 LICENSE、LICENSE 字节、DLL 权限、两个打包入口的 ZIP/DLL 一致性、lowercase SHA-256 sidecar 与 `checksums.txt` 精确条目。脚本、日志与每次产物证据存入专属 `release/` 子目录，不手改 `dist/`。

从正式 ZIP 提取同一 DLL，先与 dist DLL 做字节及 SHA-256 比较，再运行 `go -C "$W" run ./.github/scripts/integration-runner.go -abi-smoke "$EXTRACTED_DLL"`。另用专属可复用 native 验收脚本实际读取初始化 ABI table 与 `plugin.register` envelope，断言 ABI 1、schema 5、`Metadata.Version == "0.3.12"`、Object `words`、七个字段及 request-only capability，并验证 selected-auth active-after。不能以 SDK 常量或注册成功代替真实 schema 值。既有 `TestHTTPFunctionalNativeRegistrationAndPhases` 固定期望 `0.0.0-dev`，继续用于源码 integration，不直接用来验收正式版本 DLL。

正式包验收前记录 worktree HEAD、状态、源码/依赖 blob 与 SHA-256、工具链、命令、构建输出与 `go version -m`。按 [nested worktree provenance 说明](../../solutions/workflow-issues/go-worktree-build-provenance.md) 分别核查实际编译源码、VCS metadata 和产物一致性；本地 metadata 差异按证据解释，不能替代远端最终 revision 验收。

完成条件：完整命令真实通过，正式 ZIP 的同 DLL 实际注册与 active-after 通过，英语说明仅陈述已核实范围。执行者自审、修正、复审并用英语提交，仅包含本节必要文档/guard 修改；向主会话返回全部证据，不把远端 CI、七平台运行或发布写成已完成。

## U3. 主会话 main CI、tag 与七平台发行验收

依赖：整轮审查覆盖已收齐、所有已确认可执行修复及 U2 均完成。此节由主会话在获得发布授权的范围内执行；修复子任务不 push、不 tag、不合并主仓库，不创建 PR 或另发 issue。

1. 独立复核各 worktree 的规格覆盖、提交、命令证据与路径冲突，全部完成后合并 main。对最终 main SHA 运行并等待 [Build workflow](../../../.github/workflows/build.yml) 全部必需 job 成功，包括 Linux test/race/vet/integration、Windows amd64 DLL smoke 与七平台构建。main 分支包按现有 workflow 使用 `0.0.0-dev`，不把它误报成 v0.3.12 发行包。
2. 核对 local/remote v0.3.12 tag 尚未占用；最终 main CI 全部成功后才创建并推送 tag。等待相应 tag CI 和 release job 全部成功，不以本地成功代替远端成功。
3. 分别下载最终 main CI 产物与 tag 发行产物到各自新的空目录，检查七个平台：linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64、windows/arm64、freebsd/amd64。七个共享库的 `vcs.revision` 分别等于对应已通过 CI 的最终 main/tag SHA；缺失或不同保持未通过。另核对平台、`c-shared`、固定依赖与实际版本注入，tag 必须为 0.3.12。
4. 发行资产精确为七个 ZIP、七个 `.sha256` 和一个 `checksums.txt`，共 15 项。逐包验证 CRC、精确共享库与 LICENSE 成员、LICENSE 字节、lowercase sidecar 与 aggregate 的文件名/hash 对应；核对 Linux glibc 要求。保留 Linux `.so` 与 Windows amd64 DLL 的真实注册、schema/ABI/字段及 active-after 证据，其他平台若仅构建通过必须如实标明。
5. 确认本次各任务修改已合并、没有未提交修改或未合并提交后，退出对应任务 worktree，返回原工作目录，再通过 Git 清理本次已无用途的 worktree 与分支。保护用户原有与其他任务内容，不强制删除未保存内容；本机清理失败不阻塞其他工作，如实列出未清理项。

最终完成条件：全部审查范围逐项闭合，所有确定修复可复现且完成 TDD，最终 main CI、tag CI 与 15 个发行资产分别有真实验收证据。需保留的脚本/日志留在对应忽略目录；清理本次无需保留的临时内容，不提交庞大日志或撤销方案。
