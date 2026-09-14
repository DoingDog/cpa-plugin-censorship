# Models Filter 与模型名称修改插件兼容设计

## 目标

让 censorship 插件的 `filter.models` 在前序模型路由或模型名称修改插件已经改变当前执行模型时，匹配该拦截阶段实际提供的模型名。兼容目标不绑定 `cpa-plugin-model-mapper`，任何遵循 CLIProxyAPI 插件调用契约的模型路由插件都适用。

示例：模型修改插件把客户端请求的 `claude-opus-5` 路由到 `kimi-k3`，并通过 host 的 model execution callback 重入执行链时，nested request 的 `Model` 为 `kimi-k3`，`filter.models` 配置 `kimi-k3` 应在该 nested invocation 命中。outer invocation 仍在模型 Executor 之前，继续按客户端模型处理，以保持 censorship 不受 CPA model alias 影响。

censorship 仍只负责在 CPA 路由后的请求拦截阶段处理自然语言内容，且不实现 CPA model alias、模型路由、反向映射或模型列表修改。

## 术语与时序

- **客户端请求模型**：客户端原始 RPC 请求携带的模型身份，由 `RequestedModel` 表示。
- **effective model**：当前 request interceptor 执行阶段 host 放入 `RequestInterceptRequest.Model` 的模型身份。
- **最终上游模型**：provider 或 executor 最终发出的模型身份。只有在它已经成为当前拦截阶段的 `Model` 时，censorship 才能观察到它；插件不推测后续阶段的值。
- **outer invocation**：客户端请求经过 model router 后第一次进入 request interceptor 的调用。它仍处于 CPA 路由/模型执行边界之前。
- **nested callback invocation**：模型 Executor 调用 `host.model.execute*` 后，host 为实际 upstream execution 新建的调用。它的 Metadata `source` 为 `plugin_host_model_callback`，并携带该次 callback 的当前 `Model`。

固定流程：model router 先选择目标；模型 Executor 需要时通过 `host.model.execute` 重入执行链；request interceptor 在固定阶段运行。router priority 只决定 model router 之间的顺序，request interceptor priority 只决定 interceptor 之间的顺序，二者不横向比较。

在当前 pinned CLIProxyAPI v7.2.152 契约中，outer `BeforeAuth` 会在 self-handled Executor 改写模型之前运行；该 Executor 通过 host callback 重入时，host 为 nested invocation 设置 `source: plugin_host_model_callback`。CPA model alias 仍只属于 outer CPA 路由边界，不应被 censorship 从 `Model` 推断。

## 行为

1. outer invocation 的 models-only filter 继续使用 `RequestedModel` 做 exact/glob 匹配，保持 censorship 不受 CPA model alias 影响。
2. nested callback invocation 的 models-only filter 使用当前 `Model` 做 exact/glob 匹配。这样模型改名插件通过 host callback 重入后，`filter.models` 可以配置改写后的模型名；规则不依赖 model-mapper 的 ID、配置或 DSL，其他遵循同一 host callback 契约的插件同样适用。
3. `source` 只用于识别 pinned host 明确提供的 nested callback 阶段，不携带模型名，也不作为私有插件间通信。普通 outer invocation（包括 source 缺失或未知）使用 `RequestedModel`；只有精确的 callback source 才使用 `Model`。callback source 下的 `Model` 为空时不回退到 body、URL、headers、`RequestedModel`、Metadata 其他字段或 machine fields；model-only include 应 bypass，exclude 应 process。
4. API key-only filter、api key 与 model 的 `and`/`or` 组合、默认 `exclude`、空 filter 行为和 filter gate 之外的 words transform 均保持不变。filter gate 对每次 host invocation 独立计算；nested invocation 的结果不会回溯撤销已经完成的 outer invocation transform。
5. 不改写请求或响应中的 model 字段，不修改 `/models`，不解析或反向解析 CPA model alias。若模型名称只在当前 outer invocation 之后、且没有 host callback nested invocation 承载它，filter 不承诺匹配该后续最终模型；插件不能猜测。
6. nested invocation 由 host 的 skip/chain 语义决定适用调用；censorship 不通过特定插件 ID、版本或配置识别 mapper。每次 invocation 最多执行一次 censorship transform。
7. ABI v1 的输入复制、输出 buffer 所有权、长度和 `free_buffer` 生命周期不变。

## 实现范围

- 修改 `request_filter.go` 中 model filter 的输入字段。
- 更新 `request_filter_test.go` 的 focused cases，删除只要求 outer invocation 使用 `RequestedModel` 的旧单阶段预期，增加 outer/nested distinct-alias 场景、未知 source、空 callback `Model` 场景和组合语义回归覆盖。
- 检查直接相关 integration fixture/test 是否能观察 outer/nested callback 的 distinct model 和 host source。测试不引用 model-mapper 的私有规则或插件 ID；如 pinned harness 无法在现有 integration package 中构造该时序，则保持 integration fixture 不变，并将缺口明确为 host 契约测试，不在 censorship 中加入 fallback。
- 不修改 CLIProxyAPI core、native ABI、provider text selector、words transform 或 panel 输出格式，除非测试证明现有插件接口需要最小兼容调整。

## 验收标准

### Unit

- ordinary outer request `RequestedModel=client-a`、`Model=upstream-b` 时，models `client-a` 命中，models `upstream-b` 不因未来/后续模型身份而命中。
- nested callback request 带 `Metadata["source"] == "plugin_host_model_callback"`、`Model=upstream-b`、`RequestedModel=client-a` 时，models `upstream-b` 命中，models `client-a` 不命中。
- source 缺失或未知时使用 `RequestedModel`，不从 body 取值；callback source 下 `Model` 为空时不回退到 body 或 `RequestedModel`。include/exclude 对当前 invocation 的 non-match 行为保持定义，nested invocation 不回溯撤销 outer invocation 已完成的 transform。
- `Model` 与 `RequestedModel` 相同的无改名基线保持原结果。
- exact、`*`、`?`、大小写、完整字符串匹配保持不变。
- API-only、model-only、`or`、`and`、默认 exclude 逐项回归。
- body 顶层 decoy model 命中而选定阶段模型未命中时不因 body 字段改变 filter 结果。

### Integration

- 使用通用 nested callback fixture 或 pinned host contract test：outer request 的客户端模型为 `client-a`，nested callback 的 effective model 为 `upstream-b`，分别记录 `RequestedModel`、`Model` 和 `Metadata["source"]`。
- 断言 `models: [upstream-b]` 只在 nested callback invocation 命中；outer invocation 不因 CPA alias 或未来 Executor 重写而提前命中。
- 五种现有 HTTP source format 与 Responses WebSocket 使用相同阶段选择语义；provider-specific body model 位置不会成为 censorship fallback。
- 命中后只验证自然语言字段发生预期 censorship，模型字段、opaque machine fields、请求结构和 ABI buffer 行为不被改写。
- 若 executor callback 产生嵌套执行，验证调用方 router/interceptor 的 host skip 语义不被 censorship 绕过，且每个 invocation 最多处理一次 transform。

## 性能与兼容性

模型匹配只替换已解码 request envelope 中的字段读取，不新增 JSON 递归扫描、body 解析、分配、缓存或 plugin-specific 分支。保留现有 compiled glob 与 filter fast paths。旧配置无需迁移；只改变存在模型改写时 models filter 的匹配身份。

## 风险与限制

当前证据确认 `Model` 在 pinned v7.2.152 的 provider BeforeAuth 和 executor callback 相关 request 中是当前执行模型，但不同 host 版本或未遵循该契约的插件可能不提供有效值。此插件不通过解析 machine fields 兜底，因此这类 host 需要先修复其 effective-model 提供契约。

model router 与 request interceptor priority 不构成单一全局优先级。用户配置中的“模型修改插件优先于 censorship”只有在模型修改插件的执行链已通过 host callback 进入 censorship 拦截点时才有意义；本插件不自行重排 capability 阶段。
