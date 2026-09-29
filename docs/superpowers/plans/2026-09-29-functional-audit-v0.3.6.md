# v0.3.6 Functional Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 Responses 回放注释索引损坏与 Gemini 蛇形声明漏检，接入已实测有效的精确规则全未命中预检，并发布 v0.3.6。

**Architecture:** 保持已有 selector -> `textSpan` -> `applyMode` -> ABI 请求拦截链。两处选择器各作局部修复；精确预检复用 `byteMatcher`，在配置快照建立一次，在未命中的单个 span 上跳过改写规则。集成测试经已锁定 CPA 加载真实插件。

**Tech Stack:** Go 1.26、CGO/native ABI v1、CLIProxyAPI/v7 v7.2.152、`gjson`、Go 标准库测试与现有 Makefile。

**Spec:** `docs/superpowers/specs/2026-09-29-functional-audit-v0.3.6-design.md`

## Global Constraints

- 只改 censorship 插件，不改 CPA 本体；保持 schema v5、ABI v1 的指针所有权及 `C.GoBytes` 输入复制。
- `words` 为严格 Object，规则顺序始终 block -> strip -> obfs；不加递归 JSON walker、不检查机器字段。
- 既有 README/RELEASE_NOTES 可更新；不得读取旧的 `docs/superpowers/` spec/plan/tdd，也不得手改 `.integration/` 或 `dist/`。
- 本次 branch 为 `fix/20260929-functional-audit`，初始 main `7e76b3d`，最近 tag `v0.3.5`；写 spec 的提交 `a639b6e` 已完成。
- 扫描证据：Responses annotation 官方请求规范及 conversation-state 指南；Gemini REST 示例和 ProtoJSON 规范；CPA v7.2.152 的 `gemini_gemini_request.go:32-71` 保留蛇形键；现有微基准 1024 规则、20 MiB 全未命中基线 542–616 ms，预检 42–45 ms。
- 新的行为测试统一用 Object `words`；已有 legacy 测试不用重写。Git Bash 中 Windows make 会丢失 Go 模块缓存环境，验证时使用 PowerShell 显式设置 `GOPATH=C:\Users\user\go` 和 `GOMODCACHE=C:\Users\user\go\pkg\mod`。

## Review Focus

1. 带非空 annotations 的 `output_text` 且 `assistant` scope 开启时只能 block 或本地拒绝最终变更；Task 1 的 strip/obfs/block 测试覆盖。
2. 空 annotations、关闭 `assistant` scope 和未命中规则不得误拒绝合法请求；Task 1 测试覆盖。
3. 一条 Gemini 请求同时包含 `functionDeclarations` 和 `function_declarations` 时两边的描述均受 `developer` 规则检查，但 `name`/enum 不变；Task 2 测试覆盖。
4. 命中词仅在 `parameters_json_schema`/`response_json_schema` 的 description 叶子而不在普通声明 description 时仍应被发现；Task 2 测试覆盖。
5. 精确预检在一个 span 未命中、另一 span 命中以及 strip 产生 obfs 后续命中时必须维持各 span 最终文本、Changed、block 优先级；Task 3 的对照测试覆盖。

---

## 执行与合并顺序

Tasks 1、2、3 修改的源码和测试文件完全不重叠，必须同时在三个独立 git worktree 中由 Sonnet 1M xhigh 执行各自 RED -> GREEN -> 测试 -> 提交。每个实现代理只读取自己负责的源码／测试和本次新 spec／plan 中属于该任务的要求，不跨分区重复扫描；并发运行的测试只读取各自 worktree，不共享中途修改。三个任务各自完成后按 1、2、3 的顺序把提交安全整合进 `fix/20260929-functional-audit`；整合过程中遇到冲突先定位归属，不覆盖别人的更改。各任务的 Opus 1M xhigh 任务级审查可在其提交后并行进行，整合后再跑一次全套测试。Task 4 依赖 Tasks 1、2 的功能修复，Task 5 依赖全部代码和集成验证。发布只在本地 main 完成合并后进行。

### Task 1: Responses 历史输出的注释保护

**Files:** `selectors_openai_test.go`、`selectors_openai.go:418-435`。

**Interfaces:** 消费 `appendStringSpan(*[]textSpan,gjson.Result,string,scopeSet)` 和现有 `textSpan.RequiresUnmodified`/`UnmodifiedMessage`；不改接口。输出供集成测试断言 `censorship_invalid_request`。

- [ ] **Step 1: 先写失败测试。** 在 `selectors_openai_test.go` 新增 `TestOpenAIResponsesAnnotatedOutputRewrite`：用 Object 配置 `words:\n  strip: [SECRET]\nscope:\n  roles: [assistant]\n`，请求为 `{"input":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"SECRET citation","annotations":[{"type":"url_citation","start_index":7,"end_index":15,"title":"source","url":"https://example.com"}]}]}]}`。用 `interceptRPC(t,"openai-response",body)` 断言 `Terminate=true`、`StatusCode=400`、`gjson.GetBytes(resp.ResponseBody,"error.code").String()=="censorship_invalid_request"`、`len(resp.Body)==0`。表驱动再测 `words.obfs: [SECRET]`；分别检查 `words.block` 返回 `censorship_blocked`/`assistant`、`annotations: []` 可 strip 且仅更改 `text`、未命中不替换 body、`scope.roles: [user]` 不改写。
- [ ] **Step 2: 验证 RED。** `go test -run '^TestOpenAIResponsesAnnotatedOutputRewrite$' -count=1 .`，预期旧版 strip/obfs 返回改写后的 `Body` 而非本地 400；记录确切失败输出。
- [ ] **Step 3: 最小修复。** 只在 `selectors_openai.go` 的 `role == "assistant" && partType.Str == "output_text"` 且 `roles.has(partRole)` 分支，记录 `before := len(*spans)`，调用原 `appendStringSpan` 后，当 `len(*spans)!=before && part.Get("annotations").IsArray() && part.Get("annotations.#").Int()>0` 时将新 span 的 `RequiresUnmodified=true`、`UnmodifiedMessage="censorship cannot rewrite annotated text"`。别改任何索引值、其他文本字段、角色判定或 `transform.go`。核心代码形态：

```go
before := len(*spans)
appendStringSpan(spans, part.Get("text"), partRole, roles)
annotations := part.Get("annotations")
if role == "assistant" && partType.Str == "output_text" && len(*spans) != before && annotations.IsArray() && annotations.Get("#").Int() > 0 {
    span := &(*spans)[len(*spans)-1]
    span.RequiresUnmodified = true
    span.UnmodifiedMessage = "censorship cannot rewrite annotated text"
}
```

- [ ] **Step 4: 验证 GREEN 与最终抵消。** 上述测试及 `go test -run '^(TestOpenAIResponses|TestSignedRewriteCancellation)' -count=1 .` 均通过；追加一个 Object 配置 `strip: ["​"]`、`obfs: [ab]`、annotated `text:"a​b citation"`，断言最终无替换/无错误；仅 `block` 命中时仍早于注释保护。
- [ ] **Step 5: 提交。** `git add selectors_openai.go selectors_openai_test.go && git commit -m "fix: protect annotated Responses replay text"`。

### Task 2: Gemini 函数声明的 ProtoJSON 原始键名

**Files:** `selectors_gemini_test.go`、`selectors_gemini.go:32-55`。

**Interfaces:** 消费已有 `appendStringSpan` 与 `appendJSONSchemaDescriptions`；不改变 `collectGemini(gjson.Result,scopeSet,*[]textSpan)` 或 JSON body。

- [ ] **Step 1: 先写失败测试。** `TestGeminiSnakeFunctionDeclarations`：配置 Object `words.block: [SECRET]`/`roles: [developer]`，body 包含 `"tools":[{"function_declarations":[{"name":"SECRET_lookup","description":"SECRET lookup"}]}],"contents":[{"role":"user","parts":[{"text":"safe"}]}]`，期望 `censorship_blocked` 且 `error.role=="developer"`；再以 `words.strip: [SECRET]` 断言 `description` 变成 `" lookup"`，`name` 仍为 `"SECRET_lookup"`。在同测试的子用例加入 `parameters_json_schema` 和 `response_json_schema` 的 `description:"SECRET schema"`，以及 `enum:["SECRET enum"]`，用 `replaceRawTokens` 验证只改 description；同时包含 camel/snake 数组时两个独立描述都改写；`roles:[user]` 不改写。
- [ ] **Step 2: 验证 RED。** `go test -run '^TestGeminiSnakeFunctionDeclarations$' -count=1 .`，旧版 snake 描述未收集，block 不触发或 strip 无 `Body`。
- [ ] **Step 3: 最小修复。** 保持旧 `functionDeclarations` 路径，再迭代同一 tool 的 `function_declarations`，复用同一个声明处理逻辑；声明描述使用现有 helper，schema key 明确枚举 `parameters`、`parametersJsonSchema`、`parameters_json_schema`、`response`、`responseJsonSchema`、`response_json_schema`。不得泛化递归处理其它对象，也不得重写键名。工具对象内的循环：

```go
for _, declarationsKey := range []string{"functionDeclarations", "function_declarations"} {
    declarations := tool.Get(declarationsKey)
    if !declarations.IsArray() {
        continue
    }
    declarations.ForEach(func(_, declaration gjson.Result) bool {
        if !declaration.IsObject() {
            return true
        }
        appendStringSpan(spans, declaration.Get("description"), "developer", roles)
        for _, key := range []string{"parameters", "parametersJsonSchema", "parameters_json_schema", "response", "responseJsonSchema", "response_json_schema"} {
            appendJSONSchemaDescriptions(spans, declaration.Get(key), "developer", roles)
        }
        return true
    })
}
```

- [ ] **Step 4: 验证 GREEN。** `go test -run '^TestGemini' -count=1 .` 与相关 role-gate 测试通过，body 字节对照证明机器字段完全不变。
- [ ] **Step 5: 提交。** `git add selectors_gemini.go selectors_gemini_test.go && git commit -m "fix: inspect Gemini snake-case function declarations"`。

### Task 3: 生产路径的精确规则预检

**Files:** `config.go:31-46,391-448`、`transform.go:23-38,201-295`、`config_test.go`、`transform_test.go`、`benchmark_test.go`。不得修改 `matcher.go` 或 `abi_cgo.go`。

**Interfaces:** `configSnapshot.ExactRewriteMatcher *byteMatcher` 与 `textSpan.SkipExactRewrite bool` 新字段；复用 `newByteMatcher(cfg.Rules[blockEnd:],blockEnd)`、`useExactByteMatcher` 与 `byteMatcher.match(text)`；`applyMode` 的签名和返回值不变。新 flag 只标记本次快照下的单个未命中 span，不能改变 `SkipFoldRewrite`。

- [ ] **Step 1: 先写失败测试。** `config_test.go` 新增 `TestCompileSnapshotExactRewriteMatcherThreshold`，通过 `mustConfig` 创建 Object `words.strip` 含 255 与 256 个非空规则及 folded 256 规则；256 个精确规则应有 `ExactRewriteMatcher`，其余均无，已有 `ExactBlockMatcher` 只对应 block。`transform_test.go` 新增 `TestExactRewritePreflightPreservesOrderedResults`，配置 256+ 条精确 strip/obfs 与可选 block；复制快照并将基线副本的 `ExactRewriteMatcher=nil`，对 ≥16 KiB 的未命中/首中/尾中、短 span、两 span 一中一不中、重复规则、strip 后出现 obfs 命中、block 优先及最终抵消分别比较两副本的 `blocked`、`changed`、`Text`、`Changed` 和 `SkipFoldRewrite`；额外断言长文本全未命中 `SkipExactRewrite==true`、含任一初始命中/短 span 为 `false`，从而证明生产 `applyMode` 实际执行了预检。复用同一个 span 再运行另一配置时 flag 必须重置，不得错误跳过；检查不保留输入体大小副本。
- [ ] **Step 2: 验证 RED。** `go test -run '^(TestCompileSnapshotExactRewriteMatcherThreshold|TestExactRewritePreflightPreservesOrderedResults)$' -count=1 .`，新字段在旧版不存在，编译失败；若先拆结构断言，则 256 规则快照缺少 matcher。
- [ ] **Step 3: 编译 matcher。** 在 `configSnapshot` 添加 `ExactRewriteMatcher *byteMatcher`，`compileSnapshot` 开始清零它；仅 `!cfg.IgnoreCase && len(rewriteRules)>=exactByteMatcherMinRules` 时设置为 `newByteMatcher(rewriteRules,blockEnd)`。不改变 block、folded matcher 初始化或 `ExactReplacement`。精确分支在构建 `ExactBlockMatcher` 的旁边添加：

```go
if len(rewriteRules) >= exactByteMatcherMinRules {
    cfg.ExactRewriteMatcher = newByteMatcher(rewriteRules, blockEnd)
}
```

- [ ] **Step 4: 只跳过确认全未命中的 span。** 在 `applyMode` 的 block 阶段后、strip/obfs 循环前按 span 检查：`if cfg.ExactRewriteMatcher!=nil && useExactByteMatcher(len(cfg.Rules)-blockEnd,len(spans[i].Text)) { _,hit:=cfg.ExactRewriteMatcher.match(spans[i].Text); ... }`。使用 span 的 `SkipExactRewrite` 标记未命中的索引；在每次精确处理开始时清零，避免重用 span 或切换配置时留下旧值。两个改写循环只在 `!cfg.IgnoreCase && spans[i].SkipExactRewrite` 时跳过索引，匹配任何规则的 span 仍完整执行旧循环。`SkipFoldRewrite` 保持原有含义和原有折叠路径。核心代码形态（位于原有 folded preflight 之后）：

```go
if !cfg.IgnoreCase {
    for i := range spans {
        spans[i].SkipExactRewrite = false
        if cfg.ExactRewriteMatcher != nil && useExactByteMatcher(len(cfg.Rules)-blockEnd, len(spans[i].Text)) {
            _, matched := cfg.ExactRewriteMatcher.match(spans[i].Text)
            spans[i].SkipExactRewrite = !matched
        }
    }
}
```

两个原有改写循环都在调用 `stripRule`／`obfuscateRule` 前检查 `!cfg.IgnoreCase && spans[i].SkipExactRewrite`；匹配 span 不缩短原有循环。

- [ ] **Step 5: 验证 GREEN。** 运行新测试、`TestBenchmarkExactRewritePreflightMatchesBaseline`、`TestUseFoldRewritePreflightBoundary`、所有 `TestMixedRules*` 以及 `go test -count=1 ./...`。确认两个 Object 配置阶段顺序和 `RequiresUnmodified` 的最终抵消未被跳过。
- [ ] **Step 6: 验证性能而不只测假想策略。** 在 `benchmark_test.go` 加一个 `BenchmarkExactRewriteProduction`，复用 `benchmarkExactRewriteFixture`/`benchmarkSnapshot`，对子基准 `mode=strip|obfs`、`match=total-miss|last-sparse|dense`、`impl=baseline|production` 使用同一输入，baseline 是 matcher 字段置 nil 的快照副本，production 使用编译快照；每轮构造新 `textSpan` 后调用 `applyMode` 并把结果写入已有 sink。先完成预热，再在同一台机器对 baseline 与 production 跑 `go test -run '^$' -bench '^BenchmarkExactRewriteProduction$' -benchtime=2x -count=5 .`；逐模式以五次耗时中位数比较：20 MiB 全未命中 production/baseline ≤0.40，last-sparse 和 dense ≤1.25，production allocs/op 不比 baseline 多超过 1 次，bytes/op 不多超过 128 B。基准直接调用生产 `applyMode`，加上 Step 1 的 `SkipExactRewrite` 断言，两者共同证明产线确实使用预检。若门槛不成立，先调整既有大规则/大文本阈值并复测；仍不成立则撤回 Task 3 的生产代码与基准提交，不写性能发布声明，不影响 Tasks 1、2 的功能修复。
- [ ] **Step 7: 提交。** `git add config.go config_test.go transform.go transform_test.go benchmark_test.go && git commit -m "perf: preflight exact rewrite total misses"`。

### Task 4: 真实 CPA 集成断言

**Files:** `integration/http_test.go`。Task 1、2 的 RED/GREEN 单元测试先完成；本任务只加测试，不改变 CPA 与 harness。

- [ ] **Step 1: 加入 Gemini 本地 block 集成测试。** 参照 `TestHTTPGeminiSignedHistoryBlockReturnsLocalError` 新增 `TestHTTPGeminiSnakeFunctionDeclarationBlocks`：`startCPA(t,upstream.URL,true,"words:\n  block: [SECRET]\nscope:\n  roles: [developer]\n")`；POST `/v1beta/models/censorship-integration-model:generateContent`，body 含安全 `contents` 与 `tools[].function_declarations[0].description:"SECRET lookup"`，断言 HTTP 400、`decodeCensorshipError` 的 `Code=="censorship_blocked"`/`Role=="developer"`，`upstream.arrivalCount()==0`。
- [ ] **Step 2: 加入 Responses annotation 集成测试。** `TestHTTPResponsesAnnotatedReplayRejectsRewrite`：配置 Object `words.strip:[SECRET]`、`roles:[assistant]`，POST `/v1/responses`，body 含 `model:"censorship-integration-model"` 和 Task 1 的 assistant replay `input`；断言 HTTP 400、`Code=="censorship_invalid_request"`、上游到达次数为 0。
- [ ] **Step 3: 运行集成。** `make integration`（PowerShell 环境变量见 Global Constraints），期望真实 DLL 注册、HTTP/WebSocket 全部集成测试通过；失败先定位插件与 CPA 边界，再只修改本轮相关代码。
- [ ] **Step 4: 提交。** `git add integration/http_test.go && git commit -m "test: cover Gemini tools and annotated Responses in CPA"`。

### Task 5: 独立审查、完整验证与 v0.3.6 发布

**Files:** 仅 `RELEASE_NOTES.md` 及前述已计划文件。审查只看本次分支 diff，不重跑旧仓库全量扫描。发布前更新 doc，不改 Makefile/工作流，除非实际失败定位到本轮问题。

- [ ] **Step 1: 独立审查。** 用 Opus 1M xhigh 审查 `main..HEAD` 的新逻辑与测试，重点检查注释保护是否仅限历史输出、Gemini snake/camel 的明确 JSON 路径、快照 matcher 生命周期和初始命中后级联。核实每条建议的可复现输入，再修复真正的回归并重跑相关测试；不要重复先前扫描已排除的旧问题。
- [ ] **Step 2: 更新版本说明。** 在 `RELEASE_NOTES.md` 顶部增加 `# Censorship v0.3.6` 和三条针对本次实际验证的说明：Responses annotated replay、Gemini snake tool declarations、exact rewrite preflight（仅在 Step 6 的基准证实后）。保留旧发行记录，注明 CPA v7.2.152、schema 5、native ABI v1。
- [ ] **Step 3: 全量本地验证。** PowerShell 显式设置 Go 模块缓存后顺序运行 `make test`、`go test .github/scripts/package-release.go .github/scripts/package-release_test.go`、`make race`、`make vet`、`make integration`、`make build`、`make package VERSION=v0.3.6 GOOS=windows GOARCH=amd64`；检查本地 ZIP、匹配的 lowercase `.zip.sha256`、现有产物的 aggregate `checksums.txt`，`git diff --check` 和 `git status`。完整集成输出需含 `plugin loaded` 与 `plugin registered`；性能基准单独复跑。任何失败先查根因，修正后重跑。
- [ ] **Step 4: 提交说明与合并。** `git add RELEASE_NOTES.md && git commit -m "docs: release v0.3.6"`，检查 `git log main..HEAD` 与 `git diff main...HEAD --stat`；在本地切回 `main` 并 `git merge --ff-only fix/20260929-functional-audit`（若 main 前移，先核对变动再安全合并）。用 `git status --short --branch` 与 `git log -1 --oneline` 核对合并结果。
- [ ] **Step 5: 发布 tag 与触发 CI。** 先确认远端 `origin/main` 和 `v0.3.6` 不存在冲突；`git tag -a v0.3.6 -m "Censorship v0.3.6"`，`git push origin main v0.3.6`。检查 GitHub `build.yml` 的 main push、tag push 的 test、linux/darwin/windows/freebsd 构建与 tag release 结果，必要时等待完成而不提前宣布成功。用 `gh release view v0.3.6` 核对七组 ZIP、对应七个 `.zip.sha256` 和 `checksums.txt` 的名称，再 `gh release download v0.3.6 --dir .superpowers/sdd/2026-09-29-functional-audit-v0.3.6/release-assets` 下载全部实际发布附件；进入该目录，对每个 `.zip.sha256` 执行 `sha256sum -c`，再对 `checksums.txt` 执行 `sha256sum -c checksums.txt`，同时要求 checksum 文本为小写十六进制并与七个 ZIP 的集合一致。若 CI 失败，定位原因并报告/继续修复，不把未完成的 workflow 称作已验证。
