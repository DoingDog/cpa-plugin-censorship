# censorship 插件全量审计修复与性能优化设计

- 状态：设计已确认，待用户审阅
- 日期：2026-09-05
- 基线：`v0.1.3`，commit `6cd6cec`
- 范围：仅修改 `cpa-plugin-censorship`；不修改 CLIProxyAPI，不分析与插件加载、运行无关的 CPA 实现

## 1. 目标

本轮在现有插件合同上完成一次定向 hardening：修复审计中已验证的 selector、JSON 资源边界、C ABI 和 release packaging 缺陷；修正会掩盖真实回归的测试 oracle；用 benchmark 决定是否保留一个 case-sensitive rewrite 优化。

所有行为修改都使用 TDD。每个 slice 先加入一个能实际失败的 RED test，再写最小实现并运行当前全套测试。未经复现的风险、与现有合同冲突的建议和未显示收益的性能设想不进入实现。

## 2. 扫描范围与基线证据

审计按互不重叠的文件所有权拆成四个区域：

1. 核心运行时：`main.go`、`config.go`、`transform.go`、`matcher.go`、`abi_cgo.go` 及直接测试。
2. 协议 selector：`selectors*.go` 及 selector tests。
3. integration、packager、Makefile 和 GitHub Actions。
4. fuzz、benchmark 和测试 oracle。

没有两个审计区域读取同一生产模块。审计未读取 `.integration/cpa` 下与插件无关的源码；只核实了 module cache 中 `plugin.register` 的 `schema_version` 协商合同。

基线命令均通过：

```plaintext
go test ./...
go test -race ./...
go vet ./...
go test -run '^$' -bench . -benchmem ./...
```

完整 benchmark 在 Windows amd64、AMD Ryzen 7 7840H 上约耗时 208 秒。该数据只作为本轮前基线，不作为跨机器 SLO。

## 3. 已确认问题

### 3.1 Interactions direct text 绕过 hard exclusion

`collectInteractions` 对 object 型 `system_instruction` 或 fallback `systemInstruction` 的 direct `.text` 直接创建 span，没有经过 `scanTextPart`。

以下输入会错误修改 `SECRET`：

```json
{"system_instruction":{"type":"image","text":"SECRET"}}
```

```json
{"system_instruction":{"type":"text","text":"SECRET","inlineData":{"data":"BASE64"}}}
```

这违反 unknown typed node、media 和 machine discriminator 必须保持不变的现有合同。

### 3.2 深层 JSON duplicate 检查耗时近似二次增长

`hasDuplicateJSONMembers` 递归调用 nested `gjson.Result.ForEach`。深层单链 object 会在每一层重新扫描剩余 raw value。

隔离探针结果：

| nesting depth | duplicate 阶段耗时 |
|---:|---:|
| 4,000 | 49.6 ms |
| 8,000 | 190.6 ms |
| 16,000 | 845.3 ms |
| 32,000 | 3.27 s |

同一路径还使 Go recursion depth 和每层 object map 数量不受插件约束。现有 fuzz 在 depth 大于 256 时 skip，不能为生产路径提供边界证据。

### 3.3 C ABI 长度转换可能 panic

`abi_cgo.go` 在两处把 `C.size_t` 直接转换为 `C.int` 后传给 `C.GoBytes`：

1. host -> plugin request。
2. plugin -> host callback response。

长度大于 `math.MaxInt32` 时转换可能截断为负值并 panic。插件输出还通过 `(*[1 << 30]byte)(ptr)[:len(payload)]` 复制，超过 1 GiB 会发生 slice bounds panic。panic 跨越动态插件 ABI 时可能终止 CPA 进程。

### 3.4 release version 可形成路径

packager 在生成 archive path 前没有验证 normalized version 是单个安全文件名组件。实测 `-version 'foo/bar'` 返回成功并生成嵌套路径 `censorship_foo/bar_linux_amd64.zip`。合法的 slash tag，例如 `v1/foo`，可以触发 workflow，但随后 artifact path 与上传声明不一致，阻断 release。

### 3.5 WebSocket integration 可以永久等待

`readUntilCompleted` 没有 read deadline。若回归导致连接既不发送 `response.completed` 也不关闭，`make integration` 可以一直等待，测试进程无法给出失败结果。

### 3.6 测试 oracle 存在共同失效路径

以下测试缺口会让错误实现通过：

1. protocol fuzz 的 block 检查只验证 role 属于所有可命中 role，不验证同一 rule 的最早 eligible span role。
2. protocol fuzz 只比较 selected/excluded tokens，不比较 span 之间的原始 byte gaps；whole-body remarshal 可改变空白、number spelling 或未知字段而仍通过。
3. folded matcher 的部分 expected 调用生产 `containsRule` 或 `foldClassRune`，生产实现与 expected 同时出错时测试仍通过。
4. exact adaptive block 缺少跨多个 spans 的 rule-major、同 rule document-order 和 role differential coverage。
5. 核心 benchmark 在 timer 内丢弃结果且没有统一 package sink；错误的 no-op 或错误 envelope 仍可能产出貌似有效的性能数字。

## 4. 明确不修改的项目

### 4.1 `schema_version`

`schema_version` 是 registration negotiation，不是请求方要求插件必须完全等于该版本。host 发送自身当前版本；插件返回自身支持版本。host 已拒绝高于自身版本的 plugin response，并把低版本作为兼容插件加载。插件在未来 host 传入更高版本时继续返回当前版本属于正常向后兼容，因此不增加等值拒绝。

### 4.2 PR 七平台 build

现有设计明确规定 PR 只运行 test gate，七个平台在 main push 和 tag 上构建。改变该策略会显著增加每个 PR 的 CI 成本，不属于已验证的插件缺陷。本轮不改 workflow build 条件。

### 4.3 其他未采纳项

- `scope.roles: []` 已由 `transformRequest` 在进入 selector 前早退，不存在插件请求路径上的额外遍历。
- folded KMP 命中后的反向 rune 定位总工作量仍为 `O(T)`；ring buffer 建议的复杂度依据不成立。
- integration 临时端口存在理论争用窗口，但重复 runner 未复现失败。
- 不新增 content hash cache、conversation state、通用 provider abstraction 或依赖。

## 5. 运行时设计

### 5.1 JSON nesting 边界

新增常量：

```go
const maxJSONNestingDepth = 1024
```

对需要处理的 known format，`selectTextSpans` 顺序为：

```plaintext
gjson.ValidBytes
-> gjson.ParseBytes 并确认顶层 object
-> 无分配 byte scan 检查 nesting depth
-> duplicate member 检查
-> protocol selector
-> span 排序与校验
```

nesting scan 只在 JSON 已确认合法后运行。它遍历原始 bytes，跟踪 JSON string 和 backslash escape 状态，只把 string 外的 `{`、`[` 计入深度。深度定义为当前同时打开的 object/array container 数量，包含顶层 object；最大值 1024 合法，达到 1025 时返回 `errInvalidRequest`，公开结果沿用 `censorship_invalid_request`。

选择 1024 的理由：

1. 远高于五种模型协议的正常结构深度，包括复杂 tool schema 和 Interactions `steps`。
2. 在进入 duplicate walker 和 Interactions recursion 前建立确定资源上限。
3. 不使用 `json.Decoder.Token`，避免解码所有排除字符串及 20 MiB base64。
4. 保留现有 `gjson.Result.Index` raw span 语义。

duplicate key 的 canonicalization、escaped key 等价、surrogate-pair key 和任意层 duplicate 拒绝合同不变。边界内继续复用现有 walker，不实现自定义完整 JSON parser。

### 5.2 Interactions selector

object-level system instruction 的 direct text 改为：

```go
text, allowed := scanTextPart(systemInstruction, true)
if allowed {
    appendStringSpan(spans, text, "system", roles)
}
```

因此：

- 缺失 `type`、空 `type` 或 `type == "text"` 可以选择 direct `.text`。
- 未知或非 string `type` 排除 direct `.text`。
- `thought == true` 或任一现有 machine discriminator 排除 direct `.text`。
- `parts[]` 仍逐 part 独立判断。container 上未知扩展字段不使合法 parts 整体失效。

其他 Interactions role、item type、content、parts 和 steps 语义不变。

### 5.3 C ABI 边界

在调用 `C.GoBytes` 前统一检查 `C.size_t <= math.MaxInt32`。具体行为：

1. `cliproxyPluginCall` 验证 `response != nil` 后立即把 `response.ptr` 和 `response.len` 清零。
2. request length 超限时不读取 request，返回非零 rc；不构造部分 response。
3. host callback response length 超限时，先确保 host buffer 仍会执行 `free_buffer`，再向 Go 返回 error；外层沿用现有 plugin error envelope。
4. 正常长度继续使用 `C.GoBytes`，不引入 unsafe view 指向 host-owned memory。
5. plugin output 的 `malloc` buffer 使用 `unsafe.Slice((*byte)(ptr), len(payload))` 接收 copy，移除固定 1 GiB Go array 上限。

AfterAuth 不复制 request 的 fast path、host/plugin buffer ownership 和 exported ABI signatures 不变。

## 6. 发布与测试稳定性设计

### 6.1 release version grammar

现有逻辑先移除一个前导小写 ASCII `v`，随后验证 normalized version：

```plaintext
first = ASCII letter or digit
rest  = ASCII letter, digit, '.', '_', '+', or '-'
```

合法示例：

```plaintext
1.2.3
0.1.0-rc.1+build
0.0.0-dev
```

空值、`.`、`..`、slash、backslash、空格、控制字符和 Windows 文件名非法字符必须在创建任何 output path 前失败。dist aggregation mode 验证由 flag、环境或 exact tag 解析出的 version；Makefile `package-platform` 和两个 cross workflow job 在 direct mode 额外传入同一 normalized version，使 packager 在读取或创建 archive path 前调用同一 validator。未提供 version 的通用 direct mode 保持可用，因为该模式的 archive path 由调用方完整指定。失败不得留下 archive、checksum 或新建的 version 子目录。

本轮不引入 semver dependency，也不要求 release version 必须是完整 SemVer；文件名安全是该修复的边界。

### 6.2 WebSocket deadline

`readUntilCompleted` 在读取循环前设置 20 秒 read deadline。收到 `response.completed`、明确 error 或连接关闭时保持现有结果；deadline 到期时通过现有 error 返回路径使测试失败。helper 不创建 goroutine，不使用 sleep，不改变插件或 CPA runtime。

## 7. 性能实验

### 7.1 case-sensitive rewrite 全 miss preflight

实验范围只包含 `ignore_case == false` 的 `strip` 和 `obfs`。复用现有 config snapshot 中已经构造的 `byteMatcher`；不为该实验增加第二套 matcher 或新 dependency。

对每个 span，在任何 rule 修改前执行一次 matcher：

```plaintext
没有任一配置 term 命中原始 span text -> 该 span 跳过逐 rule rewrite
存在任一 term 命中 -> 执行现有完整 rule-major cascade
```

该判断不改变语义：若初始文本没有任何 rule 命中，任何 rule 都不会产生新文本，也就不可能触发后续 cascade；若有命中，则完全回退到现有实现。

### 7.2 benchmark matrix 与保留门槛

baseline 和 candidate 在同一 benchmark binary 中比较：

| 维度 | 值 |
|---|---|
| rules | 32、128、256、1024 |
| text | 4 KiB、64 KiB、1 MiB |
| match | none、first、middle、last、sparse、dense、cascade |
| mode | strip、obfs |

保留条件：

1. 128+ rules 的目标 all-miss holdout 中，代表性结果至少改善约 20%。
2. 任一 hit/cascade holdout 不出现稳定超过约 10% 的回退。
3. allocation 不高于 baseline。
4. differential tests 完全一致。

benchmark 不把时间阈值写成易受机器影响的单元测试 assertion。若 candidate 不满足条件，删除 candidate production change，只在 TDD 记录中保留 benchmark 结果。

不再实验 folded KMP ring buffer、case canonical compare 或 duplicate map pool；当前没有足够证据证明这些改动值得增加复杂度。

## 8. 测试设计

### 8.1 独立 oracle 规则

test-local oracle 不调用被测的 matcher、selector 或 canonicalization helper。

- folded equivalence 使用 test-local `unicode.SimpleFold` cycle 或等 scalar window 的 `strings.EqualFold`。
- exact block expected 使用 ordered `strings.Contains`。
- protocol oracle 独立枚举每种格式的 eligible spans。
- block expected 先按 YAML rule order，再按 raw document order选择 role。
- rewrite 后重新定位对应 output spans，逐段比较 span 外的 prefix、gaps 和 suffix bytes。

span 外 byte oracle 必须能拒绝以下无关变化：

- 外层空白变化。
- `1e+03` 被改成 `1000`。
- unknown 字段或 hard-excluded string 被重新编码。
- object member 顺序变化。

### 8.2 TDD slices

#### Slice 1：Interactions hard exclusion

RED：unknown `type=image` 和带 `inlineData` 的 object-level system direct text 当前会被 strip。

GREEN：direct text 复用 `scanTextPart(..., true)`；运行全部 selector tests。

#### Slice 2：JSON nesting resource bound

RED：1024 层合法、1025 层返回 invalid；deep duplicate 和 deep Interactions steps 不 panic；现有实现缺少定义边界。

GREEN：加入 raw nesting scan，并在 duplicate/selector recursion 前调用。

资源验证：增加 64、256、1024 和 over-limit depth benchmark；复跑 20 MiB excluded-before/middle/after，确认 allocations 不因 depth scan 增长到第二份 body。

#### Slice 3：C ABI length safety

RED：把 `C.size_t` 转换前使用的纯长度 helper 设计为接受 `uint64`，覆盖 0、`MaxInt32`、`MaxInt32+1`；正常 dynamic ABI test 仍可 init/call/free。

GREEN：加入 checked conversion 和 `unsafe.Slice` output copy；代码检查确认 `response` 在新增的 length early return 前已清零。

测试不分配 GiB 级 buffer，也不增加只为构造 `C.cliproxy_buffer` 存在的 production test bridge。

#### Slice 4：release version safety

RED：valid/invalid table；CLI 使用 `foo/bar`、`foo\\bar`、`..`、空格和控制字符时失败且 output 为空。

GREEN：normalize 后统一验证，再计算 archive/checksum path。

#### Slice 5：WebSocket deadline

RED：本地 fixture 建立连接但不发送 completion，helper 必须在 deadline 内返回 timeout error。

GREEN：设置 20 秒 read deadline。测试可使用更短的 injected deadline helper 参数；production integration caller 固定使用 20 秒，避免单测等待 20 秒。

#### Slice 6：oracle 修复

逐项 RED -> GREEN：

1. 错误选择较晚 block role 时 oracle 必须失败。
2. span 外 whitespace/number/unknown bytes 变化时 oracle 必须失败。
3. folded matcher expected 与生产 helper 脱钩。
4. exact block 在 256+ rules、多 spans、同 rule 多 role 下与 ordered `strings.Contains` 一致。
5. benchmark fixture 计时前验证正确 response，并把结果写入 package sink。

#### Slice 7：性能候选

先增加 baseline/candidate benchmark 和 exact rewrite differential test，再实现最小 preflight。按第 7.2 节门槛决定保留或删除 production candidate。

## 9. 文件范围

预计修改：

```plaintext
selectors.go
selectors_interactions.go
selectors_interactions_test.go
abi_cgo.go
abi_cgo_test.go
transform.go
matcher.go 或 config.go，仅在复用 byteMatcher 所需的最小范围内
fuzz_test.go
matcher_test.go
benchmark_test.go
.github/scripts/package-release.go
.github/scripts/package-release_test.go
integration/websocket_test.go
README.md
Makefile
.github/workflows/build.yml
```

预计新增：

```plaintext
docs/superpowers/plans/2026-09-05-censorship-audit-hardening.md
docs/superpowers/tdd/2026-09-05-censorship-audit-hardening.tdd.md
```

若现有 helper 已能承载测试，不新增额外 test-only 文件。go.mod 和 go.sum 不改；Makefile 与 `.github/workflows/build.yml` 只增加 direct packager 的 version 参数，不改变 build matrix、触发条件或 job 结构。

## 10. 最终 verification

按顺序运行：

```plaintext
gofmt -w <本轮修改的 Go 文件>
go test ./...
go test -race ./...
go vet ./...
go test -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=1x
go test -run '^$' -fuzz '^FuzzProtocolTransform$' -fuzztime=1x
go test -run '^$' -bench '<本轮相关 benchmark>' -benchmem -count=5
go test -run '^$' -bench . -benchmem
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
make integration
```

若 Windows race、CGO 或 integration 再现环境故障，必须先在仓库外最小程序复现，才能标记为环境阻塞；不能用普通测试通过替代对应 gate。

## 11. 完成标准

1. 两个 Interactions 反例保持逐 byte 不变，合法 direct text 和合法 parts 仍按 scope 处理。
2. 1024 层输入有定义行为，1025 层稳定返回 `censorship_invalid_request`，深层输入不再进入无界递归。
3. 两处 `C.GoBytes` 都无法接收超出 `C.int` 的长度，所有 ABI error path 初始化空 response，output copy 没有 1 GiB 固定 slice ceiling。
4. package version 不能影响 output 目录结构；全部现有合法版本命名不变。
5. WebSocket integration 缺失 completion 时在 deadline 内失败。
6. protocol、folded 和 exact multi-span oracle 不调用对应生产 helper，并能捕获注入的错误结果。
7. span 外 bytes 在五种格式的 fuzz seed 和随机输入中保持不变。
8. 性能 candidate 只有满足明确 benchmark 门槛才进入最终 diff。
9. 所有最终 verification 命令通过，或对每个未通过命令给出可独立复现的具体阻塞证据。
10. 不修改或提交任何 CLIProxyAPI 源码。
