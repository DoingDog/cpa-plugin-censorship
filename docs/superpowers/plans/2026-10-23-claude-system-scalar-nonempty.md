# Claude 消息内 system scalar 非空修复实施计划

日期：2026-10-23。依据：[设计规格](../specs/2026-10-23-claude-system-scalar-nonempty-design.md)。基线 `4911f30bc17bfc2b3261c578074bcc78cbefa5ae`，目标 v0.3.11。

## 执行边界

本次唯一编码子任务在独立 worktree `worktree-wf_3a105323-ec9-1` 连续完成，不再委派、不 push、不 tag、不合并 main。主会话负责独立 review、最终合并和发布。各步骤有顺序依赖；只有上一项验证完成后才继续。状态和真实命令证据保存在提交与本地记录中，不将待办写成已完成。

## 1. 文档基线提交

文件：本计划、对应设计规格。

- 核查初始 cwd、HEAD、branch、干净状态及项目规则。
- grep `appendNonEmptyStringSpan`、`RequiresNonEmpty`、`collectClaude` 的所有 helper/caller；阅读 transform 最终校验、两阶段 RPC、现有测试与集成生成流程。
- 阅读精确官方 messages shorthand/SystemMessageParam/TextBlockParam、How it works、Per-message effort 章节。记录消息内 system 与顶层 system、effort-only `[]` 的边界。
- 检查文档 diff、链接和 Markdown 格式，仅提交这两份文档，之后才编写测试和生产代码。

验收：文档范围与代码实际调用链相符，官方链接支持对应事实，Git 提交仅含本步骤文件。

## 2. 公开 RPC 的首个 RED/GREEN 切片

文件：`functional_corrections_test.go`、`selectors_claude.go`；本地记录目录的精确 `.gitignore` 规则。

- 复用 `functionalRequest`、`functionalCall`、`requireOpenAIInvalidRewrite`；加入一个合法位置的 system scalar 完整 strip case，先不堆其他测试。
- 运行 focused regression，保存真实 RED 输出和退出码，确认失败原因是返回空 replacement body且未 Terminate。
- 仅在 scalar role 条件加入无 per-message `output_config.effort` 字段的 system，使用 gjson 字段存在判断并复用 `appendNonEmptyStringSpan`，不改其他 collector/transform/RPC/ABI 实现。
- 运行相同命令取得 GREEN，再运行 Claude 现有相关测试。

验收：canonical 400、无 replacement Body/Headers/ClearHeaders、原请求不变；生产 diff 只有必要 role 条件。`C.GoBytes` 完整保留。

## 3. 两阶段行为与独立 oracle

文件：`functional_corrections_test.go`、`fuzz_test.go`；必要时局部复用 `selectors_claude_test.go`。

- 复用已有表驱动 helpers，按实际风险补无 effort 的普通 scalar 的 literal/fold/cascade 清空和 `output_config: {}`，block 优先级、partial strip、obfs、no-match、最终相消。
- 验证 roles/formats/rules 关闭、filter bypass、BeforeAuth/AfterAuth gating、未选或未知 shapes；原始空 scalar 与 effort-only `[]` 不新增验证。
- 覆盖带 per-message effort 的 system scalar 完整 strip 到空、顶层 system、最终 assistant、tool_result/mcp_tool_result scalar 保持行为，typed/user/history/title 复用现有回归。
- 避免扩大现有全局 functional fixtures；若复用全局矩阵加入 system fixture，则同步修正用 `[system]` 的 `role_disabled` 控制。
- 先向 `TestProtocolOracleAcceptsOnlyClaudeRequiredEmptyStrip` 加入无 effort system scalar、fold、空 output_config 正例和带 effort scalar 等 false-positive 反例，运行 RED；再精确修改 `oracleClaudeRequiredFieldWouldBeEmpty` 的 scalar 判断，运行 GREEN。另用伪造 accepted 空输出的负对照先 RED 后 GREEN，确保 `checkProtocolResult` 对本次应拒绝的普通 system scalar 清空强制要求 invalid，而不只检验 Invalid 分支。
- semantic/raw selector 已支持该路径，保留其独立实现和原有约束结构。

验收：两阶段和 oracle focused 命令通过；保留独立 RED/GREEN 记录，错误模式、partial、no-match、原始空输入、顶层 system 与 effort-only 不误判。

## 4. 固定 CPA 与 native/HTTP 集成

文件：`integration/http_test.go`，复用 `.github/scripts/testdata/abi_benchmark_test.go` 提供的 native host helper，不手改 `.integration` 生成文件。

- 增加当前源码 DLL native 注册与 BeforeAuth/selected-auth AfterAuth 的无 effort 普通 scalar full-strip rejection，带 effort scalar 完整 strip 到空、partial 和合法 effort-only/top-level 对照。
- HTTP fixture 使用 `claude-opus-5-5` 和合法 user -> system 位置，effort 请求携带 `mid-conversation-output-config-2026-07-01`。通过既有 provider mapping 把本地别名映射到该模型。
- 完整 strip HTTP 必须 canonical 400、JSON invalid-text 响应且 upstream arrival/request 均为零。partial/effort-only/top-level 等合法对照检查实际转发内容和机器字段；考虑 CPA `ensureCacheControl` 正规化，不错误要求 wire body 与入站逐字节相同。
- runner 自行获取/构建固定 CPA 和固定 model mapper，从当前 worktree 源码构建 DLL、复制源 integration fixtures、运行全部 HTTP/SSE/WS/native。

验收：`make integration` 完整通过，保留当前源码路径/工具链/构建产物证据；`TestDynamicABIActiveAfter` 通过。禁止用主目录旧 DLL 证明本次行为。

## 5. 英语说明与 v0.3.11

文件：`README.md`、`RELEASE_NOTES.md`、`main_test.go` 的当前发行段兼容性断言。

- 当前兼容版本及当前命令示例使用 v0.3.11，固定 CPA、schema 5、ABI v1 不变。
- 说明无 per-message effort 的消息内 system scalar 的 Changed-final empty 约束及带 effort scalar、effort-only `[]`、顶层 system 边界。
- RELEASE_NOTES prepend 2026-10-23 的最小 fix section，保留旧历史和无关 pending 原文。`main_test.go` 的 current-release guard 同步检查 v0.3.11 段，并将 v0.3.10 兼容句移入历史断言列表，保留 glibc 2.34+ 要求。
- bare `pluginVersion` 保持 `0.0.0-dev`。仅在真实命令通过后描述本机已验证范围；不宣称商业 API、CI、其他平台已验收。

验收：英语文案与实现、实际结果相符；历史段落无无关改写。

## 6. 完整本地验收与正式包

- 运行 `make test`、`make race`、`make vet`。
- 独立运行 Windows packager 的 main source、Windows helper、通用及 Windows tests，分别 test/race/vet。
- 对现有六个 fuzz 目标分别运行有界样本：`FuzzRuleEngineAgainstOracle`、`FuzzFoldKMPAgainstOracle`、`FuzzDuplicateWalkerAgainstOracle`、`FuzzProtocolTransform`、`FuzzRebuildBodyAgainstMarshalOracle`、`FuzzByteMatcherAgainstOrderedContains`。仅功能回归，不做安全或 DoS 测试。
- 运行 `make integration` 完整流程，再运行 `make build VERSION=v0.3.11`。
- 运行 selected-platform `make package GOOS=windows GOARCH=amd64 VERSION=v0.3.11` 和 aggregate `make package VERSION=v0.3.11`；检查 ZIP CRC、精确成员（DLL、LICENSE）、LICENSE 字节、注入版本、lowercase SHA-256 sidecar、aggregate 内容和 DLL 一致性。
- 从 ZIP 提取同一正式 DLL，以 byte/hash equality 核对 `dist/windows_amd64/censorship.dll`，运行 runner `-abi-smoke` 验证 active-after；另用 native 注册真实断言 `Metadata.Version == "0.3.11"`、Object words、七个字段、schema 5 和 ABI v1。记录源文件 SHA-256、Go build info 和 worktree，按[provenance 说明](../../solutions/workflow-issues/go-worktree-build-provenance.md)区分源码来源和 nested worktree VCS metadata。
- Bash make 缺 Go cache 环境时，用 Make 参数传入现有 Windows `GOPATH/GOMODCACHE/GOCACHE/USERPROFILE/LOCALAPPDATA/APPDATA/TEMP/TMP`；不调查或修复本机系统问题。长命令同步等待或后台等待通知，不轮询后台输出。

验收：所有命令真实退出 0，未验证项明确列出；本次本机正式包仅 Windows amd64，不用其代替七平台发布验收。

## 7. 逐项自审、复审与提交

- 按设计的每个完成条件检查实际 diff 和日志；排除无关变更、重复 harness、冗余抽象。
- 如有修正，由本 worktree 连续完成修正、focused 和受影响完整回归，再次逐项审查。
- `git diff --check`，核对所有待提交路径，英语 commit message；提交仅相关 source/tests/docs/精确 ignore 规则。
- 向主会话主动返回 branch/worktree、commits、stat、RED/GREEN、完整命令结果、官方依据与限制。本地 evidence 不提交庞大日志，保留可复核记录，清理无需保留的本次临时文件。

验收：本地要求全部完成、工作区无未提交相关修改、主会话可独立审查提交和证据。主会话提出有效问题后续用本 worktree，修复/验证/提交由本任务执行者完成。

## 主会话后续独立 review 与发布门槛

1. 独立检查提交、测试证据、规格覆盖，确认修改位置无冲突后合并 main；在最终 main SHA 运行 Linux/Windows 必需 CI 和七平台构建及 Linux `.so` 注册。
2. main CI 全部通过后创建 v0.3.11 tag，再等待 tag CI 全部通过。
3. 验收七个平台的 15 个资产：七 ZIP、七 sidecar、`checksums.txt`。逐项检查 ZIP CRC/内容/LICENSE、版本注入、平台/`c-shared`/固定依赖、lowercase SHA-256 和 aggregate；读取每个共享库的 `vcs.revision` 并与对应最终 main/tag SHA 独立比较，缺失或不同保持未通过。
4. 确认所有修改已合并且无未保存内容后，退出并清理本次 worktree 和对应本地分支；保留用户已有产物及其他任务 worktree。

以上远端流程由主会话执行，本子任务的本地完成不代表远端流程完成。商业 API 接受结果与其他平台 native 实际加载不在本机验收范围。

## 本地完成记录（2026-10-23）

执行环境为 Windows amd64、Go 1.26.5。保留接管时四个修改，从 `dfa14b2e52ebfd9ce15a65c011f3ae119c3bab33` 继续。EnterWorktree 因此脚本子会话从仓库根目录启动而未能切换，所有命令均显式使用本 worktree 的绝对路径或 subprocess cwd，没有 reset、discard 或额外实现 worktree。

### RED/GREEN 与范围核对

- 首个真实 RPC RED 保留在 `.local-verification/claude-system-scalar/red-rpc.log`，退出 1。随后以 OS 临时 Go overlay 映射 `4911f30` 的 selector，重新证明两阶段普通 scalar、历史位置、`output_config: {}`、fold/cascade full-strip 失败，均返回 replacement Body 而未 Terminate；`red-rpc-literal-overlay.log` 和 `red-rpc-overlay.log` 均退出 1。工作树没有回滚。
- 独立 oracle 的 base-helper overlay 对普通 system scalar 的 canonical 拒绝和 forged accepted-empty 均失败，退出 1。接管时 oracle 对带 effort 的伪造拒绝和普通 scalar 的伪造成功空输出也失败，退出 1；修正后 `green-rpc-oracle.log`、`focused-final.log` 均退出 0。semantic/raw span selector 保持不变；相消由 system functional fixtures 和既有 `TestOracleApplyTracksFinalRewriteState` 核查。
- 生产 diff 仅为 `collectClaude` 的一行 role 条件。两阶段 RPC 覆盖 literal/fold/cascade、partial/obfs/block 优先级/no-match/相消、roles/formats/rules/filter/auth gates、已有空值/未知 shapes，以及 effort 字段存在时的保留行为（包括 null、空 string 和非 string）。typed text、顶层 system、最终 assistant、tool/MCP scalar 保持既有边界。没有增加配置、export、依赖、模型或 effort 枚举验证；`C.GoBytes` 不变。
- 当前发行文档更新后，旧 v0.3.10 current-release guard 曾令 `make test` 退出 2，记录于 `make-test-final.log`。更新该 guard 至 v0.3.11、将 v0.3.10 兼容句保留为历史断言并补齐当前 glibc 说明后，focused 文档测试及完整 root suite 均退出 0。

### 完整验收

全部命令、cwd、实际退出状态及耗时保留在 `.local-verification/claude-system-scalar/commands.jsonl` 和对应 `.log` 中。

| 检查 | 实际结果 |
|---|---|
| `make test`、`make race`、`make vet` | 均退出 0；发行段修正后复跑 root suite、race、vet |
| Windows packager 的 main source、Windows helper、通用及 Windows tests | 独立 `go test -count=1`、`go test -race -count=1`、`go vet` 均退出 0 |
| 六个现有 fuzz 目标 | 每个使用 `-fuzztime=3000x -parallel=2` 并退出 0；FoldKMP 实际 3011 次，其余各 3000 次 |
| `make integration` | 两次完整流程均退出 0；最终 `make-integration-final.log` 包括 native 注册、active-after、HTTP/SSE/WS 和新两阶段 native/HTTP full-strip 矩阵 |
| `make build`、`make build VERSION=v0.3.11` | 均退出 0 |
| selected-platform 与 aggregate `make package VERSION=v0.3.11` | 均退出 0；两种 ZIP 和其中 DLL 逐字节相同 |
| ZIP CRC、成员、LICENSE、时间、权限及 checksum | 根目录仅 DLL 和 LICENSE；CRC 通过；LICENSE 字节一致；两项时间均为 1980-01-01 00:00:00；DLL 为 0755；lowercase sidecar 与 aggregate 精确一致 |
| 正式 DLL native 验收 | ZIP 提取 DLL、dist DLL、runner staged DLL 和 loader shadow DLL 字节相同；SDK loader/RPC 实际断言 Metadata.Version 为 0.3.11、schema 5、ABI 1、七个 ConfigFields、Object words、request-only、active-after；原有 `TestDynamicABIActiveAfter` 同样通过 |

首次正式包验收脚本退出 1，原因是额外假定 LICENSE 为 0644。核对既有 packager 后确认 LICENSE 保留 Windows 源文件权限 0666；验收只对 DLL 要求 0755，随后完整包验收退出 0。该过程没有修改 packager。

正式 ZIP 为 `dist/censorship_0.3.11_windows_amd64.zip`，SHA-256 为 `99b894b595c35456b9a29249e897361b84a91045e751f381a11e6fbdd22b62c4`；正式 DLL 为 `dist/windows_amd64/censorship.dll`，SHA-256 为 `9281c3b98dcc9940aa68ee3b6f4fbbef2442d21328acd83213fb7bcbb39fcf39`。完整来源记录为 `.local-verification/claude-system-scalar/formal-package.json`，包含验收时 sourceHead 和源码 SHA-256。DLL 的 nested-worktree VCS metadata 为父仓库 revision `4911f30`、module v0.3.10，注入版本已由真实注册独立核查；没有把 metadata 当作编译源码来源的替代证据。

### 交付与清理

按规格逐项核对 diff、调用者、配置和 ABI 边界，未留下无条件 system 拒绝或其他撤销实现。OS 临时 overlay 已通过临时目录机制清理；本地命令日志、复核脚本、正式 ZIP 提取 DLL 及生成的 integration/dist 产物保留供主会话独立复核，因此保留精确 `.gitignore` 规则。没有手工修改生成的 `.integration` 或 `dist` 内容。

本地验收不包含商业 API、远端 CI、七平台发行资产或其他平台 native 加载；未 push、tag 或合并 main。本 worktree 和分支保留，等待主会话独立审查及后续合并。
