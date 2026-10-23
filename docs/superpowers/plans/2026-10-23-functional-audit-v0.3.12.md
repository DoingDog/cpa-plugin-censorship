# v0.3.12 功能修复实施计划

日期：2026-10-23。依据：[设计规格](../specs/2026-10-23-functional-audit-v0.3.12-design.md)。基线 `23fe07f1c4ba2ec23879ec2addc8015b4d7acc6d`，当前 v0.3.11，目标 v0.3.12。固定 CPA v7.2.152 / `c76dfd4e0edabab9000628b1560ab8ab379eadb8`、schema 5、ABI v1。

## 目标与执行依赖

A 修复首次无效注册后的合法空/非空 models 恢复，以及尚无有效 snapshot 时无效 reconfigure 错误返回 OK，行为以规格 L1 至 L9 为准。B 修复 Gemini `generation_config` 下七条既有 schema description 路径的漏选，行为以 G1 至 G7 为准。生产修改限于 `config.go`、`main.go` 与 `selectors_gemini.go` 的已确定位置。

跨格式 full-strip -> Claude 空 typed text 已确认存在官方 schema 冲突和真实 executor 向本地 upstream 发送的证据；在固定 host/阶段/源格式行为边界下没有完整安全的纯插件修复，按规格保留为兼容限制，仅由 U2 写入 accepted limits，不分配开发任务。

执行顺序：补全 spec -> 补全本计划 -> 独立文档复审并合并最终 spec/plan -> 从同一 main SHA 开 A、B 独立 worktree 并行完成 -> 主会话逐项审查并合并 -> 独立集成发布 worktree 完成 U2 -> 主会话合并 U2 并执行 U3。文档任务不 push、不 tag、不合并 main。

### 已完成覆盖与审查依赖

首轮 `whnjtk2x2.output` 的九份报告均已返回，`missing=[]`。覆盖逐项如下，文档阶段不重做扫描。

| 报告分工 | 已检查范围 |
|---|---|
| `config-filter-lifecycle` | 严格配置、snapshot、请求 filter、phase、caller/model 语义 |
| `matcher-transform-json` | matcher、规则顺序、JSON 验证/重建、carrier 与独立 oracle |
| `openai-selectors` | Chat/Responses selectors、相关固定 consumer 与协议依据 |
| `claude-selectors` | Claude selectors、非空/长度/保护文本边界及相关 executor |
| `gemini-interactions-selectors` | Gemini/Interactions selectors、转换与 full-strip wire |
| `native-abi-host-boundary` | RPC、ABI、ownership、固定 host lifecycle 与 native 调用 |
| `build-package-release` | Make/packager/CI、发行说明、v0.3.11 七平台资产 |
| `integration-baseline` | runner、HTTP/SSE/WS/native harness、构建及正式 DLL smoke |
| `external-protocol-crosscheck` | 五种 provider 的明确路径、官方 schema、相关 host hook |

定向核验也已返回：`wrd6p10rx.output` 两份独立报告确认 A 的真实 DLL 与 watcher HTTP；`wedd9lt46.output` 的第一份报告确认 A 的初始无效 reconfigure，第二份给出跨格式限制的 `diagnosed-no-fix`；`w3vu1uq8r.output` 前两份确认 B 的官方既有 root 与 native/HTTP，第三份补齐 matcher 定向诊断且未发现新修复项。

完整性复核已收到最终结论：`wzqrgzwnz.output` 确认 `coverage_complete=true`、`additional_confirmed_fixable_issues=[]`；46 个受控实现、测试、构建、依赖及配置文件全部有全文覆盖依据，补读 `go.sum` 后 `go mod verify` 输出 `all modules verified`。未发现第三组已确认可安全执行的插件修复。v0.3.11 main/tag CI 与七平台 15 assets 已核验成功，不把 README 的 pending 历史记录误当新 bug。A、B 与 U2 的实际执行记录见本计划最终记录；最终合并仍执行下列完整回归。

仅在新的独立复现与完整安全插件修复均成立时，才精确修订同一 spec/plan 和任务责任；不加入猜测、撤销方案、网络安全扫描、CPA core 修改或无测量性能优化。

### 工作区责任

每项编码任务由一个独立 worktree 的执行者连续完成 TDD、实现、测试、规格自审、修正、复审及英语提交，达到完成条件后主动回报。主会话仅在任务完成且确认修改位置无冲突后合并；不能接收未完成的实现或测试步骤。

A、B 必须从已合并最终 spec/plan 的同一 main SHA 创建各自独立 worktree，并行连续完成；派发前记录共同 SHA 与各自绝对路径。文件所有权如下，未列出的共享文件保持只读。

| 任务 | 独占修改文件 | 依赖 |
|---|---|---|
| A，生命周期 | `config.go`、`main.go`、`main_test.go` 的 lifecycle 区域、新增 `integration/lifecycle_recovery_test.go` | 最终 spec/plan 已复审合并 |
| B，Gemini | `selectors_gemini.go`、`selectors_gemini_test.go`、`fuzz_test.go`、`functional_corrections_test.go`、新增 `integration/gemini_generation_config_test.go` | 与 A 相同，无 A 代码依赖 |
| U2，顺序集成发布 | `README.md`、`RELEASE_NOTES.md`、`main_test.go` 的版本 guard、本计划最终记录、设计规格第 9 行覆盖完成段的精确更新 | A、B 完成、审查并合并 |

B 可复用 `integration/http_test.go` 与 `integration/harness_test.go` 的现有 helper，在自己的新 fixture 编排 native/HTTP 测试，不修改共享文件或 A 的文件。U2 不与 A、B 并行修改。主会话只合并已完整完成且确认无文件冲突的任务；新增独立任务在前置依赖满足后立即派发，现有 workflow 无空闲时新增 workflow。

各执行者在自己的 `.local-verification/functional-audit-20261023/<task>/` 保存复用脚本、命令、退出码和日志，使用本次预先加入的精确 ignore 规则，不并行修改 `.gitignore`。一次性临时文件使用 OS 临时目录。所有 shell/Go 命令显式核对自己的绝对 worktree 路径；路径消失或隔离工具无法访问时报告实际限制，不切回原仓库或修改其他 worktree。不轮询后台 workflow/subagent 输出，等待完成通知。

## A. 生命周期恢复、初始错误响应与真实调用回归

依赖：最终 spec/plan 已复审合并。文件仅为 `config.go`、`main.go`、`main_test.go`，新增 `integration/lifecycle_recovery_test.go`。行为范围：L1 至 L9。

### 公开 RPC 的两轮 TDD

1. 在 `main_test.go` 的 lifecycle 测试处复用 `lifecycleJSON`、`mustHandle`、`decodeEnvelope`、`decodeResult`、`callInterceptRequestAt` 和 selected-auth helpers。使用公开 `plugin.register`、`plugin.reconfigure`、`request.intercept_before`、`request.intercept_after` 验证。每个 case 保存并恢复全局 snapshot/callback，不依赖测试顺序，不对这些全局状态测试使用 `t.Parallel`。单元 case 可设置 nil 表示未安装；初始化是否真的为 nil 必须另由 fresh-process native 验证。
2. 第一轮 RED：加入 `TestInitialRegisterFailureRecoversOnReconfigure`，空词 register 返回 `plugin_error` 后，合法非空 models reconfigure 应在 selected-auth after 拦截 `BLOCKME`。先运行这条 focused RPC 测试，记录成功响应后仍未拦截的真实失败；再加入空 models 恢复与重复合法非空 models 对照，并完成下节 native/watcher 恢复测试的 RED。
3. 第一轮最小 GREEN：仅取消 `init()` 预装 snapshot，保留 `defaultSnapshot()`；只在 `current != nil` 时拒绝 models phase 变化。重跑同一 focused RPC 与恢复 native/watcher 测试取得 GREEN。此时不提前修改无效 reconfigure 的错误分支，不把本轮 GREEN 视为 A 完成。
4. 第二轮 RED：在第一轮 GREEN 后新增 `TestReconfigureRejectsInvalidWithoutLastKnownGood`。从未安装或首次空词 register 失败后，分别提交空词、缺失/null `config_yaml`、malformed JSON 与无效 YAML，要求 `OK:false`、`Error.Code=="plugin_error"`、拒绝日志、两阶段 no-op；随后合法空/非空 models 均能恢复。记录现有错误分支仍返回 OK 的真实失败，再加入 native 的重复无效配置仍 inactive 对照。
5. 第二轮最小 GREEN：解析失败时 nil current 返回现有 `plugin_error` envelope，已有 current 继续拒绝日志、保留 last-good、返回 OK。保留原 parser、atomic store、已有 phase 拒绝分支、nil gate 与同步 `C.GoBytes`；不加初始化 boolean 或用 `len(Rules)` 判断初始化。重跑第二轮 RPC/native 取得 GREEN，再运行第一轮恢复回归。
6. 补齐 L3 至 L7、L9：首次合法注册两个 phase、同 phase 更新、两个 phase 的 malformed envelope/缺失/null/无效 YAML last-good，以及 nil intercept/transform gate。nil interceptor 对 malformed raw envelope 必须返回精确空 RPC response，输入不变；私有状态断言只作辅助。
7. 空 YAML、`words: {}`、非空 models 零规则均是有效 snapshot。验证无效更新仍 OK/保留，再分别尝试两个方向的 phase 变化并检查原拒绝日志；随后在原 phase 加入规则，用公开请求实际拦截证明 phase 未切换。

建议 focused 测试名为 `TestInitialRegisterFailureRecoversOnReconfigure`、`TestReconfigureRejectsInvalidWithoutLastKnownGood`、`TestUninitializedInterceptorsNoOp`、`TestReconfigureKeepsPhaseForEmptyRules`；扩展既有 `TestReconfigureStoresValidSnapshotAndKeepsLastKnownGoodOnError`、`TestReconfigureRejectsModelFilterPhaseClassChanges` 与 same-phase 测试。执行者记录实际名称及命令，例如 `go -C "$W" test -count=1 -run '^TestInitialRegisterFailureRecoversOnReconfigure$' .`，第二轮替换成相应 focused 名称；`$W` 为自己的已核对绝对路径。两轮 RED/GREEN 均保留实际退出码、失败断言与改动顺序，不在修复后补写推断的 RED。

### Native 与 watcher HTTP 的 TDD

1. 在专属 `integration/lifecycle_recovery_test.go` 复用 [harness](../../../integration/harness_test.go) 的 `startCPA`、`writePluginConfig`、`postChat`、upstream capture 与清理。native case 直接使用固定 `pluginhost.New` / `Host.ApplyConfig`；[既有 `withDynamicABIHost`](../../../.github/scripts/testdata/abi_benchmark_test.go) 要求首次成功注册，仅继续用于成功路径与 active-after 对照。
2. native 每个初始状态由测试父进程启动独立子进程，使用绝对测试程序路径和有界退出等待。一个子进程内保留同一 host、DLL 路径与实例，执行失败 -> 再次无效 -> 合法恢复 -> 重复合法配置。第一轮 TDD 先验证失败 -> 合法恢复；再次无效时 `PluginRegistered==false` / `RegisteredPlugins` 无有效条目的断言在第二轮加入并记录 RED/GREEN。其他子进程覆盖首次合法注册、已有有效 snapshot 的双向拒绝、零规则 phase 保护、同 phase 更新与无效 candidate 保留。malformed raw envelope 留给 RPC；native 仅发送固定 host 实际可表达的配置。`ShutdownAll` 仅做资源清理，不能用来重置 DLL 静态 snapshot。
3. HTTP 对空与非空 models 分别启动独立 CPA 进程。初始配置含空词，确认首次 register 错误日志；clean canary 必须能正常到达 mock upstream。保持进程、DLL 与模型配置不变，使用 `writePluginConfig` 写入合法规则，并以有截止时间的 watcher 观察确认新规则生效。
4. 就绪探针与 clean canary 不计入最终请求基线。确认新配置生效后记录 upstream arrival/request 基线，再发送 `BLOCKME`，要求 canonical HTTP 400 `censorship_blocked` 且两种计数增量均为零。模型 case 用 harness 的可执行 `censorship-integration-model`，不引入无效 model/provider 对照。
5. 第一轮公开 RPC 第 3 步修改生产代码前，新增 native/watcher 恢复测试须先得到 RED；最小修复后相同场景得到 GREEN。第二轮增加重复无效配置仍 inactive 的 native RED，再修错误响应并取得 GREEN。保留 clean canary 与无效配置日志的前后对照，不能用启动失败、路由失败或 watcher 尚未观察到配置代替预期 RED。

[runner](../../../.github/scripts/integration-runner.go) 已自动复制 `integration/*.go` 与 ABI fixture，无须新增通用 runner。准备顺序必须保持：清理 runner 自己的旧生成夹具 -> 核对固定 CPA checkout -> 构建干净 CPA -> 构建当前 worktree 插件及固定 model-mapper -> 复制源夹具 -> 运行测试。CPA 必须满足已有 `vcs.revision` 与 `vcs.modified=false` 检查，不手改 `.integration/`。

完成条件：两轮 RPC TDD 及 native/watcher 对应场景均有真实 RED/GREEN；L1 至 L9 逐项对应测试与输出；`make test`、`make race`、`make vet`、`make integration`、`make build` 通过。自审实际 diff 后完成必要修正、受影响回归与复审，`git diff --check` 通过，英语提交仅包含 A 的四个文件。向主会话主动返回 commit、stat、实际命令/退出码、逐项规格证据和剩余限制，不移交未完成步骤。

## B. Gemini generation_config 的明确 schema 路径

依赖：与 A 相同的最终 spec/plan main SHA，无 A 代码依赖。文件仅为 `selectors_gemini.go`、`selectors_gemini_test.go`、`fuzz_test.go`、`functional_corrections_test.go`，新增 `integration/gemini_generation_config_test.go`。行为范围：G1 至 G7。README 路径文案与 accepted limits 全部留给 A、B 合并后的 U2。

### 明确路径与独立 oracle 的 TDD

1. 复用 `TestGeminiGenerationSchemaAliases` 的七条子路径与现有 description helper 规则，增加 `generationConfig`/`generation_config` root 维度。每例单 root、单 schema 表示与正确 MIME，覆盖根/嵌套 description、三动作 × literal/fold、developer scope 开关、machine-only、unknown root/schema、非字符串 description、no-match、取消与输入不变。先运行 focused 测试记录 snake root 的真实 RED；camel 对照须通过。
2. 修订 `TestGeminiGenerationSchemaExcludesMachineFieldsAndUndocumentedRoot`，将已确认正式的 snake root 移入选中矩阵，保留真正未知 root、`response_format`、机器字段的排除断言。新增跨入口与双 root 本地 token 顺序、规则优先级和 `block -> strip -> obfs` 对照，不把双 root 当作两份正式独立配置，也不新增 alias precedence。
3. `fuzz_test.go` 的 `TestProtocolOracleCompatibleSchemaAliases` 覆盖 7 × 2，先用明确预期证明旧 semantic/raw oracle 漏选。分别修改 `oracleGeminiSpans` 与 `oracleRawGeminiSpans` 的 root 入口，保留独立 parser 与描述白名单，不调用生产 collector/helper。修订原 snake-root excluded case，保留未知路径排除与伪造 machine enum 改写反例；补齐 `FuzzProtocolTransform` 小型正常 seeds。oracle 自身选中断言应通过，尚未修生产时 production-vs-oracle 对照仍应 RED。
4. 在 `functional_corrections_test.go` 的 Gemini fixture 扩展两 root/七子路径，复用 `functionalCall` 与既有矩阵核对两阶段、selected-auth/model/filter/role/format gate、取消和 metadata/headers/原 body 不变性。`RequestedModel` 与 body model 使用相反 decoy，model-bearing before 与 model-free after 精确 no-op。
5. 完成下节 native/HTTP 的 RED 后，生产代码仅将原七条子路径在两个明确 root 下复用现有 `appendJSONSchemaDescriptions`；沿用既有 `systemInstruction`/`system_instruction` 等显式 alias 惯例。禁止 generic walker、alias parser、键名 normalization、配置/依赖增加或共享 transform 重构。运行相同 focused、oracle、functional、native/HTTP 命令取得 GREEN。

执行者记录实际测试名与 focused 命令，例如 `go -C "$W" test -count=1 -run '^TestGeminiGenerationSchema' .`、`go -C "$W" test -count=1 -run '^TestProtocolOracleCompatibleSchemaAliases$' .` 与对应 functional 测试。每次 RED 须说明实际失败的 path/断言，保留退出码；既有被误写为 excluded 的断言不能继续充当正确行为 oracle。

### 真实 native 与本地 HTTP 的 TDD

1. 在独占新 fixture 中复用现有 `withDynamicABIHost`、`functionalHTTPFixture`、`newFunctionalDirectUpstream`、`startFunctionalDirectCPA`、`postJSON`、`decodeCensorshipError` 等 helper。可在新文件声明 B 专属 fixture，不修改 `integration/http_test.go`、harness、runner 或 A 的生命周期文件。runner 自动复制新 `integration/*.go`；使用与 A 相同的干净固定 CPA 构建顺序，不能手改 generated 文件。
2. 从自己的 worktree 构建并证明实际加载同一 DLL；native 两阶段覆盖 7 × 2 × 三动作。每个正式请求只含一个 root/schema，`response_schema` 使用正确 `OBJECT`/`STRING` 类型和 JSON MIME；其他既有兼容 alias 不额外宣称 Google 在线接收。native block 返回 canonical 400，strip/obfs 比较完整预期 body、machine bytes 与输入不变；host no-op 比较合并后的原 body/headers。保留 model/selected-auth/scope gates 对照。
3. HTTP 使用本地真实 CPA 与 Gemini upstream，两个 phase 都覆盖全部 7 × 2 × 三动作；模型使用可执行的 `censorship-integration-model`。clean canary 正常转发后另记计数基线，block 要求 canonical 400、arrival/request 增量均为零；strip/obfs 要求恰好一次实际转发、description 预期值和机器字段原字节正确。保留 developer-disabled、filter bypass、machine-only、no-match 与最终取消对照，探针不计入目标计数。
4. HTTP oracle 按固定 converter 规则处理 camel `generationConfig.responseSchema` -> `generationConfig.responseJsonSchema` 的改名；snake root 原路径保留。逐字段 raw bytes 比较属性名、enum/default/required、MIME、stop sequences、metadata、`1.2300e+00` 等机器值，不能仅比较 decode 后的值或假定整个 wire body 不变。提供方固定响应仅隔离在线服务，不证明商业 Google API 接受。
5. 在生产 collector 修改前运行新增 native/HTTP，取得 snake 漏选的真实 RED，camel 与 machine-only 对照必须可用；最小实现后运行同一命令取得 GREEN。自己完成必要运行时/构建来源检查，不把这些步骤留给主会话。

完成条件：G1 至 G7 逐项对应测试与实际证据；`make test`、`make race`、`make vet`、`make integration`、`make build` 及有界 `FuzzProtocolTransform`（`-fuzztime=3000x -parallel=2`）通过。完成规格自审、修正、受影响回归及复审，`git diff --check` 通过，英语提交仅包含 B 的五个文件。向主会话主动返回 commit、stat、实际命令/退出码、RED/GREEN 与 runtime/native/HTTP 来源证据及剩余限制。A、B 的成功不能表述为已修复跨格式空 typed text。

## U2. 顺序集成、英语发行说明与正式包

依赖：A、B 均完成、经审查且已合并；不存在同文件并行修改。独立集成发布 worktree 从合并结果开始，连续完成本节全部工作。

修改文件：`README.md`、`RELEASE_NOTES.md`、`main_test.go` 的当前发行版本断言、本计划最终记录，以及设计规格第 9 行完整性复核结论的精确更新。规格修改仅将覆盖复核状态更新为已收到的完成事实，不扩大设计。执行者核查生产代码、CI 与 packager 的合并状态，不为本任务顺手修改它们。

1. 英语 README 说明首次无效注册后的合法恢复、尚无有效 snapshot 时无效 reconfigure 的错误响应、已有 phase 限制，以及 Gemini 两个明确 root 下的七条现有 schema description 路径；更新当前版本示例至 v0.3.12。accepted limits 准确记录跨格式 full-strip 经固定 CPA 可生成违反 Claude typed-text 非空 schema 的请求、BeforeAuth 目标不可知与缺少完整纯插件修复边界；保留最末 assistant scalar 行为及 prefill 接受性未核实，不声称商用 API 实测拒绝或 A/B 已修复该限制。README 直接写最终现状，历史 spec 保留原审计记录。RELEASE_NOTES 新增 A/B 实际修复及固定兼容版本，保留 v0.3.11 等历史段落与 glibc 2.34+ 要求；baseline CI/发行成功不作为新修复。bare `pluginVersion` 保持 `0.0.0-dev`。
2. `main_test.go` 的 current-release guard 改为 v0.3.12，新边界是 v0.3.11 历史标题；把 v0.3.11 compatibility 句加入历史断言，保留其余历史检查。先记录旧 guard 对新增发行段的 RED，再修改 guard 并取得 GREEN。
3. 从最终集成源码完成下表验证。项目根 suite 不包含隐藏 `.github/scripts` 的 packager tests，必须独立执行。

| 检查 | 实际命令与要求 |
|---|---|
| 根目录完整回归 | `make -C "$W" test`、`make -C "$W" race`、`make -C "$W" vet` |
| Pinned integration 与构建 | `make -C "$W" integration`、`make -C "$W" build VERSION=v0.3.12`，包括全部 HTTP/SSE/WS/native 及 `TestDynamicABIActiveAfter` |
| 独立 Windows packager | `go -C "$W" test -count=1 .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go .github/scripts/package-release_windows_test.go`；对同一文件集合另行运行 `go test -race -count=1` 与 `go vet`，均显式保留 `-C "$W"` |
| 六个有界 fuzz 目标 | `FuzzRuleEngineAgainstOracle`、`FuzzFoldKMPAgainstOracle`、`FuzzDuplicateWalkerAgainstOracle`、`FuzzProtocolTransform`、`FuzzRebuildBodyAgainstMarshalOracle`、`FuzzByteMatcherAgainstOrderedContains`，逐个使用 `go -C "$W" test -run '^$' -fuzz '^<target>$' -fuzztime=3000x -parallel=2 .`，仅验证功能 |
| Selected-platform 正式包 | `make -C "$W" package GOOS=windows GOARCH=amd64 VERSION=v0.3.12`，保存该次 ZIP 与 checksum 供比较 |
| Aggregate 正式包 | `make -C "$W" package VERSION=v0.3.12`，核对本 worktree 已构建平台的 aggregate；本机结果不代表七平台构建 |

正式包验收复用可靠 ZIP parser，检查 CRC、精确成员 DLL 与 LICENSE、LICENSE 字节、DLL 权限、两个打包入口的 ZIP/DLL 一致性、lowercase SHA-256 sidecar 与 `checksums.txt` 精确条目。脚本、日志与每次产物证据存入专属 `release/` 子目录，不手改 `dist/`。

从正式 ZIP 提取同一 DLL，先与 dist DLL 做字节及 SHA-256 比较，再运行 `go -C "$W" run ./.github/scripts/integration-runner.go -abi-smoke "$EXTRACTED_DLL"`。另用专属可复用 native 验收脚本实际读取初始化 ABI table 与 `plugin.register` envelope，断言 ABI 1、schema 5、`Metadata.Version == "0.3.12"`、Object `words`、七个字段及 request-only capability，并验证 selected-auth active-after。不能以 SDK 常量或注册成功代替真实 schema 值。既有 `TestHTTPFunctionalNativeRegistrationAndPhases` 固定期望 `0.0.0-dev`，继续用于源码 integration，不直接用来验收正式版本 DLL。

正式包验收前记录 worktree HEAD、状态、源码/依赖 blob 与 SHA-256、工具链、命令、构建输出与 `go version -m`。按 [nested worktree provenance 说明](../../solutions/workflow-issues/go-worktree-build-provenance.md) 分别核查实际编译源码、VCS metadata 和产物一致性；本地 metadata 差异按证据解释，不能替代远端最终 revision 验收。

完成条件：完整命令真实通过，正式 ZIP 的同 DLL 实际注册与 active-after 通过，英语说明仅陈述已核实范围。更新本计划最终记录，列明 A 的两轮 TDD、B 的 RED/GREEN 与 runtime/native/HTTP 证据、实际完成的本地命令/提交及仍待 U3 的检查；主会话提供完整性 critic 的最终结论，由 U2 据实记录，未收到时标记待验收，U3 不能按覆盖复核已完成放行。执行者自审、修正、复审并用英语提交，仅包含本节必要文档/guard 修改；向主会话返回全部证据，不把远端 CI、七平台运行或发布写成已完成。

## U3. 主会话 main CI、tag 与七平台发行验收

依赖：整轮审查覆盖已收齐、所有已确认可执行修复及 U2 均完成。此节由主会话在获得发布授权的范围内执行；修复子任务不 push、不 tag、不合并主仓库，不创建 PR 或另发 issue。

1. 独立复核各 worktree 的规格覆盖、提交、命令证据与路径冲突，全部完成后合并 main。对最终 main SHA 运行并等待 [Build workflow](../../../.github/workflows/build.yml) 全部必需 job 成功，包括 Linux test/race/vet/integration、Windows amd64 DLL smoke 与七平台构建。main 分支包按现有 workflow 使用 `0.0.0-dev`，不把它误报成 v0.3.12 发行包。
2. 核对 local/remote v0.3.12 tag 尚未占用；最终 main CI 全部成功后才创建并推送 tag。等待相应 tag CI 和 release job 全部成功，不以本地成功代替远端成功。
3. 分别下载最终 main CI 产物与 tag 发行产物到各自新的空目录，检查七个平台：linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64、windows/arm64、freebsd/amd64。七个共享库的 `vcs.revision` 分别等于对应已通过 CI 的最终 main/tag SHA；缺失或不同保持未通过。另核对平台、`c-shared`、固定依赖与实际版本注入，tag 必须为 0.3.12。
4. 发行资产精确为七个 ZIP、七个 `.sha256` 和一个 `checksums.txt`，共 15 项。逐包验证 CRC、精确共享库与 LICENSE 成员、LICENSE 字节、lowercase sidecar 与 aggregate 的文件名/hash 对应；核对 Linux glibc 要求。保留 Linux `.so` 与 Windows amd64 DLL 的真实注册、schema/ABI/字段及 active-after 证据，其他平台若仅构建通过必须如实标明。
5. 确认本次各任务修改已合并、没有未提交修改或未合并提交后，退出对应任务 worktree，返回原工作目录，再通过 Git 清理本次已无用途的 worktree 与分支。保护用户原有与其他任务内容，不强制删除未保存内容；本机清理失败不阻塞其他工作，如实列出未清理项。

最终完成条件：全部审查范围逐项闭合，所有确定修复可复现且完成 TDD，最终 main CI、tag CI 与 15 个发行资产分别有真实验收证据。需保留的脚本/日志留在对应忽略目录；清理本次无需保留的临时内容，不提交庞大日志或撤销方案。

## 最终执行记录

A、B 共同起点为 `15425d4f1f23d6ecef80858612d39fe7134867ed`。A 提交 `d4ad1d7d7e8481b42662d06e7031fc78b843db9d`，B 提交 `b2fe70ca5c6fd93cc8450bf6856a3ca173e1c387`；各自的规格正确性、测试证据、最小性三项独立审查均为 `findings=[]`，两任务均为 `ready=true`，随后合并至 `013e84afa1e7155567522e689ee1c13812dacd7b`。U2 独立工作区显式 `merge --ff-only` 到该基线，初始状态干净。

| 修复 | 实际 TDD 与回归证据 |
|---|---|
| A，首次恢复 | `TestInitialRegisterFailureRecoversOnReconfigure` 先 RED 1，init/phase 最小修改后 GREEN 0；native/watcher 的有效 RED 为 `make integration` 退出 2，合法非空 models 更新仍被 phase guard 拒绝，目标请求返回 200，修复后同命令 GREEN 0。较早一次 watcher 未观察到更新，不计作有效 RED，保留原日志。 |
| A，初始错误响应 | `TestReconfigureRejectsInvalidWithoutLastKnownGood` RED 1，nil snapshot 的 `plugin_error` 分支后 GREEN 0；native 再次无效配置错误显示 active 的 RED 2，修复后 GREEN 0。L1 至 L9、零规则 phase、last-good、nil gate、输入不变性均有回归；最终 `make test/race/vet/integration/build` 各退出 0。 |
| B，明确路径 | 七路径 × 两 root 的 selector、semantic/raw oracle、functional 先分别 RED 1；两个独立 oracle 修订后明确选中断言 GREEN 0，生产比较仍 RED。native/HTTP RED 的 `make integration` 退出 2，snake block 返回 200、arrival/request 各增加 1；最小 collector 修改后原 focused、oracle、functional、native/HTTP 全部 GREEN 0。G1 至 G7、两阶段三动作与排除对照覆盖；最终 `make test/race/vet/integration/build` 及 `FuzzProtocolTransform -fuzztime=3000x -parallel=2` 各退出 0。 |

A 的逐次命令与日志保存在原 `wf_cf8c9cff-bcc-1/.local-verification/functional-audit-20261023/A/`，B 在原 `wf_cf8c9cff-bcc-2/.local-verification/functional-audit-20261023/B/`。U2 只读核对已完成报告及必要实际日志，不重做全文审计。完整性复核结论已收齐并记入本计划和规格覆盖完成段。

U2 工作区为 `C:/Users/user/Downloads/cpa-plugin-censorship/.claude/worktrees/wf_fe7a1452-a55-1`，证据均保存在其 `.local-verification/functional-audit-20261023/release/`。`commands.jsonl` 与各命令的 JSON/log 记录展开后的参数、绝对 cwd、源码 SHA-256、环境、起止时间及真实退出码。

| U2 本地检查 | 实际结果 |
|---|---|
| 版本 guard | 新增 v0.3.12 发行段后，旧 `TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions` 因首标题仍要求 v0.3.11 而 RED 1；只更新当前 guard、历史分界与 v0.3.11 compatibility 断言后，同命令 GREEN 0。 |
| 完整回归 | 合并后当前源码的 `make test`、`make race`、`make vet`、`make integration`、`make build VERSION=v0.3.12` 各退出 0；integration 包含全部既有 HTTP/SSE/WS/native、A/B 新 fixture 和 `TestDynamicABIActiveAfter`。 |
| 独立 packager | Windows main/helper 与通用/Windows test 四文件的 `go test -count=1`、`go test -race -count=1`、`go vet` 各退出 0，均显式使用 `go -C` 指向本工作区。 |
| 六个有界 fuzz | 六个计划目标逐一使用 `-fuzztime=3000x -parallel=2`，各退出 0。实际 execs 按表中目标顺序为 3000、3000、3026、3021、3000、3000。 |
| 正式包入口 | `make package GOOS=windows GOARCH=amd64 VERSION=v0.3.12` 与 `make package VERSION=v0.3.12` 各退出 0，分别保留 selected/aggregate 产物。 |

中间失败也保留：首次 `make test` 退出 2，README 改写漏掉既有文档断言的精确短语，补回同义且符合当前状态的短语后完整通过；首次 `make integration` 退出 2，既有 junction 测试收到正斜线 TEMP 路径并报告 `Invalid switch - "Users"`，改用已有 Windows 原生 TEMP/TMP/GOTMPDIR 后 focused 与完整 integration 通过。一次正式验收启动器退出 1，LibreOffice Python 的 `sys.executable` 返回目录，子进程未启动；使用同一已安装的 `python.exe` 启动器继续，不修改系统、生产代码或共享测试。最终证据检查首轮因计划末尾新增空行退出 1（内部 `git diff --check` 为 2），整理末尾格式后检查退出 0。续接时核对当前源码与全部成功命令记录的 SHA-256 一致，仅 RED 使用旧 guard；文档与版本 guard 的 focused 回归、现存正式包完整性检查和最终证据验收各退出 0，未重跑已完成的大套件。

正式 ZIP 验收、专属 native 验收、提取 DLL 的 runner `-abi-smoke`、同正式路径 `GOFLAGS="-a -x" make build VERSION=v0.3.12` 及来源核对均退出 0。selected/aggregate ZIP 字节一致，SHA-256 均为 `76428b46a6047cb55bcfa7a8c858f1a79d4ba65f3e9a4193a7a34b71c9a974f9`；dist、两次包内、提取、runner staged 及强制重建 DLL 字节一致，SHA-256 为 `aa7882014f269f5ecd7f40a7e14da8402ba0c2e0bc29dc06e857adf8d9a8b2c5`。标准 ZIP parser 确认 CRC、精确 DLL/`LICENSE` 成员、LICENSE 字节、DLL 0755/许可证 0666 权限和 1980-01-01 时间；lowercase sidecar 与只含当前 Windows amd64 平台的 aggregate 精确对应。

`native-acceptance.py` 直接读取初始化 ABI table 与 register envelope，实际为 ABI 1、schema 5、`Metadata.Version=0.3.12`、Object `words`、七个 `ConfigFields`，仅 `request_interceptor=true`。before、缺 marker 与 model miss 均 no-op，selected-auth after 实际 strip，输入和机器字段保持；固定 host 的 `TestDynamicABIActiveAfter` 也通过。`source-and-artifacts.json` 与编译 trace 证明实际输入属于 U2 工作区，生产与依赖 blob 未变，同 DLL 重建一致。本地 VCS 查询来自父仓库，记录 `vcs.revision=013e84afa1e7155567522e689ee1c13812dacd7b`、`vcs.modified=false`；它不能替代 U3 最终远端 revision 验收。Go 1.26.5 Windows amd64、GCC、CPA v7.2.152/固定 commit、schema 5、ABI 1 与同步 `C.GoBytes` 均保持。

Linux 独立验证也已完成，报告为 `wnx7u3lk5.output`。固定 `013e84afa1e7155567522e689ee1c13812dacd7b` 在 Debian、Go 1.26.0、GCC 14.2.0、glibc 2.41 下的根 test/race/vet、完整 integration、dev build 和真实 `.so` active-after smoke 均退出 0，包含 A/B 的完整 native/HTTP 矩阵。Unix packager 首次在 `GO` 为绝对路径时 test/race 各退出 1、vet 退出 0；报告定位到 `TestMakeBuildIgnoresTargetOverrides` 的命令文本断言，`GO=go` 解析到同一 executable 后 test/race/vet 各退出 0。首轮总脚本退出 1，对照重跑脚本退出 0；原失败保留，未修改产品或测试，该断言的独立核验由主会话另行负责，U2 不修改 packager。Linux 临时 clone 和脚本已清理，本次 dev `.so` 不代替 U3 正式 tag `.so` 验收。

U2 仅提交本节分配的五个文档/guard 文件，提交由本文件 Git 历史标识；复用脚本与证据留在上述忽略目录。U3 仍负责最终 main 合并及 CI、随后 tag/CI、七平台 main/tag buildinfo/revision、15 assets/checksums、正式 Linux `.so` 注册和 active-after，以及合并后 worktree 清理。本地 upstream 200 不代表商用 API 接收；跨格式空 Claude typed text、末尾 assistant scalar 与 prefill 接受性边界均未改变。
