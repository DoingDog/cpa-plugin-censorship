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
- 每个行为先观察预期断言失败的 RED，编译错误不算复现；再实现 GREEN、相关测试、规格审查、修正、复审、提交。每个 agent 在自己的 worktree 连续完成这些步骤并主动报告结果。
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
| P，packager | `.github/scripts/package-release.go`、`.github/scripts/package-release_test.go` | 独立普通可行性核验完成，与其他源文件任务无冲突并行 | Sonnet[1m]/xhigh |
| E，普通 regression/oracle/native | `fuzz_test.go`、新 `functional_corrections_test.go`、`integration/http_test.go` | A/B/C/D/P 全部完成并整合 | Sonnet[1m]/xhigh |
| F，版本 docs/全量验收 | `README.md`、`RELEASE_NOTES.md` | E 完成并整合 | Sonnet[1m]/xhigh |
| G，本地 main/远端发行 | Git ref、tag、实际 GitHub 状态 | F 完成，全部审查无未修正问题 | 主会话负责已完成 commit 的整合和原授权发布 |

A 按 A0/A1/A2/A3 顺序执行，B 按 B1/B2/B3，C 按 C1/C2/C3，E 按 E1/E2；同一组留在同一个 worktree，子项分别验证、review、提交。B/C/D 互不写共享文件，必须并行。P 在自身门槛满足后独立并行，不占用 `.integration/run`。E/F 的 integration、ABI smoke及实际 runner 只允许一个进程负责。

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
- [ ] **Step 4：完成规格/质量审查并提交。** 使用 `superpowers:requesting-code-review`，修正本任务问题、重跑、复审。只提交 `transform.go`，例如 `refactor: isolate selected span transformation for functional tests`。A1/A2/A3 在同一 worktree 继续。

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
- [ ] **Step 5：审查、修正、复审、提交。** 检查五种格式全部 caller、dormant候选、空字符串和guard无遗漏；只提交本任务自有文件，例如 `fix: preserve selected JSON string characters during decoding`。

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
- [ ] **Step 8：审查、修正、复审、提交。** 特别检查image-only的零children、同坐标bounds验证、parent取消、一次全局匹配和未知字段保留，例如 `fix: filter only text inside encoded multimodal tool results`。provider尚未接入，注册验收由B和E实施，不宣称入口已经修复。

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
- [ ] **Step 5：完整foundation审查、修正、复审、提交。** 给协调方报告HEAD、各commit、共享helper签名、真实RED/GREEN和benchmark范围；A所有步骤完成后B/C/D才创建基于该完成提交的独立worktree。

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
- [ ] **Step 5：审查、修正、复审、提交。** `go test . -run '^TestOpenAI' -count=1`、`make test`；报告legacy converter反例，不把legacy直转Codex称为图片转换。提交自有文件。

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
- [ ] **Step 5：GREEN、审查、修正、复审、提交。** 聚焦suite、全部OpenAI及 `make test`；RPC确认invalid无替换body、无请求header修改、响应Content-Type=application/json。提交自有文件。

### Task B3：Chat/Responses兼容schema fallback

**Files:** `selectors_openai.go:33-46,108-131,408-413`、`selectors_openai_test.go`。

**Interfaces:** 复用 `appendJSONSchemaDescriptions`；只新增Chat tools `parametersJsonSchema` 和Responses format `json_schema.schema`，保留canonical-first存在性。

- [ ] **Step 1：三动作RED。** `TestOpenAICompatibleSchemaFallbacks`用spec两个最小fixture、developer-only分别测block/strip/obfs，根与propertiesdescription都命中：

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
- [ ] **Step 5：审查、修正、复审、提交。** 全部OpenAI、`make test`；B三子项完成后主动交付全部commit和明确限制，不修改oracle文件，留给E单一归属任务。

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
- [ ] **Step 5：全部Claude、规格review、修正、复审、提交。** `go test . -run '^TestClaude' -count=1`、`make test`；主动报告scalar/typed的不同非空边界。

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
- [ ] **Step 5：审查、修正、复审、提交。** 全部Claude与 `make test`；明确本地400/允许空结果，不声称商业API实测。

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
- [ ] **Step 5：全部Claude、review、修正、复审、提交。** C全部子项连续完成，报告三个commit，不修改shared/oracle/integration/docs。

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

### Task P：Windows Unicode打包产物路径

**Files:** `.github/scripts/package-release.go:394-435`、`.github/scripts/package-release_test.go:1212-1269`。不改Makefile、release workflow、ZIP格式或请求处理。

**Interfaces:** 现有pathsAlias/validateDirectPackagePaths preflight；实际输出写入前判定alias。此项等待协调方独立普通修复可行性报告；不得预判结果。报告若发现无法无回归修复，更新spec/plan和最终unresolved，不能把候选当作完成。

- [ ] **Step 1：核对完成报告。** 读取独立报告的实际方法、Windows/Linux/Kelvin/case-distinct结果及限制，确认根因修改可执行后才继续。
- [ ] **Step 2：Unicode basename/父目录RED。** `TestPathsAliasAbsentUnicodeOutputs`覆盖É.zip/é.zip和缺失É/é父目录内同basename。只在实际测试文件系统确认É/é属于同文件的环境要求拒绝，使用t.TempDir的临时fixture确认，不猜测系统规则。CLI测试先以未存在的输出路径运行packager，期望非零exit、库/已有输出不改变。关键preflight断言：

```go
if _, _, _, err := validateDirectPackagePaths(library, archive, checksum); err == nil {
    t.Fatal("Unicode archive/checksum aliases accepted")
}
```

- [ ] **Step 3：观察RED。** `go test .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^TestPathsAliasAbsentUnicodeOutputs' -count=1 -v`；应是实际alias被接受。保持fixture正常，不运行网络安全相关输入研究。
- [ ] **Step 4：实施经核验的共享preflight方法。** 完成报告中的方法须正确表示目标文件系统的Unicode大小写，不使用strings.EqualFold冒充Windows规则。已有SameFile、ASCII、case-distinct和Kelvin规则保留；普通非alias输入不得被拒绝，不增加依赖。写入之前拒绝alias，任何临时探测资源按原输出保留要求清理；错误明确返回。
- [ ] **Step 5：GREEN和真实CLI。** 以实际DLL调用正常packager确认不产出被checksum覆盖的ZIP；使用archive/zip和crypto/sha256检查正常输出。Windows/Linux单独全量packager test/race/vet，保留不同Unicode字符、Kelvin、已有case-distinct、ASCIIalias、既有输出保护和事务回滚测试。
- [ ] **Step 6：审查、修正、复审、提交。** 只提交两个packager文件，报告实际平台与文件系统，不声称所有filesystem已动态验证。独立报告尚未到达时保持本任务未完成，不创建虚假GREEN。

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
- [ ] **Step 5：审查、修正、复审、提交。** 验证oracle独立性，逐个fallback的主字段null对照，commit例如 `test: cover normal schema aliases in independent protocol oracles`。

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
- [ ] **Step 4：独占 runner。** 运行 `make integration`。另执行 `make build VERSION=v0.3.9` 生成当前 Windows DLL，再运行 `go run ./.github/scripts/integration-runner.go -abi-smoke dist/windows_amd64/censorship.dll`；Linux 环境对应执行 `make build VERSION=v0.3.9` 和 `go run ./.github/scripts/integration-runner.go -abi-smoke dist/linux_amd64/censorship.so`。注册/HTTP/ABI 实际日志保存到自有系统临时目录，核对 version=0.3.9；不得使用后续 F 的产物满足前置依赖。避免同时重建同一 `.integration/run`。
- [ ] **Step 5：补查覆盖与复审。** 对照已完成86文件ledger和原14域完成记录，只继承完成报告。把新增/改动文件逐项列入本次spec/compliance review；确认未恢复停止方向、未改默认oracle角色、没有漏掉普通功能候选。`make test`、`git diff --check`。
- [ ] **Step 6：审查、修正、复审、提交。** E所有测试完成并提交，主动报告每类路径/阶段/平台的具体结果和商业API未测试边界，清理本task临时实验，保留需要核验的日志。

### Task F：v0.3.9文档和最终全量验证

**Files:** `README.md:23,138-162,187,208-214`、`RELEASE_NOTES.md`顶部。不增加version源码或依赖文件修改。

**Interfaces:** 版本通过 `-X main.pluginVersion`，bare source仍0.0.0-dev；Makefile的VERSION=v0.3.9生成0.3.9。release含七个平台。

- [ ] **Step 1：更新当前说明。** README现状写v0.3.9/固定CPA；明确carrier例外、texttoken标准库语义、所有上限字段、MCPassistant、historyprefill、两个schemafallback、四Geminialias和已通过packager行为。JSON字符串通常仍整串自然文本，只有explicit识别载体按内层text处理。RELEASE_NOTES顶部新增实际GREEN修复，不重写旧版本历史；不留下撤销结论、虚假性能保证或商业API声称。
- [ ] **Step 2：全量Windows验收。** 顺序执行并核对真实输出，任何失败诊断、修正、重跑之后才继续：

```bash
make test
make race
make vet
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
make build VERSION=v0.3.9
make package VERSION=v0.3.9 GOOS=windows GOARCH=amd64
make integration
go run ./.github/scripts/integration-runner.go -abi-smoke dist/windows_amd64/censorship.dll
git diff --check
```

- [ ] **Step 3：全量Linux验收。** 在可用Linux目录使用同一完成HEAD；不要直接读取指向Windows绝对.git路径的worktree。沿用可核实Linux工作目录/干净pinnedCPA checkout，执行test/race/vet、packager、build/package VERSION=v0.3.9 GOOS=linux GOARCH=amd64、integration/.so注册smoke；host生产源保持固定commit和未修改。把实际命令和目录报告，不声称执行未运行的平台。
- [ ] **Step 4：代价核验与资产。** A基线与当前same-environment benchmark顺序比较textdecoding、常规transform、escaped规模、carrier数量/文本长度及rebuild，记录差异和采样限制。核对本地ZIP内容、library版本0.3.9、lowercaseSHA-256 sidecar和aggregatechecksums。无必要的新优化不开发。
- [ ] **Step 5：完整spec/compliance review。** 全部改动对照spec逐项核对、代码质量审查、修正、复审至无问题。每个task的RED/GREEN、阶段/机器bytes/no-op/图片/约束/注册证据均已完成，候选已明确判定；不能把待办留在发布门槛之后。
- [ ] **Step 6：提交文档及最终可核验结果。** 只提交README/RELEASE_NOTES及已批准与本task相关测试，不提交生成产物或临时日志。报告所有完成commit供主会话整合。

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

- [ ] **Step 4：等待实际CI结果。** 在后台完成通知到达后核查与目标commit/tag匹配的GitHubrun，而非反复轮询。确认test、linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64、windows/arm64、freebsd/amd64和release全部conclusion=success。CI失败在相应task worktree内定位、修正、验证、review和提交，再按原发布流程处理，不隐藏失败或改写公共asset。
- [ ] **Step 5：发行资产验收。** 下载v0.3.9实际remoteassets到新系统temp，核对恰好七个ZIP、七个sidecar和checksums.txt。用archive/zip、crypto/sha256或平台标准工具验证CRC/库/版本、64位小写hex加两个空格加basename、七条aggregatechecksums；核对isDraft=false。已有release workflow自身验证仍保留，独立下载检验不能仅用job成功代替。
- [ ] **Step 6：终结。** 返回实际commit/tag/run、每项修复及验证限制；未调用商业API，未实际加载的平台清楚说明。清理本次无需保留临时下载，仅在真实完整结果核实后宣称发布完成。

## 计划自查与证据交付

- spec第1节 -> A1；第2节 -> A2/B1/E2；第3节 -> A3/B2/C3；第4节 -> C1/C2；第5节 -> B3/E1/E2；第6节 -> D/E1/E2；第7节 -> P；保持条件和全量验收 -> E2/F/G。
- Review Focus五项各有明确失败/保留断言，parent offset不混坐标，parent 的 MaxCodePoints 和 custom 边界单独检验，MCP不用默认oracle代替确定性矩阵。
- A0的最小处理入口保持原行为，A1/A2/A3针对同一生产核心测试；不由测试复制matcher、pipeline或provider转换逻辑。
- 所有实现agent自有worktree内完成RED/GREEN、review、修正、复审和commit后主动返回；后续只在依赖完成commit上开始，不轮询后台任务。
- scalarfunction上限及106路径解码补充报告已完成，不再作为阻塞。当前唯一文档阶段待核验项是Unicode打包修复可行性报告；到达后更新TaskP和spec，不预判未完成结果。
