# v0.3.7 Claude 请求描述字段覆盖修复

## 目标与已确认问题

本次只修复 pinned CPA v7.2.152 的 Claude `/v1/messages` 请求中两类明确传给模型、却未被插件选择的自然语言字段。用户配置 `words.block: [SECRET]`、`scope.roles: [system]` 时，安全的 user message 搭配 `tools[].description: "SECRET lookup"`、`tools[].input_schema.properties.query.description: "SECRET query"` 或 `output_config.format.schema.properties.answer.description: "SECRET answer"`，当前 `collectClaude` 只检查 `system`、compaction instructions 和 `messages`，返回空 spans，请求绕过规则。Anthropic [工具定义](https://platform.claude.com/docs/en/agents-and-tools/tool-use/define-tools)说明工具定义会形成模型的特殊 system prompt；[Messages API](https://platform.claude.com/docs/en/api/messages/create)定义 `output_config.format` 的 `json_schema` 和工具 `input_schema`。

CPA 的 Claude handler 将原始请求正文传入模型执行调用，见 pinned `sdk/api/handlers/claude/code_handlers.go:70-93,171-179`；请求拦截发生于 executor 翻译之前。因此这些字段可被插件读取，修复只修改插件，不修改 CPA。

## 行为契约

- 仅在 Claude `scope.roles` 包含 `system` 时，选取顶层 `tools[]` 中用户定义工具的 `description`，以及它的 `input_schema` 对象内明确的 JSON Schema `description` 字符串。工具是具备 `name` 字符串和 `input_schema` 对象的定义；不递归检查工具调用、`input_examples`、机器 schema 值、工具名、属性名、`enum` 或 `const`。
- 仅在 `output_config.format.type` 等于字符串 `json_schema` 且 `scope.roles` 包含 `system` 时，选取其 `schema` 对象内明确的 JSON Schema `description` 字符串；其他格式和畸形结构不扩大选择范围。
- 复用现有 `appendJSONSchemaDescriptions`，保持 `block -> strip -> obfs` 顺序、已有 JSON span 重建和原始字节外字段不变；不新增递归通用字符串扫描或配置项。完整 strip 产生空的可选描述字符串时可保留空字符串，不能修改其他字段。
- `block` 返回本地 `censorship_blocked`、匹配的配置词与规范角色 `system`，不抵达上游；`strip`/`obfs` 仅改写选中字符串，保留请求 JSON 合法性和所有未选字段；`scope.roles` 不含 `system` 时不选择这些字段。

## 验收与不变项

1. 先新增失败的聚焦单元测试，分别覆盖工具说明、嵌套工具参数 schema 说明、输出 schema 说明、system 角色开关、`block` 与 `strip`/`obfs`，并断言工具名、schema 机器字段与无关字符串原样保留。
2. 在 pinned CPA 的真实插件注册和 `/v1/messages` HTTP 集成测试中，分别用只在工具说明与输出 schema 说明出现的禁词请求验证返回本地 400 且 mock upstream 到达数为零；单元与集成测试由红转绿。
3. 运行 `make test`、`make race`、`make vet`、`make build`、`make integration`、`make package`；检查插件注册测试、release 文件和小写 SHA-256；本地完整验证后将修复合入 `main`，更新 patch 版本为 v0.3.7 并推送 tag 触发 GitHub workflow，核对 CI 成功。

## 排除范围

- Query-only API key 的 wildcard 过滤已在 README 明示不支持，且 pinned `RequestInterceptRequest` 只提供经过 CPA hash 的 `caller_scope` 与 Headers，不提供 URL/query 原文；不尝试从 hash 反推出 key，也不修改 CPA。
- 不改 ABI v1、`C.GoBytes`、输出响应处理、其他 provider selector 或无证据的性能路径。发布文档仅记录这次修复与兼容版本，不读写旧 spec/plan 文件。
