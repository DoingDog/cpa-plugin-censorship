# Models Filter Post-Route Execution 修复设计

## 状态

本设计同时取代 `docs/superpowers/specs/2026-09-14-model-filter-plugin-compatibility-design.md` 与 `docs/superpowers/plans/2026-09-14-model-filter-plugin-compatibility.md` 中关于 model filter 阶段和后续执行的合同。旧 plan 不得继续执行。v0.3.1 将普通 outer invocation 固定为按 `RequestedModel` 处理，这会在模型修改 Executor 运行前终止请求，已经被真实双插件链路和用户安装测试证伪。

## 目标

当一个较高 priority 的 ModelRouter 选择 self Executor，并由该 Executor 通过 non-stream `host.model.execute` 把改写后的模型重新交给 CLIProxyAPI AuthManager 时，censorship 的 `filter.models` 必须按 `RequestAfterAuthInterceptor` 收到的 auth-selected attempt `RequestInterceptRequest.Model` 决策。该值不承诺等于 built-in 或 plugin Executor 后续写入 upstream wire body 的最终 model。

固定验收场景：

- model-mapper priority 999 将 `claude-opus` 转发到 `kimi-k3`；
- censorship priority 800，`words.block` 包含 `blocked`；
- `filter_mode: exclude`，`filter.models: [kimi-k3]`；
- 直接请求 `kimi-k3` 发送 `blocked` 返回成功；
- 请求 `claude-opus` 发送 `blocked` 也返回成功，且 upstream 收到的 model 为 `kimi-k3`。

production code 不识别 model-mapper 的 plugin ID、版本、priority、配置键、规则 DSL 或错误文本。model-mapper 只作为真实 integration regression fixture。

## 已确认根因

固定 CLIProxyAPI v7.2.152 与 model-mapper v0.5.6 的实际顺序是：

1. ModelRouter 按 priority 运行。model-mapper 返回 `Handled=true` 与 `TargetKind=self`，但 `TargetModel` 为空。
2. host 只记录将要调用的 mapper Executor，没有把 `claude-opus` 改为 `kimi-k3`。
3. host 在 mapper Executor 前调用 outer `request.intercept_before`。此时 censorship 收到 `Model=claude-opus`、`RequestedModel=claude-opus`，且没有 callback `source`。
4. v0.3.1 按 outer `RequestedModel` 计算 exclude filter。`claude-opus` 不匹配 `kimi-k3`，因此正文命中 `blocked` 后返回 terminal 400。
5. terminal response 立即结束请求。mapper Executor、`host.model.execute*` 与 nested execution 都没有发生。
6. 只有 outer 未终止时，mapper Executor 才计算 `kimi-k3`，改写 request body 顶层 model，并调用 `host.model.execute*`。
7. callback 进入 AuthManager。auth selection、alias、model pool 与当前 candidate 解析完成后，host 调用 `request.intercept_after`。该调用的 `Model` 是当前 upstream attempt 使用的模型。

因此，callback `source` 只能识别已经发生的 nested invocation，不能解决 outer 先终止。priority 也不能跨 ModelRouter、Executor 与 RequestInterceptor capability 改变这一顺序。

## 修正后的阶段合同

### 不含 model filter

当 `filter.models` 为空时，保持现有行为：

- `request.intercept_before` 执行完整 filter gate 与自然语言 transform；
- `request.intercept_after` 返回 no-op；
- API-key-only filter 的时序、HTTP 400 response、exact 与 wildcard 行为保持不变；
- 空 filter 继续在 before 处理全部适用请求。

### 含 model filter

当 `filter.models` 非空时：

1. 所有 `request.intercept_before` 调用都返回 no-op，不解析 request envelope 或 body，也不执行 block、strip、obfs。
2. `request.intercept_after` 只有在 Metadata 包含有效的 selected-auth marker 时才处理。有效 marker 是以下任一导出 key 对应的非空、非纯空白 string：
   - `executor.SelectedAuthMetadataKey`
   - `executor.SelectedAuthIndexMetadataKey`
3. 缺失、null、空 string、纯空白 string 或非 string marker 均返回 no-op。
4. 有效 after invocation 只使用 `request.Model` 作为 model filter subject。`RequestedModel`、body model、URL、headers、callback `source` 和其他 Metadata 不作为 model fallback。
5. model-only、model 与 API key 的 `and`/`or` 组合都在同一个有效 after invocation 中计算。
6. filter 命中后复用现有 transform，顺序仍是 `block -> strip -> obfs`。
7. 每个普通 AuthManager attempt 最多执行一次 transform。before 与缺少 selected-auth marker 的 early after 不得提前处理。

`selected_auth_id` 与 `selected_auth_index` 是固定 v7.2.152 的阶段信号，不宣称为跨版本 `FinalModelResolved` API。升级 pinned host 时必须重新验证。

### Reconfigure 与 lifecycle phase-class 合同

before 与 after 分别读取当时的 active snapshot，host 不会把两个 callback 固定到同一个 plugin snapshot。为防止同一已加载实例的config-only热重载让两个阶段都no-op，`plugin.reconfigure` 必须遵守：

- old与candidate的 `filter.models` 都为空时允许更新；
- old与candidate的 `filter.models` 都非空时允许更新；
- empty -> non-empty或non-empty -> empty时拒绝整个candidate，保留last-known-good snapshot，写一条不包含配置内容的error log；
- 不增加request correlation cache、header或body sentinel。

固定host还支持binary hot replacement，并会对新实例调用 `plugin.register`；enable/disable也直接替换或移除host capability snapshot。旧实例无法拒绝这些变化。因此：

- phase class、plugin binary或enabled状态变化前，operator必须先drain in-flight requests，再完整restart；
- censorship不能强制host drain；跨binary replacement、disable/enable或restart的in-flight request不在at-most-once与policy覆盖保证内；
- 在同一已加载实例、没有上述lifecycle切换时，一次执行不会因允许的config-only reconfigure跨越before与after的处理阶段；
- 允许的同phase-class reconfigure仍按现有callback边界观察当前snapshot，不承诺一次长请求的所有字段使用同一snapshot。

## 支持范围

本 release 支持：

- 直接进入普通 CLIProxyAPI AuthManager 的 built-in provider request；
- ModelRouter 选择 self/executor，随后通过 non-stream `host.model.execute` 回到普通 AuthManager 的链路；
- 该链路在 provider Executor 运行前提供 selected-auth marker 与 auth-selected attempt `Model`；
- 五种现有 `SourceFormat` 的自然语言 request body 处理；
- API-key-only filter 的原 before 阶段；
- model-only 及 model 与 API key 的 `and`/`or` 组合。

production implementation 只依赖上述 host 阶段，不依赖产生 callback 的插件身份。

## 固定 host 下无法由 censorship 单独消除的限制

以下限制已有源码与可执行 probe 证据。本设计不通过猜测或额外 production coupling 掩盖它们：

1. nested after 的 terminal response 经 native `host.model.execute*` callback 返回 outer Executor 时，CLIProxyAPI v7.2.152 会丢失 response status、headers 和 body。mapped nonexcluded 请求仍在 upstream 前被拒绝，但客户端收到 host 生成的 HTTP 500，而不是 censorship 的 HTTP 400 `censorship_blocked` body。直接 provider request 仍保留原 400。
2. opaque terminal self Executor 如果不调用 `host.model.execute*`，censorship 没有 post-route hook可观察其内部最终模型。
3. selected-auth after 之后仍可能改写 model。范围包括 plugin ProviderExecutor，以及 CLIProxyAPI built-in Executor 的 `payload`、`override` 或 `override-raw` 规则。当前 marker 不能预知这些 Executor-side rewrite；要求 filter跟随最终 wire model的配置不得在 after hook后再次改写 model。
4. non-stream Antigravity credits fallback 有一条直接调用 fallback Executor、没有再次运行 request after 的固定 host 路径。该 fallback 不在 post-route model filter 保证内。
5. mapped `host.model.execute_stream` 不在本 release 的支持范围。model-mapper v0.5.6 的 OpenAI streaming callback 与 outer handler存在双重 SSE framing，允许正文的 mapper stream也会得到无效 downstream SSE。该问题不由 censorship request transform产生，本插件不得增加 stream rewriter掩盖它。direct-provider stream与Responses WebSocket仍按现有integration回归。

完整消除第 1 至 4 项需要 CLIProxyAPI 增加结构化 callback direct-response 传递、每次 Executor 前更晚的 after hook或明确的 final-wire-model signal。修复第 5 项需要host与mapper统一stream chunk framing ownership并新增真实双插件stream matrix。这些改动违反“只修改 censorship plugin”的任务边界。

## 安全与兼容边界

- mapped nonexcluded 在受支持 callback 路径中即使响应退化为 500，blocked 正文也不得到达 upstream。
- 不把 HTTP 500 写成理想行为；integration test只把它锁定为 pinned host 的已知限制，release notes必须说明。
- 不为 opaque terminal Executor 或 Antigravity fallback 宣称 post-route model filter支持。
- 不新增跨 invocation global state、request correlation cache、marker header或 body sentinel。
- 不从 body 顶层 `model` 推断 filter subject，不修改 request/response model字段或 `/models`。
- 不实现 CPA alias routing。读取 host 在当前 attempt 提供的 `Model` 不等于在 censorship 中解析 alias。
- wildcard API-key 与 model组合在 after 阶段读取当时仍可见的 headers；exact API-key继续使用 host提供的 `caller_scope`。文档不得声称 wildcard观察的是不可变原始 credential。
- ABI v1 ownership不变。`request.intercept_after`开始读取 body后，`cliproxyPluginCall`必须同步执行 `C.GoBytes`，不得持有host input pointer。

## TDD 验收

### Phase unit tests

先写失败测试，证明：

- 配置非空 `filter.models` 时，直接传给 before 的 malformed raw interceptor envelope返回精确no-op，证明phase fast path发生在 envelope decode之前；
- 同一配置下，有效envelope中的body包含 `blocked` 或 malformed JSON时，before也返回no-op；
- model filter为空时，before保持现有block、strip、obfs、invalid body与API-key gate行为；
- 空filter与API-key-only filter的after对malformed non-empty raw envelope返回精确no-op，证明Go层不做无用JSON/body decode；
- model filter非空时，after在marker缺失、null、空string、纯空白string或非string时返回no-op；
- 仅非空 `selected_auth_id` 或仅非空 `selected_auth_index` 都激活处理；
- after按 `request.Model`匹配，`RequestedModel`、body model与 `source`使用相反 decoy；
- include、exclude、`and`、`or`、exact、`*`、`?`、大小写和完整字符串匹配保持定义；
- 五种 supported `SourceFormat` 使用同一阶段合同；
- 每个有效 invocation只执行一次 `block -> strip -> obfs`；
- reconfigure的empty -> non-empty与non-empty -> empty phase-class transition都被拒绝并保留last-known-good；
- empty -> empty与non-empty -> non-empty的reconfigure仍生效，且log不包含配置内容。

### ABI tests

先让 after request在当前空输入特例下失败，再恢复同步复制：

- active-after oracle配置非空 `filter.models` 与strip或block rule，请求携带有效selected-auth marker、匹配的 `Model`、相反的 `RequestedModel`和非空body；
- 当前no-copy实现必须因after收到空input而先RED，修复后得到非no-op transform或termination；
- `request.intercept_after`的non-empty input必须经过 `C.GoBytes`，nil pointer加nonzero length继续拒绝；
- host input bytes在调用后保持不变，plugin response buffer由ABI v1 `free_buffer`释放；
- Linux shared library与Windows amd64 DLL都通过dynamic loader执行同一个active-after oracle；Windows smoke必须在CI artifact upload前运行并成为release job依赖。Windows arm64保持build-only。

### Pinned integration

integration runner构建并验证：

- CLIProxyAPI commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`；
- model-mapper commit `8fe4839dd2c39a4b0537447c4ac35a9f1d699bbf`，对应 v0.5.6；
- 当前 censorship source build。

最小 non-stream真实矩阵：

fixture必须把 `kimi-k3` 与 `other-model` 都注册为可执行provider model，且不得把 `claude-opus` 或 `claude-control` 注册为provider alias。

1. direct `kimi-k3`，正文含 `blocked`，expect 200、upstream delta 1、upstream model `kimi-k3`、正文保留。
2. mapped `claude-opus -> kimi-k3`，正文含 `blocked`，expect 200、upstream delta 1、upstream model `kimi-k3`、正文保留。
3. mapped `claude-control -> other-model`，正文为clean canary，expect 200、upstream delta 1、upstream model `other-model`，证明route、auth与provider可用。
4. 在同一CPA进程、同一 `claude-control -> other-model` route只把正文改为 `blocked`，expect相对第3步upstream delta 0。固定v7.2.152下记录实际500与 `internal_server_error`，防止model-not-found或mapper setup失败造成false green，也不误称400已保留。
5. mapper disabled时请求 `claude-opus`，expect model-not-found，证明provider fixture没有用alias制造false green。

现有direct provider integration继续验证五种HTTP source format、Responses WebSocket、stream与non-stream、自然语言selectors、machine fields、允许的同phase-class config reload和ABI。mapped `host.model.execute_stream`不作为本release绿灯。

## 性能

- API-key-only与空 filter继续在before fast path，不承担after解析。
- 含 model filter的before应在解析request envelope与body前返回no-op。
- after每次只做两次 Metadata key查询和现有filter/transform，不增加cache、递归body扫描或额外model解析。
- 恢复after input copy会产生与输入大小相关的同步分配，这是ABI ownership正确性要求。除非benchmark证明替代方案消除该分配且不保留host pointer，否则保留 `C.GoBytes`。

## 发布

- v0.3.1已经发布且合同错误；本修复使用下一个未占用patch version，发布前同时检查local与remote tags。
- 在旧design与旧implementation plan顶部标记superseded，禁止后续agent继续执行v0.3.1方案。
- `README.md`必须改为before与selected-auth after分流、after只用auth-selected attempt `Model`、marker的pinned-host属性、同实例phase-class reconfigure限制、binary或enabled变化前drain并restart的lifecycle边界、mapped block的500、after恢复 `C.GoBytes`、Executor-side rewrite和mapped stream边界。
- `main_test.go`更新对应文档断言，并分别保留v0.3.1历史release说明与新patch当前说明。
- `RELEASE_NOTES.md`新增本次patch，明确真实用户场景、支持的non-stream callback/AuthManager路径、mapped block的500限制和不支持路径；v0.3.1内容保留为历史事实。
- GitHub Actions必须完成unit、race、vet、pinned integration、Windows amd64 active-after DLL smoke、七平台build/package、lowercase SHA-256 sidecar和aggregate `checksums.txt`。
- release asset必须用新版本号，不得出现 `0.0.0-dev`。
