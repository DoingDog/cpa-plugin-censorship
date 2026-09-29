# v0.3.6 插件功能修复与精确重写预检：设计规格

## 目标与边界

本轮只改 censorship 插件。成功标准是修复两条有具体输入的请求处理缺陷，并将已实测的精确规则全未命中慢路径接入生产预检；不修改 CLIProxyAPI 源码、native ABI v1、RPC schema v5、`C.GoBytes`、配置格式、请求筛选语义或响应处理能力。初始版本为 `v0.3.5`，锁定依赖 `CLIProxyAPI/v7 v7.2.152`。

并行审查覆盖本仓库的插件源码、测试、集成、构建发布脚本，以及 CPA 与插件相接的 ABI／请求钩子／Gemini 转发代码和相关官方请求规范。无证据表明 Claude `search_result.title` 为空时上游必然拒绝，因此不改该字段；不添加通用 JSON 字符串遍历器，也不优化未经测量的路径。

## 1. Responses 回放的 `output_text.annotations`

OpenAI [Responses API 请求规范](https://developers.openai.com/api/reference/resources/responses/methods/create/)定义 assistant `output_text` 的带索引 annotations；[conversation-state 指南](https://developers.openai.com/api/docs/guides/conversation-state)允许把 `response.output` 回送到下一次 `input`。当前 `collectOpenAIResponses` 对 assistant `input[].content[]` 中 `type: "output_text"` 的 `text` 创建普通可改写 span，却原样保留同一对象的 `annotations`。例如 `text: "SECRET citation"` 的引用起点为 7，`strip: [SECRET]` 后文本为 `" citation"`，引用索引失效。

仅当该 `output_text` part 的 `annotations` 为非空数组时，把已选中的 text span 标记为 `RequiresUnmodified`，复用现有最终变更校验和本地 `censorship_invalid_request` 路径；错误说明应明确为 annotated text。`block` 仍按原规则命中并返回本地拒绝；`strip`／`obfs` 只在最终文本真的改变时本地拒绝，不将错误索引发送给上游。最终文本与原文相同（包括多阶段抵消）时允许请求通过；空数组、字段缺失或非 `output_text` 文本保留现有行为。只处理明确的 assistant 历史输出形态，绝不重算未知 annotation 类型的索引。

验收：聚焦测试证明带注释的 `strip`／`obfs` 原本会改写而修复后本地拒绝；验证 `block`、空注释、角色门控、未命中、最终抵消和引用旁边的其他字段逐字节不变。

## 2. Gemini `function_declarations` 文本漏选

[Gemini generateContent REST 示例](https://ai.google.dev/api/generate-content)直接使用 `tools[].function_declarations`，[ProtoJSON 规范](https://protobuf.dev/programming-guides/json/)允许原始 proto 字段名及其 lowerCamelCase JSON 名。插件现在只读 `functionDeclarations`。锁定 CPA 的 Gemini 入站钩子读取原始请求体，Gemini->Gemini translator 也会把 camelCase 转为 snake_case 并保留已为 snake_case 的字段（`internal/translator/gemini/gemini/gemini_gemini_request.go:32-71`），因此不能依靠宿主替插件补查。

在现有 `collectGemini` 工具声明分支同时读取 `functionDeclarations` 和 `function_declarations`，将每个声明明确的 `description` 及现有 schema `description` 叶子按 `developer` scope 检查。既有 `parameters`、`response`、`parametersJsonSchema`、`responseJsonSchema` 保持支持，并对后两种 JSON schema 的原始 proto 拼写 `parameters_json_schema`、`response_json_schema` 选取同一组明确的 `description` 叶子。仅修改被选中的文本字符串；`name`、schema key、enum、参数值、媒体数据及签名一律不改。若请求同时包含两个不同拼写的声明数组，分别检查各自的明确文本路径；不归一化、重排或重写 JSON 键。

验收：官方 snake_case 声明里的命中词在 `developer` scope 下可 block 和局部 strip；snake_case JSON schema 只改 `description`；`user`-only scope 不检查声明；机器字段与未选中的 JSON 字节不变；camelCase 原有测试继续通过。

## 3. 大规则集精确重写全未命中慢路径

`ignore_case: false` 时现有 `applyMode` 对每条 strip／obfs 规则逐个扫描每个文本 span。用现有 `BenchmarkExactRewritePreflight` 重测 1024 条规则、20 MiB 文本全未命中，基线三次为 542–616 ms，已有 `byteMatcher` 预检策略为 42–45 ms；密集命中的三次值分别约为 58–84 ms 和 71–84 ms。这些是重写微基准，不宣称端到端 CPA 延迟改善。

复用 `byteMatcher`，在不可变 `configSnapshot` 编译阶段仅为达到现有大规则阈值的精确 strip／obfs 合并规则建立一次匹配器。`applyMode` 在 block 阶段之后，对达到文本字节阈值的 span 做一次预检：该 span 的所有精确改写规则在**原始 span 文本中都未命中**时，跳过它的全部 strip／obfs 扫描；任何规则命中则严格执行原来的顺序重写，保留级联、重复规则及最终抵消语义。预检必须逐 span 独立，短文本、小规则集、`ignore_case: true`、block 优先级及现有 `SkipFoldRewrite` 标记不变。不得在热请求中重新编译匹配器，避免为未命中预检引入输入体大小的复制。

验收：先用失败测试锁定编译阈值和生产预检使用路径；对未命中／首中／尾中／密集命中、跨 span、重复规则、strip->obfs 级联、最终抵消及混合 block/strip/obfs 比较结果与旧顺序算法；复跑全未命中、稀疏与密集命中微基准，确认产线路径的收益且无明显命中回退。若测量不支持接入，则只保留两项功能修复，不将未经证实的优化作为发布声明。

## 测试、兼容性与发布门槛

按 TDD 为每个缺陷先补能在当前版本失败的聚焦测试，观察失败后实施最小修复。用现有 `interceptRPC`/配置测试和集成 harness 验证插件加载、注册、HTTP 与 WebSocket 请求过滤；对 Gemini snake_case 与 Responses annotated replay 增加能够观察本地拒绝和零上游调用的集成断言，不改 CPA 本体。运行 `make test`、`make race`、`make vet`、`make integration`、`make build`、`make package` 及相关包装脚本测试，并检查 release ZIP、匹配的 lowercase SHA-256 sidecar 和 `checksums.txt`。本地 Windows 环境通过 PowerShell 设置 `GOPATH`、`GOMODCACHE` 运行 make，避免 Git Bash 的 Windows make 子进程丢失环境变量。

审查实现与测试后合并功能分支到本地 `main`，更新 `RELEASE_NOTES.md` 为 `v0.3.6`，以 `v0.3.6` tag 推送到 GitHub，观察 `build.yml` 的 test、各目标构建和 release 状态。若实际测试推翻某候选，先更新本规格和实施计划，不发布无法证明的改动。整个过程中不手改 `.integration/` 或 `dist/` 生成文件。
