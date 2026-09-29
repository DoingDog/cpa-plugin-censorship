# v0.3.8 Provider Text Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 Gemini 角色错位和输出 schema 漏检、OpenAI Responses 嵌套工具描述漏检，并在 RED 确认后修复 Claude 旧输出格式漏检。

**Architecture:** 保留单次请求拦截及现有 `selectTextSpans`、`appendJSONSchemaDescriptions` 路径；在各 provider 的选择器中增加固定协议路径和 CPA 等价的字段选择条件。独立的 Gemini、Responses、Claude 文件可并行实施；共享 `fuzz_test.go`、integration runner、README 和发行文件由集成阶段串行处理。

**Tech Stack:** Go 1.26、CGO native ABI v1、`gjson`、固定 `github.com/router-for-me/CLIProxyAPI/v7 v7.2.152`、Go tests、GitHub Actions。

**Spec:** `docs/superpowers/specs/2026-09-29-v038-provider-text-compatibility-design.md`

## Global Constraints

- 只开发本插件；不修改 CLIProxyAPI core、固定的 `github.com/router-for-me/CLIProxyAPI/v7 v7.2.152` 依赖或 native ABI v1。
- 配置 schema 5、`words` Object 规则、`block` -> `strip` -> `obfs`、现有 `filter.models` 阶段语义与 `cliproxyPluginCall` 内的 `C.GoBytes` 保持不变。
- 只选择明确列出的自然语言文本路径；机器字段、JSON Schema 非 `description` 值、签名、annotations 保护保持不变；不得加通用递归字符串 walker。
- 修改行为先写失败测试并观察正确 RED；普通单元测试可并行，共享 `.integration/run`、ABI smoke 与 `make integration` 必须串行。
- 不读或修改本次任务之前的 spec/plan；不手改 `.integration/`、`dist/`；不使用 windows-mcp 截图或鼠标工具。
- Claude `output_format` 只在新的聚焦单测产生预期 RED 后才实施；隔离探针的权限拒绝不是一次运行结果。

## Review Focus

1. 缺省 role 且 function response 键值为 `null`：按 CPA 的键存在性更新当前及后续角色；Task 1 写 `null` 回归。
2. 无效 role 的 function response 后跟模型历史：不选择无效条目，但也不把下一条错误选为 user；Task 1 写后续文本回归。
3. Responses 顶层描述为数字或空白，以及顶层参数为显式 `null`：字段优先级不得错误扫描备用字段；Task 3 写负例。
4. 新增 schema 中只有 `enum`、属性名或 `default` 命中：不阻断也不改写；Task 2、Task 4 写机器字段回归。
5. 相同 Responses 嵌套定义处于 `additional_tools` 且 role 缺失：保持不选中；Task 3 复用现有 `TestOpenAIResponsesAdditionalToolsSkipsUnsupportedRoles` 并加入嵌套负例。

---

### Task 1: Gemini function response 角色推断

**Files:** Modify `selectors_gemini_test.go:158-218`、`selectors_gemini.go:71-103`。

**Interfaces:** 保持 `collectGemini(root gjson.Result, roles scopeSet, spans *[]textSpan)` 与 `nextGeminiRole(previousRole string) string`；新增仅供其内部使用的 `geminiHasFunctionResponse(content gjson.Result) bool`，供 Task 5 的两个独立 oracle 对齐。

- [ ] **Step 1: 写两个聚焦失败测试。** 用 `interceptRPC(t,"gemini",body)`：在 `scope.roles:[user]`、`words.block:[SECRET]` 下，先显式 user，再缺省/null/空 role 的 `parts:[{"functionResponse":{"name":"f","response":{}}},{"text":"SECRET"}]`（snake_case 另一个子例）应 `Terminate==true`、`error.role=="user"`；在 `words.strip:[SECRET]` 下，先显式 user，再缺省或无效 role 的 function response，最后缺省 role `text:"SECRET model text"` 应无 replacement body。无效 role 的原条目若同时有 `text:"SECRET"`，仍不选中。键值 `functionResponse:null` 也按存在性更新状态。沿用现有 `TestGeminiOmittedRoleFormsAlternateCanonicalRoles` 和 `TestGeminiInvalidRolesAdvanceCanonicalAlternation` 作为普通交替对照。测试骨架：

```go
registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: [user]\n")
body := []byte(`{"contents":[{"role":"user","parts":[{"text":"ask"}]},{"parts":[{"functionResponse":{"name":"f","response":{}}},{"text":"SECRET"}]}]}`)
resp := interceptRPC(t, "gemini", body)
if !resp.Terminate || gjson.GetBytes(resp.ResponseBody, "error.role").Str != "user" {
    t.Fatalf("response = %#v", resp)
}
```

- [ ] **Step 2: 观察 RED。** 运行 `go test . -run '^TestGeminiFunctionResponse' -count=1 -v`；预期缺省 role 的 user 文本未阻断、后续模型文本被错误 strip。测试若编译失败而不是断言失败，应修正测试，不修改生产代码。
- [ ] **Step 3: 最小实现。** 仅对 `parts` 数组里的 Part 检查两种 function response 键是否 `Exists()`；missing/null/empty role 分支存在时令 `previousRole="user"`、`effectiveRole="user"`；default 无效 role 分支存在时仅令 `previousRole="user"` 后继续跳过。其余分支原样。实现的判定如下：

```go
func geminiHasFunctionResponse(content gjson.Result) bool {
    found := false
    parts := content.Get("parts")
    if parts.IsArray() {
        parts.ForEach(func(_, part gjson.Result) bool {
            if part.Get("functionResponse").Exists() || part.Get("function_response").Exists() {
                found = true
                return false
            }
            return true
        })
    }
    return found
}
```

- [ ] **Step 4: 验证 GREEN。** 重跑该测试及 `go test . -run '^TestGemini' -count=1`；观察普通 role 交替、functionCall、不选中的机器 Part 与 signed visible text 仍通过。提交本任务自有文件，例如 `fix: align Gemini function response role inference`。

### Task 2: Gemini 三个输出 schema 路径

**Files:** Modify `selectors_gemini_test.go`、`selectors_gemini.go:32-65`。与 Task 1 共享两个文件，所以 Task 2 紧随 Task 1，由同一个 Gemini 实施者完成，不与 Task 1 并行。

**Interfaces:** 重用 `appendJSONSchemaDescriptions(spans *[]textSpan, schema gjson.Result, role string, roles scopeSet)`；Task 5 更新 oracle。

- [ ] **Step 1: 写失败测试。** 在 `TestGeminiGenerationSchemaDescriptionsUseDeveloperScope` 的表中分别提供以下三个 JSON body，`assertBlockedRole(t,"gemini",body,"developer")`；user-only 设置则不得阻断，`strip` 只更改 `description`：

```go
`{"generationConfig":{"responseMimeType":"application/json","responseSchema":{"type":"object","properties":{"answer":{"type":"string","description":"SECRET"}}}}}`
`{"generationConfig":{"responseMimeType":"application/json","responseJsonSchema":{"type":"object","properties":{"answer":{"type":"string","description":"SECRET"}}}}}`
`{"generationConfig":{"responseFormat":{"text":{"mimeType":"application/json","schema":{"type":"object","properties":{"answer":{"type":"string","description":"SECRET"}}}}}}}`
```

再用仅含 `properties:{"SECRET":...}`、`enum:["SECRET"]`、`default:"SECRET"` 而无 description 的 schema 验证 no-op；文档未覆盖的 `generation_config` 不选中。与现有 `TestGeminiSelectorCanonicalRoles` 比较范围。
- [ ] **Step 2: 观察 RED。** 运行 `go test . -run '^TestGeminiGenerationSchema' -count=1 -v`；预期三个有效 body 均未阻断。
- [ ] **Step 3: 最小实现。** 在 `roles.has("developer")` 的现有块内、处理 tools 之后、检查 user/assistant 提前返回之前，对 `[]string{"responseSchema","responseJsonSchema","responseFormat.text.schema"}` 逐个调用 `appendJSONSchemaDescriptions(spans, root.Get("generationConfig."+key), "developer", roles)`。不得扫描顶层 `responseFormat` 的其他值。
- [ ] **Step 4: 验证 GREEN。** `go test . -run '^TestGemini' -count=1`、`go test ./...`；提交本任务自有文件，例如 `fix: inspect Gemini response schema descriptions`。

### Task 3: OpenAI Responses function 字段优先级

**Files:** Modify `selectors_openai_test.go:551-800`、`selectors_openai.go:166-209`。可与 Task 1/2 和 Task 4 并行，绝不同时编辑 `fuzz_test.go`、integration 或文档。

**Interfaces:** 保留 `collectOpenAIResponsesTool(spans *[]textSpan, tool gjson.Result, role string, roles scopeSet)`；复用 `appendStringSpan` 与 `appendJSONSchemaDescriptions`。Task 5 复制同一字段优先级到两个 oracle。

- [ ] **Step 1: 写失败测试。** 在顶层 `tools` 和显式 `role:"developer"` 的 `input[].additional_tools[].tools` 用 `type:"function"`、有效 `name`/`input`/`model` 和仅含 `function.description:"SECRET description"`、`function.parameters.description:"SECRET schema"` 的 fixture，断言 developer `block` 返回 400；单独用 `strip` 对原始 body 字节断言只将这两个 `description` 改成安全值，保留 `name`、属性键、`enum`、`default`。新增优先级表：description 的顶层缺失、空串、null 使用嵌套；顶层非空、空白、数字、布尔值或 Object 不使用嵌套。参数在 `parameters`、`parametersJsonSchema`、`input_schema`、`function.parameters`、`function.parametersJsonSchema` 中首个存在的路径胜出，即使为 `null` 或非 Object；扁平未提供才扫描 nested。`custom` 仅测描述，缺失 type 的具名 function 测描述和 schema；无名工具、`type:null` 与非法 additional role 跳过。首个断言示例：

```go
registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: [developer]\n")
body := []byte(`{"model":"censorship-integration-model","input":"safe","tools":[{"type":"function","name":"lookup","function":{"description":"SECRET tool","parameters":{"type":"object","description":"SECRET schema"}}}]}`)
resp := interceptRPC(t, "openai-response", body)
if !resp.Terminate || gjson.GetBytes(resp.ResponseBody, "error.role").Str != "developer" {
    t.Fatalf("response = %#v", resp)
}
```

- [ ] **Step 2: 观察 RED。** 运行 `go test . -run '^TestOpenAIResponsesNestedFunction' -count=1 -v`；至少一个嵌套描述断言应因 `Terminate=false` 失败。若无名工具或非字符串顶层对照失败，检查 fixture 是否把不应扫描的文本放进其他既有路径。
- [ ] **Step 3: 最小实现。** 对 `function` 和 `custom`，`description := tool.Get("description")`，仅当 `description.String()==""` 时改用 `tool.Get("function.description")`，最后仍经 `appendStringSpan`。function 的参数循环按五个确切路径取首个 `Exists()` 并调用 `appendJSONSchemaDescriptions` 后停止；原 `output_schema` 路径不动。缺失或字符串空 `type` 仅在 `tool.Get("name").String()` 或 `tool.Get("function.name").String()` 非空时当作 function；`type:null` 不新增选择范围。不要更改 `namespace`、`tool_search`、`mcp`、additional role 的调用结构。关键实现片段：

```go
for _, path := range []string{"parameters", "parametersJsonSchema", "input_schema", "function.parameters", "function.parametersJsonSchema"} {
    schema := tool.Get(path)
    if schema.Exists() {
        appendJSONSchemaDescriptions(spans, schema, role, roles)
        break
    }
}
```

- [ ] **Step 4: 验证 GREEN。** `go test . -run '^TestOpenAIResponses(NestedFunction|ModelVisibleDefinitions|AdditionalTools)' -count=1` 和 `go test ./...`；复查 `TestOpenAIResponsesDefinitionMachineFieldsRemainUnchanged`。提交本任务自有文件，例如 `fix: select effective Responses function descriptions`。

### Task 4: Claude legacy output_format

**Files:** Modify `selectors_claude_test.go:389-458`、`selectors_claude.go:20-35`。可与 Gemini 和 Responses 并行，但不得执行隔离代理曾被拒绝的临时 overlay 命令。

**Interfaces:** 重用 `appendJSONSchemaDescriptions`；新路径仅属 canonical `system`。

- [ ] **Step 1: 为官方仍支持的 beta 格式写测试。** 在 `TestClaudeDescriptionPaths` 表增加 `name: "output_format"`、`output_format.type=json_schema`、`schema.properties.answer.description=SECRET` 的一行；原测试已同时验证 system block 和 user-only no-op。再在 `TestClaudeDescriptionRewrites` 加入旧式 schema 的 description 与 `enum`/属性键不变断言，以及 `output_format.type=text` 不选中的负例。示例 JSON：

```go
`{"output_format":{"type":"json_schema","schema":{"type":"object","properties":{"answer":{"type":"string","description":"SECRET answer","enum":["SECRET"]}}}},"messages":[{"role":"user","content":"safe"}]}`
```

- [ ] **Step 2: 观察 RED 作为能否纳入发行的门槛。** 用普通 `go test . -run '^TestClaudeDescriptionPaths/output_format' -count=1 -v` 运行，预期仅旧式 schema 用例 `Terminate=false` 导致断言失败；若不能得到这个失败，不实施 Step 3，并从 README/RELEASE_NOTES 删去本子项。测试不需替代理运行被拒绝的临时 overlay。
- [ ] **Step 3: 最小实现。** 在已有 system 块、`output_config.format` 判断后，另读取 `root.Get("output_format")`，只在 `.Get("type").Str=="json_schema"` 时调用 `appendJSONSchemaDescriptions(spans, legacy.Get("schema"), "system", roles)`。不要试图从 selector 猜测 header 或后端 profile。
- [ ] **Step 4: 验证 GREEN。** `go test . -run '^TestClaudeDescription' -count=1`、`go test ./...`；提交本任务自有文件，例如 `fix: inspect legacy Claude output schema descriptions`。

### Task 5: 独立 oracle 与协议 seed

**Files:** Modify `fuzz_test.go:1378-1433,1667-1719,2084-2140,2394-2440` 与该文件已有协议 seed 表。只允许一个实施者修改此文件；等待 Tasks 1 到 4 的源文件工作完成，不与这些任务重叠写文件。

**Interfaces:** `oracleGeminiSpans(root []byte, spans *[]oracleProtocolSpan)` 和 `oracleRawGeminiSpans(root *oracleRawValue, spans *[]oracleRawStringToken)`；`oracleAppendOpenAIResponsesTool` 及其 raw 版本。Oracle 保留 `encoding/json`/独立 raw parser，不调用生产 `selectTextSpans` 或 `gjson` 选择器。

- [ ] **Step 1: 向 `TestProtocolOracle...` 的既有 seed 表加入 Gemini 缺省 role function response + 后续模型、三种 generationConfig schema、Responses 嵌套 description/参数的小 fixture。** 在更新 oracle 前运行 `go test . -run '^TestProtocolOracle' -count=1`，预期新样例因 oracle 不识别新增 span 或角色而失败，且非编译错误；若用其他已有 seed 测试函数，精确记录所选函数名。现有 Claude oracle 尚未覆盖既有 `output_config` 描述路径，本次 Claude 旧格式由 Task 4 聚焦测试与 Task 6 HTTP 测试覆盖，不顺手扩大 Claude fuzz oracle。
- [ ] **Step 2: 更新两个 Gemini oracle。** 在各自 contents 遍历开始处，查 `parts` 数组任意元素上 `functionResponse`/`function_response` 的字段存在性，包括 `null`。若 role 缺省、`null` 或空串且发现字段，令 `previousRole="user"`、当前 canonical role=`user`；无效 role 且发现字段时只更新 `previousRole="user"` 然后跳过；普通交替原样。三种 schema 根使用 oracle 现有 `oracleAppendJSONSchemaDescriptions` 和 raw 对应函数，role=`developer`，不读取机器字段。
- [ ] **Step 3: 更新两个 Responses oracle。** 照 Task 3 精确复制 description `.String()` 等价的非空判定和五个参数字段的首个存在优先级，保留 `custom` 仅描述、`output_schema` 与 namespace/role 分支；缺省/空 type 的具名工具按 function。`oracleFirstField` 与 `firstField` 对显式 null 返回存在，不能改成对象类型判定。
- [ ] **Step 4: 验证。** `go test . -run '^TestProtocolOracle' -count=1`、`go test ./...`、`go test . -run '^$' -fuzz '^FuzzProtocolTransform$' -fuzztime=30s`；每次只运行一个 fuzz 目标。提交 `fuzz_test.go`，不修改 CPA 或其他测试 oracle 的既有机器字段规则。

### Task 6: 插件注册与真实 HTTP 路径

**Files:** Modify `integration/http_test.go:373-505` 与 `integration/claude_descriptions_test.go:9-53`；若 Claude Task 4 的 RED 未成立，不改后一个文件。仅在 Tasks 1 到 5 后启动，避免多个进程重建 `.integration/run`。

**Interfaces:** 使用现有 `newMockUpstream(t)`、`startCPA(t, upstream.URL, true, config)`、`postJSON(t, url, body)`、`decodeCensorshipError`、`upstream.arrivalCount()`；`startCPA` 会载入插件注册及固定 CPA binary。

- [ ] **Step 1: 增加 HTTP 回归。** `TestHTTPGeminiFunctionResponseUsesUserScope` 向 `/v1beta/models/censorship-integration-model:generateContent` 发送先前 user 与缺省 role 的 `functionResponse` + 单独 `text:"SECRET"`，断言 400、role user、零 upstream。`TestHTTPGeminiResponseSchemaDescriptionsBlock` 对三个根发送含安全 user 输入及 developer-only block 的 body，断言 400/developer、零 upstream。`TestHTTPResponsesNestedFunctionDescriptionsBlock` 分别经顶层 tools 和显式 developer `additional_tools` 请求 `/v1/responses`，工具含有效名字、nested description/schema，断言 400/developer、零 upstream；另用 strip 检查 mock upstream 收到被改的 descriptions 且机器字段未变。Claude 旧字段放在 `TestHTTPClaudeDescriptionsBlockBeforeUpstream` 新的一行；该测试在拦截阶段断言零 upstream，不将它表述成远端 Anthropic 已接受 beta。若需检查 header 转发，在测试中用 `http.NewRequest`、现有 `integrationHTTPClient` 并设置 `Authorization`、`Content-Type`、`Anthropic-Beta: structured-outputs-2025-11-13`，因为现有 `postJSON` 不能附加自定义 header；不得改生产逻辑来传 header，也不把 OAuth 当已测。HTTP 断言沿用：

```go
response, err := decodeCensorshipError(body)
if err != nil || status != 400 || response.Error.Code != "censorship_blocked" || response.Error.Role != "developer" {
    t.Fatalf("status=%d body=%s error=%v", status, body, err)
}
if upstream.arrivalCount() != 0 {
    t.Fatal("blocked request reached upstream")
}
```

- [ ] **Step 2: 独占验证。** `make integration` 由 runner 重建 plugin DLL、固定 CPA binary 并运行全部 HTTP/ABI tests；不要同时执行其他 `make integration`、`-abi-smoke`、`.integration/run` 写入任务。若阶段基线历史中曾出现 `integration/censorshipplugin/doc.go` 缺失，那是并发暂存目录冲突，应查看实际输出后独占重跑，不把它称作产品 bug。
- [ ] **Step 3: 完成代码 review。** 对每个任务进行 spec 符合性与实现质量独立审查；重点复核 Gemini 角色状态、Responses 字段优先级、签名与 annotations、ABI 指针所有权。只修复可复现且归属于本次改动的高置信问题，每处行为修订再按 RED/GREEN。整体验证 `go test ./...`。

### Task 6A: Responses 完整 strip 激活嵌套描述

**发现与依赖：** Task 5 的独立 fuzz seed 揭露 flat `description:"SECRET"` 经 `strip` 清空时，固定 CPA 的 `.String()==""` 回退选用未处理的 `function.description:"SECRET nested"`。临时插件输出探针已观察到原文残留；先等待 Task 6 实施者交付其 integration 文件，再加 HTTP 回归，不并发写同一文件。此任务仅在已证实边界实施，不改 CPA core 或泛化其他字段。

**Files:** Modify `selectors_openai_test.go`、`selectors_openai.go:187-209`、`transform.go:10-20,56-91`；Task 6 文件提交后再修改 `integration/http_test.go` 增加一条回归。`fuzz_test.go` 已有完整 strip 的 oracle 回归；只在新的实际 fuzz 失败确认为 checker 边界时另行修订，不能通过删除 seed 或全面放宽断言跳过。

**Interfaces:** 现有 `textSpan` 保持原始 raw offset 和 role，可仅为已选中的非空 flat Responses function/custom 描述保留同工具的休眠 nested `*textSpan` 与非字符串标记。初始 `selectTextSpans` 仍只返回 CPA 当前有效的 active 文本。`applyMode` 仅在 flat 最终变空时对新增激活的字符串候选运行一次；追加真正改写的候选后按 `RawStart` 排序，再调用既有 `rebuildBody`。

- [ ] **Step 1: RED。** 在 `selectors_openai_test.go` 写 `TestOpenAIResponsesStripActivatesNestedDescription`：developer `words.strip:[SECRET]`、`type:function`、有效 `name`、flat `description:"SECRET"`、nested `function.description:"SECRET nested"`，期望 replacement body 的 flat 为 `""`、nested 为 `" nested"`，其余 JSON 原始字节不变。对 `custom` 与显式 developer 的 `additional_tools` 复用同一规则；另加带 `words.block:[BLOCK]`、`words.strip:[SECRET]` 的 nested `"BLOCK nested"`，应返回 censorship_blocked/developer 且无 replacement。运行 `go test . -run '^TestOpenAIResponsesStripActivatesNestedDescription' -count=1 -v`，必须观察 nested 仍含 SECRET 或本应 block 却没有 block 的断言失败，不得以编译错误冒充 RED。
- [ ] **Step 2: 最小生产改动。** 在 `collectOpenAIResponsesTool` 仅为 active 非空字符串 flat description 捕获同工具 `function.description`：nested 非空字符串保留其现有 `RawStart`/`RawEnd`/`Text`/`Role` 为休眠候选，CPA `.String()` 非空而非字符串者标记为不可安全改写；不要初始选择或改写 nested。`transformRequest` 原 `applyMode` 后，仅对 `Changed && Text==""` 的此类 flat span 激活候选；非字符串候选返回本地 `censorship_invalid_request`；将所有激活候选按 `RawStart` 排序，复用 `applyMode` 的 block -> strip -> obfs，block 立即终止，改写候选与原 spans 一起按 raw offset 排序并 `rebuildBody`。未激活时不得增加第二次 matcher、额外替换或整个请求重扫。
- [ ] **Step 3: GREEN 与负例。** 聚焦测试和 `go test . -run '^TestOpenAIResponses' -count=1` 通过；加 flat `"SECRET flat"` + nested `"SECRET nested"` 仅改 flat、flat 清空 + nested `"safe"` 放行、role 不启用时保持不变、nested 为非字符串 Object/数字时本地 invalid、nested 原始字节在 flat 前后两种顺序都正确处理的测试。保留 names/schema machine fields、原 `output_schema`、签名和 annotations 排除。`go test ./...` 通过；若并行 Task 6 integration runner 尚在写 `.integration/run`，本步骤不操作 runner。
- [ ] **Step 4: HTTP 注册回归。** 等 Task 6 提交后，用同一个 `startCPA/newMockUpstream` 在 `integration/http_test.go` 增加 `/v1/responses` 请求：strip flat 后 mock upstream 实际收到不含 SECRET 的有效 nested description，且机器字段不变；nested 命中 block 的对照应 400/developer、upstream 零请求。仅由一个实施者独占运行 `make integration`，然后执行一次 30s `FuzzProtocolTransform`；若 fuzz 的 active span 同形假设在这个动态回退上失败，增加聚焦断言并只对该确切变体调整 oracle checker，绝不放宽其他机器字段和角色断言。
- [ ] **Step 5: 单独提交与独立 review。** 仅提交上述文件；review 特别检查功能性 `strip` 未被改成 400、flat 不清空时 nested 仍未选中、非字符串 fail-closed 仅在激活时发生、offset 排序和前后角色优先级。若无法无回归实现，应回退 Task 6A 的生产改动，保留先前安全修复，并在 README/RELEASE_NOTES 明确列出此残余风险，不能宣称其修复。

### Task 7: 文档、完整验证与版本发布

**Files:** Modify `README.md:138-160`、`RELEASE_NOTES.md:1-9`；仅在确有对应 GREEN 的路径上更新。`go.mod`、CPA core、`.integration/`、`dist/` 均不提交。

**Interfaces:** GitHub `Build` workflow `.github/workflows/build.yml` 接收 `push main` 和 `tag v*`，并构建各平台包及 SHA-256 文件；`make package VERSION=v0.3.8 GOOS=windows GOARCH=amd64` 本地只检验 Windows 路径。

- [ ] **Step 1: 文档。** 在 README 的 provider selector 列表只增加实际修复的路径，明确 Gemini 缺省 role 含 function response 时 user/无效条目仍不选中；Task 6A 通过时说明 Responses flat 描述完整 strip 后仅对激活的 nested 描述继续过滤，未通过则说明残余漏检。在 RELEASE_NOTES 顶部增加 `v0.3.8` 的每项真实修复与 pinned CPA v7.2.152/ABI v1 说明；明确列出未解决的跨后端非规范角色冲突和 terminal Executor 限制，不扩大支持声明。
- [ ] **Step 2: 本地完整验证。** 顺序运行 `make test`、`make race`、`make vet`、`make build`、`go test .github/scripts/package-release.go .github/scripts/package-release_test.go`、`make package VERSION=v0.3.8 GOOS=windows GOARCH=amd64`、`make integration`、`go run ./.github/scripts/integration-runner.go -abi-smoke dist/windows_amd64/censorship.dll`；按 runner 要求提供插件 DLL 与固定 CPA 路径并运行专门的注册测试。`git diff --check` 和受追踪文件清单必须只包含本次任务改动。若任一命令失败，保留准确输出，修复并重跑后再进入下一步。
- [ ] **Step 3: 提交与合并。** 提交 README/RELEASE_NOTES/集成测试，在当前 worktree 做整体复核。退出 worktree 时用 `ExitWorktree(action:"keep")` 保留分支，再在原始 checkout 确认 local `main` 未变脏及当前 ref，执行常规非破坏性 merge；有冲突时停止发布，修复后重跑测试。
- [ ] **Step 4: 发布与 CI。** 在合并后的 `main` 创建递增 tag `v0.3.8`，先检查 `git remote -v`、现有 tag 与 remote 状态，然后推送 `main` 和 tag；查看 `gh run` 中对应 tag 的全部 jobs 与 release assets。对七个平台 ZIP、对应小写 SHA-256 sidecar 和 `checksums.txt` 的内容逐项验证，确认 `v0.3.8` GitHub release 已发布且无缺失资产。仅在远端状态实际验证后宣称完成。

## 计划自检

- spec 的四个选择器修改分别对应 Tasks 1 到 4；Task 5 动态揭露的 Responses 改写后回退缺陷另由 Task 6A 覆盖。独立 oracle、HTTP 插件注册、docs、ABI 与发布分别由 Tasks 5 到 7 覆盖。所有生产改动均在已确认和预期 RED 的范围实施。
- Review Focus 的五类边界均分配了具体测试；不存在对异后端冲突角色的全局改动。
- 所有生产代码均在对应失败测试之后修改；共享文件与 `.integration/run` 串行，三个独立 provider 的测试与实现并行。
