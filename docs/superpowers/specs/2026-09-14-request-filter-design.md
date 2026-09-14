# 请求过滤功能设计规格

**日期：** 2026-09-14

**目标版本：** v0.3.0

**范围：** 仅修改 `cpa-plugin-censorship`，保持 CLIProxyAPI v7.2.152、native ABI v1 和 RPC schema 5 兼容。

## 1. 目标

插件增加请求级过滤设置，使现有 `block -> strip -> obfs` 审查流程可以按已认证的 CPA caller API key 和客户端请求模型选择性执行。

- `exclude`：匹配过滤条件的请求绕过插件处理。
- `include`：只有匹配过滤条件的请求进入插件处理。
- 绕过表示返回空 `pluginapi.RequestInterceptResponse{}`，不回传原 body，不修改 headers，不终止请求。
- 未配置有效过滤条件时不启用过滤，所有请求继续进入现有审查流程。这保证 v0.2.6 配置向后兼容。

## 2. 方案选择

采用三个新的顶层配置字段，并按以下顺序追加到现有 panel 字段之后：

1. `filter_mode`
2. `filter_logic`
3. `filter`

选择该方案的理由：

- 用户要求 panel 具有独立的过滤模式控件、and/or 控件和过滤对象控件。
- `ConfigField` 能直接表达两个 Enum 和一个 Object，但不能为 Object 声明嵌套 properties、默认值或 required。
- 字段名沿用仓库顶层配置的 snake_case 惯例；用户指定的 Object 子键保持为 `api-keys` 和 `models`。

不采用以下方案：

- 不把新字段塞入现有 `scope`。这无法在 panel 中追加两个独立 Enum 控件，也会把内容 scope 与请求身份选择混为一体。
- 不只增加一个 `request_filter` Object。pinned panel schema 无法为其嵌套模式和逻辑生成独立选择控件。
- 不同时支持别名或多套配置形态。单一配置 contract 更容易严格验证，也避免 panel 与 handwritten YAML 分叉。

## 3. 配置 contract

### 3.1 完整形态

```yaml
filter_mode: exclude
filter_logic: or
filter:
  api-keys:
    - "sk-team-*"
    - "sk-single-key"
  models:
    - "claude-sonnet-?"
    - "gpt-*"
```

### 3.2 字段定义

| 字段 | 类型 | 合法值 | 默认值 | 含义 |
| --- | --- | --- | --- | --- |
| `filter_mode` | Enum | `exclude`、`include` | `exclude` | 匹配请求绕过，或仅处理匹配请求 |
| `filter_logic` | Enum | `or`、`and` | `or` | 两个已配置维度之间取并集或交集 |
| `filter` | Object | 可选 `api-keys`、`models` | 空 Object | 请求匹配条件 |
| `filter.api-keys` | String array | 非空字符串 pattern | 缺失 | CPA caller API key patterns |
| `filter.models` | String array | 非空字符串 pattern | 缺失 | 客户端请求模型 patterns |

默认值由插件 parser 实现。CLIProxyAPI v7.2.152 的 `ConfigField` 不支持声明机器可读 default。

### 3.3 严格解析

- `filter_mode` 和 `filter_logic` 必须是标量字符串，并且只能使用表中的小写值。
- `filter` 必须是 YAML mapping。`null`、scalar 和 sequence 均无效。
- `filter` 只接受 `api-keys` 和 `models`。未知键无效。
- 两个子字段必须是 sequence；每项必须是非空字符串。
- 重复键继续由现有全局 YAML duplicate-key 检查拒绝。
- patterns 不 trim、不改大小写、不排序、不去重。数组顺序原样保留。
- model 的匹配 subject 是未修改的 `RequestedModel`。API key 的匹配 subject 是 CPA identity 算法使用的 `TrimSpace(credential)`；pattern 本身仍按原文匹配，因此带首尾空白的 exact API key pattern 永不匹配，不能因 scope 计算中的 trim 意外命中。
- 空数组合法，按未配置该维度处理。
- `filter_mode` 或 `filter_logic` 可以在 `filter` 为空时出现，但此时没有运行时效果。
- 现有 `words` Object、legacy handwritten YAML sequence 加 global `mode`、`scope`、`obfs` 和 host-owned 字段行为保持不变。

### 3.4 空过滤语义

以下配置均禁用请求过滤，所有请求进入现有审查流程：

```yaml
# filter 完全缺失
```

```yaml
filter: {}
```

```yaml
filter:
  api-keys: []
  models: []
```

只要 `api-keys` 或 `models` 至少一个列表非空，过滤才启用。

## 4. Pattern 语义

`api-keys` 和 `models` 使用同一套完整字符串 glob 语义：

- `*` 匹配零个或多个 Unicode scalar。
- `?` 匹配恰好一个 Unicode scalar。
- 其他字符按字面匹配。
- 匹配锚定整个值，不做 substring 匹配。
- model subject 保持原值；API key subject 使用非空的 `TrimSpace(credential)`，pattern 不 trim。
- 匹配区分大小写。
- `ignore_case` 只影响 censorship terms，不影响 API key 或 model patterns。
- 空 pattern 无效。
- 不增加 character class、regex、negation 或 escape 语法。

示例：

| Pattern | 值 | 结果 |
| --- | --- | --- |
| `sk-team-*` | `sk-team-a` | 匹配 |
| `sk-team-*` | `x-sk-team-a` | 不匹配 |
| `model-?` | `model-甲` | 匹配 |
| `model-?` | `model-ab` | 不匹配 |
| `*` | 任意非空 API key 或 model | 匹配 |

每个列表内部始终使用 OR，即任一 pattern 命中即表示该维度命中。

## 5. 维度组合与模式

设：

- `K`：`api-keys` 非空且当前请求匹配其中任一 pattern。
- `M`：`models` 非空且当前请求匹配其中任一 pattern。
- 未配置或空列表的维度不参与组合。

过滤匹配结果：

| 非空维度 | `filter_logic: or` | `filter_logic: and` |
| --- | --- | --- |
| 仅 `api-keys` | `K` | `K` |
| 仅 `models` | `M` | `M` |
| 两者都有 | `K || M` | `K && M` |
| 两者都没有 | 过滤禁用 | 过滤禁用 |

处理决策：

| `filter_mode` | 过滤匹配 | 是否执行现有审查 |
| --- | --- | --- |
| `exclude` | true | 否，原样绕过 |
| `exclude` | false | 是 |
| `include` | true | 是 |
| `include` | false | 否，原样绕过 |

缺失或无法验证的 API key、缺失的 model 都使对应已配置维度结果为 false。它们不会终止请求，也不会产生错误响应。

## 6. 五种请求格式的数据来源

所有五种已支持格式使用同一个 `request.intercept_before` gate。格式差异仍只存在于现有自然语言 selectors 中。

| `SourceFormat` | API key 身份 | wildcard 原值候选 | model |
| --- | --- | --- | --- |
| `openai` | `Metadata["caller_scope"]` | 固定 credential headers | `RequestedModel` |
| `openai-response` | `Metadata["caller_scope"]` | 固定 credential headers | `RequestedModel` |
| `claude` | `Metadata["caller_scope"]` | 固定 credential headers | `RequestedModel` |
| `gemini` | `Metadata["caller_scope"]` | 固定 credential headers | `RequestedModel` |
| `interactions` | `Metadata["caller_scope"]` | 固定 credential headers | `RequestedModel` |

### 6.1 Model

- 只匹配 RPC 顶层 `RequestedModel`，它表示 alias/model-pool 改写前的客户端请求模型。
- 不回退到 `Model`，不从 body 中查找顶层或嵌套 `model`。
- `RequestedModel` 为空时，已配置的 model 维度不匹配。

这避免五种 provider body 产生不同的 model 解析和 duplicate-key 规则。

### 6.2 API key 信任边界

插件不能把客户端可控 header 直接当作已认证身份。可信身份是 CPA 在认证成功后写入的 `Metadata["caller_scope"]`：

```plaintext
lowercase_hex(SHA-256("cli-proxy-api:caller-scope:v1\x00" + TrimSpace(principal)))
```

`principal` 由 CPA auth provider 返回，不保证对所有 provider 都等于客户端发送的字面 API key。因此，`api-keys` 表示可与 authenticated Principal 绑定的 credential patterns，不承诺匹配任意 auth provider 的 literal raw API key。只有 `TrimSpace(principal) == TrimSpace(credential)` 的 provider 路径能够按字面 credential 匹配。

精确 API key pattern 不含 `*` 或 `?` 时：

1. pattern 必须等于自身的 `TrimSpace` 结果；否则按字面 subject 语义编译为永不匹配。
2. 配置编译阶段按 CPA 算法计算 pattern 的 caller scope。
3. 请求阶段直接与可信 `caller_scope` 比较。
4. 不读取 raw credential header。

API key pattern 含 `*` 或 `?` 时：

1. 读取 `caller_scope`，格式必须是 64 位小写十六进制字符串。
2. 按顺序扫描 `Authorization`、`X-Goog-Api-Key`、`X-Api-Key` 的全部值，header 名使用 HTTP case-insensitive 语义。
3. `Authorization` 接受大小写不敏感的 `Bearer <credential>`，也接受 raw credential 值。
4. 每个候选先 `TrimSpace`，空值忽略；按 CPA 算法计算 scope，只有 scope 等于可信 `caller_scope` 的候选才允许进入 glob 匹配。
5. 不缓存、不记录、不回显 raw credential。

如果 auth provider 的 Principal 与 raw credential 不同，或没有通过 scope 绑定的 candidate，API key pattern 不匹配。

### 6.3 已知限制

`RequestInterceptRequest` 没有 query 字段。因此：

- 仅通过 query `key` 或 `auth_token` 提交的请求，只有在 auth provider 返回的 `TrimSpace(principal)` 等于配置中的 exact credential 时，才能通过 `caller_scope` 使用精确 API key pattern。
- 同一请求无法使用 API key wildcard，除非 raw credential 也出现在上述 headers 中并能与 `caller_scope` 绑定。
- 当 Principal 与 literal raw API key 不同时，exact、wildcard 和 query-only credential patterns 都不匹配。
- 本功能不修改 CLIProxyAPI core，不增加 ModelRouter capability，不从 `request_path` 或 body 猜测 query。

普通 management config API 会保存并返回配置中的 API key patterns，panel 没有 secret masking 类型。这是 host-owned 配置回读行为，插件不拦截或改写；README 必须明确这一点。

## 7. 运行时数据流

处理顺序：

```plaintext
CGO C.GoBytes copy
-> decode RequestInterceptRequest once
-> reject unknown SourceFormat by existing no-op path
-> load immutable config snapshot
-> no rules fast no-op
-> request filter gate
-> existing transformRequest
-> block -> strip -> obfs
```

Gate 放在 `interceptBeforeAuth` 中，位于 body JSON 解析之前，理由如下：

- Gate 需要完整 request 的 `Headers`、`Metadata` 和 `RequestedModel`。
- 五种格式共享一次实现，不修改任何 provider selector。
- 绕过请求不解析 body，确保插件完全不检查或修改该请求。
- 由 gate 判定为绕过的请求即使 body 不是 UTF-8、不是 JSON、根不是 Object或有 duplicate member，也由后续系统处理；插件不产生本地 400。
- 由 gate 判定为需要处理的请求继续遵守现有 JSON 校验、签名保护和错误语义；这包括 `exclude` 模式下未匹配的请求。

No-op 继续返回空 `RequestInterceptResponse{}`。不要把原 body 复制到 response；非空 replacement `Body` 才表示请求已修改。

## 8. 编译与性能约束

配置先完整 parse、validate 和 compile，再通过现有 atomic snapshot 一次发布。

最小编译形态：

- 无首尾空白的精确 API key 在配置阶段预计算 caller scope；带首尾空白的 exact pattern 编译为永不匹配，保持字面 pattern 语义。
- 含 wildcard 的 patterns 预编译为 Unicode scalar slice，并标记 wildcard 类型。
- 精确 model pattern 使用直接字符串比较。
- 过滤禁用和无 censorship rules 路径不读取 metadata 或 credential headers。
- 只有配置含 API key wildcard 且需要判断 API key 维度时才扫描 raw credential headers。
- `or` 在首个命中维度后短路；`and` 在首个不匹配维度后短路。
- 不引入新依赖、regex、跨请求 cache 或 generic recursive walker。

不修改 `abi_cgo.go`，保留 request input 的 `C.GoBytes`、allocator 配对和 pointer lifetime。

## 9. Panel 可视化配置

`pluginRegistration().Metadata.ConfigFields` 现有顺序保持：

1. `ignore_case`
2. `words`
3. `scope`
4. `obfs`

在后面追加：

5. `filter_mode`，`ConfigFieldTypeEnum`，`EnumValues: ["exclude", "include"]`
6. `filter_logic`，`ConfigFieldTypeEnum`，`EnumValues: ["or", "and"]`
7. `filter`，`ConfigFieldTypeObject`

Description 必须说明默认值、完整 filter Object 形态、空 Object 禁用过滤、两维度组合和 `*`/`?` 语义。

Panel 约束：

- `words` 继续是 Object。
- ConfigFields 中不增加 global `mode`。
- panel 只提供类型和说明；strict nested validation 由插件 parser 执行。
- 不在插件中增加 custom HTML、JavaScript renderer 或虚构 nested schema。

## 10. 错误和生命周期

- register 收到无效新配置时返回现有 structured plugin error，不安装 snapshot。
- reconfigure 收到无效新配置时记录现有 host error，保留 last-known-good snapshot，并返回现有 registration。
- request filter 缺失 metadata、缺失 raw wildcard credential或 model 为空都只是 no-match，不是配置错误或请求错误。
- 插件生成的 request-intercept response、structured plugin error 和日志不得包含 API key、header credential、pattern 值或 `caller_scope`。配置错误只报告字段路径和错误类别，不拼接原值。host-owned management config API 对已保存配置的回读是唯一已知例外，插件不拦截或改写。
- block、strip、obfs 的现有错误、签名保护、header 清理和 byte-preserving rewrite contract 不变。

## 11. TDD 验收矩阵

### 11.1 配置 RED

在 `config_test.go` 先增加失败测试：

- 缺失新字段保持旧行为。
- 默认 `filter_mode=exclude`、`filter_logic=or`。
- 完整合法配置按原顺序编译。
- `filter: {}`、单个空列表、两个空列表均禁用过滤。
- 只配置 `api-keys` 或只配置 `models`。
- strict invalid：null、错误容器、未知键、错误 item 类型、空字符串、未知 enum、duplicate key。
- Object `words` 与 legacy handwritten YAML 都能与新字段共存，原有语义不变。

### 11.2 Glob 和 predicate RED

在新的 focused test 文件或现有最接近的 test 文件增加 table-driven 测试：

- `*`、`?`、零长度 star、Unicode scalar、full-match、case-sensitive。
- list 内 OR。
- 两维度下 `or` 与 `and` 真值表。
- 单维度时 logic 不影响结果。
- filter disabled 时总是处理。
- include/exclude 的四格处理决策。
- exact API key 在 pattern 无首尾空白且其规范化值与 Principal 相同时只依赖 scope，不读取 header。
- exact API key pattern 带首尾空白时永不匹配，覆盖 `include` 和 `exclude` 的处理决定。
- wildcard API key 只接受与 scope 绑定的 header candidate。
- forged、缺失、格式错误 scope，错误 carrier，稍后出现的正确 header value。
- Principal 与 literal raw API key 不同时，exact、wildcard 和 query-only patterns 均不匹配。
- model 只使用 `RequestedModel`，body decoy 和 `Model` 不影响结果。

### 11.3 Before-auth gate RED

在 `main_test.go` 增加：

- 五个 known `SourceFormat` 共用同一 key/model gate。
- include 命中执行 block/strip/obfs，include 不命中 no-op。
- exclude 命中 no-op，exclude 不命中执行现有规则。
- `or` 任一维度命中，`and` 两维度都命中。
- bypass 请求返回 nil/empty replacement body，`ClearHeaders` 为空。
- bypass 的 `not-json` 不由插件拒绝；需要处理的 `not-json` 保持现有 local 400。
- no rules fast path不读取 filter credential。

### 11.4 Panel 和生命周期 RED

在 `main_test.go` 扩展现有 registration/lifecycle 测试：

- 七个字段的精确顺序、名称、类型、EnumValues 和 description。
- `words` 仍为 Object，字段列表中没有 global `mode`。
- invalid register 失败。
- invalid reconfigure 记录 error 并保留 last-known-good filter snapshot。

### 11.5 Integration RED

在 `integration/http_test.go` 复用现有 CPA harness：

- 匹配并 block：本地 400，upstream arrival 为 0。
- 匹配并 strip：只修改允许的自然语言字段。
- 绕过：enabled plugin 与 disabled plugin 的 upstream capture 和完整响应 trace 相同。
- watcher 从 bypass snapshot A 切换到处理 snapshot B，再用 invalid config 证明 B 保留。

现有五种 provider selector fixtures 已覆盖自然语言路径，不为 request gate 重复建立 `mode × logic × format` 笛卡尔积。五格式差异通过 before-auth table 验证；只有真实 CPA carrier 转换存在差异时才增加 provider-specific integration case。

Responses WebSocket model turn沿用同一 before-auth gate；Realtime、Live、sideband 和 prewarm 的现有旁路 contract 不变。

## 12. 文档、版本和发布验收

更新：

- `README.md`：配置示例、真值表、五格式统一来源、API key 信任边界、query wildcard 限制、panel secret masking 限制。
- `RELEASE_NOTES.md`：新增 v0.3.0 section，保留旧版本内容。

不修改：

- CLIProxyAPI core 或 pinned v7.2.152 依赖。
- native ABI v1 和 RPC schema 5。
- `Makefile`、GitHub workflow 和 package scripts，除非实现过程中发现与本功能直接相关且有失败测试证明的问题。
- 生成的 `.integration/` 和 `dist/` 内容。

发布前验证：

```bash
make test
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
make race
make vet
make integration
make package VERSION=v0.3.0
```

完成 review 并修复确认的问题后：

1. 将 feature branch 合并到本地 `main`。
2. 推送 `main`，确认 main push 的 Build workflow 成功。
3. 在同一 commit 创建并推送 `v0.3.0`。
4. 确认 tag workflow 的 test、七个平台 build/package 和 release job 全部成功。
5. 确认 release 非 draft，资产恰好包含 7 个 ZIP、7 个匹配的小写 SHA-256 文件和 `checksums.txt`。
6. 下载 release 资产并独立验证每个 checksum。
