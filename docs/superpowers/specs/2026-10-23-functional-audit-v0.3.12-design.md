# v0.3.12 功能修复设计

日期：2026-10-23。基线：`main@23fe07f1c4ba2ec23879ec2addc8015b4d7acc6d`（v0.3.11）。目标：v0.3.12。

## 范围与依据

本规格定义两组已独立复现、可在插件内完成的修复：A，首次注册失败后的合法配置恢复及尚无有效 snapshot 时的错误响应；B，Gemini `generation_config` 下既有 schema description 路径的漏选。另记录跨格式 full-strip 产生 Claude 空 typed text 的已确认兼容限制，不将其列为开发任务。对应责任与执行步骤见[实施计划](../plans/2026-10-23-functional-audit-v0.3.12.md)。

首轮九范围扫描已全部回报，`whnjtk2x2.output` 的 `missing` 为空。定向独立核验见 `wrd6p10rx.output`（A 的 native/watcher）、`wedd9lt46.output`（A 的相邻错误响应与跨格式限制）、`w3vu1uq8r.output` 前两份报告（B 的官方 schema 与 native/HTTP）。完整性复核已完成，`wzqrgzwnz.output` 确认 `coverage_complete=true`、`additional_confirmed_fixable_issues=[]`；全部受控实现、测试、构建、依赖及配置均有全文覆盖依据，补读 `go.sum` 后 `go mod verify` 通过，未发现第三组已确认可安全执行的插件修复。文档阶段不重复全文代码审计或已完成的大基线。

固定 CLIProxyAPI v7.2.152、commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`、schema 5、native ABI v1，版本依据为 [go.mod](../../../go.mod) 与 [integration runner](../../../.github/scripts/integration-runner.go)。仅修改 censorship plugin，不修改 CPA core，不执行网络安全扫描，不做无关重构。

## A. 生命周期

### 已确认缺陷

两次独立验证从当前源码构建 Windows amd64 DLL，使用固定 `Host.ApplyConfig` 复现了以下调用链；其中一次同时运行真实 CPA 配置 watcher 与 HTTP 请求。

1. [config.go 的 `init`](../../../config.go) 预先存入默认空 snapshot，其中 `Rules` 与 `Filter.Models` 均为空。
2. 首次 `plugin.register` 因 `words.block` 含空词失败，未安装任何成功解析的配置。固定 host 保留已加载实例，并将其内部 `lp.registered` 设为 true，后续 `ApplyConfig` 调用 `plugin.reconfigure`。
3. 把空词改为 `BLOCKME`、保留合法非空 `filter.models` 后，[`handlePluginReconfigure`](../../../main.go) 把默认空 snapshot 当成旧有效 phase，记录 `filter.models phase change requires restart`，保留默认 snapshot，却返回成功 registration。
4. host 随后报告 `PluginRegistered("censorship") == true`。before 与 selected-auth after 均不拦截；真实 watcher 场景返回 HTTP 200，upstream 新收到一次包含 `BLOCKME` 的请求。重复应用同一合法非空 models 配置仍不能恢复。

独立新进程首次注册合法非空 models 能正常拦截；首次失败后改成合法空 models 也能恢复。缺陷来自默认 snapshot 被当成已安装配置。

同根因的相邻状态已通过公开 RPC 复现：尚无成功 snapshot 时，再次提交空词、缺失或 null `config_yaml`、malformed JSON，`plugin.reconfigure` 均记录拒绝日志却返回 `OK:true`。固定 host 会接受错误的成功 registration；该相邻核验只读确认了 host active 更新路径，未单独重建 native active 复测。已有有效 snapshot 时，无效更新返回 OK、保留 last-good 的行为正确。只删除 `init()` 和放宽 phase guard 不能完成这一组修复。

### 初始化、错误响应与 phase 设计

`activeConfig` 的原子指针初始保持 nil，表示尚未成功安装配置。删除启动时预装默认 snapshot 的 `init()`；`defaultSnapshot()` 继续提供成功解析配置所需的默认值，包括合法空 YAML 与零规则配置。

`plugin.register` 继续先完整解析、校验和编译 candidate，再通过现有 `installSnapshot` 原子发布。`plugin.reconfigure` 解析失败时保留拒绝日志：当前 snapshot 为 nil 则返回现有 `plugin_error` envelope（`OK:false`），保持未安装状态；已有有效 snapshot 则沿用记录日志、保留 last-good、返回 OK registration 的行为。空词、缺失/null `config_yaml`、malformed JSON 与无效 YAML 均使用这一判断，不增加新的错误码。

合法 candidate 仅在 `current != nil` 时比较 old 与 candidate 的 models phase。尚无有效 snapshot 时，合法空 models 与非空 models candidate 均可成为首次有效配置。

| 当前 snapshot | 无效 candidate 的结果 | 后续合法 candidate |
|---|---|---|
| nil | `plugin_error`，不安装 snapshot | 可首次建立任一 phase |
| 非 nil，包括合法零规则配置 | 保留 last-good、拒绝日志与 OK registration | 继续受既有 phase 保护 |

| 当前 snapshot | 合法 candidate 的 models | 结果 |
|---|---|---|
| nil | 空 | 安装 candidate，使用 before 阶段 |
| nil | 非空 | 安装 candidate，使用 selected-auth after 阶段 |
| 非 nil，models 为空 | 空 | 原子更新 candidate |
| 非 nil，models 非空 | 非空 | 原子更新 candidate |
| 非 nil，models 为空 | 非空 | 整体拒绝，保留旧 snapshot 与原拒绝日志 |
| 非 nil，models 非空 | 空 | 整体拒绝，保留旧 snapshot 与原拒绝日志 |

成功安装的零规则 snapshot 同样属于非 nil 状态，继续锁定 phase。不新增初始化 boolean，不用 `len(Rules)` 判断是否成功初始化，不增加配置键或请求关联状态。

[2026-09-15 phase 规格](2026-09-15-model-filter-post-route-execution-design.md) 对已有有效 snapshot 的双向 phase 拒绝、selected-auth marker、auth-selected attempt `Model`、同 phase callback 边界及 drain/restart 限制继续适用。两阶段现有 nil gate 已在 envelope decode 前返回 no-op，[`transformRequest`](../../../transform.go) 也有 nil gate，均保留原实现。

### 修改位置与调用方

| A 的独占文件 | 本次责任 |
|---|---|
| `config.go` | 仅取消启动预装 snapshot；保留 parser、默认值与 atomic 发布方式 |
| `main.go` | nil current 时的无效 reconfigure 返回 `plugin_error`；仅对已有 current 锁定 phase；保留其余请求处理行为 |
| `main_test.go` | 公开 lifecycle/RPC 的恢复、初始错误响应、last-good、nil gate 与零规则 phase 回归 |
| 新增 `integration/lifecycle_recovery_test.go` | 专属 fresh-process native `Host.ApplyConfig` 恢复、active 状态矩阵及真实 watcher HTTP 回归 |

`config_test.go`、`abi_cgo_test.go` 的既有并发、input copy、pointer 与 buffer 回归必须运行，无须修改。复用 `integration/harness_test.go`、`.github/scripts/integration-runner.go` 与 `.github/scripts/testdata/abi_benchmark_test.go` 的现有 helper、fixture 复制与 active-after oracle，保持这些共享文件只读。

生产 caller 为 `handleMethod` 分派的 register/reconfigure 与 before/after RPC。固定 host 是 lifecycle caller，保持只读；不改变其内部状态处理。`withDynamicABIHost` 要求首次注册成功，不能直接承担首次失败的 native case；新 fixture 在自己的子进程直接创建 host 并调用 `ApplyConfig`。

## 共用兼容边界

以下边界保持不变：

- `words` Object、手写 YAML legacy sequence、七个 `ConfigFields`、request-only capability、schema 5 与 ABI v1。
- `block -> strip -> obfs`、原 token 顺序、机器字段与 provider/filter 语义；selectors 仅增加 B 明确规定的 root。
- metadata 与 headers 的传递、正文改写时的 `ClearHeaders`、原请求字节不变性。
- `cliproxyPluginCall` 同步 `C.GoBytes`、host pointer 生命周期、response buffer ownership 与 `free_buffer`。
- 全部现有 C exports、依赖版本、配置默认值；不增加依赖、重试、fallback、cache 或额外初始化状态。

## A 的验收条件

| ID | 场景与可观察结果 |
|---|---|
| L1 | 首次空词 register 返回 `plugin_error`，随后合法空 models reconfigure 成功；含 `BLOCKME` 的请求在 before 返回 HTTP 400 `censorship_blocked`，after 不重复处理 |
| L2 | 同样的首次失败后，合法非空 models reconfigure 成功；before 精确 no-op，有效 selected-auth after 使用匹配 `Model` 返回 400；重复应用该合法配置后仍拦截 |
| L3 | 独立新进程首次成功注册空 models、非空 models 均保留既有行为；`RequestedModel` 与 body model 使用相反 decoy，不影响匹配结果 |
| L4 | 已有有效 snapshot 后，empty -> non-empty 与 non-empty -> empty 继续整体拒绝；旧规则、旧处理阶段及不含配置内容的拒绝日志保持不变；成功安装零规则 snapshot 的两个方向也受此限制 |
| L5 | empty -> empty、non-empty -> non-empty 更新生效；已有 snapshot 时，malformed lifecycle envelope、缺失/null `config_yaml`、无效 YAML candidate 均保留 last-good 与原响应行为 |
| L6 | nil snapshot 下，两个拦截阶段对 malformed raw envelope 均返回精确 no-op，且不修改原输入；无效注册不得安装 snapshot |
| L7 | atomic 发布只暴露完整 snapshot；既有并发、metadata、headers、输入不变性、ABI copy/free 与 active-after 回归通过 |
| L8 | native 矩阵从真实 DLL 和固定 `Host.ApplyConfig` 验证首次失败、再次无效更新仍 inactive、合法恢复、已有 phase 保护、同 phase 更新与无效配置保留；watcher HTTP 从首次无效配置修正到合法配置后返回 canonical 400，待验收请求的 upstream arrival/request 增量均为零 |
| L9 | 从未安装有效 snapshot，或首次 register 失败后，再次收到空词、缺失/null `config_yaml`、malformed JSON、无效 YAML 的 reconfigure，均返回 `OK:false` / `plugin_error`、记录拒绝日志且不安装 snapshot；两阶段仍 no-op，随后合法空/非空 models 均可恢复。已有有效 snapshot（含零规则）收到同类输入则保持 last-good、拒绝日志与 OK |

RPC 使用真实公开方法 `plugin.register`、`plugin.reconfigure`、`request.intercept_before`、`request.intercept_after`；no-op 是空 `RequestInterceptResponse`，固定 host adapter 对 no-op 保留原 body 与 headers。malformed lifecycle envelope 通过公开 RPC 测试，native 配置路径使用 host 实际可发送的输入。以 envelope、实际规则处理、`PluginRegistered`/`RegisteredPlugins` 和 upstream 计数验收，不用私有初始化 boolean 代替行为。

native 矩阵每个独立初始状态必须使用独立子进程。同一场景的失败、修正与重复应用必须留在同一个 host/DLL 实例；不能用 `ShutdownAll` 或 `cliproxyPluginShutdown` 代替 Go DLL 静态状态重置。

watcher 等待必须有截止时间，并确认修正配置已经被观察；同步确认后再记录 upstream 基线并发送待验收请求。保留 clean canary 成功转发对照，排除 provider/auth/路由不可用造成的假通过。

## B. Gemini generation_config schema descriptions

### 已确认缺陷与协议依据

[`collectGemini`](../../../selectors_gemini.go) 仅在 `generationConfig` 下选取七条 generation schema 路径，漏选 `generation_config`。两份独立报告已确认公开 RPC、重新构建 DLL 的 native BeforeAuth/selected-auth AfterAuth 与真实本地 HTTP 链中的 block、strip、obfs 均受影响。最小 block 对照只改变 root：camel 返回 HTTP 400、upstream 增量 0；snake 返回 200、upstream 增量 1，description 仍含 `SECRET answer`。

[固定于 2024-11-19 的 Google proto](https://raw.githubusercontent.com/googleapis/googleapis/c70bd1ef0b0d0691f165b864681c6637f9cec150/google/ai/generativelanguage/v1beta/generative_service.proto) 已定义 `GenerateContentRequest.generation_config`、`GenerationConfig.response_schema` 和 `response_mime_type`，GenerateContent 的 HTTP binding 使用请求体 `*`。[同 commit 的 Schema](https://raw.githubusercontent.com/googleapis/googleapis/c70bd1ef0b0d0691f165b864681c6637f9cec150/google/ai/generativelanguage/v1beta/content.proto) 包含字符串 `description`、递归 `properties` 与正式 `OBJECT`/`STRING` enum。[ProtoJSON 字段名规则](https://protobuf.dev/programming-guides/json/#field-names-as-json-keys) 要求接受 lowerCamelCase/显式 `json_name` 及原 proto 字段名，因此单独使用 `generation_config.response_schema` 有正式依据。已读取这些字段及必要上下文；本地 forwarding 不构成 Google 商用 API 接收结果。

[v0.3.9 历史规格](2026-10-02-v039-functional-corrections-design.md) 的第 6 节曾明确排除该 root；官方既有 schema 与固定 host 的实际 forwarding 已否定其排除依据。历史规格保留审计记录，本规格修订这一边界。最终 README 直接描述支持路径，不反复叙述历史错误。

### 最小实现与兼容边界

仅在 `collectGemini` 既有 `roles.has("developer")` 分支，将以下七条子路径同时用于 `generationConfig` 与 `generation_config` 两个明确 root，并复用现有 `appendJSONSchemaDescriptions`：

1. `responseSchema`
2. `response_schema`
3. `_responseJsonSchema`
4. `response_json_schema`
5. `responseJsonSchema`
6. `response_json_schema_ordered`
7. `responseFormat.text.schema`

上述七条是现有支持集合；2024 proto 为正式 root 与 `response_schema` 提供依据，不把其他兼容 alias 宣称为该历史 proto 的全部定义。沿用既有 description keyword 白名单与字符串门控，不增加 generic walker、alias parser、键名 normalization、新配置或依赖；`response_format`、未知 root 与未知 schema 字段继续不选取。仅 description 叶子进入规则，schema key、名称、enum、const、default、required、MIME、stop sequences、metadata、数字原始表示、媒体与签名均保持原字节。

有效单 root 请求是正式保证范围。两个 root 同时出现属于同一 ProtoJSON 字段的逻辑重复，不保证作为两份独立配置合并，不引入新的 alias 优先级。collector 可沿用 `systemInstruction`/`system_instruction` 等既有显式 alias 选择惯例；双 root 混合 case 仅检查现有原 token 顺序和规则优先级，不宣称 Google 接受此类请求。

生产 caller 是 `interceptRequest` 经 `transformRequest` 与 Gemini selector dispatch 到 `collectGemini`。共享 `appendJSONSchemaDescriptions`、span 排序、规则执行与 byte rebuild 保持只读；其他 provider 不变。

| B 的独占文件 | 本次责任 |
|---|---|
| `selectors_gemini.go` | 七条已有子路径在两个明确 root 下复用 description helper |
| `selectors_gemini_test.go` | 7 × 2 路径矩阵、动作/fold、scope、机器字段、顺序与 no-op |
| `fuzz_test.go` | 独立 semantic/raw oracle 的 `oracleGeminiSpans`/`oracleRawGeminiSpans`、正常 alias 检查、excluded case 与 `FuzzProtocolTransform` seeds |
| `functional_corrections_test.go` | 对应 Gemini fixture 覆盖两阶段、filter/selected-auth gate、取消与输入/metadata/headers 不变性 |
| 新增 `integration/gemini_generation_config_test.go` | 专属真实 native/HTTP 矩阵；复用已有 helper，不改 A 的文件或共享 harness |

### 验收条件

| ID | 场景与可观察结果 |
|---|---|
| G1 | 全部七条子路径 × 两个 root，每例只有一个 root 和一个 schema 表示，正确 MIME，根与嵌套 description；block/strip/obfs × literal/fold 均产生预期结果，block 的 term/role 保留原规则词与 developer |
| G2 | developer scope 启用才处理；user-only、format disabled、machine-only、未知 root/未知 schema、非字符串 description 与 no-match 精确 no-op，原输入不变 |
| G3 | 机器字段及非目标字节逐字节保持，包括属性名、enum/const/default/required、MIME、stop sequences、metadata 与 `1.2300e+00`；改写只替换选中的 description token，现有 `ClearHeaders` 行为不变 |
| G4 | 原 token 顺序、规则优先级、`block -> strip -> obfs` 和最终取消保持；取消无 replacement body。两个 root 的混合顺序仅作为本地排序回归 |
| G5 | 两个独立 oracle 直接同步明确 root，保留各自 parser，不调用生产 collector/helper；修订错误的 snake-root excluded case，保留真实未知路径/machine 排除，加入小型正常 seeds 与伪造机器字段改写反例 |
| G6 | 公开 RPC 两阶段、model-bearing before no-op、model-free after no-op、缺 selected-auth marker、filter miss、相反 `RequestedModel`/body model decoy、role/format gate 均保持既有行为，metadata/headers/输入不变 |
| G7 | 新构建 DLL 与固定 host 的 native 两阶段和真实本地 HTTP 均覆盖两个 root、七条子路径及三动作；block 返回 canonical 400、HTTP upstream arrival/request 增量均为零；strip/obfs 实际转发 description 符合预期，机器字段原始字节保持，并保留 clean canary、scope/filter bypass、no-match 与取消对照 |

native no-op 比较 host 聚合后的原 body/headers；公开 RPC 精确比较空 `RequestInterceptResponse`。HTTP oracle 必须考虑固定 converter 仅将 camel `generationConfig.responseSchema` 改名为 `generationConfig.responseJsonSchema`，snake root 保留原路径；不能把整个 upstream body 等同客户端原文，也不能只比较 decode 后的机器值掩盖字节改写。真实本地 upstream 只验证发送内容，不模拟 Google 的商业 schema 校验。

## 已确认兼容限制：跨格式 full-strip -> Claude 空 typed text

独立诊断已从非空 Gemini/Interactions 文本经插件 full-strip、固定 translator 和真实 `ClaudeExecutor.Execute` 向本地 upstream 复现 `{"type":"text","text":""}`。原文直接转换保留非空文本；改写结果违反 [Messages API 的 `TextBlockParam.text` 最小长度 1](https://platform.claude.com/docs/en/api/http/messages/create)。本地接收器返回 200 只证明请求实际发出；未调用 Anthropic/Google 商用 API，不声称实测提供方拒绝。

在固定 schema 5 / ABI v1 与不修改 CPA core 的范围内，保留现有合法原生 Gemini/Interactions full-strip、全部入口覆盖和阶段行为时，没有完整、安全的最小纯插件修复：

- BeforeAuth 的 `ToFormat` 为空，相同源请求与模型可选到不同 provider，不能按模型名或 caller metadata 推断目标。
- 普通 selected-auth AfterAuth 有 `ToFormat`，但不透明 Executor 内部目标及全部入口不由此覆盖；model-free after 仍须保留现有早期 no-op。
- 现有 `RequestInterceptResponse` 没有 metadata/provenance，无法完整携带原 filter 判定、配置与逐字段清空来源。

该项保留为已确认、未修复的兼容限制，不分配开发任务。不得全局拒绝合法源格式 full-strip、只修 AfterAuth 子集后宣称完整修复、改变阶段、增加 state/provenance、删除元素或插入占位文本。Claude 最末 assistant scalar 的 full-strip 保留既有行为，供应商 prefill 接受性未核实。最终发布文档任务须在 README accepted limits 准确说明这些边界。

## 回归与发布完成条件

A 必须先完成一条初始恢复 RPC 的 RED -> 最小 GREEN，再完成尚无 snapshot 时相邻错误响应的 RED -> GREEN；native/watcher 恢复也保留真实 RED/GREEN。B 在修订目标行为测试、两个 oracle 与真实 native/HTTP fixture 后取得 RED，再作最小 collector 修改并取得 GREEN。各 owner 连续完成相关完整回归、规格自审、修正、复审及英语提交，记录实际命令、退出状态和日志，不把未完成步骤留给主会话。不手改 `.integration/` 或 `dist/`。

A、B 从包含最终 spec/plan 的同一 main 基线开独立 worktree 并行执行，文件责任互不重叠。两者完成并经主会话合并后，独立集成发布 worktree 才顺序更新英语 `README.md`、`RELEASE_NOTES.md`、`main_test.go` 的发行版本断言及 plan 最终记录，目标 v0.3.12。

保留完整发布门槛：最终 `make test`、`make race`、`make vet`、`make integration`、`make build`、两种 `make package VERSION=v0.3.12`、独立 packager test/race/vet、六个有界 fuzz；正式 ZIP 的同 DLL 实际注册 version 0.3.12 / schema 5 / ABI 1 / Object words / 七字段及 active-after；最终 main CI 全成功后才 tag，随后 tag CI、七平台 15 assets/checksums/buildinfo/revision 分别验收。v0.3.11 的 main/tag CI 与七平台 15 assets 已核验成功，当前 README 的 pending 历史记录不构成本次新 bug。具体命令、工作区责任、来源核对与发布依赖由[实施计划](../plans/2026-10-23-functional-audit-v0.3.12.md)规定。
