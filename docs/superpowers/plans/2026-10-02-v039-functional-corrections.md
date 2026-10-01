# v0.3.9 普通功能修正 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 连续完成已确认的插件普通功能修复、TDD、回归、原生注册、全量验收、本地 main 合并及 v0.3.9 的 GitHub 构建与发布。

**Architecture:** 保留现有明确 provider selectors、共享 matcher 和 JSON token 重建。共享 foundation 负责标准库文本解码、编码载体的 parent/children 和最终字段约束；OpenAI、Claude、Gemini 分别接入无冲突文件。正常 oracle、阶段回归、native integration 和发行验收在依赖合并后由单一实施者处理共享文件。

**Tech Stack:** Go 1.26、CGO native ABI v1、`encoding/json`、`unicode/utf8`、已固定 `gjson v1.18.0`、`github.com/router-for-me/CLIProxyAPI/v7 v7.2.152`、Go tests、GitHub Actions。

**Spec:** `docs/superpowers/specs/2026-10-02-v039-functional-corrections-design.md`

## Global Constraints

- 基线 `2d4dc5da26f4bbea574ad3008fd0943e9a510e23`/v0.3.8；固定 CPA host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`、配置 schema 5、native ABI v1。
- 只开发本插件；不修改 CPA、本次无关代码、依赖 pin、旧 spec 或历史记录；不手改 `.integration/`、`dist/`。不使用 windows-mcp 截图或鼠标工具。
- 网络安全方向保持停止，不新增 fuzz campaign、攻击测试或相关研究。既有正常 oracle/seed 的同步只通过普通 `go test` 执行。
- 保留 `words` Object、原词条及 bucket 顺序、legacy handwritten YAML、`block` -> `strip` -> `obfs`、rule-major、最早节点、no cross-node、cascade 和取消语义。
- BeforeAuth/selected-auth AfterAuth、filter、metadata、canonical role、signature/annotations/dormant description 与 `C.GoBytes` 保持现有语义。无变化不得改 body 或 entity headers。
- 只选明确自然语言字段；复用现有 parser、matcher 和 rebuild helper；禁止泛用 walker、额外依赖、虚假数据、为通过测试的 fallback、额外配置和无必要抽象。
- 每个行为先观察预期断言失败的 RED，编译错误不算复现；组内阶段各自完成 GREEN、相关回归、自查和提交。每个完整 worktree task 由负责该任务的 workflow 对完整 `BASE..HEAD` 组织独立规格及质量审查；问题在原 worktree 修正、重测、复审并新建提交，全部完成后主动报告。
- 主会话只整合已完成且文件范围无冲突的 commit。不能把未完成测试、review 或提交交回主会话。等待后台结果采用主动通知，不轮询日志或状态。
- 所有命令在负责任务的 worktree 根执行；shell 中需要路径时使用该 worktree 的绝对路径。临时测试、日志和下载资产使用系统临时目录，保留用户已有文件。

## Review Focus

1. 编码载体 children 的 offset 小于前一个普通 span 的 offset：全局 block 仍按 parent 的原始位置和规则顺序处理，不能混合坐标排序。Task A2、B1、E2 覆盖。
2. 多个字段改变，但一个 carrier 的 strip/obfs 最终取消：该 carrier 的原外层 token 和 headers 的 no-op 条件不能被其他 parent 的 Changed 传播污染。Task A2、B1、E2 覆盖。
3. function scalar output 编码为多模态数组，text 单独没有超限但重建后的完整 parent 字符串超限：按 parent 总字符数拒绝；custom output 不继承限制。Task A3、B2 覆盖。
4. Claude 相邻 assistant/MCP history，tool-only scope：普通 assistant 不改，MCP 用 tool；非末尾 scalar 的非空判断按真实数组位置。Task C1/C2、E2 覆盖。
5. canonical schema 显式 `null`/`{}` 且 alias 含词条：canonical 存在时仍遮蔽 alias，legacy `functions` 不凭名称扩大。Task B3、E1/E2 覆盖。

## 文件、接口与执行依赖

| worktree 任务 | 文件归属 | 启动条件 | 使用模型 |
|---|---|---|---|
| A，共享 foundation | `selectors.go`、`transform.go`、`transform_test.go`、新 `selectors_text_test.go`、`benchmark_test.go` | 文档审查完成；当前基线 | Opus[1m]/xhigh |
| B，OpenAI | `selectors_openai.go`、`selectors_openai_test.go` | A 完成并整合 | Sonnet[1m]/xhigh |
| C，Claude | `selectors_claude.go`、`selectors_claude_test.go` | A 完成并整合 | Sonnet[1m]/xhigh |
| D，Gemini | `selectors_gemini.go`、`selectors_gemini_test.go` | A 完成并整合，与 B/C 并行 | Sonnet[1m]/xhigh |
| P，packager | `.github/scripts/package-release.go`、相邻 windows/other 平台源码、`package-release_test.go`、相邻 Windows 原生测试、`Makefile`、`.github/workflows/build.yml` 的相关编译命令与 Windows 回归步骤 | 普通 Unicode 与 CLI/cwd 补核已完成，限定文档复核完成后与 A 无冲突并行 | Sonnet[1m]/xhigh |
| E，普通 regression/oracle/native | `fuzz_test.go`、新 `functional_corrections_test.go`、`integration/http_test.go` | A/B/C/D/P 全部完成并整合 | Sonnet[1m]/xhigh |
| F，版本 docs/全量验收 | `README.md`、`RELEASE_NOTES.md`、`main_test.go` 中 `TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions` | E 完成并整合 | Sonnet[1m]/xhigh |
| G，本地 main/远端发行 | Git ref、tag、实际 GitHub 状态 | F 完成，全部审查无未修正问题 | 主会话负责已完成 commit 的整合和原授权发布 |

A/B/C/E 各是表中一个完整的 worktree 编码任务，组内编号仅表示同一实施者连续完成的 TDD 阶段，不单独派发 agent。A 按 A0/A1/A2/A3 顺序执行，B 按 B1/B2/B3，C 按 C1/C2/C3，E 按 E1/E2；阶段分别 RED/GREEN、回归、自查、提交。任务启动时记录实际 BASE，负责该任务的 workflow 在整项完成后对完整 `BASE..HEAD` 组织独立规格和质量审查。发现问题仍由原实施者在同一 worktree 修正、重测、复审并新建提交；全部完成后才交付主会话，不自行额外派发重复 reviewer。D/P/F 也按各自完整 task 执行同一审查门槛。B/C/D 互不写共享文件，必须并行；P 与 A 无源文件冲突，不占用 `.integration/run`。E/F 的 integration、ABI smoke 及实际 runner 只允许一个进程负责。

共享接口约定：

```go
// 现有公开内部入口保持原签名。
func selectTextSpans(body []byte, sourceFormat string, roles scopeSet) ([]textSpan, error)
func transformRequest(body []byte, sourceFormat string, cfg *configSnapshot) (transformResult, error)
func applyMode(spans []textSpan, cfg *configSnapshot) (*blockMatch, bool)
func rebuildBody(body []byte, spans []textSpan) ([]byte, error)

// A 为真实共享处理建立最小测试入口，B/C/D 不改其实现。
func transformSelectedSpans(body []byte, spans []textSpan, cfg *configSnapshot) (transformResult, error)
func appendToolResultStringSpan(spans *[]textSpan, value gjson.Result, role string, roles scopeSet)
```

`textSpan` 新增 `MaxCodePoints int` 与 `EncodedChildren []textSpan`。`EncodedChildren==nil` 表示普通 span；非 nil 表示已识别 parent，即使 slice 长度为零。parent.Text 为解码后的完整内层 JSON，child.RawStart/RawEnd 相对该字符串。映射 parent 到 flat slice 的范围只用本次处理局部数据，不增加持久状态或新的 matcher。

---

### Task A0：保持行为的共享处理测试入口

**Files:** `transform.go:59-123`；验证 `transform_test.go`、全量既有测试。

**Interfaces:** 保留 `transformRequest`；产出上述 `transformSelectedSpans`，使真实 carrier helper 的结果能在 provider 尚未接入时测试同一生产 pipeline。

- [ ] **Step 1：记录当前状态。** 核对 worktree HEAD、branch、`git status --short`，运行 `make test`。同时运行并保存现有 benchmark 基线，使用同一环境、顺序执行：

```bash
go test -run '^$' -bench '^(BenchmarkTextPartScanning|BenchmarkEmptyStringSpans|BenchmarkDisabledRoleSelectors|BenchmarkGeminiSystemOnly|BenchmarkTransformScenarios)$' -benchmem -benchtime=100ms -count=3 .
go test -run '^$' -bench '^BenchmarkTransformMatrix$/^body=(1024|1048576|20971520)$/^words=(0|1)$/^fold=false$' -benchmem -benchtime=100ms -count=3 .
```

- [ ] **Step 2：只提取当前核心。** 把 `transformRequest` 在 `selectTextSpans` 成功后的当前正文，即基线第 70-122 行，原样移入 `transformSelectedSpans`。原入口最后一行改为下式，不更改任何 gate 或错误处理、不重排已有逻辑：

```go
return transformSelectedSpans(body, spans, cfg)
```

新增函数接收已选择 spans、原 body 和 snapshot；其初始行为完全等于被移动正文。该提取只服务本次共享数据流和真实单元测试，不增加 runtime interface、mock selector 或重复 pipeline。

- [ ] **Step 3：核对行为不变。** `make test`、`git diff --check`；审查只有上述局部移动，已有 block、nested fallback、取消、签名和 bytes 断言都通过。
- [ ] **Step 4：阶段自查并提交。** 核对局部提取与行为不变证据，修正后重跑；只提交 `transform.go`，例如 `refactor: isolate selected span transformation for functional tests`。A1/A2/A3 在同一 worktree 继续，独立规格/质量审查由 A workflow 在整项完成后覆盖完整 `BASE..HEAD`，本阶段不另行派发 reviewer。

### Task A1：selected string token 的标准库解码

**Files:** `selectors.go:115-132,207-216`；Create `selectors_text_test.go`；必要的 `transform_test.go` 定向回归。

**Interfaces:** `appendStringSpan`、`appendNonEmptyStringSpan` 的 caller 结构保持；复用已有标准库解码方式，`Text` 的语义与 `json.Unmarshal` 一致，Raw offset和role不变。

- [ ] **Step 1：新增 `TestSelectedTextUsesStandardJSONDecoding`。** 独立标准库解码同一个 raw token作为期望，测试 selected span 的文本。surrogate fixture 必须保持原 JSON escape，不能先经 `json.Marshal` 变成 replacement rune。最小测试内容：

```go
func TestSelectedTextUsesStandardJSONDecoding(t *testing.T) {
    raw := []byte("\"" + string([]byte{92}) + "ud800" + string([]byte{92}) + "u0065xample note\"")
    var want string
    if err := json.Unmarshal(raw, &want); err != nil { t.Fatal(err) }
    body := append([]byte(`{"messages":[{"role":"user","content":`), raw...)
    body = append(body, []byte(`}]}`)...)
    spans, err := selectTextSpans(body, "openai", scopeSet{"user": {}})
    if err != nil { t.Fatal(err) }
    if len(spans) != 1 || spans[0].Text != want {
        t.Fatalf("spans=%#v want=%q", spans, want)
    }
}
```

将同样 token 放入五种格式正常文本入口，分别做 high/low surrogate 后续 BMP、有效 pair和普通 escape；覆盖 exact/folded 的三动作以及 strip 另一词条后的未匹配字符保留。增加 Responses flat 全 strip 后激活 nested 的标准库解码断言，未激活对照必须 no-op。

- [ ] **Step 2：观察 RED。** `go test . -run '^TestSelectedTextUsesStandardJSONDecoding' -count=1 -v`；预期现有 `Text` 丢失 `e` 的断言失败。新增 mode/nested 用例也必须出现符合已完成报告的失败，不以编译失败代替。
- [ ] **Step 3：最小修复。** 未转义 token 保持快速路径；escaped `value.Raw` 走 `json.Unmarshal`，复用 `canonicalJSONMemberName` 的成熟解码实现方式，保持 key 去重行为。赋给 Text 的是标准库结果。意外解码错误明确显露，不返回错误 Str、不静默跳过、不增加所有 provider 的通用 parser。只在本任务确有必要时调整最小内部 helper 名称/调用。
- [ ] **Step 4：GREEN和逐字节对照。** 重跑定向测试和 `make test`；`TestSelectedTextNoOpPreservesBytes` 应验证无命中 body=nil，另一字段变化时未选字段、原始空白、`1e+03`、有效pair和 dormant token 原 bytes不变。测试标准库期望与生产解码不能共用新 helper。
- [ ] **Step 5：阶段自查、修正、提交。** 检查五种格式全部 caller、dormant 候选、空字符串和 guard 无遗漏，重跑受影响回归；只提交自有文件，例如 `fix: preserve selected JSON string characters during decoding`。在同一 A worktree 继续 A2，不另行派发阶段 reviewer。

### Task A2：carrier parent/children 和一次全局匹配

**Files:** `selectors.go`、`transform.go`、`selectors_text_test.go`、`transform_test.go`、`benchmark_test.go`。

**Interfaces:** `appendToolResultStringSpan`；`textSpan.EncodedChildren`；真实 `transformSelectedSpans` 复用现有 `applyMode`/`rebuildBody`。

- [ ] **Step 1：补齐惰性元数据和最小入口。** 新增 `EncodedChildren []textSpan`；初始 helper 仅委托现有 `appendStringSpan`，不改变行为。此时先运行 `make test`。新增测试能编译，尚未识别载体时应产生真正的保留断言 RED。
- [ ] **Step 2：写 `TestEncodedToolResultPreservesImageAndMachineBytes`。** 用合法 image URL、小型 typed text和未知字段，通过真实 helper收集，然后调用生产核心：

```go
func TestEncodedToolResultPreservesImageAndMachineBytes(t *testing.T) {
    registerConfig(t, "words:\n  strip: [SECRET]\nscope:\n  roles: [tool]\n")
    inner := `[{"type":"text","text":"SECRET tail"},{"type":"input_image","image_url":"https://example.test/SECRET.png","file_id":"SECRET-id"},{"type":"future","text":"SECRET machine"}]`
    token, err := json.Marshal(inner)
    if err != nil { t.Fatal(err) }
    body := append([]byte(`{"output":`), token...)
    body = append(body, '}')
    var spans []textSpan
    appendToolResultStringSpan(&spans, gjson.ParseBytes(body).Get("output"), "tool", scopeSet{"tool": {}})
    result, err := transformSelectedSpans(body, spans, loadedSnapshot())
    if err != nil { t.Fatal(err) }
    var got struct { Output string `json:"output"` }
    if err := json.Unmarshal(result.Body, &got); err != nil { t.Fatal(err) }
    want := strings.Replace(inner, `"SECRET tail"`, `" tail"`, 1)
    if got.Output != want { t.Fatalf("output=%q want=%q", got.Output, want) }
}
```

同一 suite 明确增加 image-only、多个 parent及普通 span、三个text type、file-ID识别、非carrier Object/原语/纯文本数组、关闭role的期望。图片命中而 text 不命中的 block 必须 no-op；普通字符串仍整串处理。

- [ ] **Step 3：观察 RED。** `go test . -run '^TestEncodedToolResult' -count=1 -v`；应是图片/ID/未知字段被整串修改或错误 block，不能是 fixture语法错误。
- [ ] **Step 4：实现明确识别。** 使用现有 parser验证内层合法数组，检查 spec中的image_url/input_image字符串URL或file_id。保留外层 parent，选 text/input_text/output_text.text children；non-nil空children代表image-only。所有内层文本仍经A1正确解码 helper，按各自Raw位置验证；其他JSON字符串调用原appendStringSpan。
- [ ] **Step 5：实现 flatten与重建。** 在 `transformSelectedSpans` 按外层token顺序展开 parent，普通spans和children共同调用一次applyMode，不把不同坐标offset全局排序；用局部parent -> flat起止范围取回changed children。保留原nested description激活逻辑。changed carrier用rebuildBody重建内层，再把changed parent与普通/activated replacements按外层位置重建。任何层的rebuild错误都返回，不忽略。
- [ ] **Step 6：固定全局顺序和取消。** 新增 `TestEncodedToolResultGlobalRuleOrder`：前方tool parent text含SECOND，后方user普通text含FIRST，配置block=[FIRST,SECOND]，期望FIRST/user；同词同时命中期望最早parent/child的role。新增 `TestEncodedToolResultCancellationPreservesOuterToken`：`a`+U+200B+`b` 的inner text，在strip U+200B再obfs ab后无replacement；另一普通span被改时该parent原token仍不变。词条分到两个inner节点时不得命中。同时覆盖cascade、duplicate rule、dormant、quotes/newline/Unicode。
- [ ] **Step 7：GREEN、分支覆盖和代价。** 重跑suite和 `make test`。使用足量正常rule/text触发现有exact matcher、fold matcher、fold preflight和KMP，断言同一规则结果；不改变threshold或matcher实现。benchmark仅增加本功能escaped文本/载体的真实transform测量，按text长度和span数量测量，不把decode放在计时之外。
- [ ] **Step 8：阶段自查、修正、提交。** 特别检查 image-only 的零 children、同坐标 bounds 验证、parent 取消、一次全局匹配和未知字段保留，修正并重跑后提交，例如 `fix: filter only text inside encoded multimodal tool results`。在同一 A worktree 继续 A3，不另行派发阶段 reviewer。provider 尚未接入，注册验收由 B 和 E 实施，不宣称入口已经修复。

### Task A3：最终字符数机制

**Files:** `transform.go`、`transform_test.go`；必要的shared测试。

**Interfaces:** 新增 `textSpan.MaxCodePoints int`；0无约束，使用`utf8.RuneCountInString`，最终changed字段检查。

- [ ] **Step 1：加入惰性字段和失败测试。** 新字段不改变旧行为，写 `TestFinalMaxCodePointsRejectsExpansion`，直接给真实selected核心传入受限span：

```go
func TestFinalMaxCodePointsRejectsExpansion(t *testing.T) {
    registerConfig(t, "words:\n  obfs: [ab]\n")
    body := []byte(`{"text":"ab"}`)
    var spans []textSpan
    appendStringSpan(&spans, gjson.ParseBytes(body).Get("text"), "user", scopeSet{"user": {}})
    spans[0].MaxCodePoints = 2
    result, err := transformSelectedSpans(body, spans, loadedSnapshot())
    if err != nil { t.Fatal(err) }
    if !result.Invalid || result.Body != nil {
        t.Fatalf("result=%#v want invalid without replacement", result)
    }
}
```

加入limit最终恰好接受、limit+1拒绝、0无约束、unchanged原超限不校验、strip->obfs最终合法/超限、取消、block优先、Unicode、有效surrogatepair。carrier parent设置小limit的synthetic正常fixture检验完整重建后的inner JSON长度，不能仅按子text长度。

- [ ] **Step 2：观察RED。** `go test . -run '^TestFinalMaxCodePoints' -count=1 -v`，应是现有核心接受超限输出。
- [ ] **Step 3：最终检查。** 全部动作、取消和activated block结束后，按最终Changed检查约束；children检查最终text，parent在inner rebuild后检查完整Text、outer rebuild前拒绝。返回已有InvalidMessage，不截断或跳规则：

```go
if span.Changed && span.MaxCodePoints > 0 && utf8.RuneCountInString(span.Text) > span.MaxCodePoints {
    return transformResult{Invalid: true, InvalidMessage: "censorship rewrite would make a text field invalid"}, nil
}
```

保持RequiresNonEmpty/RequiresUnmodified的含义和优先级；如child/parent需同一约束判定，复用同一段明确逻辑，不新增可配置validator。

- [ ] **Step 4：GREEN和共享验收。** `go test . -run '^(TestSelectedText|TestEncodedToolResult|TestFinalMaxCodePoints)' -count=1 -v`、`make test`、`make race`、`make vet`；顺序复测A0 benchmark，记录escaped/carrier实际成本。新field默认不附加provider限制，provider字段由B/C设置。
- [ ] **Step 5：完整 foundation 审查、修正、复审、提交。** 完成 A3 自查和阶段提交后，由负责 A 的 workflow 对全部 A0-A3 的 `BASE..HEAD` 组织独立规格及质量审查。原实施者在同一 worktree 修正、重跑、复审并新建提交，直至全部问题完成。给协调方报告最终 HEAD、各 commit、共享 helper 签名、真实 RED/GREEN、完整 task review 和 benchmark 范围；A 全部完成后 B/C/D 才创建基于该完成提交的独立 worktree。

### Task B1：OpenAI明确载体入口

**Files:** `selectors_openai.go:48-95,152-164`、`selectors_openai_test.go`。不得修改shared、oracle、integration或docs。

**Interfaces:** 消费A的 `appendToolResultStringSpan`、parent/children处理；保留现有OpenAI实际array选择。

- [ ] **Step 1：四入口RED。** 新增 `TestOpenAIEncodedMultimodalToolResults`，四个正常body分别为Chat `role=tool`、legacy `role=function` scalar content，以及Responses function/custom scalar output。inner为合法text+image数组，scope=[tool]，三动作分别检查自然文本命中，以及只有 image/ID/unknown 字段命中时的 no-op。最小Chat fixture构造：

```go
inner := `[{"type":"text","text":"SECRET tail"},{"type":"image_url","image_url":{"url":"https://example.test/SECRET.png"}}]`
token, err := json.Marshal(inner)
if err != nil { t.Fatal(err) }
body := append([]byte(`{"messages":[{"role":"tool","tool_call_id":"call-1","content":`), token...)
body = append(body, []byte(`}]}`)...)
registerConfig(t, "words:\n  strip: [SECRET]\nscope:\n  roles: [tool]\n")
resp := interceptRPC(t, "openai", body)
var got struct { Messages []struct { Content string `json:"content"` } `json:"messages"` }
if err := json.Unmarshal(resp.Body, &got); err != nil { t.Fatal(err) }
if got.Messages[0].Content != strings.Replace(inner, `"SECRET tail"`, `" tail"`, 1) {
    t.Fatalf("rewritten body=%s", resp.Body)
}
```

- [ ] **Step 2：观察RED。** `go test . -run '^TestOpenAIEncodedMultimodal' -count=1 -v`，应是四入口仍整串改写/错误block。
- [ ] **Step 3：只接入已有canonical tool。** Chat归一化role为tool时scalarcontent调用新helper；其他role保持appendStringSpan。Responses function/custom scalaroutput调用helper，实际array的input_text.text仍按现有规则。shell/MCP等不新增载体识别。
- [ ] **Step 4：GREEN和边界。** 四入口验证text/input_text/output_text、image-only、file_id、普通JSON文本、actual array、disabledrole、多parent混合、全局顺序和取消原token。使用真实logo或小型标准库生成有效PNG验证URL未变与可解码，不能用伪base64宣称图片保持有效。
- [ ] **Step 5：阶段回归、自查、提交。** `go test . -run '^TestOpenAI' -count=1`、`make test`；核对 legacy converter 反例，不把 legacy 直转 Codex 称为图片转换。修正并重跑后提交自有文件，在同一 B worktree 继续 B2；不另行派发阶段 reviewer。

### Task B2：Responses明确字段上限

**Files:** `selectors_openai.go:152-164,255-336`、`selectors_openai_test.go`。

**Interfaces:** 消费 A 的 `MaxCodePoints`。将 function/custom 共享 helper 的签名改为 `appendOpenAIResponsesInputTextOutput(output gjson.Result, roles scopeSet, spans *[]textSpan, maxCodePoints int)`，function 传 `10485760`，custom 传 `0`，所有直接 caller 同步该参数。

- [ ] **Step 1：实际字段RED。** `TestOpenAIResponsesOutputMaxCodePoints`表覆盖function scalar、function actual input_text.text、shell stdout、shell stderr、apply_patch output和program result。为每条构造最终limit+1：

```go
const limit = 10485760
text := strings.Repeat("x", limit-2) + "ab"
token, err := json.Marshal(text)
if err != nil { t.Fatal(err) }
body := append([]byte(`{"input":[{"type":"function_call_output","call_id":"call-1","output":`), token...)
body = append(body, []byte(`}]}`)...)
registerConfig(t, "words:\n  obfs: [ab]\nscope:\n  roles: [tool]\n")
resp := interceptRPC(t, "openai-response", body)
if !resp.Terminate || resp.StatusCode != 400 || resp.Body != nil || gjson.GetBytes(resp.ResponseBody, "error.code").Str != "censorship_invalid_request" {
    t.Fatalf("response=%#v", resp)
}
```

为每条增加最终limit可接受、完整strip允许空。custom scalar/typed、local_shell、普通message/promptvariable input_text为无本表限制的对照。测试大文本顺序运行，避免子测试并行造成不必要内存占用。

- [ ] **Step 2：观察RED。** `go test . -run '^TestOpenAIResponsesOutputMaxCodePoints' -count=1 -v`，应为schema超限输出被接受。
- [ ] **Step 3：标记精确span。** helper只标记本次真正追加的function scalar parent或实际typedtext；custom保持0。拆分local_shell/apply_patch共同case中的长度标记，不改变选择语义。stdout/stderr、program标记10485760。不要给function编码carrier的所有text type另加独立typed约束，scalarparent约束检查其完整Text。
- [ ] **Step 4：parent边界回归。** 新增 `TestOpenAIResponsesEncodedOutputParentMaxCodePoints`：合规的整个编码数组在最终rewriting后整体超过limit，虽子text远小于limit仍拒绝。使用正常的未知 metadata 字段填充到边界，图片URL不改；同形custom输出继续允许。未改变已超限function输出且另一字段发生改写时原token保留；Unicode、escape、strip->obfs、取消和block优先保持。
- [ ] **Step 5：GREEN、阶段自查、提交。** 聚焦 suite、全部 OpenAI 及 `make test`；RPC 确认 invalid 无替换 body、无请求 header 修改、响应 Content-Type=application/json。自查修正并重跑后提交自有文件，在同一 B worktree 继续 B3；不另行派发阶段 reviewer。

### Task B3：Chat/Responses兼容schema fallback

**Files:** `selectors_openai.go:33-46,108-131,408-413`、`selectors_openai_test.go`。

**Interfaces:** 复用 `appendJSONSchemaDescriptions`；只新增Chat tools `parametersJsonSchema` 和Responses format `json_schema.schema`，保留canonical-first存在性。

- [ ] **Step 1：三动作 RED。** `TestOpenAICompatibleSchemaFallbacks` 用下面 Chat fixture，以及 Responses `{"input":"safe","text":{"format":{"type":"json_schema","json_schema":{"schema":{"type":"object","description":"SECRET tail","properties":{}}}}}}`，developer-only 分别测 block/strip/obfs，根与 properties description 都命中：

```go
body := []byte(`{"messages":[{"role":"user","content":"safe"}],"tools":[{"type":"function","function":{"name":"lookup","parametersJsonSchema":{"type":"object","description":"SECRET tail","properties":{}}}}]}`)
registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: [developer]\n")
resp := interceptRPC(t, "openai", body)
if !resp.Terminate || gjson.GetBytes(resp.ResponseBody, "error.role").Str != "developer" {
    t.Fatalf("response=%#v", resp)
}
```

- [ ] **Step 2：观察RED。** `go test . -run '^TestOpenAICompatibleSchemaFallbacks' -count=1 -v`；预期两个alias漏选。
- [ ] **Step 3：存在性回退。** Chat tools用下式；Responses format同理，fallback为format.Get("json_schema.schema")，仍在type=json_schema及developer gate内：

```go
schema := function.Get("parameters")
if !schema.Exists() { schema = function.Get("parametersJsonSchema") }
appendJSONSchemaDescriptions(spans, schema, "developer", roles)
```

现有Responses function五项链保持原样。legacy functions现有description/parameters/output_schema保留，不新增alias；不为了helper复用自动扩大其选择范围。

- [ ] **Step 4：GREEN和优先级。** `TestOpenAICompatibleSchemaPrecedence`表覆盖主字段missing、null、{}、安全object，alias含词条；只有missing回退。root/propertydescription变化外，name、keys、enum、const、default、extensions、metadata原bytes保留。developer关闭、unknown type和machine-only no-op。legacyalias-only维持未选，legacycanonical继续三动作正常；明确记录Chat->Claude不消费legacy的对照。
- [ ] **Step 5：完整 B task 审查、修正、复审、提交。** 全部 OpenAI、`make test`，先完成 B3 阶段自查和提交；负责 B 的 workflow 对 B1-B3 完整 `BASE..HEAD` 组织独立规格及质量审查。原实施者在同一 worktree 修正、重跑、复审并新建提交，全部完成后主动交付最终 HEAD、全部 commit、审查结果和明确限制。不修改 oracle 文件，留给 E 单一归属任务。

### Task C1：Claude assistant MCP history

**Files:** `selectors_claude.go:58-140`、`selectors_claude_test.go:86-149`。

**Interfaces:** 普通assistant own scope，MCP canonical tool；普通tool_result user-only不变。

- [ ] **Step 1：确定性12case矩阵。** `TestClaudeAssistantMCPHistory`遍历scalar/typed、三动作、[tool]/[assistant,tool]。正常assistant MCP result带tool_use_id/name/server_name/is_error等保留字段。最小block断言：

```go
registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: [tool]\n")
body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"safe"},{"role":"assistant","content":[{"type":"mcp_tool_result","tool_use_id":"call-1","content":"SECRET tail"}]},{"role":"user","content":"continue"}]}`)
resp := interceptRPC(t, "claude", body)
if !resp.Terminate || resp.StatusCode != 400 || gjson.GetBytes(resp.ResponseBody, "error.role").Str != "tool" {
    t.Fatalf("response=%#v", resp)
}
```

- [ ] **Step 2：观察RED。** `go test . -run '^TestClaudeAssistantMCPHistory' -count=1 -v`；预期12个漏选断言失败。
- [ ] **Step 3：修复两处gate。** system维持独立gate；user/assistant自身或tool启用时遍历content。mcp_tool_result只允许user/assistant且tool已启用。保留scalar appendStringSpan、typed appendNonEmptyStringSpan、ordinarytext own scope和ordinarytool_result user-only。
- [ ] **Step 4：GREEN及保留。** 同矩阵全部通过；typed全strip本地invalid、scalar全strip允许空、userMCP兼容、tool关闭、普通assistant tool-only不选和ordinarytool_result assistant不选均测。用replaceRawTokens/bytes.Equal只替换预期文字，保留ID/名称/input/cachecontrol/is_error/unknown。不新增fuzz、不改变默认oracle角色。
- [ ] **Step 5：全部 Claude、阶段自查、提交。** `go test . -run '^TestClaude' -count=1`、`make test`；核对 scalar/typed 的不同非空边界，修正并重跑后提交，在同一 C worktree 继续 C2，不另行派发阶段 reviewer。

### Task C2：非末尾assistant scalar非空

**Files:** `selectors_claude.go:54-85`、`selectors_claude_test.go`。

**Interfaces:** 使用ForEach数组index和实际数组长度判定末尾；复用RequiresNonEmpty，不删除/合并消息。

- [ ] **Step 1：RED。** `TestClaudeHistoryAssistantScalarNonEmpty`用以下正常history和全strip词条：

```go
registerConfig(t, "words:\n  strip: [Hello.]\nscope:\n  roles: [assistant]\n")
body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"Greet me."},{"role":"assistant","content":"Hello."},{"role":"user","content":"Continue."}]}`)
resp := interceptRPC(t, "claude", body)
if !resp.Terminate || resp.StatusCode != 400 || resp.Body != nil || gjson.GetBytes(resp.ResponseBody, "error.code").Str != "censorship_invalid_request" {
    t.Fatalf("response=%#v", resp)
}
```

- [ ] **Step 2：运行观察。** `go test . -run '^TestClaudeHistoryAssistantScalarNonEmpty' -count=1 -v`，当前仍放行空history的断言失败。
- [ ] **Step 3：精确标记。** user或实际非末尾assistant scalar走appendNonEmptyStringSpan，实际最后assistant走appendStringSpan；其他role、typed块和top-levelsystem保持原行为。不要把最后一个“被选中的消息”当作真正末尾。
- [ ] **Step 4：GREEN和相邻message对照。** 最后assistant全strip允许空、已有空prefill无改写、历史部分strip正常；相邻assistant中前一个/非末尾元素全strip拒绝，仅最后元素清空允许。历史和最后typedtext全strip仍拒绝，部分strip正常；user非空和scope-disabled no-op不变。
- [ ] **Step 5：阶段回归、自查、提交。** 全部 Claude 与 `make test`；明确本地 400/允许空结果，不声称商业 API 实测。修正并重跑后提交，在同一 C worktree 继续 C3，不另行派发阶段 reviewer。

### Task C3：Claude document.title上限

**Files:** `selectors_claude.go:147-194`、`selectors_claude_test.go`。

**Interfaces:** 消费MaxCodePoints，复用appendClaudeResultText，direct user与nested tool document均500；title原RequiresNonEmpty保留。

- [ ] **Step 1：两条真实路径RED。** `TestClaudeDocumentTitleMaxCodePoints`构造 `strings.Repeat("x",498)+"ab"` 并通过json.Marshal生成title，分别放在userdocument和usertool_result.content内document；scope为各自user/tool，obfs ab后要求400无body。断言核心：

```go
if !resp.Terminate || resp.StatusCode != 400 || resp.Body != nil || gjson.GetBytes(resp.ResponseBody, "error.code").Str != "censorship_invalid_request" {
    t.Fatalf("response=%#v", resp)
}
```

- [ ] **Step 2：观察RED。** `go test . -run '^TestClaudeDocumentTitleMaxCodePoints' -count=1 -v`，原500title扩张为501仍放行。
- [ ] **Step 3：只标记title。** 使用before/len模式仅对成功追加的document.title span设500，context/search_result.title不加上限。
- [ ] **Step 4：GREEN和边界。** direct/nested每条最终500接受、501拒绝；中文、emoji、escape/pair按codepoints；strip->obfs最终合法/超限、取消、block优先、未变化title bytes、disabledrole、title全strip非空拒绝。context/search_resulttitle正常扩张不受500限制。
- [ ] **Step 5：完整 C task 审查、修正、复审、提交。** C3 全部测试、自查和阶段提交完成后，由负责 C 的 workflow 对 C1-C3 完整 `BASE..HEAD` 组织独立规格及质量审查。原实施者在同一 worktree 修正、重跑、复审并新建提交，全部完成后报告所有阶段及修正 commit。不得限定为恰好三个 commit，不修改 shared/oracle/integration/docs。

### Task D：Gemini四个generationConfig alias

**Files:** `selectors_gemini.go:58-60`、`selectors_gemini_test.go:310-371`。

**Interfaces:** 复用appendJSONSchemaDescriptions和developergate；保留既有三个schema路径、role推断和signedtext。

- [ ] **Step 1：七路径正常表。** `TestGeminiGenerationSchemaAliases`表为responseSchema、response_schema、_responseJsonSchema、response_json_schema、responseJsonSchema、response_json_schema_ordered、responseFormat.text.schema。前六设置responseMimeType=application/json，第七设置responseFormat.text.mimeType=application/json，每个请求仅一种schema表示；分别三动作、根/propertydescription、developer-only。最小alias例子：

```go
body := []byte(`{"contents":[{"role":"user","parts":[{"text":"safe"}]}],"generationConfig":{"responseMimeType":"application/json","response_schema":{"type":"object","description":"SECRET tail","properties":{}}}}`)
registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: [developer]\n")
resp := interceptRPC(t, "gemini", body)
if !resp.Terminate || gjson.GetBytes(resp.ResponseBody, "error.role").Str != "developer" {
    t.Fatalf("response=%#v", resp)
}
```

- [ ] **Step 2：观察RED。** `go test . -run '^TestGeminiGenerationSchemaAliases' -count=1 -v`，四alias漏选，三个既有路径正常。
- [ ] **Step 3：只扩展固定列表。** 现有列表增加四个确切名称，不加generation_config根或response_format，不更改proto键名。只description进入matcher。
- [ ] **Step 4：GREEN。** 七路径三动作、user-only、machine-only、keys/enum/default/required/metadata逐bytes保留；既有function_declarations、role推断和signatureguard通过。
- [ ] **Step 5：审查、修正、复审、提交。** `go test . -run '^TestGemini' -count=1`、`make test`；给E报告确切字段列表。禁止同时编辑fuzz_test.go。

### Task P：Windows Unicode 打包产物路径

**Files:** Modify `.github/scripts/package-release.go:394-453`、`.github/scripts/package-release_test.go` 的 Unicode/CLI/Make/workflow 断言、`Makefile:50,56`、`.github/workflows/build.yml:25,58-66,135,182`；Create `.github/scripts/package-release_windows.go`、`.github/scripts/package-release_other.go`、`.github/scripts/package-release_windows_test.go`。所有新源码均为相邻 `package main`；README 统一由 F 修改。其他 task 不修改 Makefile/build.yml；发布资产格式、release job 和请求处理保持。

**Interfaces:** 所有既有 `pathsAlias(firstPath, secondPath string) (bool, error)` caller 保持；`existingPathAncestor(path string) (os.FileInfo, string, []string, error)` 增加实际祖先路径；两个平台源码各实现 `unicodeNamesAlias(ancestorPath, firstName, secondName string) (bool, error)`。测试复用 `packagerCommand(t *testing.T, args ...string) *exec.Cmd` 选择当前运行平台的 helper 并构造绝对源码的双文件 Go 命令；caller 原 `Dir`/`Env` 和参数不变。ASCII 比较仍由当前 `asciiEqualFold` 负责，平台函数只补充其没有判为相同的名称。

- [ ] **Step 1：核对基线并准备可编译的文件列表。** 普通核验已确认 `pluginOnlyFix=true`，使用 spec 第 7 节的实测对照。先记录现有 packager suite 结果，再新增相邻 windows/other 源码，初始 `unicodeNamesAlias` 仅返回 `false, nil`，尚不接入 `pathsAlias`，保持原行为以观察真正 RED。公共 CLI 的七个构造点 `710、838、880、944、963、1006、1182` 改用以下 helper，删掉由此未使用的原局部 `script`/`err`；原 `cmd.Dir`/`cmd.Env` 和参数不动，Git/validate-version caller 不修改：

```go
func packagerCommand(t *testing.T, args ...string) *exec.Cmd {
    t.Helper()
    script, err := filepath.Abs("package-release.go")
    if err != nil { t.Fatal(err) }
    helper := "package-release_other.go"
    if runtime.GOOS == "windows" { helper = "package-release_windows.go" }
    sources := []string{"run", script, filepath.Join(filepath.Dir(script), helper)}
    return exec.Command("go", append(sources, args...)...)
}
```

此时 Windows 用主源码、windows helper、公共 test 三文件，Linux 用主源码、other helper、公共 test 三文件重跑既有 suite，必须仍通过。负向 CLI 用例的非零返回不足以证明业务拒绝，保留全部正常成功用例。根因修改始终位于共享 `pathsAlias`，覆盖 direct 与 aggregate 的全部 library/output、output/output 和 LICENSE/output caller。
- [ ] **Step 2：Unicode basename/父目录 RED。** 新增 `TestPathsAliasAbsentUnicodeOutputs` 和 `TestDirectPackageRejectsAbsentUnicodeOutputAliases`，使用 `t.TempDir` 中正常库、保留文件、未存在的 `É.zip`/`é.zip`，以及未存在的 `É`/`é` 父目录内同 basename。测试在自有另一探测目录用 `os.Stat`/`os.SameFile` 确认当前目录属性和实际名称身份，不能创建目标输出后再测 preflight。默认 Windows 不敏感目录必须拒绝；Unix 或敏感目录中的合法不同输出按相应成功用例测试。最小 preflight 断言：

```go
if _, _, _, err := validateDirectPackagePaths(library, archive, checksum); err == nil {
    t.Fatal("Unicode archive/checksum aliases accepted")
}
```

CLI 使用 `packagerCommand(t, "-version", "1.2.3", "-library", library, "-archive", archive, "-checksum", checksum)`，保留原 caller cwd。要求非零 exit 且错误包含 `archive and checksum must refer to different files`，库及保留文件 bytes 不变，目标 archive/checksum 不出现，无 `.tmp-`/`.backup-` 残留。禁止把 `undefined`、源码不存在或 module 导入错误记作业务拒绝。
- [ ] **Step 3：观察 RED。** Windows 运行 `go test .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go -run '^(TestPathsAliasAbsentUnicodeOutputs|TestDirectPackageRejectsAbsentUnicodeOutputAliases)$' -count=1 -v`；必须是实际 Unicode alias 被接受的断言失败，不能是 fixture、导入或平台编译错误。
- [ ] **Step 4：实现平台 helper 与原生测试。** Windows 使用 Go 标准库 `syscall` 调用 `kernel32.dll`：通过目录 handle 的 `GetFileInformationByHandleEx(FileCaseSensitiveInfo)` 查询属性，敏感目录返回 `false, nil`，不敏感目录用 `CompareStringOrdinal` 忽略大小写比较；字符串到 UTF-16 使用标准库转换。区分 API 的相等、不等与失败，关闭 handle 并传递资源清理错误。缺失父目录沿用已核验的属性继承；错误不得改用 Unicode folding 或继续生成输出。非 Windows 不补充 Unicode alias：

```go
func unicodeNamesAlias(ancestorPath, firstName, secondName string) (bool, error) {
    return false, nil
}
```

`package-release_other.go` 使用 `//go:build !windows`；显式命令仍须只列一个平台 helper。新增 Windows test 文件，`TestUnicodeNamesAliasWindowsCaseRules` 对照真实文件身份：不敏感目录 `É`/`é` 相同，`K`/`K`、`Σ`/`ς`、`S`/`ſ`、`𐐀`/`𐐨` 不同；敏感目录 `É`/`é` 不同。不得用 `strings.EqualFold`/`strings.ToUpper` 实现预期或生产比较。
- [ ] **Step 5：接入共享 preflight。** `existingPathAncestor` 成功返回 `info, path, suffix, nil`，错误返回四项，同步其两个 caller。保留相同路径判定、SameFile 和 suffix 长度判断；局部比较循环为：

```go
for i := range firstSuffix {
    if asciiEqualFold(firstSuffix[i], secondSuffix[i]) {
        continue
    }
    same, err := unicodeNamesAlias(firstAncestor, firstSuffix[i], secondSuffix[i])
    if err != nil { return false, err }
    if !same { return false, nil }
}
```

`firstAncestor` 来自新返回项，第二个祖先路径可丢弃，因为已确认 SameFile。Windows `TestUnicodeNamesAliasDirectoryError` 用已不存在的自有目录和 Unicode 不同名称确认 API 错误；原 caller 必须传递错误而不写输出。不增加可配置 facade 或 fallback。
- [ ] **Step 6：Make host 编译条件的 RED。** 新增 `TestMakePackagerUsesHostCompilation` 与 source-selection dry-run 断言。把当前真实 Makefile 和三个 packager 生产源码原 bytes 复制到 `t.TempDir` 的原目录结构，放入相对 dist 中的正常库和 LICENSE；不改写 recipe，不读写用户 dist。为选定实际 aggregate recipe，用 `GOOS= GOARCH=` 解析原 Makefile，再通过第二个只设环境的临时 Makefile，使用 GNU Make 自身 parser 在之后导出 cross tuple。Windows host 的该文件内容为：

```makefile
override GOOS := linux
override GOARCH := arm64
export GOOS GOARCH
```

Linux host 的目标设 windows/arm64。执行 `make -f Makefile -f target-env.mk package GOOS= GOARCH= VERSION=1.2.3`，cwd 为自有复制目录；实际执行原 aggregate recipe 和 `packageExistingArtifacts`，无需跨目标 library build。期望真实 ZIP/checksum 成功，库 bytes、CRC、固定时间和 SHA-256 正确；修复前应因 recipe 继承 cross target 导致 host 工具无法编译/运行而 RED，不能只 printf helper 或复制一个不同的 recipe。另用当前真实 Makefile 的 `make -n package-platform VERSION=1.2.3 GOOS=linux GOARCH=arm64` 断言 library build 保留原目标、packager 单独使用 host；`validate-version` 在不存在 Go 命令时仍维持原行为。
- [ ] **Step 7：只在 packaging recipe 内选择及强制 host。** 两个 recipe 分别查询 GOHOSTOS、GOHOSTARCH，任何查询失败终止；主文件和一个 helper 由 host 选择。保持 library build 的 GOOS/GOARCH，仅给最终 packager Go 命令加 host 环境。direct 的完整后续 recipe 为：

```makefile
host_os="$$( $(GO) env GOHOSTOS)" || exit $$?; \
host_arch="$$( $(GO) env GOHOSTARCH)" || exit $$?; \
helper=./.github/scripts/package-release_other.go; \
if [ "$$host_os" = windows ]; then helper=./.github/scripts/package-release_windows.go; fi; \
GOOS="$$host_os" GOARCH="$$host_arch" $(GO) run ./.github/scripts/package-release.go "$$helper" -version "$$PACKAGER_VERSION" -library "$(LIBRARY)" -archive "$(ARCHIVE)" -checksum "$(CHECKSUM)"
```

aggregate 保持相同前四行，最后一行为：

```makefile
GOOS="$$host_os" GOARCH="$$host_arch" $(GO) run ./.github/scripts/package-release.go "$$helper" -dist dist -out dist -version "$$PACKAGER_VERSION"
```

不新增解析期 Go 调用或全局改变 GOOS/GOARCH。重跑真实 cross-target 环境测试确认 GREEN，dry-run 断言 host/target 分离，不能以环境观测 marker 代替产物。
- [ ] **Step 8：GREEN、合法 Unicode 与 cwd/相对参数。** 重跑 alias RED suite，用当前真实 DLL 的双源码 CLI 确认拒绝及输入保留。默认目录不同重音、Unicode sidecar、四组原生不等名称应成功；Windows 敏感目录的 `É.zip`/`é.zip` 与未存在共同父目录中的同组输出、Unix 同形输出应成功。另新增 `TestPackagerRelativePathsOutsideModule`，无 go.mod 的 cwd 中放 library/LICENSE，传相对 archive/checksum，实际核验 ZIP 中库与 LICENSE bytes、CRC、固定时间和 SHA-256。原六个 CLI test 均须通过，Dir/Env 与 Git version 来源不变。特殊目录属性准备不可用时明确 skip，不计为通过；默认 Windows alias 用例不得跳过。保留 ASCII alias、已存在 case-distinct/SameFile、输出保护和 aggregate 集合。
- [ ] **Step 9：平台完整 file-list test/race/vet。** Windows 使用四个文件，Linux 使用三个文件，均在同一完成源码根内顺序执行：

```bash
go test .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go .github/scripts/package-release_windows_test.go -count=1 -v
go test -race .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go .github/scripts/package-release_windows_test.go
go vet .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go .github/scripts/package-release_windows_test.go
```

```bash
go test .github/scripts/package-release.go .github/scripts/package-release_other.go .github/scripts/package-release_test.go -count=1 -v
go test -race .github/scripts/package-release.go .github/scripts/package-release_other.go .github/scripts/package-release_test.go
go vet .github/scripts/package-release.go .github/scripts/package-release_other.go .github/scripts/package-release_test.go
```

当前补核没有计入合规的新 Linux runtime 验收，P 须在自身获允许的工作区完成实际 Linux 生产回归。其他 Windows 文件系统、旧 Windows、macOS 未动态核验的限制保留。
- [ ] **Step 10：同步三个 Ubuntu 命令和 Windows CI。** `build.yml:25` 改为 `go test .github/scripts/package-release.go .github/scripts/package-release_other.go .github/scripts/package-release_test.go`；`:135/:182` 的 run 在主文件后加入 `./.github/scripts/package-release_other.go`，其余 version/library/archive/checksum 参数原样保留。公共 test 的 `1901/1962/1982` 同步断言完整源码列表；Windows amd64 matrix 的 MSYS2 setup 后、package 前新增：

```yaml
- name: Test Windows packager path comparison
  if: "${{ matrix.GOOS == 'windows' && matrix.GOARCH == 'amd64' }}"
  shell: msys2 {0}
  run: go test .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go .github/scripts/package-release_windows_test.go -count=1 -v
```

先更新命令及新 step 的 workflow 断言观察旧 YAML RED，再改 YAML 并 GREEN。其他 matrix/触发条件/release job 保持。G 必须确认该 Windows step 实际 success；不能用交叉编译/Ubuntu tests 或 skip 代替。
- [ ] **Step 11：完整 P task 审查、修正、复审、提交。** 阶段自查和提交后，由 P workflow 对完整 `BASE..HEAD` 组织独立规格及质量审查，原实施者在同一 worktree 修正、重测、复审并新建提交。只提交 packager/平台源码及测试、Makefile、build.yml；向 F 提供准确的双源码/cwd/host 编译/Unicode 说明，主动报告全部真实 RED/GREEN、最终 HEAD、commit 和平台限制。补核 API/cwd marker 不作为生产 ZIP/sidecar GREEN。

### Task E1：两个正常oracle和普通seed同步

**Files:** `fuzz_test.go:1582-1660,1782-1805,1987-2015,2346-2415,2557-2580,2780-2810`及现有正常seed区。只有E拥有此文件。

**Interfaces:** 现有oracleAppendOpenAIChatTool/oracleOpenAIResponseSpans/oracleGeminiSpans及raw对应函数；保留独立encoding/json/raw parser，不调用生产selector。

- [ ] **Step 1：新增定向 oracle 断言。** `TestProtocolOracleCompatibleSchemaAliases` 用 B3 两个 fixture 和 D 四个 alias，检查根/property description 与 developer role。两个现有接口为 `oracleProtocolSpans(format string, body []byte) ([]oracleProtocolSpan, bool)` 和 `oracleRawProtocolSpans(format string, body []byte) ([]oracleRawStringToken, bool)`。测试不复制生产 helper，先因 oracle 漏选而 RED：

```go
func TestProtocolOracleCompatibleSchemaAliases(t *testing.T) {
    body := []byte(`{"messages":[{"role":"user","content":"safe"}],"tools":[{"type":"function","function":{"name":"lookup","parametersJsonSchema":{"type":"object","description":"SECRET tail"}}}]}`)
    spans, ok := oracleProtocolSpans("openai", body)
    if !ok { t.Fatal("oracle rejected normal body") }
    found := false
    for _, span := range spans {
        if span.Text == "SECRET tail" && span.Role == "developer" { found = true }
    }
    if !found { t.Fatalf("missing alias description: %#v", spans) }
    raw, ok := oracleRawProtocolSpans("openai", body)
    if !ok { t.Fatal("raw oracle rejected normal body") }
    found = false
    for _, span := range raw {
        if span.Text == "SECRET tail" && span.Role == "developer" { found = true }
    }
    if !found { t.Fatalf("raw oracle missing alias description: %#v", raw) }
}
```

将测试扩成上述六条正常路径的表，增加 canonical null/{}、安全 canonical、machine-only 和 legacy alias 不选择对照。oracle 保持现有默认角色；关闭 scope 的运行行为由 B/D/E2 的真实 transform/RPC 测试验证，不假定 oracle 有未定义的 scope 参数。
- [ ] **Step 2：运行RED。** `go test . -run '^TestProtocolOracleCompatibleSchemaAliases' -count=1 -v`；生产已修复而oracle尚未识别新字段。
- [ ] **Step 3：同步精确路径。** 两个oracle仅加Chat tools参数fallback、Responsesformatfallback和Gemini四路径；主字段存在含null/{}仍优先。legacyalias不扩展。默认roles保持system/developer/user，不把tool/assistant矩阵强塞入默认oracle，也不加入carrier的泛用递归解析。
- [ ] **Step 4：GREEN。** 普通 `go test . -run '^TestProtocolOracle' -count=1`、`make test`运行既有和小型正常seed，保留rawtoken/machinebytes断言。不运行 `-fuzz`。
- [ ] **Step 5：阶段自查、修正、提交。** 验证 oracle 独立性、逐个 fallback 的主字段 null 对照，修正并重跑后提交，例如 `test: cover normal schema aliases in independent protocol oracles`。在同一 E worktree 继续 E2；不另行派发阶段 reviewer。

### Task E2：阶段回归、实际native注册与HTTP

**Files:** Create `functional_corrections_test.go`；Modify `integration/http_test.go`。复用 `.github/scripts/integration-runner.go`，不改生成的CPA checkout。

**Interfaces:** `callInterceptRequestAt(method string, request pluginapi.RequestInterceptRequest)`、`selectedAuthMetadata()`、`requireNoOpResponse`、`newMockUpstream`、`startCPA`、`postJSON`、`decodeCensorshipError`和upstream.arrivalCount。fixture路径与当前harness一致，不猜测新host API。

- [ ] **Step 1：确定性RPC阶段表。** `TestFunctionalCorrectionsBothInterceptionPhases`对每类新增路径测试三动作，model-free用BeforeAuth；include target-model且selectedAuthMetadata用AfterAuth。AfterAuth的Model=target-model、RequestedModel/body.model/source为decoy，不得变更gate。最小request调用：

```go
resp, err := callInterceptRequestAt(pluginabi.MethodRequestInterceptAfter, pluginapi.RequestInterceptRequest{
    RequestID: "functional-after", SourceFormat: "openai-response",
    Model: "target-model", RequestedModel: "requested-decoy",
    Metadata: selectedAuthMetadata(), Body: body,
})
if err != nil { t.Fatal(err) }
```

model-bearingBeforeAuth、无selected marker、filter不命中、role/format关闭分别no-op。rewrite的ClearHeaders为原三项，blocked/invalid无replacement，取消无ClearHeaders，metadata和机器bytes保留。受限typed/MCP/历史scalar的非空及limit错误使用独立预期，不统一所有路径。

- [ ] **Step 2：HTTP正常回归。** 给每类明确增加 `TestHTTPFunctional...`：Claude MCP、历史scalar/title，Chat toolsfallback，Responsesformatfallback/载体/上限，Gemini四alias，共享decode五种格式。使用实际原生插件注册+固定CPA，localfixture检查block/invalid=400且zero upstream；rewrite检查接收文本、图片、机器字段。原生注册版本、capabilities/ConfigFields及ABI保留由已有smoke加实际注册日志核对。

```go
response, err := decodeCensorshipError(body)
if err != nil || status != 400 || response.Error.Code != "censorship_blocked" || response.Error.Role != "developer" {
    t.Fatalf("status=%d body=%s error=%v", status, body, err)
}
if upstream.arrivalCount() != 0 { t.Fatal("blocked request reached upstream") }
```

carrier需三个实际图片consumer路径：Chattool->Codex、Responsesfunction/custom->Chat；legacyfunction只验证插件语义及原始字段，不能称其converter转换图片。file-ID结构保留不称为商业文件服务验证。MCP/directClaude经过实际CPA本地fixture，不声称商业API接受。

- [ ] **Step 3：integration的RED/GREEN证据。** 在系统临时目录中保存本任务新测试，用Go overlay/独立临时构建让基线v0.3.8运行同样正常assertions确认对应RED，再使用当前集成HEAD确认GREEN；不复制或修改CPA core生产源。若有基线运行条件无法满足，保留B/C/D已有真实RED和精确限制，不能捏造新integration RED。测试本身仍须在当前native注册下实际通过。
- [ ] **Step 4：独占 runner。** 运行 `make integration`。另执行 `make build VERSION=v0.3.9` 生成当前 Windows DLL，再运行 `go run ./.github/scripts/integration-runner.go -abi-smoke dist/windows_amd64/censorship.dll`；Linux 环境对应执行 `make build VERSION=v0.3.9` 和 `go run ./.github/scripts/integration-runner.go -abi-smoke dist/linux_amd64/censorship.so`。注册/HTTP/ABI 实际日志保存到自有系统临时目录。现有 `make integration` 的 `buildPlugin` 不注入版本，其功能验证库保持 `0.0.0-dev`；版本 `0.3.9` 由本步骤另行构建的正式库和 staged ABI smoke 核对，不修改 runner 或增加第二个版本来源。不得使用后续 F 的产物满足前置依赖。避免同时重建同一 `.integration/run`。
- [ ] **Step 5：补查覆盖与复审。** 对照已完成86文件ledger和原14域完成记录，只继承完成报告。把新增/改动文件逐项列入本次spec/compliance review；确认未恢复停止方向、未改默认oracle角色、没有漏掉普通功能候选。`make test`、`git diff --check`。
- [ ] **Step 6：完整 E task 审查、修正、复审、提交。** E2 测试、自查和阶段提交完成后，由负责 E 的 workflow 对 E1-E2 完整 `BASE..HEAD` 组织独立规格及质量审查。原实施者在同一 worktree 修正、重跑、复审并新建提交，全部完成后主动报告最终 HEAD、全部 commit、每类路径/阶段/平台的具体结果和商业 API 未测试边界。清理本 task 临时实验，保留需要核验的日志。

### Task F：v0.3.9文档和最终全量验证

**Files:** `README.md:23,138-162,187,208-214`、`RELEASE_NOTES.md` 顶部；`main_test.go:1184-1227` 中 `TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions` 的当前与历史版本断言。不增加 version 源码或依赖文件修改，不改其他 main_test.go 测试。

**Interfaces:** 版本通过 `-X main.pluginVersion`，bare source 仍 `0.0.0-dev`；Makefile 的 `VERSION=v0.3.9` 生成 `0.3.9`。release 含七个平台；现有文档测试同步当前 v0.3.9，保留 v0.3.8 及更早历史。

- [ ] **Step 1：版本断言 RED。** 先只修改既有 `TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions`：当前 header 和错误信息更新为 v0.3.9，结束边界指向原 v0.3.8 section，当前 compatibility 使用以下确切文本：

```go
const currentHeader = "# Censorship v0.3.9\n\n## v0.3.9 fixes\n"
currentEnd := bytes.Index(raw, []byte("\n# Censorship v0.3.8\n"))
const currentCompatibility = "v0.3.9 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+."
```

历史 compatibility sentence 列表新增原 v0.3.8 的完整句子，其余旧版本和 superseded notice 断言全部保留：

```go
"v0.3.8 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.",
```

运行 `go test . -run '^TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions$' -count=1 -v`，旧文档必须因当前标题仍为 v0.3.8 产生预期 RED。不要改 `pluginVersion`、注入方式或降低断言。
- [ ] **Step 2：更新当前说明并 GREEN。** README 现状写 v0.3.9/固定 CPA；明确 carrier 例外、text token 标准库语义、所有上限字段、MCP assistant、history prefill、两个 schema fallback、四 Gemini alias 和已通过 packager 行为，采用 P 返回的准确 Unicode/ASCII/目录属性、双源码 CLI、原 cwd/相对参数和 host 编译说明。README 的 Windows CLI 使用主文件加 `package-release_windows.go`，Unix 使用主文件加 `package-release_other.go`；从仓库外 cwd 运行时两个源码使用绝对路径，flags/输出路径仍相对 caller cwd。历史审计命令原文保留。JSON 字符串通常仍整串自然文本，只有明确识别载体按内层 text 处理。RELEASE_NOTES 顶部新增 `# Censorship v0.3.9`、`## v0.3.9 fixes`、实际 GREEN 修复及上述兼容句；原 v0.3.8 和全部更早 section 原文保留。重跑同一文档 test 确认 GREEN；不留下撤销结论、虚假性能保证或商业 API 声称。
- [ ] **Step 3：全量 Windows 验收。** 顺序执行并核对真实输出，任何失败诊断、修正、重跑之后才继续：

```bash
make test
make race
make vet
go test .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go .github/scripts/package-release_windows_test.go -count=1 -v
go test -race .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go .github/scripts/package-release_windows_test.go
go vet .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go .github/scripts/package-release_windows_test.go
make build VERSION=v0.3.9
make package VERSION=v0.3.9 GOOS=windows GOARCH=amd64
make package VERSION=v0.3.9
make integration
go run ./.github/scripts/integration-runner.go -abi-smoke dist/windows_amd64/censorship.dll
git diff --check
```

- [ ] **Step 4：全量 Linux 验收。** 在可用 Linux 目录使用同一完成 HEAD；不要直接读取指向 Windows 绝对 .git 路径的 worktree。沿用可核实 Linux 工作目录/干净 pinned CPA checkout，执行 test/race/vet；packager test/race/vet 显式使用 `package-release.go`、`package-release_other.go`、`package-release_test.go` 三文件，命令与 P9 的 Linux block 完全相同。继续 build/package VERSION=v0.3.9 GOOS=linux GOARCH=amd64、integration/.so 注册 smoke；host 生产源保持固定 commit 和未修改。报告实际命令和目录，不声称执行未运行的平台。
- [ ] **Step 5：代价核验与资产。** A 基线与当前同环境 benchmark 顺序比较 text decoding、常规 transform、escaped 规模、carrier 数量/文本长度及 rebuild，记录差异和采样限制。核对本地 ZIP 内容、library 版本 0.3.9、lowercase SHA-256 sidecar 和 aggregate checksums。无必要的新优化不开发。
- [ ] **Step 6：完整 spec/compliance review。** 全部改动对照 spec 逐项核对、代码质量审查、修正、复审至无问题。每个 task 的 RED/GREEN、阶段/机器 bytes/no-op/图片/约束/注册证据均已完成；版本文档 test 已在旧文档观察 RED，并在当前文档 GREEN，历史记录保留。不能把待办留在发布门槛之后。
- [ ] **Step 7：提交文档及最终可核验结果。** 只提交 README.md、RELEASE_NOTES.md 和 main_test.go 中明确归属的版本文档断言，不提交生成产物、临时日志或其他测试改动。报告所有完成 commit 供主会话整合。

### Task G：本地main合并和GitHub构建/发行

**Files:** Git本地分支/tag、真实GitHub工作流和release资产，不写生产代码。

**Interfaces:** 原 `.github/workflows/build.yml` 已在push main及tag v*触发Build；test依赖先完成，七个平台build后release。所有前置task必须先完成。

- [ ] **Step 1：核对整合范围。** 检查每个worktree完成HEAD、允许文件和clean状态，确认无重叠未完成修改，再将完成commits非破坏性整合到集成branch。跨文件review和全量验收通过后，按既有worktree流程回到本地main；不得主动删除用户worktree或重置其他修改。
- [ ] **Step 2：合并本地main。** 检查main/ref/workingtree、remote与tag，正常merge完成branch；有冲突先真实解决、重跑受影响及全量门槛，不发布冲突未验证的代码。保护用户现有修改，不用reset --hard或forcepush。
- [ ] **Step 3：最终HEAD验收与版本。** 确认main为已验收HEAD，tag不存在且当前源仍只有buildinjection版本来源。创建v0.3.9并pushmain/tag：

```bash
git status --short
git remote -v
git tag --list v0.3.9
git tag v0.3.9
git push origin main
git push origin v0.3.9
```

命令只能在前置检查与原发布授权范围内执行；现有同tag时核对实际对象，不重复创建或移动tag。

- [ ] **Step 4：等待实际 CI 结果。** 一次读取由该次 main/tag push 产生的 Build run，核对实际 `headSha`、`headBranch` 和 event；使用已经核对的 `RELEASE_RUN_ID` 在后台执行 `gh run watch "$RELEASE_RUN_ID" --exit-status`，由工具完成通知唤回，再运行 `gh run view "$RELEASE_RUN_ID" --json conclusion,jobs,headSha,headBranch,url`。主会话不轮询后台任务日志或状态。确认 test、linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64、windows/arm64、freebsd/amd64 和 release 全部 conclusion=success；Windows amd64 job 的 `Test Windows packager path comparison` step 必须实际 success。终态后一次读取该 step 日志，核对默认 Unicode alias 拒绝用例实际通过，特殊目录模式的 skip 不计为通过。main push 的独立 run 也要核对 test/build 结论。CI 失败在相应任务 worktree 内定位、修正、验证、review 和提交，再按原发布流程处理，不隐藏失败或改写公共 asset。
- [ ] **Step 5：发行资产验收。** 下载v0.3.9实际remoteassets到新系统temp，核对恰好七个ZIP、七个sidecar和checksums.txt。用archive/zip、crypto/sha256或平台标准工具验证CRC/库/版本、64位小写hex加两个空格加basename、七条aggregatechecksums；核对isDraft=false。已有release workflow自身验证仍保留，独立下载检验不能仅用job成功代替。
- [ ] **Step 6：终结。** 返回实际commit/tag/run、每项修复及验证限制；未调用商业API，未实际加载的平台清楚说明。清理本次无需保留临时下载，仅在真实完整结果核实后宣称发布完成。

## 计划自查与证据交付

- spec第1节 -> A1；第2节 -> A2/B1/E2；第3节 -> A3/B2/C3；第4节 -> C1/C2；第5节 -> B3/E1/E2；第6节 -> D/E1/E2；第7节 -> P；保持条件和全量验收 -> E2/F/G。
- Review Focus五项各有明确失败/保留断言，parent offset不混坐标，parent 的 MaxCodePoints 和 custom 边界单独检验，MCP不用默认oracle代替确定性矩阵。
- A0的最小处理入口保持原行为，A1/A2/A3针对同一生产核心测试；不由测试复制matcher、pipeline或provider转换逻辑。
- 所有实现agent自有worktree内完成RED/GREEN、review、修正、复审和commit后主动返回；后续只在依赖完成commit上开始，不轮询后台任务。
- scalar function 上限、106 路径解码补充及 Unicode packager 普通修复可行性报告均已完成并纳入明确任务；P 拥有相邻平台源码、Makefile 的 host 编译条件、全部 CLI/workflow caller 和 Windows CI 步骤，F 拥有双源码调用说明及当前与历史版本断言。可行性与基线测试均不作为生产修复 GREEN 或发布完成证据。
