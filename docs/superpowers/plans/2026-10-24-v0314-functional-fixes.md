# v0.3.14 功能修复实施计划

日期：2026-10-24。依据：[设计规格](../specs/2026-10-24-v0314-functional-fixes-design.md)。源码基线 `a270cc658c6cd178c19324faa4caa759d648298d`（v0.3.13），目标 v0.3.14。固定 CPA v7.2.152 / `c76dfd4e0edabab9000628b1560ab8ab379eadb8`、model-mapper `8fe4839dd2c39a4b0537447c4ac35a9f1d699bbf`、schema 5、native ABI 1。

## 目标、当前证据与启动条件

G 完成 Gemini typed Schema `any_of` description 的明确选择，保留全部基线 description 选择及 JSON Schema 的未知 any_of 排除；R 完成 Make GO 命令与实际 executable 的边界修复，并给 full 集成保留用户覆盖能力的 30 分钟默认预算。最终交付包括 v0.3.14 文档、正式包、本地验收、最终 main/tag CI 与七平台产物核对。

文档 owner 已完整读取 G1/R1/R2 指定基线证据、G1 的 `wcgc5223x.output` 三方复核、R1 的 `wi5o1f51h.output` 三方复核和 R2 的独立预算报告，并核对相关正式来源。本轮初始九分区 `wuavn3cvf.output` 均为 done；独立完整性 critic `wlhsct59c.output` 已收齐，完整解析五份报告的全部 17 个结果正文，逐组核对基线 107 个 tracked 文件和 53 份历史 docs。其唯一阅读记录缺口 `go.sum` 已补读且 `go mod verify` 退出 0，覆盖门槛完成；去重仅 G1/R1/R2 三项功能修复，另有 U3 的 README 时限文案纠正。G1/R1 定向独立复核、R2 预算复核、完整性核查和七平台工具预检均已完成；前次文档审查 `wgveys0re.output` 的两项阻塞已修订并通过本次独立复审，U0 完成。复审者全文核对两份文档、前次审查与新修订 diff，并对照相关源码和 critic 覆盖表，未重复九分区扫描或全部历史 docs 的全文阅读，未运行 v0.3.14 产品测试。

U0 已完成，当前 spec/plan 可以启动 G/R。共同 baseline 为包含本次独立复审通过记录的准备就绪提交本身，完整 SHA 取该提交完成后的 HEAD；主会话将该提交快进到本轮独立集成分支，G/R 均从这个 SHA 创建独立 worktree，不更新 main。新报告新增可执行修复或改变已定边界时，先更新同一 spec/plan、测试责任与验收，再审查并记录新的共同起点。没有新证据时，不添加新功能、性能优化、重试、通用 parser 或 CPA core 修改，不做安全扫描。

## U0. 收齐扫描、独立复核与文档审查

主会话接收实际报告，执行者完成后主动回报；不通过 sleep、文件监视或反复读取后台任务输出轮询。本轮证据目录为 `C:/Users/user/AppData/Local/Temp/claude/C--Users-user-Downloads-cpa-plugin-censorship/bc5f7f79-933b-435d-a289-457482c5073e/tasks/`。初始九分区见 `wuavn3cvf.output`，历史文档审查见 `waqiiepxn.output`，G1/R1 三方复核见 `wcgc5223x.output`/`wi5o1f51h.output`，R2 见 `wqhzsh92v.output`，完整性结论及逐文件覆盖表见 `wlhsct59c.output`。下表据 critic 实际核对结果登记，区段阅读不计作全文覆盖；文档 owner 未重复全部扫描。

| 本轮初始分区 | 已覆盖的内容 | 已核对的阅读与执行依据 |
|---|---|---|
| config-filter | 严格 YAML/Object words、filter、snapshot/LKG、phase class、caller/model 与重配置 | owner 1 全读 config/main/request_filter 及测试共 6 文件；focused、根包 test/race 已有成功终态 |
| match-transform | 规则顺序、literal/fold、JSON 验证/重建、carrier、取消、immutable、size/final-text、独立 oracle | owner 2 全读 matcher/transform 4 文件及 selectors/fuzz/functional 等 8 文件；差分、race、六个基线 fuzz 均有终态 |
| openai | Chat/Responses 明确文本路径、机器字段、固定 converter/consumer | owner 3 全读 selectors_openai 及测试；focused/race 通过，官方 OpenAPI 请求引用链/相关组件及固定 translator 已核对 |
| claude | 明确路径、roles、description、非空/长度/保护文本与固定 executor | owner 4 全读 selectors_claude 及测试；focused/race、完整 integration 通过，相关 API 页面/SDK 类型及固定 translator 已核对 |
| gemini-interactions | Gemini/Interactions 路径、typed/JSON Schema、aliases、签名/annotations、转换 | owner 5 全读两组 selector 及测试共 4 文件；focused/race、原 native/HTTP 矩阵通过，G1 独立 RPC/HTTP 各 312 场景；官方生成 parser 已接受 96 个 typed 输入，不将全部 312 场景称为官方合法请求 |
| lifecycle-abi | RPC、C.GoBytes、pointer/length、ownership/free_buffer、固定 host lifecycle/两阶段 | owner 6 全读 abi_cgo 及测试、相关固定 host ABI 文件；Windows 实际 DLL、生命周期、缓冲区及两阶段探针已运行 |
| integration-host | runner、HTTP/SSE/WS/native/watcher、隔离、readiness/child/I/O、真实 DLL 来源 | owner 7 全读 integration 的 8 文件、runner 两文件及 ABI fixture；完整矩阵、独立 runner test/race/vet、R2 日志与 DLL 来源已核对 |
| build-package-ci | Make/packager/CI、Go 命令、版本注入、checksum/发行入口 | owner 8 全读 5 个 packager 文件、Makefile/build.yml 与固定构建 action 相关文件；本机 test/race/vet/build/package、R1 独立反例已核对，Unix helper 未在 Windows 执行 |
| external-contracts | 五种格式显式路径、正式 schema、固定 CPA hook/translator 与已接受限制 | owner 9 与各 provider owner 核对相关正式字段、完整请求 translator 及 executor 相关函数；固定接口/阶段与既有边界已汇总，未声称通读全部外部 API |

已完成的 U0 覆盖与复审门槛：

1. critic 已核对基线 53 项 docs：52 份 specs/plans/tdd 由历史文档 owner 全读，另 1 份 build-provenance solution 由 integration owner 及 critic 全读；当前 README/RELEASE_NOTES 另有全文阅读依据。历史规则、现行边界及多范围交叉关系已核对。本轮新增两份文档另列，不改变基线清单。
2. critic 逐组核对全部 107 个 tracked 文件，生产、tests、fixtures、配置、依赖、构建/CI 和文档均有全文阅读 owner 或合理排除项；`go.sum` 已补读且 `go mod verify` 退出 0，LICENSE 有打包字节依据，logo 作为品牌资源排除装饰性审查。没有未分配文件或尚待补查的覆盖缺口。
3. 基线 root test/race/vet、独立 runner/Windows packager test/race/vet、build/两种 package、30m 完整 Make 集成均有已核查的成功终态；六个基线 fuzz 各以 `-fuzztime=30s -parallel=1` 退出 0。默认 full 的成功及 10m 超时、GO=-C 的失败均按真实日志保留；这些结果不替代修复后的验收。
4. 全部候选已按实际输入、基线、命令/退出码、源码/DLL 来源、正式依据和独立复核去重，只保留 G1/R1/R2 三项功能修复；README WebSocket completion 的 20 秒文案与实际 5 秒不符，交 U3 仅改文案。R1/R2 组合测试仍由同一 R owner 完整实施；既有 accepted limits 保持，不将未运行范围写作通过。
5. 独立规格/可执行性审查 `wgveys0re.output` 的两项阻塞已通过本次独立复审：全部基线 description 选择保留，新增 any_of 仅沿规定 typed 边，JSON Schema 入口及专属分支不扩大；G/R 的共同 baseline、合回和 U3 最终验收均使用独立集成分支，全部本地完成后才由 U4 更新 main。复审者已全文读取两份文档与指定审查/定向复核/完整性/预检报告，核对相关 selectors/helper、两个 oracle、README、runner/Make 和固定 converter；基线清单确为 107 个 tracked 文件及 53 份历史 docs，覆盖依据沿用 critic 的逐文件记录，不重复九分区扫描。U0 已完成，当前文档可以启动 G/R；共同 baseline 为本次准备就绪提交完成后的 HEAD，主会话将同一提交快进到独立集成分支。当前复审者只提交这两个文件，不合并 main、不 push、不派生代理。

## 工作区责任与检查规则

| 任务 | 独占可改文件 | 依赖及交付 |
|---|---|---|
| G，typed Schema | `selectors_gemini.go`；`selectors_openai.go` 顶部共享 schema helper 的必要内部组织；`selectors_gemini_test.go`；`fuzz_test.go`；`functional_corrections_test.go`；新增 `integration/gemini_typed_schema_test.go` | U0 完成；独立 worktree 连续完成全部 G 验收、审查和英语提交 |
| R，Make/runner | `.github/scripts/integration-runner.go`；`.github/scripts/integration-runner_test.go`；负责核对 Makefile，当前 recipe 保持只读 | U0 完成；与 G 无文件依赖，独立 worktree 连续完成 R1/R2 及 wrapper/GOFLAGS 组合、相关回归、审查和英语提交 |
| U3，最终集成 owner | 英语 `README.md`、`RELEASE_NOTES.md`；`main_test.go` 的 README/当前发行 guard 必要区域；本 spec/plan 的实际记录 | G/R 均完整完成、审查并合回本轮独立集成分支；从其合并 SHA 创建独立最终集成 worktree，完成共享文档、版本和全量本地验收，期间不更新 main |
| U4，主会话 | 审查、最终 main 合并、授权范围内远端 CI/tag/release、资产核对、清理 | U3 完成；只接收已完成提交，不开 PR，不由文档/G/R owner push |

每个编码 owner 在自己的 worktree 完整执行规格审查 -> 单条纵向测试真实 RED -> 最小 GREEN -> 其他聚焦用例逐条 TDD -> 全部相关回归 -> 简化/自审 -> 必要修正和受影响回归 -> 复审 -> 英语提交。不能先把大批预期测试全部写好再一次性实现，也不能把未完成测试、规格审查或提交留给主会话。独立审查发现必须修正项时，返回原 owner 在原 worktree 完整处理后再回报。

各 owner 记录自己的绝对 worktree、branch、共同起点、HEAD/状态、GOMOD、Go/CGO/compiler 和实际 argv/cwd/env/起止时间/退出码。可复用脚本、RED/GREEN 源码、日志、产物放自己的 `.local-verification/functional-fixes-20261024/<owner>/`，用该专用目录内的局部 ignore 文件和 `git check-ignore` 验证忽略；不修改共享 `.gitignore` 或 Git/Claude/权限配置。一次性数据用 OS 临时机制，不能修改或删除 finder 的借用证据。

`$W` 在以下命令中始终表示该 owner 已核对的绝对 worktree；不切换到原仓库。所有来源核对使用本次实际源码和产物，不能复用旧版本报告冒充新结果。生成 `.integration/`、`dist/` 只由现有 Make/runner/packager 产生。发现需要修改文件责任表外的 tracked 内容，先说明与规格的必要关系并修订分工，再实施。

## U1. G，typed Schema 的纵向修复

覆盖规格 G-A1 至 G-A6。生产 caller 从 `interceptRequest`/`transformRequest` 的 Gemini dispatch 到 `collectGemini`；共享 helper 还被 OpenAI、Claude、Interactions 使用。修改前检索 `appendJSONSchemaDescriptions` 的全部 caller，逐一确认默认路径不变；同样检查两个 oracle 的全部调用方。

### 第一条 RED 与最小 GREEN

1. 在 `selectors_gemini_test.go` 新增单一聚焦 case，建议命名 `TestGeminiTypedSchemaAnyOf`。输入只用 `generation_config.response_schema.properties.answer.any_of[0].description="SECRET branch"`，合法 typed OBJECT/STRING 与 JSON MIME，developer scope、block SECRET。要求 canonical 400/censorship_blocked、term SECRET、role developer；相同输入改为 anyOf 的对照正常。运行真实 focused 命令，保存旧 production 漏选的失败断言和退出码。
2. 保留八个输入入口全部基线 description 选择，只增加从这些入口仅经 properties 属性值、对象 items、anyOf/any_of 对象元素可达的 string description；新增路径不能经 JSON Schema 专属分支传播。共享 appendJSONSchemaDescriptions 的默认集合、行为及其他 provider caller 保持不变。owner 选择最少且完整的实现，不预定单一内部组织，不复制完整 walker，不全局接受 any_of。重跑完全相同 focused 命令取得 GREEN。
3. 之后按下一节的独立行为逐条加入测试；每条先观察其失败或确认已有保护测试，再作对应最小修改并重跑。若首条修复自然覆盖其余正式路径，记录这些新增回归是覆盖证明，不虚构额外 RED。

```bash
go -C "$W" test -count=1 -run '^TestGeminiTypedSchemaAnyOf$' .
```

### 入口、递归、oracle 与 RPC 回归

- 完成规格 typed 八种入口：两种 declaration 容器各自的 parameters/response、两种 generation root 各自的 responseSchema/response_schema；每例单独使用一种合法表示。覆盖 anyOf/any_of、根/属性/items/多层混合、两种 token 顺序、三动作和 literal/fold。保留 developer、format gate 和非字符串 description/错误 array 形状检查。
- 在同一测试文件为全部 18 个 Gemini JSON Schema roots 和 OpenAI/Claude/Interactions 相邻入口构造未知 any_of 负例，同时保留合法 anyOf/description 正例。八个 typed roots 各自为规格列出的 15 个既有 keyword 添加 description 不回归正例；其 JSON Schema 专属分支中的 any_of 负例须保持不选，包括先经专属分支再经 properties/items/anyOf 的情况，以及合法 any_of 之后进入专属分支的情况。default/example/enum/metadata 内相同结构仍排除；properties 下名为 any_of/description 的属性值依既有规则处理，不改属性名。无需修改其他 provider 生产 caller。
- 在 `fuzz_test.go` 为 `oracleGeminiSpans` 和 `oracleRawGeminiSpans` 各自表达的基线集合、typed 新增路径及上述 JSON Schema 负例加直接 span 断言，建议命名 `TestProtocolOracleGeminiTypedSchemaAnyOf`。两套 oracle 均保留自己的 parser，不导入生产 helper。若在第一条生产 GREEN 后补 oracle，用保存的旧 production 或单项局部变异证明新增路径的 oracle/production 对照会失败，标明这是重放/敏感性验证，不冒充修复前 RED。
- 给 `FuzzProtocolTransform` 增加正常 typed any_of、嵌套 mixed keyword、既有 description 递归正例、JSON Schema 入口/专属分支的未知 any_of 和机器对象 decoy 小型 seed；对伪造的机器字段改写结果，两套 oracle 都应拒绝。不要把 production 输出当作预期值。
- 在 `functional_corrections_test.go` 复用 functional request/call、raw token replacement 与既有 generation ordering case，加入 typed 样例两阶段、model/filter/selected-auth、相反 RequestedModel/body model decoy、role/format gates。分别核对 RPC 的精确空 no-op 与 native host 聚合的原 body/headers，保持 metadata/headers/原输入不变。
- 按规格验证机器字段 raw bytes、规则词优先、原 token 顺序、不跨节点、block 优先、strip 后 obfs、完全取消无 replacement、ClearHeaders、受保护文本和 final-text 限制的既有回归。不要对未知 any_of 增加通用错误或 provider schema 校验。

聚焦命令使用实际新增名称；下列入口与现有命名可直接复用，owner 必须保存实际执行的表达式及对应场景：

```bash
go -C "$W" test -count=1 -run '^(TestGeminiTypedSchema|TestGeminiGenerationSchema|TestGeminiToolDeclaration)' .
go -C "$W" test -count=1 -run '^TestProtocolOracleGeminiTypedSchemaAnyOf$' .
go -C "$W" test -count=1 -run '^(TestFunctionalCorrections|TestGeminiGenerationSchemaOrderingBothPhases)' .
```

### 当前 DLL、native 与真实 HTTP

新增独占 `integration/gemini_typed_schema_test.go`，带现有 integration build tag。复用 `withDynamicABIHost`、`startFunctionalDirectCPA`、`newFunctionalDirectUpstream`、post/capture/error helpers；现有 harness、`integration/http_test.go`、`gemini_generation_config_test.go` 保持只读。参考现有 generation fixture 按 phase/action 共用实例的组织，不复制完整 helper，也不重构其他矩阵。

1. 从当前 worktree 构建 DLL，记录 source/build/staged/实际加载 hash。native 按既有 fresh child 组织保留 90 秒截止时间；HTTP 使用实例独立 auth-dir、本地 provider 和 canary。至少一个新 native/HTTP 原场景在保存的旧 production 上取得与 G1 相同的可观察失败；这是明确的基线重放，若正式新增测试早于生产修改执行则记录其真实 RED。
2. native 与 HTTP 覆盖 typed 全部八入口 × anyOf/any_of × 三动作 × 两个 phase；developer-disabled、format-disabled、filter bypass、machine-only、no-match、取消和 JSON Schema any_of 排除作为明确对照。没有需求扩张时不把所有负例相乘成无关大矩阵。
3. 每次待验收 HTTP 请求前另记 arrival/request 基线。block 为精确 400/censorship_blocked、term/role 正确、两个增量均为零；strip/obfs 为恰好一次转发、description 符合预期，全部机器 token raw bytes 保持。clean canary 必须先返回 200，不能用路由/auth 不可用制造阻断通过。
4. wire oracle 核对固定 converter 把两种 declaration 的 parameters 改到 function_declarations[*].parametersJsonSchema、response 改到 function_declarations[*].response，以及 generationConfig.responseSchema -> responseJsonSchema；其余三个 typed generation 路径保持，内部 any_of 不归一化。converter 将 parameters 和 generationConfig.responseSchema 的原对象转入 JSON Schema 字段且无 typed 校验，基线 description 选择保持；新增资格按插件输入入口及完整递归路径判断。转发不能证明 Google typed 语义或商业接受性。测试不修改 CPA，不调用商业 API。
5. 运行完整 `make -C "$W" integration` 自动刷新生成夹具。G 的 worktree 尚无 R 修复时可以使用 `GOFLAGS=-timeout=30m make -C "$W" integration`，明确记录临时开发期覆盖；不复制 R 未完成提交，不为此手改 runner。

生成源码只能由 runner 刷新。需要重复 focused native/HTTP 时，先比对源夹具和生成夹具 hash，再用 runner 已生成的本机 CPA binary/plugin 绝对路径运行，例如 Windows：

```bash
CPA_INTEGRATION_BIN="$W/.integration/bin.exe" CENSORSHIP_PLUGIN_DIR="$W/.integration/run/plugins" \
  go -C "$W/.integration/cpa" test -tags=integration -count=1 -timeout=30m -v \
  -run '^TestGeminiTypedSchema(Native|HTTP)$' ./integration/censorshipplugin
```

测试名为拟新增名称，以实际代码为准；其他宿主使用生成的实际 binary 名称。此 focused 入口不替代完整 Make 集成。

完成条件：G-A1 至 G-A6 各有测试/证据；根 test/race/vet、完整 integration、make build 和 `FuzzProtocolTransform -fuzztime=3000x -parallel=2` 全部通过。完成简化和规格审查，确认其他 provider/default helper 范围不变，清除本次变异及废弃实现，`git diff --check` 通过后只提交 G 的六个文件。英语提交后主动返回 commit、worktree/branch、stat、RED/GREEN/重放区别、实际命令/退出码及 DLL/native/HTTP 证据。

## U2. R，Make driver 与 full 总预算

覆盖规格 R-A1 至 R-A7。R1/R2 共用 Make/runner/test 文件，留在一个 owner/worktree 顺序完成；与 G 并行。先全文核对 `goCommand`、`runCommand`、buildCPA/buildPlugin/buildModelMapper/runIntegrationTests 的 caller、Make integration 两行和现有 mode tests。

### R1 的单条纵向 TDD

1. 在 `.github/scripts/integration-runner_test.go` 增加 `TestBuildCPAUsesGoCommandTextAndCheckoutDirectory`。t.TempDir 创建最小 CPA module 和可运行的 cmd/server，插件根放不可编译源码；GO=go -C . 调用真实 buildCPA，再运行生成程序。旧 runner 在启动 Go 前失败，记录真实 RED；此前独立 verifier 的临时 prototype 只作证据，不代替 owner 自己的 TDD。
2. 仅为四个 Go caller 增加专用命令执行边界，通用 Git runCommand 保持直接 argv 执行。裸 executable 保留直接执行；复杂 GO 文本交给现有 POSIX shell，完整保留原文本，并逐项正确引用 runner 生成参数。保持 cmd.Dir/extraEnv/输出/退出码，不写 parser/strings.Fields，不移除 -C，不用裸 driver 替代 wrapper。运行同一真实 buildCPA 测试 GREEN。
3. 再增加真实 Go/Make 命令边界测试，覆盖默认、实际绝对路径、带空格 quotes、绝对路径加 -C、env wrapper。验证三个 cwd 下原生 -C、wrapper 设置/移除 runner 额外 marker 的每次调用行为，以及空参数、单/双引号、反斜线、空格 argv。已有 MSYS positional 参数反例要求实际比对收到的 argv，不凭 shell 代码外观断言正确。
4. 单独用 Windows 无 shell 的 PATH 验证裸实际 executable 仍能直接运行，不能给原有 standalone 入口新增依赖；复杂 GO 只使用已有 shell，缺失时明确失败，不新增自动寻找或安装层。保留 Make 显式空 GO 失败、standalone 空 GO 默认，以及缺失 executable、错误 Go 子命令、不存在 -C 目录的非零退出传播。
5. 原始 make integration 'GO=go -C .' 的真实基线失败和修复后完整成功均有日志；默认/环境 GO/Make 命令行 GO 的导出行为保持。Makefile recipe 当前无须修改。完整成功安排在 R2 完成后，不以 dry-run、最小 module 或默认入口成功代替。

### R2 的单条纵向 TDD

1. 新增调用级聚焦测试，建议命名 `TestFullIntegrationTimeoutUsesEffectiveGoFlags`。复用最小真实 Go 测试 child，在短时间内读取实际 `test.timeout`/argv/env；无用户覆盖时应为 30m。旧 runner 默认仍为 10m，记录真实 RED，不等待十分钟。
2. 最小修改只作用于 full 最终测试进程，通过同一 Go 调用入口和目标 cwd 执行 go env -json GOFLAGS，标准 encoding/json 取得有效原值；在该次子进程环境前置默认 -timeout=30m 并保留原值 quotes/顺序，由 Go 自己解析。不在 argv 无条件覆盖用户 timeout，不改持久配置。wrapper 主动重写或清除 GOFLAGS 的原生优先权保留，不新增反制机制。原 invalid GOFLAGS/query/child 错误正常传播，同一聚焦测试 GREEN。
3. 逐个增加 OS GOFLAGS、wrapper 设置 GOFLAGS 和持久 GOENV 覆盖：短/长 timeout、0、-test.timeout、重复值最后生效、含 quotes/空格的其他有效 flags。联合测试要求 wrapper 显式 timeout 优先，wrapper 重写/清除 GOFLAGS 行为未被改写；不宣称默认能越过主动环境重置。持久设置只对 t.TempDir 专属 GOENV 调用 go env -w，不改真实用户 go/env。验证 OS 值优先及 OS 空值读取持久值，不自行解析/归一 flags。
4. 对短 child 使用短用户 timeout，要求实际终止并非零；不靠比较字符串代替生效证明。短 child 仅验证 Go 调用预算、quote 和退出传播，完整套件的时长证据另由正式 Make 提供。
5. 复跑 `TestIntegrationTestArgsSelectsRunnerMode`、`TestParseRunnerOptionsSelectsExactlyOneMode`，增加 smoke 优先于 BENCH=1 的环境断言。full tags/count/verbose/package 保持；ABI smoke 的单测试、benchmark 的 run/bench/benchmem 与原有效 GOFLAGS/default budget 不受影响。
6. 只读核对 5 秒 I/O、20 秒 readiness、45/90 秒 child、实例 auth-dir 与 process-tree cleanup 未变；不通过延长局部 timeout、减少矩阵、skip 或重试取得通过。

### R 的命令和完整回归

```bash
go -C "$W" test -count=1 -run '^TestBuildCPAUsesGoCommandTextAndCheckoutDirectory$' .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
go -C "$W" test -count=1 -run '^TestFullIntegrationTimeoutUsesEffectiveGoFlags$' .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
go -C "$W" test -count=1 .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
go -C "$W" test -race -count=1 .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
go -C "$W" vet .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
make -C "$W" test
make -C "$W" race
make -C "$W" vet
make -C "$W" build
make -C "$W" integration
make -C "$W" integration 'GO=go -C .'
```

两个完整 Make 运行均须记录有效 GOFLAGS/GOENV 和实际 timeout，不能由外部遗留 `-timeout=30m` 意外代替默认预算验证。只在测试/子进程临时环境中隔离这些输入，不改本机设置。运行针对本机 helper 的独立 packager test/race/vet，确认原 GO 命令覆盖 guard 仍通过；命令见 U3 表。R 改动不影响 production/oracle，R owner 不重复 G 的 fuzz。

完成条件：R-A1 至 R-A7 及 wrapper/GOFLAGS 交互均有真实调用/参数/环境/失败传播证据；默认及 GO=-C 的完整 Make 成功；root、runner、packager 回归通过。规格审查、简化、修正和复审后 git diff --check 通过，英语提交仅含 R 两个 runner 文件；Makefile 保持原语义且不作无依据改动。主动返回完整证据，不留下完整入口测试给主会话补做。

## U3. 合并后的独立最终集成与正式包

依赖：U0 完成，G/R 均已完整提交、经独立审查并由主会话确认无文件冲突后合回本轮独立集成分支。U3 从该分支的合并 SHA 创建独立 worktree；最终共享 README/RELEASE_NOTES/main_test 文档版本区域不与 G/R 并改。主工作区及初始 main 保持不变，直到 U3 全部本地验收、修正、复审和英语提交完成。

### 版本与英语文档的 TDD

先把 `TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions` 的当前标题/兼容句改为 v0.3.14、历史分界为 v0.3.13，并保留 v0.3.13 及所有更早历史断言。README/RELEASE_NOTES 尚未更新时运行 focused guard，记录真实 RED，再更新英语文档并取得同命令 GREEN。

README 直接描述 typed any_of 的新增路径边界、全部基线 description 选择保留、JSON Schema 未知 any_of 排除、Make GO driver 和 full 默认预算/用户覆盖方式；只更新必要当前版本示例与实际验证范围。将 accepted limits 中 WebSocket completion 的 20 秒改为实际 5 秒，依据 `integration/websocket_test.go` 的 websocketCompletionReadTimeout 引用 `integration/harness_test.go` 的 integrationIOTimeout；只纠正文案，不修改测试、I/O 或其他局部时限。RELEASE_NOTES 顶部新增 v0.3.14 三项修复及固定兼容句，保留历史和 glibc 2.34+。其余 accepted limits 及 phase/LKG、C.GoBytes、roles、机器字段、取消/immutable/size/final-text 规则保持，不以修改这些边界通过断言。`main.go` 默认仍为 `0.0.0-dev`，正式版本只由现有 VERSION 注入。

```bash
go -C "$W" test -count=1 -run '^TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions$' .
```

### 合并后的完整命令

下表全部为 pending，只有实际执行后才能在本计划末尾登记结果；owner 记录实际参数和退出码，不沿用 v0.3.13 的 fuzz 或包通过记录。

| 检查 | 命令及完成条件 |
|---|---|
| 根目录完整回归 | `make -C "$W" test`、`make -C "$W" race`、`make -C "$W" vet` 全部成功 |
| 独立 runner | `go -C "$W" test -count=1 .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go`；同两文件另跑 `go test -race -count=1` 与 `go vet` |
| Windows 独立 packager | `go -C "$W" test -count=1 .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go .github/scripts/package-release_windows_test.go`；同四文件另跑 `go test -race -count=1` 与 `go vet` |
| Unix 独立 packager | 在实际 Unix owner/CI 中对 `package-release.go`、`package-release_other.go`、`package-release_test.go` 同样运行显式 test/race/vet；不能在 Windows source list 中同时列两种 helper |
| 六个 bounded fuzz | 逐个目标运行 `go -C "$W" test -run '^$' -fuzz '^<target>$' -fuzztime=3000x -parallel=2 .`，全部成功，不作性能声明 |
| 默认 full 集成 | `make -C "$W" integration`，无用户 timeout 覆盖时使用新默认；包含 G/R、HTTP/SSE/WS/native/watcher/lifecycle/mapper/auth 隔离及 active-after 全部回归 |
| GO 参数的真实 full 入口 | `make -C "$W" integration 'GO=go -C .'`，同一完整矩阵成功；不以直接 CPA go test 或 Make dry-run 替代 |
| 正式本机构建 | `make -C "$W" build VERSION=v0.3.14`，使用现有本机 CGO compiler |
| Selected-platform package | `make -C "$W" package GOOS=windows GOARCH=amd64 VERSION=v0.3.14`，保存该次 ZIP/sidecar/DLL hash |
| Aggregate package | 未设置目标 GOOS/GOARCH 的子进程中 `make -C "$W" package VERSION=v0.3.14`，保存并核对本次存在平台的 aggregate，不把单机平台集合写作七平台 |

六个 fuzz 目标为 `FuzzRuleEngineAgainstOracle`、`FuzzFoldKMPAgainstOracle`、`FuzzDuplicateWalkerAgainstOracle`、`FuzzProtocolTransform`、`FuzzRebuildBodyAgainstMarshalOracle`、`FuzzByteMatcherAgainstOrderedContains`。此次 production traversal 与 oracle 有修改，必须重新运行全部六个。根 `./...` 不覆盖隐藏 `.github/scripts`，runner/packager 的显式检查不可省略。

U3 也复核两组 scope 边界及实际命令来源。发现合并引入问题时，由对应 owner 完整修正、回归、复审和提交后重新接入；主会话不接收半成品。必要修正后重跑受影响检查，最终证据关联实际最终输入，不能只沿用修正前日志。

### 正式 ZIP 与提取 DLL

1. 用标准 ZIP parser 读取 selected/aggregate 两份正式包的全部成员，核对 CRC、精确 `censorship.dll`/`LICENSE` 成员、LICENSE 字节、库 0755/许可证 0666 权限、1980-01-01 时间、两入口 ZIP/DLL 一致性、lowercase 64hex/两空格/basename/LF sidecar 和 aggregate 精确条目。只验收本次存在的平台，不手改产物补齐。
2. 每次 ZIP 下载/提取使用新的空目录，脚本与数据分目录；Python 处理外来 archive/JSON 时使用 `-I`。通过标准 parser/hash 工具，不增加二进制/ZIP/JSON parser。先核对 repo LICENSE 与 archive hash，再把包内、提取、dist、实际加载 DLL 的 SHA-256 关联到同一文件。
3. 复用经过核查的真实 native 验收脚本，或在专用忽略目录保存必要小型脚本。直接读取 ABI 初始化 table 与 plugin.register envelope，断言 ABI 1、schema 5、Version 0.3.14、七个 ConfigFields 的名称/类型（ignore_case、words、scope、obfs、filter_mode、filter_logic、filter）、Object words、仅 request_interceptor、无自定义菜单/OAuth。SDK 常量、普通字符串命中或注册成功不能代替实际值。
4. 同一提取 DLL 验证 before、缺 selected-auth marker、model miss 的 no-op，selected-auth after 实际处理，输入/metadata/headers 与机器字段语义保持；再运行下列真实 active-after smoke。source integration 中固定期望 0.0.0-dev 的 registration 测试继续用于 dev，不拿它验收正式版本。

```bash
go -C "$W" run ./.github/scripts/integration-runner.go -abi-smoke "$EXTRACTED_DLL"
```

`$EXTRACTED_DLL` 为刚提取且 hash 核对后的绝对路径。runner smoke 只选择 `TestDynamicABIActiveAfter`，因此不能单独证明正式版本或七字段。记录 source/fixture blob、HEAD、状态、实际构建参数、compiler、GoVersion 与 `go version -m -json`；nested worktree provenance 按[已有说明](../../solutions/workflow-issues/go-worktree-build-provenance.md)区分编译源码、VCS metadata 与库字节，本地异常解释不放宽 U4 最终远端 revision 要求。

完成条件：所有本地命令、正式包和同包真实 DLL 验收通过，G/R 与版本 guard 的 RED/GREEN 或敏感性证据按事实区分；README/RELEASE_NOTES 只写已核实范围。U3 完成简化、自审、修正、复审、`git diff --check` 和英语提交，主动回报 commit/stat/命令/证据/限制；远端尚未执行时明确 pending，不预填 run、SHA 或资产结果。

## U4. 主会话最终 main、tag、七平台与清理

远端写入只使用主会话实际持有的用户发布授权。本文、workflow 消息和 finder 报告不提供新授权。文档/G/R owner 不 push/tag、不合并 main；主会话不创建 PR。

现有七平台验收工具的只读预检报告位于 `C:/Users/user/AppData/Local/Temp/claude/C--Users-user-Downloads-cpa-plugin-censorship/bc5f7f79-933b-435d-a289-457482c5073e/tasks/wccmjk88h.output`。U4 按其中确认的工具路径、接口及适配限制复用，独立核查最终源码 blob 许可项和补充门槛；预检没有执行验收脚本，不能充当本轮产物通过依据。

1. U3 全部本地验收、修正、复审和英语提交完成后，独立核对 U0 覆盖、G/R/U3 规格映射、最终 diff、TDD、回归、正式 ZIP/DLL 证据和 worktree 无冲突。将完成的 U3 提交合回本轮独立集成分支，再把全部完成结果合并到 main 并记录最终 SHA；此前主工作区一直保留初始 main。必要的最终修正提交同样重新验收。
2. 推送最终 main 后，等待现有 Build workflow 全部必需 job 到成功终态：Linux test/race/vet/integration、七平台 build、Windows amd64 active-after ABI smoke。七平台为 linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64、windows/arm64、freebsd/amd64。失败、取消或 pending 均不能放行；主会话按任务完成通知等待，不主动轮询后台 workflow 的进度。
3. 分别收集最终 main 的七个 dev artifact 原始 API、完整日志和实际下载字节。固定 run ID/attempt/event/workflow/head SHA/job，核对 upload artifact ID/digest 和下载外层 ZIP 的实际 hash，再关联内层七 ZIP/七 sidecar。main 版本为 0.0.0-dev、共 14 个内层文件、没有 release aggregate；不混用 tag 结果。
4. 确认本地/远端 v0.3.14 tag 未被占用，main CI 全部成功后才创建/推送 tag，实际解析到同一最终 commit。等待 tag 的 test、七平台 build、Windows smoke 和 release job 全部成功终态，release 非 draft。记录 tag run/attempt/event/SHA 与 main 独立，不用 target_commitish 字符串代替 tag commit。
5. 每阶段七库分别使用 `go version -m -json` 核对 GOOS/GOARCH、c-shared、CGO 1、compiler gc、CPA v7.2.152 无 Replace、vcs.revision 为各自最终 SHA、vcs.modified=false、GoVersion 与相应 job 一致。用已核对的 Make/workflow/packager blob、实际 VERSION/MAKE_VERSION 正规化和库 -> 打包 -> 上传路径证明注入来源；Windows arm64 另核对固定 action revision、GOFLAGS、x-flags、实际合并 Flags 和输出改名。
6. `-trimpath` 导致 buildinfo 不保留 ldflags 时保留 actual_ldflags=null / metadata unobservable；只有完整 CI 来源链及源码 `main.pluginVersion` 的包级 string 初始化、可达注册使用、无后续覆盖/地址暴露资格核对完成，才能另记 version_proof_method 为来源证明。Main.Version、tag、workflow success 或普通字符串命中不单独证明版本。G/R 后重新检查实际 blob，不能接受工具对新源码未经核查的自证。
7. 下载正式 release 精确 15 项：七个 `censorship_0.3.14_<goos>_<goarch>.zip`、七个对应 `.zip.sha256`、一个 `checksums.txt`。按 asset ID/digest/实际 bytes 与同次 tag artifact 逐项关联；可靠 parser 检查每 ZIP CRC、精确共享库/LICENSE 成员、LICENSE raw/必要 CRLF->LF normalized hash、permissions、固定时间、lowercase sidecar 和 aggregate 精确七条。不得缺项、多项、混入旧版本或混用不同 attempt。Linux 库保持 glibc 2.34+ 兼容要求，用现有标准工具检查，不能新增自制 binary parser。
8. 从本次正式 Windows amd64 ZIP 重新提取 DLL，实际加载 hash 必须与包内相同，重新读取 Version 0.3.14、ABI 1、schema 5、七字段/Object words/request-only，验证 active-after、no-op、输入/机器字段保持；再用同包 DLL 运行 runner ABI smoke。其他平台 native 若未实际运行，逐平台写 not_run；商业 API 同样如实记录未测试。七平台 buildinfo/CI 不充当七平台 native 运行证据。
9. 复用既有收集/验收工具前核对当前路径、源码、参数、已审查 blob、真实正例和失败输入处理。来源缺失保留 incomplete，不输出通过；版本/revision/hash 矛盾明确失败。所有脚本在 owner 专用忽略目录，下载数据各自新空目录，不保存 token/auth header/签名下载 URL，不修改认证或权限。
10. 所有结果实际取得后，最终记录 main/tag SHA、run/attempt/job 状态、15 资产/七库来源、Windows 同包 DLL 注册与 active-after、六 fuzz、未运行范围。只有实际成功才记录成功，不复制历史报告计数或 hash。
11. 确认本轮提交全部合并、无未提交内容和未合并提交后，退出本轮任务 worktree、返回原目录，再用 Git worktree/branch 管理命令清理本轮已无用途的目录及本地分支。文档 owner 的 worktree 在合并前保留；不清理用户原有、其他任务或 finder 临时证据。清理失败继续其他工作并列出未清理项，不强制删除。

## 最终完成条件与尚未完成依赖

- U0 已完成：九分区、53 份历史 docs、107 个 tracked 文件覆盖及全部定向独立复核已有依据，本次独立文档复审通过，当前 spec/plan 可以启动 G/R。
- G/R 从独立集成分支的共同 baseline SHA 创建各自 worktree，该 SHA 为本次准备就绪提交完成后的 HEAD；完成全部规格、TDD、相关回归、简化、修正、复审及英语提交后合回集成分支。当前未启动实现，修复后验收仍为 pending。
- U3 从 G/R 合并 SHA 独立完成共享文档/版本 guard、WebSocket 时限文案纠正及全量本地验收，目标 v0.3.14、源码默认 dev 不变；六 fuzz、两 package、正式 ZIP 同 DLL 注册和 active-after 均须有本次实际结果。全部复审并提交前保持主工作区及初始 main 不变；当前 pending。
- U4 在 U3 完成后更新 main，最终 main CI 成功后才 tag；tag CI/release、七平台 dev/正式资产来源、精确 15 assets/checksums/buildinfo、Windows 正式 DLL 动态验收分别有证据。其他 native/商业 API 未运行范围明示；当前 pending。
- 本次准备就绪提交只更新本文与 spec 的 U0/当前状态，已完成独立复审；自查链接、事实/未来状态、文件责任与验收覆盖，运行 `git diff --check` 后英语提交，不 push、不清理本 worktree。本次通过仅解除扫描/规格启动门槛，不预填 G/R/U3/U4 的实现、测试、构建、CI 或发布结果。
