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
- 仅在 scalar role 条件加入 system，复用 `appendNonEmptyStringSpan`，不改其他 collector/transform/RPC/ABI 实现。
- 运行相同命令取得 GREEN，再运行 Claude 现有相关测试。

验收：canonical 400、无 replacement Body/Headers/ClearHeaders、原请求不变；生产 diff 只有必要 role 条件。`C.GoBytes` 完整保留。

## 3. 两阶段行为与独立 oracle

文件：`functional_corrections_test.go`、`fuzz_test.go`；必要时局部复用 `selectors_claude_test.go`。

- 复用已有表驱动 helpers，按实际风险补 ordinary 与 effort scalar 的 literal/fold/cascade 清空，block 优先级、partial strip、obfs、no-match、最终相消。
- 验证 roles/formats/rules 关闭、filter bypass、BeforeAuth/AfterAuth gating、未选或未知 shapes；原始空 scalar 与 effort-only `[]` 不新增验证。
- 覆盖顶层 system、最终 assistant、tool_result/mcp_tool_result scalar 保持行为，typed/user/history/title 复用现有回归。
- 避免扩大现有全局 functional fixtures；若复用全局矩阵加入 system fixture，则同步修正用 `[system]` 的 `role_disabled` 控制。
- 先向 `TestProtocolOracleAcceptsOnlyClaudeRequiredEmptyStrip` 加入 system scalar、fold、effort 正例及 false-positive 反例，运行 RED；再精确修改 `oracleClaudeRequiredFieldWouldBeEmpty` 的 scalar 判断，运行 GREEN。
- semantic/raw selector 已支持该路径，保留其独立实现和原有约束结构。

验收：两阶段和 oracle focused 命令通过；保留独立 RED/GREEN 记录，错误模式、partial、no-match、原始空输入、顶层 system 与 effort-only 不误判。

## 4. 固定 CPA 与 native/HTTP 集成

文件：`integration/http_test.go`，复用 `.github/scripts/testdata/abi_benchmark_test.go` 提供的 native host helper，不手改 `.integration` 生成文件。

- 增加当前源码 DLL native 注册与 BeforeAuth/selected-auth AfterAuth 的 ordinary/effort scalar full-strip rejection，partial 和合法 effort-only/top-level 对照。
- HTTP fixture 使用 `claude-opus-5-5` 和合法 user -> system 位置，effort 请求携带 `mid-conversation-output-config-2026-07-01`。通过既有 provider mapping 把本地别名映射到该模型。
- 完整 strip HTTP 必须 canonical 400、JSON invalid-text 响应且 upstream arrival/request 均为零。partial/effort-only/top-level 等合法对照检查实际转发内容和机器字段；考虑 CPA `ensureCacheControl` 正规化，不错误要求 wire body 与入站逐字节相同。
- runner 自行获取/构建固定 CPA 和固定 model mapper，从当前 worktree 源码构建 DLL、复制源 integration fixtures、运行全部 HTTP/SSE/WS/native。

验收：`make integration` 完整通过，保留当前源码路径/工具链/构建产物证据；`TestDynamicABIActiveAfter` 通过。禁止用主目录旧 DLL 证明本次行为。

## 5. 英语说明与 v0.3.11

文件：`README.md`、`RELEASE_NOTES.md`。

- 当前兼容版本及当前命令示例使用 v0.3.11，固定 CPA、schema 5、ABI v1 不变。
- 说明消息内 system scalar 的 Changed-final empty 约束及 effort-only `[]`、顶层 system 边界。
- RELEASE_NOTES prepend 2026-10-23 的最小 fix section，保留旧历史和无关 pending 原文。
- bare `pluginVersion` 保持 `0.0.0-dev`。仅在真实命令通过后描述本机已验证范围；不宣称商业 API、CI、其他平台已验收。

验收：英语文案与实现、实际结果相符；历史段落无无关改写。

## 6. 完整本地验收与正式包

- 运行 `make test`、`make race`、`make vet`。
- 独立运行 Windows packager 的 main source、Windows helper、通用及 Windows tests，分别 test/race/vet。
- 对现有六个 fuzz 目标分别运行有界样本：`FuzzRuleEngineAgainstOracle`、`FuzzFoldKMPAgainstOracle`、`FuzzDuplicateWalkerAgainstOracle`、`FuzzProtocolTransform`、`FuzzRebuildBodyAgainstMarshalOracle`、`FuzzByteMatcherAgainstOrderedContains`。仅功能回归，不做安全或 DoS 测试。
- 运行 `make integration` 完整流程，再运行 `make build VERSION=v0.3.11`。
- 运行 selected-platform `make package GOOS=windows GOARCH=amd64 VERSION=v0.3.11` 和 aggregate `make package VERSION=v0.3.11`；检查 ZIP CRC、精确成员（DLL、LICENSE）、LICENSE 字节、注入版本、lowercase SHA-256 sidecar、aggregate 内容和 DLL 一致性。
- 用正式 `dist/windows_amd64/censorship.dll` 运行 runner `-abi-smoke`，验证 active-after 与注册版本。记录源文件 SHA-256、Go build info 和 worktree，按[provenance 说明](../../solutions/workflow-issues/go-worktree-build-provenance.md)区分源码来源和 nested worktree VCS metadata。
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
