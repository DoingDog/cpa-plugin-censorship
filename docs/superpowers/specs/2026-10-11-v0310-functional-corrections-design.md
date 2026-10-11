# v0.3.10 功能修正设计

## 目标与完成条件

让 OpenAI Chat 请求中被 pinned CPA Chat->Codex 消费的 flat custom tool `description` 按已有 developer scope 执行 block、strip、obfs。同步独立协议 oracle、公开 seeds、RPC、真实 native 注册和 Chat->Codex HTTP 验证，修正两项已复现的 fuzz 测试断言错误。后两项的生产行为已经正确，修改仅限测试。

基线为 `2bbe8bbf75934437efd6ff2d570523ef9e0d830d`。保持 `github.com/router-for-me/CLIProxyAPI/v7 v7.2.152`、host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`、配置 schema 5、native ABI v1。文档文件名沿用任务开始日期 2026-10-11；发行说明使用实际发布日期。

每项行为修改须有预期断言失败的 RED、修复后的 GREEN、相关回归、独立规格和质量审查、必要修正、复审及提交。最终实现树重新执行完整测试、六项 bounded fuzz、native/HTTP integration、build/package；合并后的真实 main SHA 和 v0.3.10 tag SHA 分别通过新 CI，并验收七个平台资产。基线结果和旧 CI 不计为新版本验收。

## 证据与修改范围

| 已确认项目 | 当前根因或已观察结果 | 修改范围 |
|---|---|---|
| Chat flat custom description 漏选 | `appendOpenAIChatTool` 的 custom 分支只有 `custom.description`；正常 flat description 的 block、strip、obfs 已独立复现漏选 | 唯一生产修改位于 `selectors_openai.go`；同步相关测试、集成和 README |
| Claude/Gemini definitions oracle 漏选 | 18 个正常 definitions fixtures 的生产 block 正确；checker 返回 `request without eligible match was blocked` | `fuzz_test.go` 的两套 Claude/Gemini oracle、focused assertions 和 seeds |
| Claude title 上限 checker 错报 | 原 title 500 code points，obfs 后 501；生产正确 Invalid，checker 返回 `valid JSON object marked invalid`；499 -> 500 对照正确通过 | `fuzz_test.go` 的 `checkProtocolResult`、独立 title 判定、focused assertions 和 seeds |

已完成审计报告覆盖基线 94 个受追踪文件，未发现其他需要本次实施的生产缺陷。上述报告用于确认范围；本文档阶段以相关源码核对机制，没有重新执行审计实验。临时实验修正已经恢复，基线仍带两项测试断言错误。

直接 consumer 证据是 pinned CPA 的 `internal/translator/codex/openai/chat-completions/codex_openai_request.go` 中 `ConvertOpenAIRequestToCodex`：custom tool 复制完整 `t.Raw`，只处理顶层 `name` 的 shortening，再写入 Responses `tools`。顶层 `description` 因而进入 Codex upstream 请求；nested `custom` 是否存在不决定该 passthrough。本设计限于正常 flat custom 输入，不将 converter 的非字符串 coercion 扩大成插件自然语言选择规则。只读核对这一相关 consumer，不修改 CPA。

## 保持不变的行为

- `words` 保持 strict Object 和有序 `block`、`strip`、`obfs` arrays，原词条不做 trim、改写或重排。legacy sequence/global `mode` 仍只接受 handwritten YAML；panel 只渲染和输出 Object `words`。
- 动作固定为 `block` -> `strip` -> `obfs`。block 按 rule-major 顺序，同一规则使用原请求 token 顺序中最早命中的 canonical role。保留 no cross-node match、leftmost nonoverlapping、once-only cascade 和取消语义。
- `filter.models` 为空时 BeforeAuth 处理、AfterAuth no-op；非空时 BeforeAuth 在 envelope/body parsing 前 no-op，仅 selected-auth AfterAuth 处理。selected auth 沿用非空、非空白字符串 `selected_auth_id` 或 `selected_auth_index`。
- model filter 只使用 `request.Model`；`RequestedModel`、body model、metadata source model、headers 和 URL 均不作为 fallback。filter、metadata 和 request 输入不被修改。
- 无命中、scope/format/filter bypass 或最终改写取消时，插件原始响应不返回 replacement body、不清除 entity headers。真正 rewrite 仍清除 `Content-Encoding`、`Content-Length`、`Transfer-Encoding`。
- 保留 signature-bound、annotations、非空和 code-point 约束、Responses dormant `NestedDescription` 激活及非字符串备用描述拒绝，不修改 matcher、shared transform 或 parser。
- `cliproxyPluginCall` 保留 `C.GoBytes`，保持 ABI ownership、pointer lifetimes、注册字段和 capabilities。不手改 `.integration/` 或 `dist/` 生成文件。
- 只选择明确自然语言字段。未选的名称、IDs、grammar、schema machine values、metadata、signature、URL、binary/media、未知机器字段及请求空白、成员顺序、数字表示保持原始 bytes。只按已有规则重新编码确实改变的 selected token。

## 1. Chat flat custom description

### R1. 精确选择条件

在现有 `appendOpenAIChatTool` custom 分支增加顶层 `description`。仅当 tool 为 Object、`type` 是字符串 `custom`、顶层 `name` 为非空字符串且 developer role 已启用时，选择顶层非空字符串 `description`，canonical role 为 `developer`。名称仅用于判断正常 flat 形态，不参与匹配；不新增名称长度、trim 或格式校验。缺失、空、`null` 或非字符串 description 不生成 span。

已有 `custom.description` 的 nested 选择原样保留，不增加顶层 name gate。flat 和 nested 独立选择：`custom` 缺失、`{}`、`null` 或 nested description 安全，均不能遮蔽合格 flat description。两处都合格时都参与已有匹配，并由原始 token 位置确定顺序；不新增 fallback、precedence 或 dormant activation。flat name 不合格时，只排除新增 flat 路径，既有 nested 路径仍按原行为处理。

Chat function、legacy `functions`、response format、Responses tools 和其他 provider 的生产选择范围不变。尤其不把 Responses 的 flat/nested description fallback 引入 Chat custom。

### R2. 三动作与原始 bytes

最小正常请求为：

```json
{"model":"target-model","messages":[{"role":"user","content":"safe"}],"tools":[{"type":"custom","name":"safe","description":"SECRET tail","format":{"type":"grammar","syntax":"lark","definition":"start: \"SECRET\""}}]}
```

默认或显式 developer scope、词条 `SECRET` 时：

- block 返回本地 HTTP 400，code 为 `censorship_blocked`，term 为 `SECRET`，role 为 `developer`。插件原始响应无 replacement body；实际 HTTP fixture 的 upstream arrival 为零。
- strip 的解码 description 为 ` tail`。只有该 selected raw token 被替换，`name`、grammar 和其余 request bytes 原样保留。
- 默认 obfs 的解码 description 为 `S​ECRET tail`，其中 `​` 表示实际 U+200B。输出仍为合法 JSON，机器字段不变。

完整 strip 允许 description 变成空字符串；Chat custom 不新增非空约束。exact/folded、普通 Unicode、escaped Unicode、surrogate 后续字符均使用已有解码和 matcher。fixtures 须包含机器字段也有 `SECRET` 的情况，证明它们不触发 block、不被 rewrite。

### R3. 阶段和负向对照

selector、transform、RPC 和真实 native 矩阵覆盖 block/strip/obfs、完整 strip、无命中、strip -> obfs cascade、最终取消、rule-major 和最早 token role。default developer、developer-only、developer disabled、format disabled、filter miss、model-bearing BeforeAuth、model-free AfterAuth、unselected AfterAuth 均须明确断言。

负向形态包含缺失/空/`null`/非字符串 flat name，缺失/空/`null`/Object/数字 description，非 custom type、非 Object tool，以及 machine-only 命中。每项排除只针对 flat 路径；单独命中的已有 nested description 仍使用原规则。mixed fixture 包含 flat 与 nested 不同文本、两处都命中、不同原始成员位置和安全 nested。

raw RPC 的 block/invalid 断言与 host 聚合后的 native 断言分开：host no-op 返回原 Body/Headers，host block 保留原请求 Body/Headers 并携带终止响应，host rewrite 已应用 ClearHeaders。禁止把聚合结果当作插件原始 RPC 输出。

## 2. 独立协议 oracle 与公开 seeds

### R4. Chat oracle 同步

`fuzz_test.go` 的 `oracleAppendOpenAIChatTool` 和 `oracleRawAppendOpenAIChatTool` 分别同步 R1。前者继续使用 `encoding/json`/`json.RawMessage`，后者继续使用已有 `oracleRawParser`；各自解析 type、name、description 和 nested custom。不得调用生产 selector，不共享生产路径决策函数，不新增 parser。

直接 caller 为 semantic `oracleOpenAISpans` 和 raw `oracleRawOpenAISpans`；`oracleExcludedTokens` 也使用 semantic helper 判断 tool 是否整体 excluded。同步后检查 full-strip 空 description、mixed tool 和 machine-only no-op，确保不会将已有 selected 字段误标为 excluded，或失去未选 bytes 的断言。

公开 seeds 覆盖正常 flat、nested-only、mixed、空/非字符串 gate、grammar/name/ID machine-only、escaped token、full strip。每个正常 seed 使用现有三模式 × exact/folded 扩展；focused tests 用独立 expected literals 断言两套 span 的文本、canonical role、raw token 范围和 transform 结果。两套 oracle 互相一致仅是附加检查，不能取代独立 literal expectations。

### R5. Claude/Gemini definitions 测试断言修正

生产 `collectClaude`、`collectGemini` 及 shared schema traversal 保持不变。仅补 `oracleClaudeSpans`、`oracleRawClaudeSpans`、`oracleGeminiSpans`、`oracleRawGeminiSpans` 的已支持入口：

| 格式 | 现有生产入口与 gate | canonical role |
|---|---|---|
| Claude tools | `tools[]` 为 Object，`name` 为字符串，`input_schema` 为 Object；选择 tool `description` 与 `input_schema` 的已支持 schema descriptions | `system` |
| Claude output_config | `output_config.format.type` 为字符串 `json_schema`；选择 `schema` 的 descriptions | `system` |
| Claude legacy output | `output_format.type=json_schema`；选择 `schema` 的 descriptions | `system` |
| Gemini declarations | Object `tools[]` 下 `functionDeclarations[]` 和 `function_declarations[]` 的 Object declaration `description` | `developer` |
| Gemini declaration schemas | 每个 declaration 的 `parameters`、`parametersJsonSchema`、`parameters_json_schema`、`response`、`responseJsonSchema`、`response_json_schema` | `developer` |

各套 oracle 复用自己的 `oracleAppendJSONSchemaDescriptions` 或 `oracleRawAppendJSONSchemaDescriptions`。沿用既有 keyword whitelist；不扩大 schema root、aliases、roles 或机器字段。Claude name gate 是字符串类型，不新增非空条件；Gemini 六项 schema 都独立读取，不改成 first-present fallback。

先将四个 Claude fixtures、两种 Gemini declaration key × 七项 description/schema fixtures 共 18 个已复现输入写为永久 focused assertions 和公开 seeds。测试先证明生产 block 的 term/role 正确，checker 却拒绝它；补 oracle 后 checker 正确接受。随后覆盖 strip/obfs、safe messages、非字符串/空值、不合格 tool/declaration 和 machine-only/schema unknown keyword 对照。名称、`enum`、`default`、`const`、`required`、schema keys 和未知字段的 bytes 必须保留。

### R6. Claude document.title 超限测试断言修正

`selectors_claude.go` 和 `transform.go` 保持不变。仅修正 `checkProtocolResult` 对正确 Invalid 的接受条件，保留既有完整 strip 导致非空字段为空的分支。

新增独立判定只检查当前 default user scope 中明确的 `messages[role=user].content[type=document].title` 非空字符串。用独立 oracle 的 strip/obfs 和当前 fold 计算最终解码 title；只有文本确实改变且最终 `utf8.RuneCountInString` 大于 500 时，才允许 title 超限拒绝。计数不能使用原字符串长度、JSON bytes 或 escape 长度。

可接受的结果还必须同时满足：format 为 `claude`，selected mode 为 strip 或 obfs，`Invalid=true`，`Blocked=nil`，无 replacement body，message 精确为 `censorship rewrite would make a text field invalid`。block 模式不会接受此类 Invalid。存在其他 selected 字段命中不能代替 title 本身改变的证据。

永久公开边界 fixture 为 `x` 重复 493 次加 `SECRET`，499 -> 500；以及重复 494 次加 `SECRET`，500 -> 501。默认 obfs 插入一个 U+200B，第一项得到 rewrite，第二项得到上述 Invalid。补充中文、emoji、escaped Unicode，以及 strip 改写后仍超限和降至合法长度的 focused 对照。

checker 必须拒绝伪造 Invalid：不匹配或未改变的超长 title、仅其他字段超过 500、`search_result.title`、其他 provider、default-disabled assistant/tool、错误 message、同时 Blocked、携带 replacement body，以及合法 500 边界的错误拒绝。已有 Claude empty-strip、Gemini signature-bound、Responses dormant/non-string、Interactions annotations 和 outside-span byte mutation 断言保持。

### R7. fuzz 覆盖与边界

保留当前 `FuzzProtocolTransform` 的 generator、默认 `system/developer/user` 角色、固定 `SECRET`、三模式、fold 参数和已有输入上限：format 长度 32、body 64 KiB、JSON nesting 256。assistant/tool 和 Responses `instructions` developer fallback 是当前生成覆盖限制，不扩展 fuzz generator 或生产语义；运行已有确定性 canonical-role、default-disabled 与 `TestOpenAIResponsesInstructionsScope` 回归。

六个现有 fuzz target 都须在最终树实际执行有界运行：`FuzzRuleEngineAgainstOracle`、`FuzzFoldKMPAgainstOracle`、`FuzzDuplicateWalkerAgainstOracle`、`FuzzProtocolTransform`、`FuzzRebuildBodyAgainstMarshalOracle`、`FuzzByteMatcherAgainstOrderedContains`。每项记录预算、命令、实际耗时、退出状态和失败输入；短预算通过不等于覆盖全部角色、字段形态或商业 API。不得忽略、删掉或放宽新失败来制造 PASS；先判定实现、oracle 或输入边界，并仅在本范围内修正。

## 3. 真实注册与 HTTP consumer 验证

### R8. registered native 与实际 Chat->Codex

复用 `functional_corrections_test.go` 的 request/RPC helpers，及 `integration/http_test.go` 的 `withDynamicABIHost`、`newFunctionalDirectUpstream`、`startFunctionalDirectCPA`、`runFunctionalHTTPMatrix`。给 native registration/two-phase fixture 集合加入 flat custom；验证实际动态库加载、ApplyConfig、注册 metadata、七项 ConfigFields 和两阶段输出，不注入 mock selector。

新增实际 HTTP fixture 使用 provider `codex` 和 `/v1/chat/completions`，由 pinned CPA 转换后发送到本地 `/responses` upstream。描述输入在 Chat 层，捕获断言在 Codex Responses `tools[].description`。使用无需 shortening 的正常短名称与有效 grammar，检查转换后的 name/format 和已保留机器字段。block 零 upstream arrivals；strip/obfs 恰好一次到达且接收正确 description；无命中、developer disabled、filter bypass、取消保持文本。两 auth 阶段的阶段 gate 用 native/RPC 矩阵证明，不以单一 HTTP route 猜测 callback 次数。

插件 rewrite 的原始 bytes 保留由 selector/RPC/native 测试证明；CPA converter 允许自身正常结构转换，HTTP 不要求转换后整个请求与原 Chat bytes 相同。fixture 同时检查 arrivalCount 和 requestCount，防止 upstream body 读取失败掩盖到达。完整 `make integration` 必须再运行；不手改 runner 生成的 CPA checkout，不并行重建同一个 `.integration/run`。

集成构建未注入 release VERSION，native registration 期望 `0.0.0-dev`；正式 build/package 注入 `0.3.10`。本地 upstream 仅验证真实 pinned CPA、插件和 consumer 路径，不代表商业 Codex API acceptance。

## 4. 文档、构建与发行

### R9. 版本文档与兼容性

实施时只对 README 必要字段说明加入 Chat flat custom description、developer gate 和 mixed/nested 边界，保留既有功能和纯插件限制。RELEASE_NOTES 新增 v0.3.10，区分生产 flat custom 修复与测试 oracle/checker 修正，保留历史 sections 原文。`TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions` 先更新当前版本预期、观察旧文档 RED，再更新文档 GREEN；历史兼容断言继续有效。

不新增第二个版本来源。沿用 Makefile `VERSION=v0.3.10` 去除首个小写 `v`，用 `-X main.pluginVersion` 注入 `0.3.10`。非 tag CI 保持 `0.0.0-dev`，tag CI 使用 `0.3.10`；未完成的 CI 或资产检查不得写成已通过。

### R10. 新树验收与发布完成条件

实现 owner 在提交后最终树完成 focused tests、公开 seeds、`make test/race/vet/build/integration/package`、packager 独立平台 file-list test/race/vet、六项 bounded fuzz、native ABI smoke 和产物检查。无新性能修改，不增加 benchmark 或性能保证。只提交必要文件，独立规格/质量审查覆盖完整 task `BASE..HEAD`；问题在同一 worktree 修正、重测、复审、新提交，禁止留下未完成步骤给主会话。

发布 owner 只整合已经完成、范围无冲突的 commits，合并本地 main 后核对真实 SHA，推送并等待该 SHA 的新 Build。确认 Ubuntu 根测试、race、vet、integration/.so 注册、packager file-list race/vet，以及 Windows 原生回归和 active-after DLL smoke 实际执行成功；全部 test 与七个平台构建成功后才创建并推送 v0.3.10。随后核查对应 tag SHA 的新 test、七个平台构建及 release job。CI watch/workflow 采用完成后主动通知，不主动轮询；真实失败由原任务 owner 修正并重新按新 SHA 验收。

七个平台为 linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64、windows/arm64、freebsd/amd64。下载各 ZIP、对应 lowercase SHA-256 sidecar 和 aggregate `checksums.txt`，验证精确文件集合、hash、ZIP integrity/CRC、库平台、build info、注入版本和非 draft 发布状态。aggregate 必须恰有七条正确记录，公开 assets 不因重试覆盖。

依据 `docs/solutions/workflow-issues/go-worktree-build-provenance.md`，源码输入、VCS metadata 和 ABI version 注入分别核查。本地 nested linked worktree 的 `vcs.revision` 不能单独证明编译源码；记录 HEAD/blobs、构建工作目录/输入、toolchain、库 hash 和 metadata。main/tag 七库的 `vcs.revision` 必须匹配各自实际 SHA，`vcs.modified=false`；缺失或不同保持未通过，不能用正确 metadata version 代替源码证明，不修改 Git metadata。

## 范围边界

本轮不修改 CPA core、pin、schema、ABI、配置/panel、matcher、shared transform、其他 production selector 路径或 schema whitelist，不做性能优化、额外 parser、通用递归字符串扫描或推测性支持。

本阶段交付仅此 spec 与对应实施 plan，不修改生产、测试、README、RELEASE_NOTES，不 push、不改 main、不创建 PR。实施和发布由后续 workflow 依据计划完成。商业 provider API 和七个平台全部 native 实际加载不在本地证据范围；发布按实际 CI/资产结果声明覆盖，不把跨平台构建等同于全部平台运行。
