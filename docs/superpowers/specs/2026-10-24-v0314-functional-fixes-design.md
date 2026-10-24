# v0.3.14 功能修复设计

日期：2026-10-24。源码基线：`a270cc658c6cd178c19324faa4caa759d648298d`（v0.3.13）。目标：v0.3.14。执行责任、检查命令与完成条件见[实施计划](../plans/2026-10-24-v0314-functional-fixes.md)。

## 范围与实施前置条件

本轮包含三个已有实际复现的可执行修复：G1，Gemini typed `Schema.any_of` 下的 description 漏选；R1，Make integration 将合法 `GO` 命令文本交给仅接受 executable 的 runner；R2，完整串行集成矩阵可能超过 Go 默认十分钟总预算。G1 属于插件请求处理，R1/R2 属于本仓库开发与验收入口。

本轮初始九分区扫描已完成，`wuavn3cvf.output` 的九个执行者均为 done。独立完整性 critic `wlhsct59c.output` 已完整解析五份报告的 17 个结果正文，核对源码基线全部 107 个 tracked 文件和 53 份历史 docs 的全文阅读 owner 或合理排除依据；唯一缺少明确阅读记录的 `go.sum` 已补读，`go mod verify` 退出 0。其覆盖结论支持进入 G1/R1/R2 三项修复，另将 README 的 WebSocket completion 时限文案纠正纳入 U3，不存在尚待补查的文件覆盖缺口。G1/R1 独立复核、R2 预算复核及文档审查 `wgveys0re.output` 已读取，本次按文档审查修订选择兼容性与分支顺序；唯一剩余启动门槛为修订文档复审。通过后在本轮独立集成分支确定共同起点，再启动 G/R。新事实影响范围、方案或验收时，须更新同一 spec/plan 并复审。

仅开发 censorship plugin；不修改 CPA core，不执行安全扫描，不增加未测量的性能优化、新功能、依赖、配置项或公共接口。固定 CLIProxyAPI v7.2.152 / `c76dfd4e0edabab9000628b1560ab8ab379eadb8`、model-mapper revision `8fe4839dd2c39a4b0537447c4ac35a9f1d699bbf`、schema 5、native ABI v1 与现有 Go 依赖。

## 已核对的复现证据

文档 owner 只读借用 finder 的 OS 临时证据，不修改、移动或清理这些目录；它们不构成后续测试的运行依赖。

| 项目 | 实际证据与结论 | 仍待完成 |
|---|---|---|
| G1 | finder `a2ed8eb63423fc996` 的 `cpa-gemini-interactions-audit-esb37oa5/http-evidence.json` 全部 12 项结果与 `wcgc5223x.output` 三方最终报告已完整读取。独立 RPC/真实 HTTP 各 312 项：8 typed roots 的 any_of 在两阶段三动作均漏选，anyOf 正常；18 JSON Schema roots 的 anyOf 正常、未知 any_of 保持不选。block 漏选为 200、upstream 增加一；strip/obfs description 仍含 SECRET。新构建 DLL 与固定 CPA provenance、generated ProtoJSON parser、clean/active canary 均有独立证据。 | 正式回归、两个独立 fuzz oracle 的修订及新实现 RED/GREEN；启动前须完成修订文档复审。 |
| R1 | `aae181b4eaaac7fe1.output` 最终报告及 `wi5o1f51h.output` 三份独立报告已完整核对。真实 `make integration 'GO=go -C .'` 连续退出 2：runner 单测成功，随后 `exec: "go -C .": executable file not found in %PATH%`。quoted executable 同样失败；同环境 make build 成功。独立反例证明裸 driver 替代会丢失 wrapper 每次调用行为；literal 直接执行、复杂文本由现有 sh 逐参数 quote 的 prototype 已通过相关对照。 | 正式修复自身的 TDD、Windows standalone/跨平台参数边界及完整真实入口回归，不能用 prototype 代替。 |
| R2 | `wqhzsh92v.output` 已完整读取。独立重算三份日志：默认 full 在 `600.489s` 超时；原样生成源码/DLL 的直接 `-timeout=30m` full 在 `602.803s` 通过；正式 Make 入口 `GOFLAGS=-timeout=30m` 在 `487.988s` 通过。成功日志的全部测试名称、状态、层级和顺序一致，失败日志的已完成顶层测试是其前缀。 | 只给 full 默认预算的实现、有效 GOFLAGS 覆盖测试及合并后完整回归。 |

R2 证据支持串行场景累计预算不足，未证明每次默认运行均失败，也未证明 30 分钟是数值上的最小必要值。此次没有单个测试长期挂起的证据；独立复核未进行长期资源泄漏监测，其引用的已删除 worktree 无法重新比对当时生成源码。不能据此作零泄漏保证，或附加未复现的清理机制修改。

本文件尚不记录任何 v0.3.14 实现、测试、构建、CI 或发布通过结果。

## G1. Gemini typed Schema 的 any_of description

### 正式协议依据

已读取固定 Google commit `bc7e3baa28fbb223fa93782e130260fab8205bfc` 的相关 message 与必要上下文：

- [content.proto 的 Schema 与 FunctionDeclaration](https://raw.githubusercontent.com/googleapis/googleapis/bc7e3baa28fbb223fa93782e130260fab8205bfc/google/ai/generativelanguage/v1beta/content.proto) 定义 `Schema.description = 3`、递归 `properties`/`items` 和 `repeated Schema any_of = 18`；`FunctionDeclaration.parameters`、`response` 使用 typed `Schema`，`parameters_json_schema`、`response_json_schema` 使用 `google.protobuf.Value`。
- [generative_service.proto 的 GenerationConfig](https://raw.githubusercontent.com/googleapis/googleapis/bc7e3baa28fbb223fa93782e130260fab8205bfc/google/ai/generativelanguage/v1beta/generative_service.proto) 定义 typed `Schema response_schema = 14`；其 JSON Schema 表示为其他 `Value` 字段，并列出 `anyOf` 等 JSON Schema keyword。
- [固定 ProtoJSON 文档的 Field names as JSON keys](https://raw.githubusercontent.com/protocolbuffers/protocolbuffers.github.io/7a68ef28113f81f7ce7eabf72df89bc105e5cf13/content/programming-guides/json.md) 要求 parser 接受 lowerCamelCase 或显式 `json_name`，以及原 proto 字段名。因此 typed `Schema` 中的 `anyOf` 与 `any_of` 都有正式依据。该规则不为任意 JSON Schema 对象创建 snake_case keyword。

[官方 google-ai-generativelanguage 0.6.18 changelog](https://googleapis.dev/python/generativelanguage/0.6.18/CHANGELOG.html) 的 2025-04-29 记录已加入 Schema.any_of，已读取对应版本段；该字段早于本轮 v0.3.13 基线。上述核查范围是相关字段、类型、命名规则和该版本记录，不是 Google 全部 API 的全文审查，也未调用商业 API。固定 CPA 向本地接收器实际转发只证明本地链路行为。

### 明确选择范围

仍使用 canonical `developer`，沿用现有 role/format gate、原 token 顺序和 description 字符串门控。下表 8 个 typed 输入入口保留全部基线 description 选择，并仅沿 Generative Language v1beta Schema 的明确递归边补充 `any_of`；18 个 JSON Schema 入口及其他 provider 保留原有 JSON Schema helper 语义。

| 入口 | 本次新增 any_of 递归 |
|---|---|
| `tools[*].functionDeclarations[*].parameters`、`.response` | 是 |
| `tools[*].function_declarations[*].parameters`、`.response` | 是 |
| `generationConfig.responseSchema`、`.response_schema` | 是 |
| `generation_config.responseSchema`、`.response_schema` | 是 |
| 两种 declaration 容器下的 `parametersJsonSchema`、`parameters_json_schema`、`responseJsonSchema`、`response_json_schema` | 否；保留现有 JSON Schema description 选择 |
| 两种 generation root 下的 `_responseJsonSchema`、`response_json_schema`、`responseJsonSchema`、`response_json_schema_ordered`、`responseFormat.text.schema` | 否；保留现有 JSON Schema description 选择 |
| OpenAI、Claude、Interactions 的 schema 入口 | 否；全部既有选择范围不变 |

选中集合保留基线全部 description，并补充从这 8 个入口仅经对象映射 `properties` 的属性值、对象 `items`、数组 `anyOf`/`any_of` 的对象元素可达的 string `description`。新增路径支持多层合法嵌套和 mixed spelling；null、scalar、错误数组形状及非对象元素不产生新的选中路径。属性名恰好为 `any_of` 或 `description` 时仍按 properties 的属性值处理，不改写属性名。

八个入口的既有 description 递归全部保留，包括共享 helper 的 15 个其他 keyword：`patternProperties`、`$defs`、`definitions`、`dependentSchemas`、`additionalProperties`、`unevaluatedProperties`、`propertyNames`、`contains`、`not`、`if`、`then`、`else`、`prefixItems`、`allOf`、`oneOf`。这些既有路径作为不回归正例；新增 `any_of` 选择不能经这些 JSON Schema 专属分支传播，即使分支内再出现 properties/items/anyOf 也不恢复新增路径资格。所有新增 description 都必须满足上一段的完整路径条件。`title`、format、pattern、metadata、example、default、enum、required、propertyOrdering 等机器值及任意非目标嵌套对象继续排除。

共享 `appendJSONSchemaDescriptions` 的默认 keyword 集合及行为保持不变，现有 JSON Schema caller 保留原语义。G owner 按上述选中集合选择最少且完整的实现，验证基线选择保留和新增路径边界；不预定单一内部组织，不把 any_of 无条件加入共享 JSON Schema 白名单，不复制整份 walker，不编写通用 alias parser、键名 normalization 或递归字符串扫描。仅在实现确有需要时修改 `selectors_openai.go` 顶部共享 helper 的内部组织，其他 provider 生产 caller 不变。

semantic/raw 两个 fuzz oracle 各自独立表达基线选中集合与 typed 新增路径，不调用生产 collector/helper，不合并两套 parser。补齐新增路径正例、15 种既有递归正例，以及 JSON Schema 入口和专属分支中未知 any_of 的排除反例。

### G 验收条件

| ID | 可观察结果 |
|---|---|
| G-A1 | 全部 typed 入口 × `anyOf`/`any_of`，包括根、properties、items 和多层混合组合子下的 string description，block/strip/obfs × literal/fold 均正确；block 保留原规则词和 developer role。每个正式请求只有一个 generation root 和一个互斥 schema 表示。 |
| G-A2 | 全部 18 个 Gemini JSON Schema 入口和其他 provider 的未知 any_of 子树保持不选；合法 anyOf/原有 description 路径继续正常。8 个 typed 入口的全部基线 description 选择保持，包括上述 15 个 keyword 的不回归正例；这些 JSON Schema 专属分支中的未知 any_of 仍不选，新路径只经规定的 typed 边可达。机器字段、错误类型、developer disabled、format disabled、no-match 维持精确 no-op。 |
| G-A3 | 只有被选中的 description token 可以改写；名称、keys、enum/const/default/example/required、MIME、stop sequences、metadata、媒体/签名、数字 `1.2300e+00` 等非目标字节保持。原 body、metadata、headers 不被原地修改；改写时沿用 ClearHeaders，no-op/完全取消不提供 replacement。 |
| G-A4 | 原 JSON token 顺序、规则词优先、`block -> strip -> obfs`、跨节点不匹配、最终文本取消、immutable-text 与长度/非空限制保持。公开 RPC 两阶段、model-bearing before、model-free after、缺 selected-auth marker、filter miss 和相反 RequestedModel/body model decoy 均保持既有阶段行为。 |
| G-A5 | semantic/raw oracle 对 typed 新增路径、15 种既有递归正例及 JSON Schema 入口/专属分支的 any_of 负例均有独立预期断言；修订 oracle 后旧 production 在新增路径上仍失败。伪造机器字段改写结果被独立 oracle 拒绝，FuzzProtocolTransform 包含对应 seed。 |
| G-A6 | 从当前 worktree 新构建且核对 hash 的 DLL，通过固定 host native 两阶段及真实 CPA HTTP 验证全部 typed 入口 × 两种 keyword × 三动作。block 为 canonical 400/censorship_blocked、upstream arrival/request 增量均为零；strip/obfs 为一次转发、目标 description 正确、机器字段 raw bytes 保持；clean canary、JSON Schema 排除和 gate 对照可用。 |

独立复核已实际执行固定 converter：两种 declaration 拼写下的 parameters 改到 `function_declarations[*].parametersJsonSchema`，response 改到 `function_declarations[*].response`；`generationConfig.responseSchema` 改到 responseJsonSchema，其余三个 typed generation 入口保持原路径，内部 any_of 不归一化。parameters 和 camel-case responseSchema 的原对象被转入 JSON Schema 字段，没有 typed Schema 校验；这些 source 入口的基线 description 选择保持。HTTP oracle 按实际 wire 路径核对 description/机器字节，新增选择资格由插件输入入口及完整递归路径决定。转发结果不证明 Google 按 typed Schema 解释或接受这些对象，也不修改 CPA 转换来解决此限制。

不能仅比较 decode 后的机器值，也不能假定整个 upstream body 等于客户端原文。两个 generation root 或同一 proto 字段的两种拼写同时出现，仅可作为本地排序回归，不新增 provider 接受性或合并优先级保证。

## R1. Make 命令文本与 runner executable

Make 的 `GO ?= go` 接受 shell 命令文本，已有规格允许绝对路径、quotes、`-C` 与 `env` wrapper。Make 通过 shell 解释该文本；runner 的 `exec.Command` 需要单个 executable。生产 caller 是 `buildCPA`、`buildPlugin`、`buildModelMapper` 和 `runIntegrationTests` 对 `goCommand`/`runCommand` 的调用。

四处 Go 调用通过专用 Go 命令执行边界保留原 GO 文本的完整语义；Git 等调用继续使用通用 runCommand 的直接 executable/argv 行为。已经可作为单个 executable 执行的默认 go、裸绝对路径和简单 wrapper 保留直接执行，在原生 Windows 没有 POSIX shell 的环境仍可工作。复杂 GO 命令可以使用 Make 已有的 POSIX shell 解释原文，runner 自己生成的每个参数须完整编码后独立保留；不得解析或重新组合用户 GO 文本。

各调用保留原 cmd.Dir、extraEnv、stdout/stderr 和退出状态。`-C` 按 Go 原生语义相对于每次子命令 cwd 执行，不能删除、改写成插件根或只在外层执行一次。wrapper 每次调用对额外环境的设置/移除同样保留，不用裸 driver 替代完整命令。不承诺任意指向外部目录的 -C 都适合这组多仓库构建。

不手写 shell parser，不使用 strings.Fields，不改变 Make 通用 GO 语义，不增加 shell 自动寻找层、新依赖、公开 runner 参数或配置。独立原型确认本机 MSYS 边界下简单 `sh -c` 加 positional 参数的方式可能破坏复杂 quotes，正式方案必须用真实参数/环境测试核查。裸 executable 的 standalone Windows 无 shell 路径必须实际验证；复杂命令使用现有 shell，缺失时明确失败，不静默更换 driver 或丢弃参数。直接 runner 的空 GO 默认行为、Make 显式空 GO 的既有失败均保留。

| ID | 可观察结果 |
|---|---|
| R-A1 | 默认、实际绝对 executable、带空格并引用的 executable、`go -C .`、引用的绝对 executable 加 `-C .`、env wrapper 的完整命令/工具链语义保持；wrapper 可移除 runner 额外注入的非必要测试 marker，证明没有被裸 driver 替代。各 cwd 下原生 -C 生效，包含单引号、双引号、反斜线、空格及空参数的 runner argv 不损坏。 |
| R-A2 | 小型真实 Go/Make 边界测试覆盖 argv、cwd、环境和退出状态；真实 buildCPA 构建可运行的 checkout fixture，插件根放不可编译对照以证明 cwd。裸 executable 在 Windows 无 shell 的 PATH 下仍可执行；至少默认与 GO='go -C .' 两个正式完整 make integration 真实通过。 |
| R-A3 | driver/shell 启动、错误 Go 子命令及不存在的 -C 目录均传播非零退出；保留 Make 显式空 GO、standalone 空 GO 默认、模式选择、固定 checkout revision/cleanliness、containment、生成清理及 packager 既有 GO 断言。 |

## R2. 仅 full 的有限默认总预算

只为 full 集成测试进程设置默认 `30m`；不把准备 checkout 或编译阶段误计为已测量的套件耗时，不引入新模式、总重试或并行矩阵优化。

full 通过相同 Go 调用入口、cwd 和环境执行 go env -json GOFLAGS，用标准 JSON parser 读取包括 go/env 持久配置的有效原值。在该次 full 测试子进程环境中前置 -timeout=30m，再接原有效 GOFLAGS，完整保留原顺序和 quotes，由 Go 自己解析，后续显式 timeout 继续覆盖默认。不要在 argv 无条件固定高优先级 timeout，不要只读取 os.Getenv，不改写用户持久 Go 配置。

R1/R2 由同一 owner 核验组合行为。用户 wrapper 每次主动设置、重写或清除 GOFLAGS 的原生优先权保留，默认预算不越过这种环境重置；不增加反制 wrapper 的机制。联合测试证明显式 timeout（含 0）、wrapper 设置 timeout 和环境重置仍生效，完整普通入口没有这些覆盖时获得 30m。实现 owner 在同一 worktree 根据正式运行修正、复审并完成全部实现，不把组合验收留给主会话。

这一选择依据已读取的 Go 1.26.5 [GOFLAGS 解析与顺序](https://raw.githubusercontent.com/golang/go/go1.26.5/src/cmd/go/internal/base/goflags.go)、[test timeout 默认值及命令行优先级](https://raw.githubusercontent.com/golang/go/go1.26.5/src/cmd/go/internal/test/testflag.go)、[有效环境与 go/env 文件读取](https://raw.githubusercontent.com/golang/go/go1.26.5/src/cmd/go/internal/cfg/cfg.go)。`GOFLAGS` 使用 Go 的 quoted parser；命令行 timeout 优先于 GOFLAGS；空 OS 环境值仍可能读取持久 go/env。实现不复刻这些 parser 或优先级规则。

| ID | 可观察结果 |
|---|---|
| R-A4 | full 没有有效 timeout 覆盖且 wrapper 未主动重置 GOFLAGS 时，实际测试进程获得 30 分钟；package、tags、count=1、verbose 不变。 |
| R-A5 | OS GOFLAGS、wrapper 设置的 GOFLAGS 与隔离 GOENV 中经 go env -w 写入的较短、较长、0、-test.timeout 及重复 timeout 按 Go 既有优先级生效。wrapper 主动重写/清除 GOFLAGS 的行为保持，不宣称 30m 默认会越过重置；其他有效 flags 的 quotes/空格保持完整，查询/测试错误正常返回，不以 fallback 掩盖。 |
| R-A6 | ABI smoke、benchmark、BENCH=1 与 smoke 优先级的 argv、GOFLAGS 和默认预算均保持原状；30m 只作用于 full 的测试子进程，不传播到其他构建或模式。 |
| R-A7 | 不放宽 5 秒 HTTP/TCP/WS I/O、约 20 秒 CPA readiness、45 秒 lifecycle child、90 秒 Gemini native child，也不改已有 process-tree 清理。短测试证明用户 timeout 确实能终止测试并返回非零；完整串行集成在新预算下结束，不删、skip 或放宽断言。 |

## 共用兼容边界与文件责任

- 严格 Object `words`、手写 YAML legacy sequence、词语原样和顺序、七个 ConfigFields、request-only 注册保持。不存在新增全局 mode 或自定义 panel。
- 首次有效 snapshot、last-known-good、同实例 models phase class、selected-auth marker 和 request.Model/caller_scope 语义保持；不加跨请求状态或重配置 fallback。
- 五种 SourceFormat、canonical roles、显式 description allowlist、机器字段排除保持；仅 G1 规定的 typed 入口新增 `any_of`。签名/annotations immutable-text、最终文本取消、size、非空与 final-text 判定继续执行。
- native ABI v1 的 exports、pointer/length 检查、ownership、response free_buffer、同步 `cliproxyPluginCall` 内 `C.GoBytes` 保持。没有支持移除该 input-sized allocation 的本轮 benchmark，不作 no-copy 或性能提升声明。
- v0.3.13 的两个 helper 各自绝对 auth-dir、合成 home、clean canary、本地 receiver 与隔离语义保持。`.integration/`、`dist/` 只由现有命令生成，不手改。
- [README 的 accepted pure-plugin limits](../../../README.md#accepted-pure-plugin-limits) 和[既有 phase 规格](2026-09-15-model-filter-post-route-execution-design.md)继续适用，包括跨格式 full-strip 后的 Claude typed-text 限制、final assistant scalar/prefill 未核实边界；本轮不改变 CPA 或这些源格式行为。

G 独占 `selectors_gemini.go`、`selectors_openai.go` 顶部共享 helper 的必要内部改动、`selectors_gemini_test.go`、`fuzz_test.go`、`functional_corrections_test.go` 与新增 `integration/gemini_typed_schema_test.go`。R 独占 `.github/scripts/integration-runner.go`、`.github/scripts/integration-runner_test.go`；负责核对 Makefile，但目前没有证据要求改动其 recipe，保持只读。共享生产 caller、现有 harness、通用 Git runCommand 与 packager 保持原语义。

修订文档复审通过后，主会话在本轮独立集成分支合并文档并记录共同 baseline SHA；G、R 从该 SHA 创建各自独立 worktree 并行执行。各 owner 在自己的 worktree 连续完成规格审查、单条纵向测试真实 RED、最小 GREEN、剩余验收、完整相关回归、简化、自审、修正、复审及英语提交；完成后主动回报，不把未完成步骤留给主会话。G 验证时若尚无 R，允许显式 `GOFLAGS=-timeout=30m` 运行旧 runner，并记录这是开发期覆盖；最终集成必须验证修正后的默认入口。

两组完整完成、审查且无文件冲突后，由主会话将提交合回该独立集成分支。U3 从该合并 SHA 创建独立最终集成 worktree，owner 独占英语 `README.md`、`RELEASE_NOTES.md`、`main_test.go` 的文档/当前发行 guard，以及本 spec/plan 的实际记录。当前版本更新到 v0.3.14，保留历史兼容句；README 的 WebSocket completion 文案改为实际 5 秒，不改现有测试时限。源码 `main.go` 的默认 `pluginVersion=0.0.0-dev` 不改，正式构建继续由 VERSION 注入。

## 验收与发布门槛

G/R 合回独立集成分支后，U3 完整执行根 test/race/vet、独立 runner 和 packager tests/race/vet、六个 bounded fuzz、正式 make build、selected-platform/aggregate 两个 package、默认与 GO=-C 的完整 make integration。真实 ZIP 验证 CRC、精确成员、LICENSE、permissions、固定时间、lowercase SHA-256 sidecar 与 aggregate；从正式包提取同一 Windows amd64 DLL，实际读取 ABI 1、schema 5、Version 0.3.14、七字段/Object words、request-only，并验证 selected-auth active-after 及必要 no-op/字节保持。

主工作区始终保留初始 main，直到 U3 的全部本地验收、修正、复审和英语提交完成；U4 才由主会话将已完成的集成结果合并到 main，不开 PR。远端写入只能在主会话已持有的用户发布授权范围内执行，本文件和 finder 报告不提供授权。最终 main CI 的 test、七平台 build、Windows amd64 ABI smoke 均达成功终态后才创建 tag；随后 tag CI/release 分别等到终态并验收，不将 pending、取消或失败写作通过。

七平台 main dev 产物与正式 v0.3.14 的 15 资产分别核对来源及字节：run/attempt/job/artifact/asset、ZIP/CRC/LICENSE/permission/checksum/aggregate、每库最终 SHA、vcs.modified=false、CPA pin、c-shared、CGO 1、compiler 与对应 GoVersion。正式 Windows 包的提取 DLL 重新实际注册与 active-after；其他平台 native 和商业 API 未运行时逐项如实记录 not_run。main 的七份 dev 包不冒充正式包，workflow success、tag、Main.Version 或字符串命中不单独充当版本注入证明。

本次文档任务仅新增指定的两份文档，核查后英语提交；不实现、不执行产品测试、不合并 main、不 push/tag，不清理本 worktree。主会话合并后只清理本轮创建、已无未提交内容和未合并提交的 worktree/branches，保护原有及其他任务目录。
