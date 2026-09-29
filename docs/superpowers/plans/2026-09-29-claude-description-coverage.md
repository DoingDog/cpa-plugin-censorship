# v0.3.7 Claude Description Coverage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. 本轮用户已授权无人值守执行；主会话把互不重叠的文件任务分别交给代理，代理不重复读取此文件或别人的源码。

**Goal:** 修复 Claude 请求的工具与结构化输出 schema 描述绕过词规则，发布 v0.3.7 patch 版本。

**Architecture:** 在既有 `collectClaude(gjson.Result, scopeSet, *[]textSpan)` 的 system 角色分支追加明确的顶层路径选择，复用 `appendJSONSchemaDescriptions`，让原有 `transformRequest` 处理 block/strip/obfs 与 JSON 字节重建。真实 CPA HTTP 集成测试验证插件注册及零上游到达；不修改 CPA。

**Tech Stack:** Go 1.26、CGO native ABI v1、CLIProxyAPI v7.2.152 / plugin schema 5、`github.com/tidwall/gjson`、`go test`、`make`、GitHub Actions。

**Spec:** `docs/superpowers/specs/2026-09-29-claude-description-coverage-design.md`

## Global Constraints

- 只修改插件；不修改 CPA 或 pinned ABI。保留 `cliproxyPluginCall` 的 `C.GoBytes`。
- `words` 保持严格 Object 与既有 legacy 手写 YAML 兼容；`block -> strip -> obfs` 顺序不变。
- 仅选明确的自然语言字段。`name`、JSON keys、schema 的 `enum`/`const`、`input_examples`、工具调用参数和媒体/思维字段不改写；不增加通用递归字符串 walker。
- 没有实证的性能改动不纳入本版；query-only wildcard 由于 ABI 缺 URL/query 原文且 README 明示不支持，不纳入修复。
- 本机 MSYS make 调用需要在 PowerShell 中设置 `$env:GOPATH = (go env GOPATH)`；否则 `go` 报 module cache not found，此为环境问题。
- 原有 `.integration/`、`dist/` 是生成产物，不手工修改；release ZIP 各有小写 SHA-256，聚合 `checksums.txt` 覆盖当次打包平台。

## Review Focus

1. 只有 `scope.roles: [user]` 时，`tools` 与输出 schema 的 system 描述不应触发 block：Task 1 的 role 子用例。
2. 不是用户定义工具的服务器工具项或缺失 `input_schema` 的畸形项不能靠任意 `description` 扩大扫描：Task 1 的工具鉴别子用例。
3. `output_config.format.type` 不为字符串 `json_schema` 时 schema 描述必须保持原样：Task 1 的格式门控子用例。
4. schema `properties` 的键、`enum`/`const`、工具 `name`、`input_examples` 中的同词不可随 description 被改：Task 1 的精确字节断言。
5. `strip` 将可选描述完全清空后不应把输入变成非 JSON 或触发无关字段修改；`obfs` 应只改描述：Task 1 的整段 strip 与 obfs 子用例。

---

### Task 1: 单元级红色回归测试

**Files:** Modify `selectors_claude_test.go` only.

**Interfaces:** Consumes `registerConfig(t, yaml)`, `interceptRPC(t, "claude", body)`, `assertBlockedRole(t, "claude", body, "system")`, `replaceRawTokens(t, body, replacements...)` already used by that test file. Produces `TestClaudeDescriptionPaths` and `TestClaudeDescriptionRewrites`.

- [ ] **Step 1: 仅新增测试，不改生产代码。** `TestClaudeDescriptionPaths` 分表构造三个正文，每个只在目标字段包含 `SECRET`，safe user message 保持不变。完整代表项：

```go
cases := []struct{ name, body string }{
    {"tool description", `{"tools":[{"name":"lookup","description":"SECRET lookup","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"safe"}]}`},
    {"tool nested schema description", `{"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"query":{"type":"array","items":{"type":"string","description":"SECRET query"}}}}}],"messages":[{"role":"user","content":"safe"}]}`},
    {"output schema description", `{"output_config":{"format":{"type":"json_schema","schema":{"type":"object","properties":{"answer":{"type":"string","description":"SECRET answer"}}}}},"messages":[{"role":"user","content":"safe"}]}`},
}
for _, tc := range cases {
    t.Run(tc.name, func(t *testing.T) {
        assertBlockedRole(t, "claude", tc.body, "system")
        registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: [user]\n")
        resp := interceptRPC(t, "claude", []byte(tc.body))
        if resp.Terminate || resp.Body != nil { t.Fatalf("user-only response = %#v", resp) }
    })
}
```

在 `TestClaudeDescriptionRewrites` 中用 `words: {strip: [SECRET]}` 和 `scope.roles: [system]`，对同一 JSON 中工具说明、嵌套 schema 说明与输出 schema 说明三处分别断言原始字符串 token 被替换；在相邻 `name`、`properties.SECRET`、`enum`、`const`、`input_examples` 中保留 `SECRET`，其他字节完全一致。另用表项验证 `tools` 的 server item/缺 `input_schema`、非 `json_schema` 输出格式与非字符串 description 均不产生改写；完全 strip 允许可选 `description:""`，obfs 只在描述第一个 Unicode scalar 后插入 `U+200B`。使用现有 `replaceRawTokens` 对正文生成精确期望，不检查泛化字符串计数。

- [ ] **Step 2: 运行新增单元测试，记录红色结果。** PowerShell: `go test . -run '^TestClaudeDescription' -count=1`。预期三个 block 子项至少失败，strip/obfs 也失败；现有测试仍通过。

### Task 2: 真实 CPA HTTP 红色回归测试（与 Task 1 的测试编辑并行）

**Files:** Modify `integration/http_test.go` only.

**Interfaces:** Consumes existing `newMockUpstream(t)`, `startCPA(t, upstream.URL, true, config)`, `postJSON(t, url, body)`, `decodeCensorshipError(body)`, `upstream.arrivalCount()` from integration harness. Produces `TestHTTPClaudeDescriptionsBlockBeforeUpstream`.

- [ ] **Step 1: 新增三个真实请求子用例。** 表项中分别将禁词只放进工具 `description`、工具 `input_schema.properties.query.description` 与 `output_config.format.schema.properties.answer.description`。每个 JSON 都包含 `model:"censorship-integration-model"`、`max_tokens:16`、一个安全 user message。沿用 `TestHTTPRequestFilterMatchesAllSupportedFormats` 的启动方式：

```go
upstream := newMockUpstream(t)
cpa := startCPA(t, upstream.URL, true, "words:\n  block: [SECRET]\nscope:\n  roles: [system]\n")
status, _, body := postJSON(t, cpa.baseURL+"/v1/messages", []byte(tc.body))
errorResponse, err := decodeCensorshipError(body)
if err != nil || status != 400 || errorResponse.Error.Code != "censorship_blocked" || errorResponse.Error.Role != "system" {
    t.Fatalf("status=%d body=%s error=%v", status, body, err)
}
if upstream.arrivalCount() != 0 { t.Fatal("blocked Claude request reached upstream") }
```

- [ ] **Step 2: 运行集成 runner，记录红色结果。** PowerShell: `$env:GOPATH = (go env GOPATH); make integration`。预期新子用例的本地阻断断言失败；其他已有注册、HTTP/WS 用例不应退化。若完整 integration 输出过长，聚焦保留新用例错误行。

### Task 3: 最小选择器修复（依赖 Task 1、2 的红色结果）

**Files:** Modify `selectors_claude.go` only.

**Interfaces:** Consumes existing `collectClaude`, `appendStringSpan`, `appendJSONSchemaDescriptions`；不更改函数签名、角色集或 `transformRequest`。Produces exactly three new selected path families in canonical `system` scope.

- [ ] **Step 1: 在现有 system 范围内增加明确的路径选择。** 放在 `messages := root.Get("messages")` 之前：

```go
if roles.has("system") {
    tools := root.Get("tools")
    if tools.IsArray() {
        tools.ForEach(func(_, tool gjson.Result) bool {
            schema := tool.Get("input_schema")
            if tool.IsObject() && tool.Get("name").Type == gjson.String && schema.IsObject() {
                appendStringSpan(spans, tool.Get("description"), "system", roles)
                appendJSONSchemaDescriptions(spans, schema, "system", roles)
            }
            return true
        })
    }
    format := root.Get("output_config.format")
    if format.Get("type").Type == gjson.String && format.Get("type").Str == "json_schema" {
        appendJSONSchemaDescriptions(spans, format.Get("schema"), "system", roles)
    }
}
```

若上面与现有 `roles.has("system")` 分支重复，可直接合并到最早的分支；不另造 helper。`appendJSONSchemaDescriptions` 已只递归 JSON Schema 的指定容器键，不遍历任意字符串。

- [ ] **Step 2: 运行 `go test . -run '^TestClaudeDescription' -count=1`，预期通过；随后运行 `$env:GOPATH = (go env GOPATH); make integration`，预期新集成子用例及全部注册/HTTP/WS 测试通过。** 如有失败，定位同一根因并先改测试或实现，不扩充功能。

### Task 4: 发布文档与版本断言（可与 Task 3 的代码编辑并行）

**Files:** Modify `README.md` at `Claude explicit text paths`；prepend `RELEASE_NOTES.md`；modify only release documentation expectations in `main_test.go`.

**Interfaces:** Documents the same three path families produced by Task 3; CI reads tag `v0.3.7` and injects plugin version via `-X main.pluginVersion`, no Makefile/workflow edit.

- [ ] **Step 1: 精确扩充 README 的 Claude 路径清单**，增加一句 `Top-level user-defined tool descriptions and input_schema JSON Schema description leaves, plus output_config.format.schema description leaves when type is json_schema, use canonical system scope; tool names, schema keys, machine schema values, and input_examples remain excluded.` 保留原 `Claude explicit text paths:` 标题与其他 provider 文字不变。
- [ ] **Step 2: 在 RELEASE_NOTES.md 最前新增 v0.3.7 区块**，列出 Claude 自定义工具 `description`/`input_schema` 描述与 `output_config.format.schema` 描述的修复、规范角色 `system`、机器字段保留，并附与 v0.3.6 相同的 `CLIProxyAPI v7.2.152, schema 5, host commit c76dfd4e0edabab9000628b1560ab8ab379eadb8, native ABI v1, glibc 2.34+` 兼容句，不改历史段落。
- [ ] **Step 3: 对齐 `main_test.go` 文档测试。** 在 required token 列表加入上一步 README 新句的稳定短语；将 `currentHeader` 更新为 `# Censorship v0.3.7\n\n## v0.3.7 fixes\n`，`currentEnd` 分隔符改成 `\n# Censorship v0.3.6\n`，`currentCompatibility` 改成 `v0.3.7 targets CLIProxyAPI v7.2.152, schema 5, at host commit ...`，并把 v0.3.6 的兼容句加到历史版本列表。不要编辑测试读取的旧 spec/plan 文件。
- [ ] **Step 4: 运行 `go test . -run '^Test(DocumentationListsConfigAndLimits|ReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions)$' -count=1`，预期通过。**

### Task 5: 验证、审查、合并和发布

**Files:** Only this run's spec/plan, Task 1-4 文件；生成的 `.integration/`、`dist/` 文件不手工编辑。

- [ ] **Step 1: 检查独立任务合并后的 `git diff --check`、`git status --short` 和 diff；审查所有变更均可追溯到 spec。** 对修改过的 selector、测试、README 和 release notes 做不重叠文件范围的 review；主会话审查跨文件行为与过滤安全，不安排第二个子代理重读别的子代理已审查源码。
- [ ] **Step 2: 在 PowerShell 中设置 `$env:GOPATH = (go env GOPATH)` 后按序运行 `make test`、`make race`、`make vet`、`make build VERSION=v0.3.7`、`make integration`、`make package VERSION=v0.3.7 GOOS=windows GOARCH=amd64`、`make package VERSION=v0.3.7`。** 检查 `dist/censorship_0.3.7_windows_amd64.zip.sha256` 为 64 个小写 hex、两个空格和 ZIP 基名；聚合 `dist/checksums.txt` 包含这条对应项；插件注册集成日志须显示 loaded 与 registered。失败则定位并回到相应 task 的红绿循环，不掩盖失败。
- [ ] **Step 3: 审查 GitHub remote 与本地 main 的快进可行性；提交修复分支；合入本地 main（不覆盖用户变更）；在 main 上创建 tag `v0.3.7`，推送 main 和 tag。** 用户已明确授权 push/release，无需额外确认。CI 工作流 `build.yml` 的 push branches `[main]` 与 tags `[v*]` 会被触发，不修改工作流。
- [ ] **Step 4: 用 `gh run list --workflow build.yml --branch v0.3.7 --json databaseId,status,conclusion` 定位 tag run，以 `gh run watch <id> --exit-status` 阻塞等待 CI。** 检查 tests、七平台构建、release job 成功，release assets 与各平台 SHA-256 一致；如果 CI 未成功，不宣称完成，修复插件侧原因并再次按 patch 版本规则发布。禁止主动 `sleep` 轮询。
