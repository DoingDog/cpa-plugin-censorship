# Claude 消息内 system scalar 非空改写约束

日期：2026-10-23。目标补丁版本：v0.3.11。基线：`4911f30bc17bfc2b3261c578074bcc78cbefa5ae`。

## 已确认问题与官方依据

`collectClaude` 已选择 `messages[*].role: "system"` 的 string `content`，但 scalar 分支仅对 user 和实际非末尾 assistant 调用 `appendNonEmptyStringSpan`。无 per-message `output_config.effort` 的普通 system scalar 经完整 strip 后返回空 string 的 replacement body，未触发现有本地 invalid-text 响应。本 worktree 的首个两阶段 RPC regression 已观察到 `Terminate: false`、replacement Body 和 ClearHeaders；HTTP 结果须由本 worktree 的激活插件集成测试自行验证。

2026-10-23 阅读以下官方章节：

- [Create a Message 的 Body parameters > messages](https://platform.claude.com/docs/en/api/messages/create.md)：说明原文为 “Using a `string` for `content` is shorthand for an array of one content block of type `"text"`.” 同一 messages union 的 `SystemMessageParam` 明确列出 `content: string or array of TextBlockParam`、`role: "system"`，其嵌套 `TextBlockParam.text` 为 string 且 `minLength: 1`。
- [Mid-conversation system messages 的 How it works](https://platform.claude.com/docs/en/build-with-claude/mid-conversation-system-messages#how-it-works)：消息内 system 的 content 使用 plain string 或 content blocks，与 user/assistant 同形。携带文本的 system 消息紧随 user，位于末尾或紧接 assistant 之前；测试应使用符合此规则的位置。
- [Effort 的 Per-message effort (beta)](https://platform.claude.com/docs/en/build-with-claude/effort#per-message-effort-beta)：合法 effort-only system 使用 `content: []` 和 `output_config.effort`。Claude API 支持列表包括 Claude Opus 5.5，要求 `mid-conversation-output-config-2026-07-01` beta。测试使用该模型和 beta 形状，不调用商业 API。

上述 shorthand 约束针对 messages 中的 string content。顶层 `system` 是独立参数，本次不认定顶层 `system: ""` 非法。effort-only `[]` 没有文本 span。官方 How it works 同时概括了 empty content 仅设置 effort 的例外；带 `output_config.effort` 的空 scalar 是否按该例外正规化尚未核实，因此本次保持该 scalar 的既有行为，不宣称供应商接受性。仅有 `output_config: {}` 不属于 effort 字段存在。

## 实现范围与调用链

最小生产变更是在 `selectors_claude.go` 的 scalar role 条件中加入无 `output_config.effort` 字段的 system，使用现有 gjson 字段存在判断，复用已有 helper 和最终验证。不增加配置、依赖、抽象、parser、重试或 fallback。

已核查的关联位置：

1. `selectors.go`：`collectTextSpans` 调用 `collectClaude`；`appendNonEmptyStringSpan` 复用 `appendStringSpan` 并标记 `RequiresNonEmpty`。所有 helper caller 还包括 Claude 顶层 typed system、message typed text、tool/MCP typed text 和 document/search-result typed text。
2. `transform.go`：`transformRequest` -> `transformSelectedSpans` -> `applyMode` -> `changedSpanInvalidMessage`。block 先返回；仅最终 `Changed` 的 span 进行非空验证；取消回原文返回 no-op。
3. `main.go`：BeforeAuth/selected-auth AfterAuth 的现有 gates -> `interceptRequest` -> `terminatedRequest`。非法改写复用 canonical HTTP 400，`error.code: censorship_invalid_request`，`error.message: censorship rewrite would make a text field invalid`。
4. `functional_corrections_test.go` 和 `selectors_claude_test.go`：公开 RPC helpers、两阶段矩阵、非空及允许空的现有边界。
5. `fuzz_test.go`：独立 `oracleClaudeRequiredFieldWouldBeEmpty` 须识别 system scalar；semantic/raw selector 已选择该路径，不新增约束字段或重复实现。
6. `integration/http_test.go`、现有 native registration helpers、`.github/scripts/integration-runner.go`：从本 worktree 当前源码构建 DLL，经固定 CPA 运行本地 HTTP 与 native 两阶段检查。仅编辑源 fixture，生成文件由 runner 写入。
7. `README.md`、`RELEASE_NOTES.md`：英语说明和当前版本 v0.3.11；`main.go` 的 bare `pluginVersion` 保持 `0.0.0-dev`。

失败响应不包含 replacement Body、request Headers、ClearHeaders 或部分改写；ResponseHeaders 仅沿用 canonical JSON 响应。原请求字节、metadata、headers 不变。本地 HTTP 完整 strip 必须零 upstream。

## 非范围与保持行为

- 带 per-message `output_config.effort` 的 system scalar、顶层 system scalar、最终 assistant scalar prefill、tool_result/mcp_tool_result scalar 允许完整 strip 到空的既有行为不变。不新增模型、effort 枚举或非法输入验证。
- Claude typed text、user scalar、非末尾 assistant scalar、document title/context 及 title 上限保留现有规则。
- 原本为空的 scalar 不新增验证；空数组、effort-only `[]`、未选 role、关闭 format、过滤 bypass、未知 role/content/format 保持 no-op。
- block 优先级、partial strip、obfs、literal/fold 语义、按顺序 cascade、最终相消与机器字段字节保持不变。
- CLIProxyAPI core、ABI v1、配置 schema、`C.GoBytes` 的同步请求复制保持不变。不做安全扫描或系统修复。

## 测试边界与完成条件

1. 一个公开 RPC 完整 strip regression 先真实失败，再以单一 role 条件修复通过；保存 RED/GREEN 命令、退出状态和输出。
2. 两阶段检查无 effort 的普通 scalar 的 literal、fold、cascade 清空与 `output_config: {}`；带 effort scalar 保留完整 strip 到空的对照；block 优先于 strip；partial/obfs/no-match/取消；disabled rules/roles/formats、过滤和阶段 gates。复用现有矩阵及响应 helper。若将 system fixture 加入全局矩阵，其 `role_disabled` 控制必须确实关闭 system。
3. 独立 oracle 的无 effort system scalar 正反例先 RED 后 GREEN，并用伪造 accepted 空输出的负对照强制检查应拒绝结果；验证合法空数组、已有空输入、partial/no-match/obfs、顶层 system 和错误结果不会被误接受。保留既有 fuzz false-positive checks。
4. 固定 CPA commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8` 和当前源码 DLL 完成 native 注册、两阶段、`TestDynamicABIActiveAfter`、全部 HTTP/SSE/WS。新增普通 system scalar HTTP regression 证明 canonical 400 与零 upstream；带 effort scalar 和合法 effort-only 等对照转发到本地 upstream，按既有 CPA `ensureCacheControl` 正规化后的实际结构断言。
5. `make test`、`make race`、`make vet`，独立 Windows packager tests/race/vet，六个有界 fuzz 目标，`make integration`、`make build`，两种 `make package VERSION=v0.3.11` 均通过。检查 ZIP CRC、成员、LICENSE、注入版本、sidecar 和 aggregate；加载 ZIP 中同一正式 DLL，真实断言注册 `Metadata.Version == "0.3.11"`、Object words、七个字段、schema 5、ABI v1 和 active-after。
6. 逐项规格自审及 diff 检查通过，相关修改提交到本 worktree 分支。提交消息英语；不 push、tag 或合并 main。

## 主会话后续验收

本子任务完成后，主会话独立审查实际 diff 和证据，有效问题由本 worktree 修正、复测、提交。确认无修改冲突后合并。最终 main SHA 的 Linux/Windows 必需 CI 全部通过后才能创建 v0.3.11 tag；tag CI 通过后验收七个平台、15 个资产：七个 ZIP、七个 lowercase SHA-256 sidecar 和 aggregate `checksums.txt`。逐一核对 ZIP/CRC/LICENSE、版本注入、平台、`c-shared`、固定依赖和每个共享库 `vcs.revision` 与最终 main/tag SHA。工作流成功或版本字符串不能替代这些核对。

商业 API 与其他平台 native 实际加载不在本机验收范围。nested worktree 的 Go VCS metadata 按[来源核查说明](../../solutions/workflow-issues/go-worktree-build-provenance.md)区分源码与 metadata；本地 metadata 的已知限制不放宽远端验收。合并后确认无未提交内容和未合并提交，再由主会话退出并清理本次 worktree 及分支；不得删除用户已有产物或其他 worktree。
