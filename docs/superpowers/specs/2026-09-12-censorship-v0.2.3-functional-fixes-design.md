# Censorship Plugin v0.2.3 Functional Fixes Design

## 状态

- 日期：2026-09-12
- 基线：`main` at `b6d0c92787fea81a52b69549dd00b3b7566200a6`，release `v0.2.2`
- 目标版本：`v0.2.3`
- 范围：仅修改 `github.com/DoingDog/cpa-plugin-censorship`，保持 CLIProxyAPI v7.2.152、native ABI v1 和 schema 5 兼容

## 审计结论

互斥文件域审计、CLIProxyAPI 插件调用链审计、HTTP/SSE/WebSocket/JSON 标准核对、provider schema 核对、真实 integration、race、vet、原生构建、release packaging 和六个 fuzz target 确认三项可执行问题：

1. HTTP integration 使用 GJSON 路径查询代替完整 JSON 解码，截断错误响应仍可能通过测试。
2. Responses WebSocket integration 对 status 执行 GJSON 数值转换，并以容错路径查询识别完成事件；截断 JSON、string/fractional status 或 binary JSON 可能被当成有效事件。
3. CLIProxyAPI 在调用 request interceptor 前可把压缩 request body 解码为 JSON bytes，同时保留原始 request headers。插件替换 body 时只返回 `Body`，没有通过 `ClearHeaders` 删除描述旧 body 的 `Content-Encoding`、`Content-Length` 和 `Transfer-Encoding`；host 会先应用该响应再调用后续插件，因此 stale headers 可进入同一 request 的后续 plugin chain。

Provider-contract recovery 推翻了两个初次 selector finding。当前 first-party OpenAI schema 将 Responses `mcp_call.error` 定义为 `McpToolCallError` discriminated object union 或 null；string/null 是 `mcp_list_tools.error` 的契约。当前 first-party Anthropic schema 要求 `search_result.title` 存在，但没有声明 `minLength: 1`。因此不增加 unsupported scalar `mcp_call.error` path，也不把空 `search_result.title` 视为已证明的 invalid request。

未确认需要修改的部分：没有重复展开 JSON；插件只声明 `RequestInterceptor`，不修改 model response、SSE 或 WebSocket output；现有真实 integration 的 enabled/disabled wire trace 相同，HTTP/1.1 固定长度、chunk、trailer 和 EOF 检查通过；ABI host callback 生命周期由 CLIProxyAPI active-call drain 保证；lifecycle request `schema_version` 无需由插件再次拒绝；其他条件式 codec、stream boundary 和 CPA fail-open 风险要么不可从当前插件路径到达，要么只能修改 CPA core，均不进入本版本。

## 设计原则

- 保持 request-only capability，不增加 response、stream 或 WebSocket capability。
- 继续只选择 provider 明确记录的 natural-language text paths，不增加 recursive string walker。
- 复用现有 `RequestInterceptResponse.ClearHeaders` 和 stdlib JSON 解码，不增加抽象层或依赖。
- 只在 body 实际变化时清理旧 entity/framing headers。no-op、invalid request 和 block termination 路径保持现状。
- integration oracle 允许未知 JSON 字段，以兼容 provider 演进；只强制 logical message type、完整 JSON 文档和已检查字段的 JSON 类型。
- 不为未复现的性能问题开发优化。

## 行为规范

### Changed-body headers

当 `interceptBeforeAuth` 返回 changed body 时，同时返回：

```plaintext
ClearHeaders = [Content-Encoding, Content-Length, Transfer-Encoding]
```

要求：

- `Body` 是 CLIProxyAPI 传入的已准备 JSON bytes 经 selector/rewrite 后的结果，不根据 `RequestHeaders` 再次解压。
- header 名使用 Go canonical spelling；CLIProxyAPI 的 `http.Header.Del` 提供大小写不敏感删除。
- 不设置新的 `Content-Length`；由 host/outgoing transport 根据新 body 决定 framing。
- `Content-Type` 保留，因为 transformed body 仍为同一 JSON media type。
- no-op response 的 `ClearHeaders` 为空，避免无条件改变请求。

### Strict HTTP integration oracle

新增一个只负责完整解码 plugin error envelope 的 test helper：

- 使用 `encoding/json.Unmarshal` 解码整个 body 到 typed struct。
- syntax error、截断、trailing non-whitespace、top-level/error null，以及 present checked string field 的 JSON null 或其他类型错误均返回 error。
- 未知字段允许存在。
- `TestHTTPBlockIncludesTermAndRole`、`TestHTTPRejectsDuplicateJSONMembers` 和 `TestLegacyCompletionsPromptUsesConvertedUserRole` 先严格解码，再比较 `code`、`term`、`role`。
- helper 自身用 valid、truncated 和 wrong-type bodies 进行 focused test。

### Strict Responses WebSocket integration oracle

新增 typed Responses WebSocket event decoder：

- 只接受 `websocket.TextMessage`。
- 使用 `encoding/json.Unmarshal` 解码完整 JSON 文档。
- present `status` 必须是非 null integer；present `type` 必须是非 null string；top-level JSON null 返回 error。
- 未知字段允许存在；`error.term` 和 `error.role` 用 presence-preserving raw fields 检查。
- block event test 和 terminal timeout fixture 使用 typed status。
- `readUntilCompletedWithTimeout` 对每个 logical message 先执行 strict decoder，仅在 typed `Type == "response.completed"` 时结束。
- malformed/truncated JSON、top-level null、present null/string/fractional status、present null type 和 binary JSON 均返回 error；valid text event 通过。
- Gorilla `ReadMessage` 已组合 continuation frames，因此不解析 raw frames，也不改变真实消息转发。

## 测试与验收

TDD 顺序：每项 production 或 test-helper 行为先增加会在 `b6d0c927` 上失败的 focused test，保存 RED 输出，再做最小修改并保存 GREEN 输出。

必须通过：

- changed-body `ClearHeaders` unit test。
- HTTP strict decoder valid/truncated/wrong-type/null test。
- WebSocket strict decoder valid/truncated/wrong-type/null/opcode test。
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- integration runner 和其复制到 pinned CPA 的完整 suite。
- host-native `c-shared` build。
- `v0.2.3` package generation、per-archive lowercase SHA-256 和 aggregate `checksums.txt`。

最终 review 必须确认：

- 没有新增 recursive JSON traversal。
- 五种 `scope.formats` selector 的现有 tests 全部通过。
- changed/no-op header 行为有区分。
- strict oracle 不拒绝未知字段。
- plugin capability 仍只有 `request_interceptor`。
- 未修改 CLIProxyAPI module cache 或生成的 `.integration/`、`dist/` 内容。

## 发布

- 更新 `RELEASE_NOTES.md` 的当前 compatibility target 和 v0.2.3 修复说明；仅在现有 README 内容需要与行为同步时更新 README。
- 更新 version-sensitive tests 到 v0.2.3。
- 在 feature branch 完成并通过 verification 后 commit，合并到本地 `main`。
- 创建 annotated tag `v0.2.3`，push `main` 和 tag 到 `origin`；tag 触发现有 GitHub release workflow。
- push 前确认 `main` fast-forward/merge 状态、remote URL、tag 不存在且工作树干净。