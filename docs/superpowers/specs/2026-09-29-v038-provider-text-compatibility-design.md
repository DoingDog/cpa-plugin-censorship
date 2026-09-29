# v0.3.8 Provider Text Compatibility Design

## 目标与边界

修复固定 CLIProxyAPI v7.2.152 请求路径中已经定位的文本漏检与 Gemini 角色错位，使 `block`、`strip`、`obfs` 只作用于实际可见、明确指定的自然语言文本。保持插件 native ABI v1、配置 schema 5、阶段选择、过滤器以及 `block` -> `strip` -> `obfs` 顺序不变。不修改 CPA core、固定 Go 依赖、生成的 `.integration/` 或 `dist/` 文件。

本次扫描的动态复现确认了 Gemini `functionResponse` 的角色状态错位、Gemini 输出 schema `description` 的漏检，以及 Responses 嵌套 `function` 描述透过 CPA 的 Chat 兼容转换到达上游。Claude 旧 `output_format` 的可达性由官方文档和固定 CPA 的直通代码确认，但隔离探针被权限护栏拒绝，没有声称已动态执行；实施前必须先用正常的仓库 TDD 测试观察预期 RED，若不能复现则不实施该子项，也不在发行说明宣称已修复。

## 请求路径与设计

### Gemini 缺省角色的 `functionResponse`

`selectors_gemini.go` 当前将缺失、`null` 或空 `role` 一律按上一条 user/model 交替推断。固定 CPA 的 Gemini 转换器先检查 `parts[*].functionResponse` 或 `function_response`，存在时把缺失或无效 role 推断为 user，并将后续推断的 `previousRole` 更新为 user。例子：显式 user -> 缺 role 的 function response -> 缺 role 的模型历史；插件当前把第三条当 user 并可能错误改写。

仅当 `parts` 数组的 Part 中存在上述两个键时对齐 CPA 状态。缺失、`null`、空 role 的当前条目选 `user`，`previousRole=user`；无效 role 的当前条目仍不选中，但若带 function response，令 `previousRole=user`。其余无效 role 保持现有交替推进；显式 `user`/`model`、普通缺省 role、`functionCall` 和机器 Part 的扫描规则不变。这里的存在性与 CPA 一致，包括键值为 JSON `null`，不是对 function response 内容做全文扫描。现有签名绑定文本的阻断与禁止改写继续有效。

### Gemini 输出 schema 描述

[Google 的 GenerateContent structured-output 文档](https://ai.google.dev/gemini-api/docs/generate-content/structured-output)和[API 定义](https://ai.google.dev/api/generate-content)明确列出 `generationConfig.responseSchema`、`generationConfig.responseJsonSchema` 与 `generationConfig.responseFormat.text.schema`。前两项属于仍受支持但已弃用的输出格式，后一项为新格式。固定 CPA 对旧 `responseSchema` 可更名为 `responseJsonSchema` 后继续发送。插件当前只读取 Gemini 工具声明和 contents，未读取这些输出 schema。

在 `developer` 范围内，仅从上述三个确切 schema 根复用 `appendJSONSchemaDescriptions`，选取 JSON Schema 中已知位置的字符串 `description`，包括嵌套 property。保留 JSON keys、`type`、`required`、`enum`、`const`、`default`、签名和其他机器值。不要添加递归遍历任意 JSON 字符串，不把 Interactions 的 `generation_config.response_schema` 或未记录的蛇形变体当作新增入口。

### OpenAI Responses 工具字段回退

`collectOpenAIResponsesTool` 对顶层 `tools`、显式合法 role 的 `additional_tools`、`namespace` 子工具和已选中的加载工具使用同一个选择器。目前 function 只取扁平 `description`、`parameters`，但固定 CPA 的 Chat、Claude、Gemini 转换会在扁平描述为空时读取 `function.description`，并按固定优先级选参数 schema。隔离 HTTP/mock-upstream 已证实顶层工具与 `additional_tools` 中的嵌套描述没有被 `block` 选中，却实际进入上游 Chat 工具定义。

描述的候选只取一个：`tool.description.String() != ""` 时用扁平字段，否则用 `tool.function.description`；最终仍要求字符串类型才能生成 span。参数候选依次为 `parameters`、`parametersJsonSchema`、`input_schema`、`function.parameters`、`function.parametersJsonSchema`，取首个 `Exists()` 字段，包括显式 `null`；只在所取候选为 Object 时选其 JSON Schema `description`。`custom` 仅使用描述回退，不扫描其参数或 grammar。固定 CPA 将缺失/空 `type` 的具名工具视作 function；只在工具有非空扁平或嵌套函数名时让它走相同的 function 分支。保留原本 `output_schema`、namespace、工具结果选择范围以及角色门控，不对同名工具做跨后端统一去重。

### Claude 旧版结构化输出

[Anthropic 迁移文档](https://platform.claude.com/docs/en/models/opus-5-5/migration-guide)仍允许在 `structured-outputs-2025-11-13` beta 下使用 `output_format: {type: "json_schema", schema: ...}`。固定 CPA 的 Claude 入站至默认 caller-owned/API-key Claude 上游路径保留请求体和 beta；插件目前只选 `output_config.format.schema`。正式 RED 确认后，仅在 canonical `system` 范围且 `output_format.type == "json_schema"` 时对该 schema 使用既有 `appendJSONSchemaDescriptions`。不假定 OAuth 或跨提供商转换也保留 beta，不检查 schema 机器字段，不改变新式 `output_config` 路径。

## 明确不纳入的候选

- Responses `role:user` 配 `type:output_text` 及 Interactions `type:user_input,role:assistant` 在固定 CPA 的 OpenAI、Gemini、Claude 后端具有相反的转换角色。无 `filter.models` 时插件只在 BeforeAuth 工作，没有可信 `ToFormat`。全局改角色会错误处理其他后端历史；迁移全部处理到 AfterAuth 又改变认证前阻断时机和单次规则语义。现阶段没有经验证的插件内零回归修法，故保留已发布角色契约，记录为剩余限制。
- 非空 `filter.models` 与其他插件的 opaque terminal Executor 不属于 README 已定义的 selected-auth 后置过滤范围；删除 selected-auth marker 检查会在未选定 auth 时错误套用模型过滤，故不修改。
- Interactions `generation_config.response_schema` 虽可被固定 CPA 某些转换器接收，Google 文档对 Interactions 输出 schema 明确记录的是 `response_format`。本项目只选择文档明确的自然语言路径，故不增加该字段。

## 测试与验收

每个行为修复先写聚焦失败测试并观察正确 RED，再以最小修改 GREEN。Gemini 测试包含缺失、`null`、空、无效 role 的 function response，对比普通交替、`functionCall`、签名保护和机器 Part 排除；schema 测试包含三个正式根、developer-only 门控以及 enum/键名不变。Responses 测试覆盖顶层、`additional_tools`、`namespace`、优先级、空值、非字符串字段和机器字段不变；用已注册插件与 mock 上游确认阻断前零请求。Claude 测试覆盖旧式 schema、新式对照、user-only 不选中及 beta 直通条件。修改所影响的独立 fuzz oracle 和 seed，不放宽已有断言。

通过 `make test`、`make race`、`make vet`、`make build`、`make integration`、`make package` 以及插件注册/ABI smoke；共享 `.integration/run` 相关任务串行运行。只更新 README 的实际新增路径、对应版本的 RELEASE_NOTES，保留 `C.GoBytes` 和 ABI 所有权约定。提交后退出 worktree、合并本地 `main`、发布 `v0.3.8` 并推送触发 GitHub 自动构建；核对远端 CI、发行资产与 SHA-256，未通过的项目必须如实报告，不宣称成功。
