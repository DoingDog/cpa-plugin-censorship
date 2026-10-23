# v0.3.13 集成测试 auth-dir 隔离实施计划

日期：2026-10-24。依据：[设计规格](../specs/2026-10-24-v0313-integration-auth-isolation-design.md)。源码基线 `185850fea6c47b431ac59a2a4621e49158d1cfbe`（v0.3.12），目标 v0.3.13。固定 CPA v7.2.152 / `c76dfd4e0edabab9000628b1560ab8ab379eadb8`、native ABI 1、schema 5。

## 目标与完成边界

集成用例的模型表和请求路由只来自该用例声明的配置与本地 fixture，开发环境中的账号不再参与验收。实现同时覆盖两个启动 helper，行为与限制以规格 S1 至 S5 为准。

当前只提交本计划及对应规格。文档经独立审查并由主会话合并后，记录包含两份最终文档的 main SHA，再从该 SHA 启动一个独立 worktree 的 fix worker。该 worker 连续完成 U1、U2、U3 的测试、最小修复、全部本地验收、规格自审、修正、复审及英语提交，不向主会话移交未完成的步骤。主会话负责 U4 的独立审查、合并和发布；fix worker 不修改 main，不 push，不 tag。

没有需要新增的依赖、公共接口、启动参数或测试框架。现有 `gopkg.in/yaml.v3 v3.0.1`、HTTP fixture、capture、日志和进程清理足够使用。

## 已核查范围与可改文件

| 文件或区域 | 用途与修改权限 |
|---|---|
| 本次新 spec、plan | 本次文档提交仅含这两个新文件 |
| `integration/harness_test.go` 的 `startCPAWithOptions` | fix worker 仅增加本实例绝对 auth-dir 的生成配置及必要局部路径处理 |
| `integration/http_test.go` 的 `startFunctionalDirectCPA` | 同上，独立完成第二处修复 |
| 新增 `integration/auth_isolation_test.go` | 两个 helper 的聚焦配置/路由回归，带现有 `integration` build tag |
| `main_test.go` 的 `TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions` | 仅更新当前发行 guard、历史分界及 v0.3.12 历史兼容断言 |
| `README.md`、`RELEASE_NOTES.md` | 仅更新本次修复、当前 v0.3.13 版本和经核实的验收说明；沿用英语 |

下列文件已核对，保持只读：`Makefile`、`.github/workflows/build.yml`、`.github/scripts/integration-runner.go`、packager 与其 tests、`go.mod`、`go.sum`，以及全部插件生产源码和 C/CGO 文件。现有 `integration/websocket_test.go`、`lifecycle_recovery_test.go`、`claude_descriptions_test.go`、`gemini_generation_config_test.go` 及 `http_test.go` 的其他区域也保持只读。

调用范围已逐项检查：

- `startCPA` 委托 `startCPAWithOptions`；HTTP block/strip/filter、mapper 正负对照、HTTP/SSE trace、Responses WS、watcher、生命周期恢复和 Claude description 测试使用这条路径。
- `startFunctionalDirectCPA` 被 direct Claude system/MCP/document、Gemini schema/description、Chat->Codex、raw surrogate、tool output、encoded carrier、非空/长度边界及 generation-config HTTP 矩阵调用。
- native-only helper 不启动 CPA 账号服务，无需增加 auth-dir；其注册、两阶段及 input/metadata/header 回归继续运行。
- runner 的 `copyIntegrationFiles` 自动复制所有 `integration/*.go`，也复制现有 ABI fixture；`runIntegrationTests` 只增加 binary/plugin-dir 环境，没有现成 home 隔离。无需修改 runner。

两个 helper 的签名、`cpaStartOptions`、调用方、默认模型、readiness、错误预期和清理流程均不改变。若实施发现必须修改表外 tracked 文件，先给出与 S1 至 S5 的具体关联并更新计划，经审查后再继续，不能顺手重构。

## 工作目录、证据与依赖

1. 文档 owner 核对 HEAD 等于源码基线，工作目录干净；本次已完成该核对。只创建指定 spec、plan，检查相对链接、事实边界和 `git diff --check`，英语提交后返回分支与 commit，不合并或推送。
2. fix worker 记录自己的绝对 worktree、branch、起始 SHA、GOMOD、Go/CGO/compiler 和初始状态。起始 SHA 必须包含已经审查合并的两份文档，不直接从旧源码基线启动实现。
3. 需要保留的脚本、RED/GREEN 源码、日志和产物证据放在 owner 自己拟创建的 `.local-verification/integration-auth-isolation-20261024/<owner>/`。该目录内放只作用于本目录的 ignore 文件，并用 `git check-ignore` 核实；不修改共享 Git 配置或扩大顶层忽略范围。
4. 每条证据保存实际 argv、绝对 cwd、源码/fixture hash、环境中的必要非敏感值、起止时间、退出码及失败断言。测试临时 home 使用 OS/Go 临时目录，结束自动清理；不保存真实 token 或用户账号。
5. 不手改 `.integration/`、`dist/`；只用现有 runner/Make/packager 生成。复用脚本前必须核对文件存在、内容与调用参数，使用本次版本和本次实际产物，不能引用旧版本报告作为本次成功。
6. 独立 workflow/subagent 在完成后主动回报，等待通知，不用 sleep、文件监视或反复读输出文件轮询。测试内部有截止时间的模型/路由观察属于回归逻辑，和代理任务进度检查分开。

本地执行依赖 Go 1.26、当前 Windows amd64 可用的 CGO compiler、Make/Git Bash、固定 CPA 与 model-mapper checkout。使用已有工具链和认证，不修复本机系统，不改权限或 Claude 配置。远端发布步骤另依赖主会话的发布授权和现有 GitHub 认证。

## U1. OpenAI compatibility helper 的纵向 TDD

依赖：最终文档已合并，fix worker 从记录的 SHA 启动。覆盖 S1、S3、S4、S5。修改文件为 `integration/auth_isolation_test.go` 和 `integration/harness_test.go` 的指定区域。

### 测试与 RED

1. 新增聚焦测试，建议命名 `TestCPAAuthDirIsolation`。用 `t.TempDir` 创建合成 home，通过 `t.Setenv` 同时设置 `HOME`、`USERPROFILE`，保留自动恢复，整个测试串行执行。账号输入直接由测试通过标准 JSON encoder 生成，不引用旧 `.audit-probes` 或复制完整 helper。
2. 复用审计确认的 Claude 账号形状：`type=claude`、合成 `access_token`、远期 `expired`、指向本地代理的 `proxy_url`，以及 `claude-opus-5-5` -> `claude-opus` 的合法 `model_aliases`。本地代理只记录 arrival 并对 CONNECT 返回 502，不向目标建立连接。fixture upstream 与代理使用独立计数。
3. 通过真实 `startCPAWithOptions` 启动实例，不配置 mapper，声明 `kimi-k3`、`other-model` 等已有本地 provider 模型。另经 `startCPA` 启动第二个实例，核对委托路径与跨实例独立性；可复用插件启用/关闭子测试承担这两个实例，避免重复启动。
4. 读取每个实例真实 `cpa.config`，用已有 yaml.v3 解码顶层 `auth-dir`。断言字符串存在、为绝对路径、精确等于各自 config 目录下的 `auths`，并断言两个路径不同；包含空格的合成临时目录应正确还原。配置断言与行为断言独立执行，缺字段时记录失败，仍收集路由证据，不能因首个 `Fatal` 丢失行为 RED。
5. 从带认证的 `/v1/models` 和真实请求检查 ambient alias 与本地模型。先验证配置模型的 clean canary 返回 200、正确 wire model 和 fixture arrival/request 增量；另记基线，再请求未映射 `claude-opus`，期望精确 HTTP 400/model_not_found、fixture/代理增量均为零。
6. 首次 RED 同时记录缺失 `auth-dir` 和合成账号加载后的路由变化。原探针三秒窗口仅作为调查起点：用有截止时间的轮询等到未隔离实例确实出现 alias 或 CONNECT arrival，确认账号有效后保留实际加载耗时。未观察到加载只能说明行为复现尚不充分，不能把立即请求的 400 计为行为 GREEN。
7. 保留插件启用与关闭对照，且关闭插件时生成配置与 model-not-found 隔离要求相同。空 home、无关 alias、停用账号对照仅按需要复用已确认输入，不增加无关 provider 矩阵。

### 最小 GREEN 与回归

仅在 `startCPAWithOptions` 内从本实例 `runDir` 派生绝对 `auths` 路径，并写入顶层 CPA YAML。路径计算失败按既有 `t.Fatal` 风格及时报告；不增加参数、抽象或配置选项。固定 CPA 已负责创建显式 auth-dir，除非实际测试证明必要，不额外实现目录生命周期。

重跑相同聚焦测试，要求 YAML 路径断言、两个实例独立性、本地模型 canary、ambient alias 不进入模型表、未知模型 400/model_not_found 和两个接收器的计数全部满足。行为观察覆盖已经证明认证会加载的启动阶段，不使用固定短 sleep 判定成功。确定性 YAML 断言始终保留，不以时间窗口取代配置证明。

此时只完成第一处 helper。保存真实 RED/GREEN、helper 最小 diff 和计数，进入 U2，不将第一处通过表述为全部隔离完成。

## U2. Direct-provider helper 的纵向 TDD

依赖 U1 已 GREEN；仍由同一个 fix worker 在同一 worktree 完成。覆盖 S2、S3、S4、S5。修改 `integration/auth_isolation_test.go` 与 `integration/http_test.go` 的指定 helper 区域。

1. 新增独立聚焦测试，建议命名 `TestFunctionalDirectAuthDirIsolation`。复用 U1 的合成 home、可靠 parser 和局部断言，不复制启动 helper；每个顶层测试保有自己的 home、代理及计数。
2. 使用不同于 U1 和 `censorship-integration-model` 的 ambient alias，例如 `ambient-direct-only`。可选择 direct `claude` provider：账号沿用已经确认的 Claude 格式，alias 对应 `claude-opus-5-5`，`proxy_url` 指向本例本地 CONNECT 502 代理，请求真实 `/v1/messages`。以实际 RED 证明该 alias 对该 provider/account 路径可执行，不能仅假定别名会注册。
3. 通过真实 `startFunctionalDirectCPA` 启动实例。对实际 YAML 作 S2 的绝对路径/目录归属断言，并检查 ambient alias 不在模型表中；发送 alias 请求要求 400/model_not_found、fixture 和代理计数不变。发送声明的 `censorship-integration-model` clean canary，要求 200、wire model 为 `claude-opus-5-5`，fixture arrival/request 各增加一，代理仍为零。
4. 在 U1 修复已经存在、第二个 helper 仍未改动时运行该测试，观察其独立 RED。保存缺失字段、账号加载/alias 注册、代理 arrival、错误响应和加载时序，证明第一处修复不会替第二处测试通过。若所选别名未实际注册，调整本测试的合成账号/alias/provider 输入并重新取得有效 RED，不修改生产模型表或降低断言。
5. 只在 `startFunctionalDirectCPA` 的原配置生成处增加本实例绝对 `runDir/auths`。运行同一测试取得 GREEN，再重跑 U1，确认两个 helper 均正确。
6. 验证测试敏感性：在 owner 临时单项变异中分别去掉一处 auth-dir，运行对应真实聚焦测试，应由实际 YAML 检查稳定失败，且行为探针在已确认加载后能观察到该 helper 的 ambient alias 路由。变异只服务测试验证，立即恢复，不提交副本、overlay 或遗留代码。

U1/U2 的精确测试命名、加载同步信号和局部组织属于开发时核实项。允许选择有证据、更简单且等价的实现；必须保留规格 S3 的四项证明及两个 helper 各自的真实 RED/GREEN，不引入大框架、完整 helper 副本、通用 auth sandbox 或产品配置。

## TDD 的实际运行入口

根模块无法直接运行引用 CPA `internal` 包的 integration 测试，必须经过现有 runner 复制到固定 CPA checkout。每次源测试或 helper 改动后，运行现有 `make integration` 刷新生成夹具并执行真实测试；输出中的聚焦测试是本次 RED/GREEN 依据。不得为加速而手工编辑生成目录，也不新增 runner 模式。

runner 已刷新夹具后，可以在 Windows Git Bash 中用以下形态重复聚焦检查，`W` 为已核对的 owner 绝对 worktree。先核对生成文件与该 worktree 源文件 hash 相同，避免测试旧副本。

```bash
CPA_INTEGRATION_BIN="$W/.integration/bin.exe" CENSORSHIP_PLUGIN_DIR="$W/.integration/run/plugins" \
  go -C "$W/.integration/cpa" test -tags=integration -count=3 -v \
  -run '^(TestCPAAuthDirIsolation|TestFunctionalDirectAuthDirIsolation)$' ./integration/censorshipplugin
```

U1/U2 尚未同时存在时，选择实际已经新增的测试名。其他宿主使用 runner 生成的实际 binary 路径，不照搬 `.exe`。每次 RED 必须保留修改前的真实退出码及失败断言，不能在 GREEN 后推断 RED。

## U3. 版本、完整本地验收与提交

依赖 U1/U2 均完成，仍由唯一 fix worker 连续执行。

### 当前发行 guard 的 RED/GREEN

先将 `TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions` 的当前预期改为 v0.3.13、历史分界改为 v0.3.12，并将 v0.3.12 compatibility 句加入历史列表。保留全部更早历史断言和 superseded 文档检查。README/RELEASE_NOTES 尚未更新时运行该 focused 测试，记录因当前标题仍为 v0.3.12 而失败的真实 RED。

随后在 `RELEASE_NOTES.md` 顶部新增英语 v0.3.13 段，说明两个测试启动 helper 显式隔离 auth-dir、合成账号回归和生产行为不变，保留原 v0.3.12 及更早段落。README 只更新当前兼容版本、当前 package 示例和本次集成隔离/验证范围。兼容句保持 CPA v7.2.152、固定 SHA、schema 5、ABI v1 与 glibc 2.34+。未执行的验收不写成已通过，六个 fuzz 说明为沿用基线；不复制旧版本的验收日期、hash 或运行结论。运行同一 guard 得到 GREEN。

```bash
go -C "$W" test -count=1 -run '^TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions$' .
```

`main.go` 中的 `pluginVersion=0.0.0-dev` 和 source/integration 注册预期保持不变；当前发行版本只由既有正式 `VERSION` 注入。

### 完整本地命令

| 检查 | 命令与通过条件 |
|---|---|
| 根回归 | `make -C "$W" test`、`make -C "$W" race`、`make -C "$W" vet` 全部成功 |
| 固定 host 集成 | `make -C "$W" integration` 成功，包含两个新回归、全部 HTTP/SSE/WS/native/watcher 及 active-after |
| 正式本机构建 | `make -C "$W" build VERSION=v0.3.13` 成功，使用已有 Windows amd64 CGO compiler |
| Windows packager tests | `go -C "$W" test -count=1 .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go .github/scripts/package-release_windows_test.go` |
| Windows packager race | `go -C "$W" test -race -count=1 .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go .github/scripts/package-release_windows_test.go` |
| Windows packager vet | `go -C "$W" vet .github/scripts/package-release.go .github/scripts/package-release_windows.go .github/scripts/package-release_test.go .github/scripts/package-release_windows_test.go` |
| Selected-platform package | `make -C "$W" package GOOS=windows GOARCH=amd64 VERSION=v0.3.13`；单独保存本次 ZIP、sidecar 与库 hash |
| Aggregate package | 在未设置目标 `GOOS`、`GOARCH` 的环境运行 `make -C "$W" package VERSION=v0.3.13`，证明实际走 aggregate 入口；只验收本次实际已构建的平台集合 |

隐藏 `.github/scripts` 的 packager tests 不由根 `./...` 覆盖，不能省略其显式命令。`make integration` 每次重新构建源码 dev DLL，正式包与正式 DLL 验收使用明确版本的 build/package 产物，不混用 `.integration/run/plugins` 中的 dev 库。

### 正式包与真实 DLL

1. 使用可靠 ZIP parser 打开 selected/aggregate 两次保存的正式包并读取所有成员，核对 CRC、精确成员 `censorship.dll` 与 `LICENSE`、LICENSE 与仓库内容、库权限/固定时间，比较两入口 ZIP 和 DLL 字节。sidecar 必须为 lowercase SHA-256、两空格和精确 archive basename；aggregate `checksums.txt` 只含本次存在平台的正确条目。
2. 从本次正式 ZIP 提取 DLL 到新的空目录，记录包内、提取、dist 和实际加载库的 SHA-256，一致后才运行 native 验收。脚本与解压数据分目录，处理下载/解压数据的 Python 使用 `-I`，不从不可信数据目录启动解释器。
3. 复用经核查的 native 验收脚本，或在 owner 忽略目录保存必要的小型验收脚本，实际读取 ABI table 与 `plugin.register` envelope：ABI 1、schema 5、Version `0.3.13`、七个 `ConfigFields` 的名称/类型、Object `words`、只有 `request_interceptor`、无自定义菜单/OAuth。SDK 常量与普通版本字符串不能代替实际返回值。
4. 用同一正式 DLL 验证 before、缺 selected-auth marker、model miss 为 no-op，selected-auth after 实际处理，输入、metadata、headers 与机器字段保持既有语义。再运行 `go -C "$W" run ./.github/scripts/integration-runner.go -abi-smoke "$EXTRACTED_DLL"`，`EXTRACTED_DLL` 必须是刚提取并核对 hash 的绝对路径。
5. `TestHTTPFunctionalNativeRegistrationAndPhases` 固定期望 `0.0.0-dev`，仅用于 source integration；runner `-abi-smoke` 只选 `TestDynamicABIActiveAfter`，不能单独证明正式版本或 ConfigFields，必须保留上一步的实际注册验收。
6. 记录源码/依赖 blob、工具链、构建命令和 `go version -m`。若 nested worktree 的 VCS metadata 不同，按[源码来源核查说明](../../solutions/workflow-issues/go-worktree-build-provenance.md)区分编译输入、metadata 与库字节；本地解释不放宽 U4 对最终远端 revision 的要求。

### 已完成的 bounded fuzz 基线

已完整核对审计 journal 的 `matcher-transform-json` 结果。以下运行属于源码基线 `185850fea6c47b431ac59a2a4621e49158d1cfbe`，每个目标均为 `go test . -run '^$' -fuzz '^<target>$' -fuzztime=15s -parallel=2`，各自通过。

| 目标 | 已记录 executions |
|---|---:|
| `FuzzRuleEngineAgainstOracle` | 169497 |
| `FuzzFoldKMPAgainstOracle` | 21271 |
| `FuzzProtocolTransform` | 216921 |
| `FuzzDuplicateWalkerAgainstOracle` | 350656 |
| `FuzzByteMatcherAgainstOrderedContains` | 195104 |
| `FuzzRebuildBodyAgainstMarshalOracle` | 456530 |

本次生产代码与这些 oracle 保持不变，因此不要求重复六个 bounded fuzz。根测试仍会运行适用的普通测试及 seed checks。最终报告明确区分沿用基线与本次执行；若实际修改超出 S5，先复审范围，再补跑真正受影响的检查。

### 自审、修正、复审与英语提交

逐项把 S1 至 S5 映射到实际测试、退出码和证据；检查两个 helper 都写入独立绝对路径、测试不会访问真实账号或商业服务、调用者预期未被削弱。检查 `git diff` 只有允许文件和必要行，gofmt 不改无关代码，移除本次变异、副本和废弃尝试。

完成必要修正及受影响回归，复审后执行 `git diff --check`。只提交 U1/U2 修复、新回归、v0.3.13 README/RELEASE_NOTES 与发行 guard，使用英语 commit，不 amend、不绕过 hook。返回 worktree、branch、commit、实际 stat、逐命令结果、两个 helper 和发行 guard 的 RED/GREEN、包/native 证据和剩余限制；达到本节完成条件后主动回报。

## U4. 主会话独立审查、合并与发布

依赖 fix worker 已完整提交且无未完成本地验收。此节不由本次文档 worker 或 fix worker 执行；远端写入仅在主会话已有发布授权范围内进行。

1. 从独立审查视角核对最终 diff、S1 至 S5、两处真实 RED/GREEN、完整本地命令、版本 guard 和正式 DLL 证据。提出必须修正项时由原 fix worker 在其 worktree 修正、回归并复审，不能由主会话接收半成品。确认修改位置无冲突后合并 main，记录最终 SHA。
2. 在最终 main SHA 上运行并等待现有 Build workflow。Linux test/race/vet/integration、全部七平台 build 与 Windows amd64 active-after ABI smoke 均成功才放行。七平台为 linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64、windows/arm64、freebsd/amd64。main 包按现有 workflow 使用 `0.0.0-dev`，不能当作正式 v0.3.13 包。
3. 核对本地/远端 `v0.3.13` 未占用，确认最终 main CI 全部成功后创建并推送该 tag，tag 解析到同一个最终 commit。等待该 tag 的 test、七平台 build、Windows ABI smoke 和 release job 全部成功，release 为非 draft。失败或缺证据时不宣称发布完成。
4. 分别收集最终 main 与 tag 的 run ID、attempt、event、head SHA、job、构建参数、artifact ID/digest 和真实下载字节，保持两个阶段独立。七库逐一使用 `go version -m -json` 验证目标 GOOS/GOARCH、`-buildmode=c-shared`、CGO 1、compiler、对应 job GoVersion、CPA v7.2.152 无 Replace、`vcs.revision` 等于各自最终 SHA、`vcs.modified=false`。
5. 通过已有构建程序与本次日志关联正式 `VERSION`/`MAKE_VERSION` 正规化值、`-X main.pluginVersion=0.3.13`、库到 packager/上传的路径。Windows arm64 按固定 go-cross action 的 `GOFLAGS`/`x-flags` 和实际合并参数核对。`-trimpath` 未在 buildinfo 中保留 ldflags 时如实记录缺失，用经核查的源码注入位置、同 run/attempt/job 构建日志与 artifact hash 建立来源证明；不能只搜索版本字符串或接受 `Main.Version`。
6. 下载本次 release 的精确 15 项资产：七个 `censorship_0.3.13_<goos>_<goarch>.zip`、七个同名 `.zip.sha256`、一个 `checksums.txt`。每次下载到新的空目录，核对 asset ID/digest/实际 hash，以及各 ZIP 的 CRC、精确库/LICENSE 成员、LICENSE 内容、权限和固定时间。验证七份 lowercase sidecar 和七条 aggregate，无缺项、多项或旧版本资产；关联同次 tag artifact 与 release bytes。
7. 从本次正式 Windows amd64 ZIP 重新提取 DLL并实际注册，要求版本 `0.3.13`、ABI 1、schema 5、ConfigFields/request-only 和 active-after 全部通过，hash 与包内一致。本地 owner 的旧报告、dev DLL 或其他版本的成功不能代替该步骤。其他平台 runtime 未执行时逐平台记录 `not_run`，不将 buildinfo 或 CI build 当作动态加载证明。
8. 可复用已核实的本机验收工具，但先核对当前路径、源码、参数能力与输出版本，使用本次真实数据重新执行；旧工具路径不存在或参数不适用时如实处理，禁止假报运行。最终报告包含实际 main/tag SHA、CI 状态、15 资产核对、七库来源、Windows 实际注册、沿用的 fuzz 基线及未执行的平台 runtime。
9. 确认本次提交全部合并且没有未提交内容或未合并提交后，退出本次任务 worktree，返回原目录，再用 Git worktree/branch 管理命令清理本次已无用途的 worktree 与分支。文档/fix worktree 在合并前保留供审查；用户旧 worktree 和其他任务目录保留。清理失败时继续其他工作并明确列出未清理项，不强制删除。

## 最终完成条件

- 文档提交只含本次两份新文档；实现从独立审查合并后的文档 SHA 开始。
- 唯一 fix worker 完成两个 helper 各自的有效 RED/GREEN、版本 guard RED/GREEN、完整本地回归与正式包/native 验收，并完成自审、修正、复审及英语提交。
- 生产源码、依赖、CPA revision、ABI 1、schema 5、同步 `C.GoBytes` 和机器字段处理未变；两处配置隔离均满足规格，既有调用语义完整保留。
- 主会话独立审查后合并，最终 main CI 全部成功后才 tag；tag CI/release、精确 15 资产、七库最终 revision/平台/CGO/依赖与正式 Windows DLL 注册分别有本次真实证据。
- 报告区分沿用基线、本次本地验证、远端验证和 `not_run`；不加入无关历史问题或未经证实的安全/生产错误结论，仅清理本次已完成任务内容。
