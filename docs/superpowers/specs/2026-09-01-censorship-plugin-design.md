# CLIProxyAPI censorship 插件设计规格

- 状态：已完成设计确认，尚未开始实现
- 日期：2026-09-01
- 目标仓库：`cpa-plugin-censorship`
- CLIProxyAPI 基线：[`router-for-me/CLIProxyAPI@81e1b5374f99c212f196f34956eeed964a46b8fa`](https://github.com/router-for-me/CLIProxyAPI/commit/81e1b5374f99c212f196f34956eeed964a46b8fa)

## 1. 摘要

本项目实现一个名为 `censorship` 的 CLIProxyAPI 动态插件。插件只审查进入 CLIProxyAPI 的请求内容，不读取、不修改、也不观察任何上游模型输出。

插件从 CPA YAML 配置读取唯一屏蔽词表，支持 `block`、`strip`、`obfs` 三种模式。配置通过 CPA 现有的 `plugin.reconfigure` 生命周期热更新，不需要重启非 Home 模式的 CPA。规则处理必须保留 YAML 顺序，输出必须在相同输入、相同配置快照和相同插件版本下逐字节确定。

本设计选择纯插件方案，不修改 CLIProxyAPI。该选择不能完整满足 raw ingress 和所有 WebSocket 路径，限制见第 13 节。

## 2. 已确认的上游事实

### 2.1 请求拦截能力

当前基线的 `RequestInterceptor` 可以替换请求 body，也可以通过 `Terminate`、`StatusCode`、`ResponseHeaders` 和 `ResponseBody`终止请求。[`sdk/pluginapi/types.go:L994-L1034`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/pluginapi/types.go#L994-L1034)

BeforeAuth成功返回新 body时，CPA会同时替换 execution request的 `Payload` 和 `OriginalRequest`。[`sdk/api/handlers/handlers_interceptors.go:L448-L472`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/api/handlers/handlers_interceptors.go#L448-L472)

该 hook不是 raw ingress。普通执行在它之前已经运行 model router、provider/model解析和部分metadata准备。[`sdk/api/handlers/handlers_execution.go:L44-L94`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/api/handlers/handlers_execution.go#L44-L94)

### 2.2 输出 capability彼此独立

`ResponseInterceptor`、`StreamChunkInterceptor`和 `WebSocketResponseObserver`是独立 capability。[`sdk/pluginapi/types.go:L933-L957`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/pluginapi/types.go#L933-L957)

censorship只声明 `RequestInterceptor`。上游已有测试证明没有 stream interceptor时，stream chunk保持原样。[`sdk/api/handlers/handlers_interceptors_test.go:L1195-L1238`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/api/handlers/handlers_interceptors_test.go#L1195-L1238)

### 2.3 配置热更新

非 Home 模式的 config watcher使用150 ms debounce，检测内容hash变化后重新加载配置。[`internal/watcher/watcher.go:L83-L89`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/watcher/watcher.go#L83-L89) [`internal/watcher/config_reload.go:L29-L85`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/watcher/config_reload.go#L29-L85)

配置提交会调用 plugin host `ApplyConfig`；已加载插件收到 `plugin.reconfigure`。[`sdk/cliproxy/service_config.go:L124-L192`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/cliproxy/service_config.go#L124-L192) [`sdk/cliproxy/service_plugins.go:L89-L103`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/cliproxy/service_plugins.go#L89-L103) [`internal/pluginhost/host.go:L944-L984`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/pluginhost/host.go#L944-L984)

Home 模式不启动本地 config watcher。[`sdk/cliproxy/service_lifecycle.go:L175-L198`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/cliproxy/service_lifecycle.go#L175-L198)

### 2.4 WebSocket限制

Responses WebSocket在读取client frame前已经完成HTTP 101 Upgrade，之后才调用 `ReadMessage`。[`sdk/api/handlers/openai/openai_responses_websocket.go:L251-L258`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/api/handlers/openai/openai_responses_websocket.go#L251-L258) [`sdk/api/handlers/openai/openai_responses_websocket.go:L375-L385`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/api/handlers/openai/openai_responses_websocket.go#L375-L385)

本地 `generate=false` prewarm在调用 model execution和 `RequestInterceptor`前直接返回。[`sdk/api/handlers/openai/openai_responses_websocket_prewarm.go:L14-L23`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/api/handlers/openai/openai_responses_websocket_prewarm.go#L14-L23) [`sdk/api/handlers/openai/openai_responses_websocket.go:L526-L541`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/api/handlers/openai/openai_responses_websocket.go#L526-L541)

`/v1/realtime`和sideband使用直接双向relay，不调用 `RequestInterceptor`。[`internal/client/codex/live/websocket.go:L22-L32`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/client/codex/live/websocket.go#L22-L32) [`internal/client/codex/live/websocket.go:L203-L223`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/client/codex/live/websocket.go#L203-L223) [`internal/client/codex/live/sideband.go:L464-L490`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/client/codex/live/sideband.go#L464-L490) [`internal/client/codex/live/sideband.go:L610-L644`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/client/codex/live/sideband.go#L610-L644)

现有Responses WebSocket error builder不读取plugin direct `ResponseBody`或 `ResponseHeaders`；正常的non-retryable request fault路径会发送error payload后关闭连接。[`sdk/api/handlers/openai/openai_responses_websocket_forward.go:L212-L248`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/api/handlers/openai/openai_responses_websocket_forward.go#L212-L248) [`sdk/api/handlers/openai/openai_responses_websocket_forward.go:L528-L598`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/api/handlers/openai/openai_responses_websocket_forward.go#L528-L598)

## 3. 目标与非目标

### 3.1 目标

1. 所有屏蔽词只来自 YAML `words` sequence，无内置、fallback或示例词表参与运行。
2. 默认模式为 `block`。
3. `block`按词表顺序选择第一个匹配词，返回HTTP 400，并在HTTP错误中包含匹配词和canonical role。
4. `strip`和 `obfs`按词表顺序处理全部规则。每条规则一次处理所有eligible文字节点中的全部左到右非重叠occurrence。
5. 支持请求历史中的 `system`、`developer`、`user`，并允许配置 `assistant`。`tool`只覆盖OpenAI `role=tool`的string content，以及Claude `tool_result.content[]`中的typed text block。
6. 不修改tool call、tool schema、arguments、reasoning、thinking、其他tool/function result、JSON key、machine JSON、上传二进制和media/base64。
7. 处理 `openai`、`openai-response`、`claude`、`gemini`、`interactions`五种已确认的SourceFormat。
8. 处理现有hook可见的Codex Responses WebSocket model-executed turn。
9. 非 Home 模式下修改CPA YAML后动态生效。
10. 相同输入、相同配置快照和相同插件版本产生逐字节相同结果。
11. 永远不接触上游输出和流式响应。
12. 提供参考 `model-mapper` 风格的GitHub CI/CD、跨平台动态库、zip和checksum发布。

### 3.2 非目标

1. 不提供自定义管理面板、菜单或Management API。
2. 不隐藏CPA通用plugin list中的插件条目。
3. 不支持regex、word boundary、大小写折叠或Unicode normalization。
4. 不保证转换结果再次进入插件后保持不变。幂等性不是合同。
5. 不维护请求内容hash cache或conversation state。
6. 不处理Realtime/Live/sideband/DataChannel。
7. 不处理Responses WebSocket本地prewarm。
8. 不处理Alpha Search、raw image/video multipart或其他绕过 `RequestInterceptor` 的入口。
9. 不修改CLIProxyAPI上游代码。
10. 首个实现周期不提交CLIProxyAPI-Plugins-Store registry条目。

## 4. 架构

### 4.1 Capability

插件registration只声明 `RequestInterceptor`：

- `InterceptRequestBeforeAuth`：读取一次当前配置快照，选择eligible文字节点，执行模式逻辑，返回no-op、新body或direct termination。
- `InterceptRequestAfterAuth`：固定返回no-op，绝不再次处理内容。

插件不声明以下capability：

- `ResponseInterceptor`
- `StreamChunkInterceptor`
- `WebSocketResponseObserver`
- `RequestLifecyclePlugin`
- `ManagementAPI`
- `ModelRouter`
- `Executor`

`Metadata.ConfigFields`为空。

### 4.2 模块

```plaintext
abi_cgo.go     CPA ABI v1 init/call/free/shutdown
main.go        RPC dispatcher、registration和生命周期分派
config.go      YAML解析、校验、immutable snapshot和原子更新
transform.go   mode engine、raw span重建和错误body
selectors.go   SourceFormat selector和canonical role映射
```

所有生产代码保持 `package main`。不增加单实现interface、provider factory或通用递归JSON walker。

### 4.3 请求数据流

```plaintext
CPA handler已完成的前置处理
-> RequestInterceptor.BeforeAuth
-> 读取一次Config snapshot
-> 根据SourceFormat收集eligible spans
-> scope过滤
-> block、strip或obfs
-> no-op、替换Body或Terminate 400
-> CPA auth、provider translation和executor
-> 上游响应直接经过CPA现有路径
```

## 5. YAML合同

### 5.1 示例

```yaml
plugins:
  enabled: true
  configs:
    censorship:
      enabled: true
      mode: block

      words: []

      scope:
        formats:
          - openai
          - openai-response
          - claude
          - gemini
          - interactions
        roles:
          - system
          - developer
          - user

      obfs:
        char: "​"
```

CPA要求plugin instance使用mapping node才能读取host-owned `enabled`，所以词表不能直接作为 `plugins.configs.censorship` 的bare sequence。[`internal/config/config_types.go:L39-L99`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/config/config_types.go#L39-L99)

### 5.2 字段语义

| 字段 | 缺省 | 约束 |
|---|---|---|
| `mode` | `block` | 只允许 `block`、`strip`、`obfs`。 |
| `words` | `[]` | 必须是string sequence。它是唯一词表来源。 |
| `scope.formats` | 所有已实现格式 | 必须是string sequence；显式 `[]`表示全部关闭。 |
| `scope.roles` | `system,developer,user` | 必须是string sequence；显式 `[]`表示全部关闭；可加入 `assistant`和 `tool`。 |
| `obfs.char` | U+200B | 只允许单个U+200B或U+2060。 |

词条规则：

1. 保留YAML顺序、重复项、大小写、首尾空白和换行。
2. 不排序、不去重、不 `Trim`。
3. 空字符串和非string值使候选配置无效。
4. 匹配是大小写敏感的literal substring。
5. 不做Unicode normalization。
6. `mode: obfs`时，每个词条至少包含两个Unicode scalars。
7. `mode: obfs`时，词条不得包含当前 `obfs.char`。

Scope规则：

1. `scope.formats`只允许大小写敏感的 `openai`、`openai-response`、`claude`、`gemini`、`interactions`。
2. `scope.roles`只允许大小写敏感的 `system`、`developer`、`user`、`assistant`、`tool`。
3. 非string或未知值使整个候选配置无效。
4. 重复scope值在语义上等价于出现一次。
5. `plugin.register`和 `plugin.reconfigure`使用同一套校验。

parser接受已知host-owned键 `enabled`、`priority`、`store`，并拒绝其他未知键，防止拼写错误静默失效。

### 5.3 注册与热更新

`plugin.register`和 `plugin.reconfigure`调用同一个 `parse -> validate -> compile`函数。

- 首次register无效：返回lifecycle error，不发布capability。
- 已有有效snapshot后的reconfigure无效：保留last-known-good snapshot，调用host log记录错误，并返回旧的有效registration。
- reconfigure有效：构造完整immutable snapshot，再执行一次atomic store。
- atomic store成功是配置生效的线性化点；文件写入和150 ms debounce到期都不是生效确认。
- BeforeAuth开始时只load一次snapshot。load发生在线性化点前时可观察旧配置A；发生在线性化点后时必须观察新配置B。
- 一个请求从开始到结束只使用该次load得到的snapshot。
- watcher黑盒测试先轮询一个只被B命中的sentinel请求，直到观察到B，再断言之后启动的请求只使用B。并发测试只允许完整oracle A或完整oracle B。

## 6. Scope和canonical role

### 6.1 Canonical role

| 来源 | Canonical role |
|---|---|
| 顶层instructions、system instruction | `system` |
| 协议role精确等于 `developer` | `developer` |
| user message、CPA已转换的legacy prompt | `user` |
| assistant message、Gemini `model`、Interactions `model_output` | `assistant` |
| OpenAI `role=tool` string content、Claude typed text tool result | `tool` |

`assistant`只指下一轮请求中携带的assistant历史文字。启用该role不会让插件读取实时assistant响应。

### 6.2 Selector通用规则

1. 本节所有path都相对于 `RequestInterceptRequest.Body`的顶层JSON object。
2. `role`、`type`和其他discriminator按大小写敏感的精确字符串比较。
3. 未知role、未知type、缺少本节要求的discriminator、candidate parent不是object或leaf不是JSON string时，该candidate或subtree no-op。
4. 合法JSON中的candidate类型错误不会终止整个请求。
5. Selector只进入表中列出的leaf，不递归扫描其他string，也没有fallback walker。
6. 每个eligible row都必须有公开RPC dispatcher正例。该fixture同时包含该格式全部hard exclusions和一个unknown typed node，并断言只有表中列出的raw JSON string token变化。

### 6.3 硬排除

以下内容不受scope控制，始终排除：

- JSON key和协议discriminator。
- tool call、tool schema、tool name、tool ID和arguments。
- reasoning、thinking、thought和redacted thinking。
- Responses `function_call_output`、`custom_tool_call_output`，即使 `output`是string。
- Claude `tool_result.content` string或object。
- Gemini `functionResponse`。
- Interactions `function_result`和其他tool data。
- model name、ID、metadata和control field。
- image、audio、video、file、URL和base64。
- 上传二进制、multipart part及其header、filename和boundary。
- 未知item、content block和event type。
- 所有response、SSE chunk和server WebSocket event。

## 7. SourceFormat selector

### 7.1 `openai`

| JSON path | Parent条件 | Leaf type | Canonical role | 所需scope role |
|---|---|---|---|---|
| `messages[i].content` | `messages[i].role`精确为 `system`、`developer`、`user`或 `assistant` | string | 与role同名 | 对应role |
| `messages[i].content[j].text` | message role精确为 `system`、`developer`、`user`或 `assistant`；part `type == "text"` | string | 与message role同名 | 对应role |
| `messages[i].content` | `messages[i].role == "tool"` | string | `tool` | `tool` |

未知或缺失message role、role=`tool`的array content以及其他part type全部no-op。

Legacy Completions没有额外raw `prompt` selector。该handler在调用hook前已把prompt转换为 `messages[0].content`和 `role=user`，插件只处理转换后的path。[`sdk/api/handlers/openai/openai_handlers.go:L182-L208`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/api/handlers/openai/openai_handlers.go#L182-L208)

### 7.2 `openai-response`

| JSON path | Parent条件 | Leaf type | Canonical role | 所需scope role |
|---|---|---|---|---|
| `instructions` | 顶层field | string | `system` | `system` |
| `input` | 顶层field | string | `user` | `user` |
| `input[i].content` | item `type`缺失、空字符串或精确为 `message`；item `role`精确为 `system`、`developer`、`user`或 `assistant` | string | 与item role同名 | 对应role |
| `input[i].content[j].text` | item满足上一行；content part `type`缺失、空字符串或精确为 `input_text` | string | 与item role同名 | 对应role |
| `input[i].content[j].text` | item `type`缺失、空字符串或精确为 `message`；item `role == "assistant"`；part `type == "output_text"` | string | `assistant` | `assistant` |

item缺少role时no-op。`output_text`只允许在assistant message下出现。任何其他item或part type全部no-op，包括所有function/custom-tool input和output。

Responses WebSocket model-executed turn进入hook前已完成CPA normalization，但仍使用该表相对于当前execution body选择span。[`internal/translator/openai/openai/responses/openai_openai-responses_request.go:L166-L217`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/translator/openai/openai/responses/openai_openai-responses_request.go#L166-L217)

### 7.3 `claude`

| JSON path | Parent条件 | Leaf type | Canonical role | 所需scope role |
|---|---|---|---|---|
| `system` | 顶层field | string | `system` | `system` |
| `system[i].text` | block `type == "text"` | string | `system` | `system` |
| `messages[i].content` | message `role`精确为 `system`、`user`或 `assistant` | string | 与message role同名 | 对应role |
| `messages[i].content[j].text` | message role精确为 `system`、`user`或 `assistant`；block `type == "text"` | string | 与message role同名 | 对应role |
| `messages[i].content[j].content[k].text` | message `role == "user"`；outer block `type == "tool_result"`；inner block `type == "text"` | string | `tool` | `tool` |

`tool_result.content`为string、object或非text array时全部no-op。`tool_use`、thinking、redacted thinking、image和document source始终排除。[`internal/translator/openai/claude/openai_claude_request.go:L149-L223`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/translator/openai/claude/openai_claude_request.go#L149-L223)

### 7.4 `gemini`

| JSON path | Parent条件 | Leaf type | Canonical role | 所需scope role |
|---|---|---|---|---|
| `systemInstruction.parts[i].text` | part不含machine discriminator，`thought`不是boolean true且无 `thoughtSignature` | string | `system` | `system` |
| `system_instruction.parts[i].text` | 与上一行相同 | string | `system` | `system` |
| `contents[i].parts[j].text` | content `role`缺失或精确为 `user`；part满足system part的排除条件 | string | `user` | `user` |
| `contents[i].parts[j].text` | content `role == "model"`；part满足system part的排除条件 | string | `assistant` | `assistant` |

machine discriminator包括 `functionCall`、`functionResponse`、`inlineData`、`inline_data`、`fileData`、`file_data`、`executableCode`和 `codeExecutionResult`。同一part包含任一machine discriminator时，即使也包含string `text`，整个part仍no-op。content role存在但不是 `user`或 `model`时，整个content no-op。[`internal/translator/openai/gemini/openai_gemini_request.go:L132-L265`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/translator/openai/gemini/openai_gemini_request.go#L132-L265)

### 7.5 `interactions`

System instruction rows：

| JSON path | Parent条件 | Leaf type | Canonical role | 所需scope role |
|---|---|---|---|---|
| `system_instruction` | 顶层field | string | `system` | `system` |
| `system_instruction.text` | `system_instruction`为object | string | `system` | `system` |
| `system_instruction.parts[i].text` | part `type`缺失、空字符串或精确为 `text`，且不含Gemini machine discriminator | string | `system` | `system` |
| `input` | 顶层field | string | `user` | `user` |
| `input[i]` | array element自身为string | string | `user` | `user` |

其余input使用以下递归item合同：

1. 根input item的继承role为 `user`。
2. object `role`缺失时继承；精确为 `user`时解析为 `user`；精确为 `model`或 `assistant`时解析为 `assistant`；其他role使整个item subtree no-op。
3. `steps[]`中的item递归继承包含它的object所解析出的role。
4. item `type`缺失、空字符串或精确为 `user_input`时，使用解析出的role。
5. item `type == "model_output"`时，强制canonical role为 `assistant`。
6. item `type == "thought"`、`function_call`、`function_result`或任何其他非空type时，整个item no-op。
7. 允许的文字leaf只有：item `content` string；`content[]`或content object中part `type`缺失、空字符串或精确为 `text`的string `.text`；`parts[]`中满足相同type条件且不含Gemini machine discriminator的string `.text`。

每个leaf所需scope role等于递归解析出的canonical role。Interactions translator接受string、array、steps、user_input和model_output等shape；插件只采用上述严格子集，不复制其对unknown type的宽松fallback。[`internal/translator/gemini/interactions/interactions_gemini_common.go:L704-L773`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/translator/gemini/interactions/interactions_gemini_common.go#L704-L773) [`internal/translator/gemini/interactions/interactions_gemini_common.go:L979-L1019`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/translator/gemini/interactions/interactions_gemini_common.go#L979-L1019)

### 7.6 未知格式和类型错误

未知 `SourceFormat`立即no-op。已知格式中的未知role、未知typed node、缺失必需discriminator和candidate leaf类型错误只跳过对应candidate或subtree，不终止整个合法JSON请求。

## 8. Mode语义

### 8.1 共同定义

1. `words`按YAML sequence顺序编号。
2. eligible nodes按当前execution body中原始JSON string token的byte offset升序排列。
3. 匹配在decoded string value上执行，不匹配JSON escape拼写。
4. 不跨node匹配。
5. 同一规则的occurrence按左到右、非重叠定义。
6. 所有规则只运行一轮，不运行到fixpoint。
7. 后一规则读取前一规则产生的当前node text。

### 8.2 `block`

参考语义：

```go
for _, word := range words {
	for _, node := range nodes {
		if strings.Contains(node.Text, word) {
			return blocked(word, node.Role)
		}
	}
}
```

词表顺序优先于document order。对获胜词条，同词多处出现时，最早eligible node决定role。命中后停止，不修改请求body。

HTTP 400 body固定为：

```json
{
  "error": {
    "type": "invalid_request_error",
    "code": "censorship_blocked",
    "message": "request blocked by censorship rule",
    "term": "matched word",
    "role": "user"
  }
}
```

`Content-Type`为 `application/json`。`term`和 `role`使用标准JSON string escaping。不回显原消息、相邻文本、path或完整配置。

### 8.3 `strip`

参考语义：

```go
for _, word := range words {
	for i := range nodes {
		nodes[i].Text = strings.ReplaceAll(nodes[i].Text, word, "")
	}
}
```

一条规则必须处理全部nodes中的全部非重叠occurrence，然后才进入下一条规则。所有规则必须执行。

### 8.4 `obfs`

`obfs`采用与 `strip`相同的rule-major和all-nodes顺序。每个match替换为：

```plaintext
word的第一个Unicode scalar
+ obfs.char
+ word剩余Unicode scalars
```

每个match只插入一个字符。插入位置按Unicode scalar boundary计算，不按UTF-8 byte index。算法不使用随机数、轮换、时间、进程状态或conversation state。

## 9. 确定性与body重建

合同只要求确定性，不要求幂等性：

```plaintext
Transform(body, snapshot, pluginVersion)
```

对相同三项输入必须产生相同返回类型、相同status、相同headers和相同body bytes。

实现步骤和优先级：

1. load一次snapshot。
2. `SourceFormat`未知、`words`为空、`scope.formats`显式为空、当前格式未启用或 `scope.roles`显式为空时，立即no-op且不解析body。
3. 其余已启用的已知格式要求顶层为合法JSON object。JSON syntax错误或合法但非object的顶层值返回400 `censorship_invalid_request`。
4. 用显式selector收集 `{rawStart, rawEnd, decodedText, canonicalRole}`。
5. 按 `rawStart`排序并验证span不重叠。
6. 对内存中的node text执行mode逻辑。
7. no-op时不构造新body，返回空replacement。
8. 只对已变化的JSON string token调用标准JSON encoding。
9. 从后向前替换span，最后只分配一次新body。

Dispatcher tests分别用invalid body覆盖步骤2的每个早退条件，断言no-op；另用启用规则和scope的invalid body断言步骤3返回400。

span外bytes必须保持不变，包括object member顺序、空白、number spelling、未知字段、tool和media subtree。

运行时不使用内容hash cache。SHA-256只用于测试和发布artifact校验：

- 同一转换重复100次，完整bytes和SHA-256相同。
- 并发请求在同一snapshot下产生相同hash。
- 配置热更新后输出可以有意改变。

## 10. 错误处理

| 条件 | 结果 |
|---|---|
| 命中 `block` | HTTP direct termination 400和固定error body。 |
| 已启用的已知SourceFormat，且顶层不是合法JSON object | direct termination 400，code=`censorship_invalid_request`，不回显body。 |
| 未知SourceFormat或第9节步骤2任一早退条件成立 | no-op，不解析body。 |
| 合法body中的未知typed node | 跳过该node。 |
| 首次配置无效 | plugin register失败。 |
| 热更新配置无效 | 保留last-known-good并记录日志。 |
| plugin RPC error、panic或host fuse | 沿用CPA现有fail-open行为，插件无法改变。 |

HTTP direct response保留plugin指定status和body。[`sdk/api/handlers/handlers_errors.go:L78-L154`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/api/handlers/handlers_errors.go#L78-L154)

Responses WebSocket已经upgrade。该路径只能收到 `status:400` error event后关闭，并且当前CPA会丢弃plugin自定义 `term`和 `role` body。这是已接受限制，不通过错误声明或header绕过。

## 11. 性能设计

定义：

- `N`：完整body bytes。
- `B`：eligible decoded text bytes总量。
- `E`：eligible node数量。
- `R`：词条数量。
- `K`：match数量。

### 11.1 Selector

JSON扫描为 `O(N)`。selector不decode排除的base64内容。

每次BeforeAuth或AfterAuth call都会JSON编码一个包含完整 `Body []byte`的RPC request；`encoding/json`把该字段编码为base64。Unix loader随后通过 `C.CBytes`再复制整份已编码request，Windows loader直接传Go buffer指针。[`sdk/pluginapi/types.go:L994-L1016`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/pluginapi/types.go#L994-L1016) [`internal/pluginhost/rpc_schema.go:L89-L92`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/pluginhost/rpc_schema.go#L89-L92) [`internal/pluginhost/rpc_client.go:L488-L503`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/pluginhost/rpc_client.go#L488-L503) [`internal/pluginhost/loader_unix.go:L165-L189`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/pluginhost/loader_unix.go#L165-L189) [`internal/pluginhost/loader_windows.go:L253-L296`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/internal/pluginhost/loader_windows.go#L253-L296)

每次handler execution在进入AuthManager前调用一次BeforeAuth。每个实际开始的credential/model attempt在auth准备成功后调用一次AfterAuth；没有可执行auth/model时AfterAuth为0，fallback时可大于1。[`sdk/cliproxy/auth/conductor_execution.go:L420-L502`](https://github.com/router-for-me/CLIProxyAPI/blob/81e1b5374f99c212f196f34956eeed964a46b8fa/sdk/cliproxy/auth/conductor_execution.go#L420-L502)

完整ABI benchmark必须报告GOOS、loader路径以及观测到的BeforeAuth/AfterAuth call数。

### 11.2 Matcher

首版使用标准库：

- `block`：ordered `strings.Contains`，最坏 `O(R × B)`。
- `strip/obfs`：ordered `strings.ReplaceAll`语义，复杂度取决于每条规则开始时的当前文本总量。

不使用combined regex，因为它会改变规则顺序和重叠优先级。

只有benchmark证明代表性 `block`场景不满足实际预算时，才评估保留原YAML index的Aho-Corasick。任何优化都必须继续通过reference oracle、differential fuzz和byte-preservation tests。

### 11.3 资源测试矩阵

- body：`1 KiB`、`1 MiB`、`20 MiB`。
- words：`0`、`1`、`32`、`256`、`1024`。
- nodes：`1`、`1000`。
- match：none、sparse、dense、overlap、ordered cascade。
- excluded payload：20 MiB base64位于目标文字前、中、后。
- concurrent reconfigure：A/B snapshots并行。

报告 `ns/op`、`B/op`和 `allocs/op`。设计阶段不虚构性能SLO。

## 12. TDD计划

每个slice严格执行一个 RED test、最小GREEN实现、当前全套测试。不得先写全部测试再批量实现。

### Slice 1：ABI registration和空配置

- RED：调用公开RPC dispatcher，断言只声明 `RequestInterceptor`；response、stream和management capabilities均为false；空words对invalid body仍no-op。
- GREEN：实现最小ABI、dispatcher、registration、empty snapshot和第9节步骤2的早退。
- 后续单独RED：未知format、空formats、当前format disabled和空roles分别对invalid body no-op；启用的已知format加非空words时，invalid或非object JSON返回400。

### Slice 2：OpenAI `block`

- RED：`words=["ab","a"]`，更早user node只含 `a`，更晚developer node含 `ab`。断言400、`term=ab`、`role=developer`。
- GREEN：实现OpenAI string selector和朴素block oracle。

### Slice 3：`strip`全量处理

- RED：三条user messages共出现五次同词，断言一次删除全部五处。
- GREEN：实现mutable nodes和 `strings.ReplaceAll`。
- 后续单独RED：固定有序cascade和非重叠语义。

### Slice 4：`obfs`和确定性

- RED：Unicode、多nodes、多occurrence；断言每个match在固定位置插入一次。
- GREEN：实现Unicode scalar边界replacement。
- 后续单独RED：同一input执行100次，body bytes和SHA-256全部相同；单scalar word使candidate config无效。

### Slice 5：热更新

- RED：直接调用reconfigure，把配置A切换到B；在线性化点后的下一请求只使用B。
- GREEN：共用parser和atomic immutable snapshot。
- 后续单独RED：非法B保留A；并发request/reconfigure只观察完整A或B；`go test -race`通过。
- watcher黑盒RED：写入B后轮询只被B命中的sentinel，观察到B后再启动普通请求并断言只使用B；不使用固定sleep推断reload完成。

### Slice 6：Raw span和硬排除

- RED：OpenAI fixture同时包含第7.1节每一行eligible leaf、tool call、schema、JSON key、assistant默认排除、unknown typed node和20 MiB base64。断言只有目标string token变化。
- GREEN：实现gjson span收集和一次final rebuild。

### Slice 7：可配置 `assistant`和 `tool`

- RED：显式启用assistant后只修改assistant文字，不修改tool call和thinking。
- GREEN：增加canonical assistant映射。
- 后续单独RED：OpenAI只处理 `role=tool`的string content；Claude只处理 `tool_result.content[]`内typed text block。Claude string/object tool_result、Responses function outputs、Gemini functionResponse和Interactions function_result始终不变。

### Slice 8：协议selector

按以下顺序分别执行独立RED -> GREEN：

1. `openai-response`
2. `claude`
3. `gemini`
4. `interactions`

每个格式为第7节表中的每一类eligible row增加公开dispatcher正例；同一fixture包含该格式全部hard exclusions、unknown role、unknown type、缺失discriminator和错误leaf type，断言只有规范表列出的raw string token变化。每次只增加当前格式的明确路径，不提前创建通用递归walker。

### Slice 9：Responses WebSocket子集

- RED：固定CPA checkout的Gorilla WebSocket harness发送model-executed `response.create`，断言strip/obfs后的execution body。
- GREEN：不增加新capability，复用 `openai-response` selector。
- 回归：prewarm和Realtime保持记录为已接受的uncovered contract；block只能断言status 400 event和close。

### Slice 10：输出与流式透明性

- RED 1：使用不命中规则的请求和固定mock upstream，比较启用与禁用插件时的status、headers和完整response写序列。
- RED 2：先断言命中 `strip`或 `obfs`时mock upstream收到转换后的input，再向启用与禁用两条转发路径注入同一组预录status、headers和chunks，比较输出写序列。不同input产生的真实模型输出不要求相同。
- GREEN：无需response实现；registration保持request-only。
- HTTP/SSE harness强制HTTP/1.1，关闭HTTP/2和压缩，解析raw chunked frames并比较chunk payload与flush顺序，不比较 `resp.Body.Read`的偶然分段。
- Responses WebSocket按message opcode、payload和顺序比较。

### Slice 11：Differential和fuzz

- `FuzzRuleEngineAgainstOracle`：Unicode、重复词、overlap、cascade、nodes和roles。
- `FuzzProtocolTransform`：escaped JSON、unknown blocks、tool/media/base64、invalid JSON和deep input。
- 断言：无panic/overflow；block term/role一致；excluded spans不变；输出JSON有效。

### Slice 12：Benchmark

先记录标准库实现和完整动态ABI baseline。只优化明确的copy和allocation。只有数据证明需要时才增加block多模式matcher。

### Slice 13：Release packager和CI/CD

- RED：packager test断言七个平台matrix、zip根目录library名称、可选LICENSE、0755 mode和sha256sum格式。
- GREEN：实现最小packager、Makefile targets和GitHub Actions workflow。
- 后续单独RED：`vX.Y.Z` tag、`VERSION=vX.Y.Z`和 `-version vX.Y.Z`三种CLI入口生成相同、不含前导 `v`的archive basename。

## 13. 已接受的纯插件限制

1. hook不是raw ingress，document order按当前execution body span定义。
2. ModelRouter和部分handler预处理可以先看到未审查内容。
3. Responses WebSocket只覆盖进入model execution的turn。
4. `generate=false`本地prewarm不进入plugin。
5. `/v1/realtime`、Live、sideband和DataChannel不进入plugin。
6. Alpha Search不进入plugin。
7. Responses WebSocket block无法携带自定义 `term`和 `role`，只能产生status 400 error event和close。
8. 当前RequestInterceptor error、panic或fuse为fail-open。
9. 每次handler execution调用一次BeforeAuth；AfterAuth按实际credential/model attempt调用0次、1次或多次。每次调用都携带完整body，AfterAuth no-op不能消除相应body编码成本。
10. Home模式直接修改本地YAML不会触发reconfigure。
11. 未知SourceFormat和未来新增content type默认不审查，升级CPA基线时必须review schema drift。

这些限制必须写入README和release notes，不使用skipped test或模糊措辞宣称完整覆盖。

## 14. GitHub CI/CD

CI/CD模仿 [`DoingDog/cpa-plugin-model-mapper@41e5591`](https://github.com/DoingDog/cpa-plugin-model-mapper/commit/41e559193126428ba4a753c83d79e640e06c0951) 的结构，并将插件名改为 `censorship`。

- 参考workflow：[`build.yml:L1-L271`](https://github.com/DoingDog/cpa-plugin-model-mapper/blob/41e559193126428ba4a753c83d79e640e06c0951/.github/workflows/build.yml#L1-L271)
- 参考Makefile：[`Makefile:L1-L69`](https://github.com/DoingDog/cpa-plugin-model-mapper/blob/41e559193126428ba4a753c83d79e640e06c0951/Makefile#L1-L69)
- 参考packager：[`package-release.go:L1-L260`](https://github.com/DoingDog/cpa-plugin-model-mapper/blob/41e559193126428ba4a753c83d79e640e06c0951/.github/scripts/package-release.go#L1-L260)

### 14.1 Trigger

```yaml
on:
  pull_request:
  push:
    branches: [main]
    tags: ["v*"]
  workflow_dispatch:
```

### 14.2 Test gate

Ubuntu test job依次执行：

```plaintext
make test
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
make race
make vet
make integration
```

`make integration`固定校验CLIProxyAPI checkout SHA为 `81e1b5374f99c212f196f34956eeed964a46b8fa`，再运行HTTP、SSE和Responses WebSocket黑盒测试。

### 14.3 Build matrix

| GOOS | GOARCH | Runner/方式 |
|---|---|---|
| linux | amd64 | `ubuntu-24.04` |
| linux | arm64 | `ubuntu-24.04-arm` |
| darwin | amd64 | `macos-15-intel` |
| darwin | arm64 | `macos-15` |
| windows | amd64 | `windows-2025`加MSYS2 MinGW |
| windows | arm64 | `go-cross/cgo-actions@v1` |
| freebsd | amd64 | `go-cross/cgo-actions@v1` |

PR只运行test gate。main push和tag在test通过后构建全部平台。每个build job执行：

```plaintext
make package VERSION=0.0.0-dev GOOS=<该行GOOS> GOARCH=<该行GOARCH>
```

Tag build使用tag解析出的release version代替 `0.0.0-dev`。`make build`只作为host-specific便捷target，不是共享完成门槛；调用者必须自行提供当前host所需的CGO compiler和cross-compiler。

### 14.4 Artifact

动态库名称：

```plaintext
censorship.dll
censorship.so
censorship.dylib
```

archive名称：

```plaintext
censorship_<version>_<goos>_<goarch>.zip
```

`vX.Y.Z` tag、`VERSION=vX.Y.Z`和packager `-version vX.Y.Z`都先去除一个前导ASCII `v`，统一生成 `censorship_X.Y.Z_<goos>_<goarch>.zip`。CLI-level regression覆盖这三种入口。

zip根目录只包含动态库和可选 `LICENSE`。每个平台生成 `<archive>.sha256`，格式为：

```plaintext
<hex sha256><two spaces><archive basename>
```

### 14.5 GitHub Release

`v*` tag触发release job：

1. 下载并合并全部build artifacts。
2. 对各 `.sha256`排序后生成 `checksums.txt`。
3. 已有release时使用 `gh release upload --clobber`。
4. 不存在时使用 `gh release create --verify-tag`。
5. 上传全部zip和 `checksums.txt`。

## 15. 完成标准

本地和CI必须通过：

```plaintext
go test ./...
go test -race ./...
go vet ./...
go test -run '^$' -bench . -benchmem
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
make integration
```

第14.3节七个build job还必须分别完成对应平台的 `make package`，并成功产出zip和sha256。`make build`不作为共享门槛。

行为验收：

1. 默认mode为block，无words时no-op且没有内置词。
2. block按词表优先级返回正确term和role。
3. strip/obfs单条规则处理所有nodes的全部非重叠occurrence。
4. strip/obfs执行到最后一条rule。
5. assistant历史只有显式配置后处理。
6. `tool`显式启用后，只处理OpenAI `role=tool` string content和Claude typed text tool result；其他tool/function output保持不变。
7. tool call、machine JSON、binary和base64保持不变。
8. 五种SourceFormat的每个规范selector row都有正例，unknown和hard exclusions保持不变。
9. 相同输入重复转换的bytes和SHA-256相同。
10. 合法热更新无需重启，非法热更新保留旧snapshot。
11. response-side capabilities全部为false。
12. 不命中请求在启用与禁用插件时得到相同response写序列；给定相同预录mock输出时，命中strip/obfs的转发路径也保持相同status、headers、payload和flush/message顺序。
13. GitHub Actions七个平台artifact名称、zip内容和checksum格式符合合同。
14. 所有已接受限制在README中逐项列出。

## 16. 已确认决策

1. 不修改CLIProxyAPI，只开发纯插件。
2. Codex WebSocket支持范围为现有hook可见的Responses WebSocket model-executed turns。
3. `scope.roles: [tool]`显式启用时，只覆盖OpenAI `role=tool` string content和Claude `tool_result.content[]`中的typed text block；其他tool/function result始终排除。
4. assistant历史通过 `scope.roles: [assistant]`显式启用，默认关闭。
5. 插件无论scope如何都只处理输入，永远不处理上游输出。
6. 匹配采用大小写敏感literal，不做normalization。
7. obfs使用单个配置字符，默认U+200B，固定插在第一个Unicode scalar后。
8. obfs拒绝单Unicode scalar词条。
9. 热更新合同只覆盖非Home本地配置模式。
10. 只要求相同输入和配置下结果确定，不要求幂等。
11. CI/CD在首个实现周期内完成，并模仿model-mapper的test、build、package、checksum和GitHub Release流程。
