# v0.3.13 集成测试 auth-dir 隔离设计

日期：2026-10-24。源码基线：`185850fea6c47b431ac59a2a4621e49158d1cfbe`（v0.3.12）。目标：v0.3.13。执行顺序与验收命令见[实施计划](../plans/2026-10-24-v0313-integration-auth-isolation.md)。

## 问题与证据

这是插件仓库的集成测试启动配置缺陷。测试进程继承开发环境的 home，两个启动 helper 生成的 YAML 都没有 `auth-dir`。固定 CPA 将空值解析为 `~/.cli-proxy-api`，加载其中的账号及合法 `model_aliases`，因此未在用例中配置的模型也可能进入模型表和请求路由。

证据来自审计 workflow `wf_ee0c458f-d7a` 的 journal 第 20 条 `integration-harness` finding，以及第 23 条独立 verifier 最终结果。两条记录均已完整核对，源码基线一致。

- 探针把 `HOME`、`USERPROFILE` 指向合成临时 home，在其 `.cli-proxy-api` 中写入合法格式的 Claude 账号，将 `claude-opus-5-5` 映射为 `claude-opus`。账号仅使用合成 token，未读取真实用户认证文件。
- 不配置 model-mapper，请求 `claude-opus` 和 `clean input`，预期为 HTTP 400、`error.code=model_not_found`、fixture upstream arrival 为零。原始 helper 连续复现 HTTP 500、`internal_server_error`、fixture arrival 为零、本地代理 arrival 为一。代理在 CONNECT 阶段返回 502，没有向商业模型 API 发送请求。
- 关闭插件也能复现。空 home、无关 alias、停用账号对照维持 400/model_not_found；启用目标 alias 才改变路由。结论不涉及插件生产过滤逻辑错误，也不将此项归类为安全修复。
- 独立 verifier 在 helper 副本中仅加入指向各自 `runDir/auths` 的 `auth-dir` 后，原探针与全部对照连续三次通过，代理 arrival 为零。恢复原始 helper 后再次连续三次失败。
- HTTP readiness 早于认证加载时，立即发出的一次请求曾通过。原探针三秒观察窗口能够稳定发现干扰；这三秒是已执行探针的观察范围，不是认证加载的完成信号。

原探针位于旧 worktree 的 `.audit-probes/ambient_env_probe_test.go`，可以只读参考；本设计保留了必要输入、结果和边界，后续测试不得依赖该文件或旧 worktree 始终存在。`startFunctionalDirectCPA` 的同类遗漏已经通过源码确认，独立 verifier 没有执行它的专属干扰探针，修复任务必须补齐。

## 最小完整范围

| 位置 | 当前用途与本次责任 |
|---|---|
| `integration/harness_test.go` 的 `startCPAWithOptions` | 生成 OpenAI compatibility 配置；为本实例显式设置独立绝对 `auth-dir` |
| 同文件的 `startCPA` | 委托 `startCPAWithOptions`，签名、默认模型及调用方式保持不变 |
| `integration/http_test.go` 的 `startFunctionalDirectCPA` | 生成 Claude、Gemini、Codex direct-provider 配置；同样设置本实例独立绝对 `auth-dir` |
| 新增 `integration/auth_isolation_test.go` | 直接调用真实 helper，验证生成配置及合成账号干扰下的模型表、路由和本地 fixture |
| `README.md`、`RELEASE_NOTES.md`、`main_test.go` 当前发行 guard | 修复任务内完成 v0.3.13 的最小发行更新，保留历史内容与断言 |

已核查两个 helper 的全部调用位置及相关测试：`integration/http_test.go`、`websocket_test.go`、`lifecycle_recovery_test.go`、`claude_descriptions_test.go`、`gemini_generation_config_test.go`。这些用例均声明本地 provider/model，不要求读取用户认证目录。`startCPA` 的委托关系使其 HTTP/SSE/WS/watcher 调用方统一获得隔离；direct-provider 调用方由第二处配置修改覆盖，无需逐个改写调用点。

`.github/scripts/integration-runner.go` 已自动复制 `integration/*.go`，新增测试无需修改 runner。`Makefile`、`.github/workflows/build.yml`、runner、packager、现有其他 fixture 及生成的 `.integration/`、`dist/` 保持只读；生成产物仅由现有命令产生。

## 规格

### S1. OpenAI compatibility helper 隔离

`startCPAWithOptions` 必须在实际写出的 CPA YAML 顶层显式配置 `auth-dir`，值为本次 `runDir/auths` 的绝对路径。目录只属于当前实例，两个实例的路径不同，不使用用户 home、共享目录或相对路径。测试须使用已有 `gopkg.in/yaml.v3` 读取 `cpa.config`，断言字段存在、类型正确、为绝对路径，且精确等于该配置所在 `runDir` 下的 `auths`。

`startCPA` 继续委托该 helper；`cpaStartOptions` 不增加参数。目录创建沿用固定 CPA 对显式 auth-dir 的现有处理，无需新增抽象或产品配置选项。

### S2. Direct-provider helper 隔离

`startFunctionalDirectCPA` 独立满足 S1 的路径与实际 YAML 要求，使用它自己的 `runDir/auths`。必须为该 helper 编写自己的聚焦回归，并在只完成 S1 修复、尚未修改第二个 helper 时观察真实 RED，随后作最小修改并取得 GREEN。

第二处测试使用独立 ambient alias，并选择能够由该合成账号实际注册和路由的 provider 路径；可以沿用合法 Claude 账号、本地 CONNECT 代理及 `/v1/messages`，与 direct fixture 的 `censorship-integration-model` 分开。测试必须证明第二个 helper 的隔离，不能仅复跑 `startCPAWithOptions` 的探针或检查第一处源码。

### S3. 可重复的账号干扰回归

两个 helper 的回归均只使用 `t.TempDir` 内的合成 home、账号和本地服务。通过 `t.Setenv` 同时设置 `HOME`、`USERPROFILE` 并自动恢复，测试及包含这些测试的父测试不使用 `t.Parallel`。不得读取、枚举、复制或修改真实账号目录，不调用商业模型服务。合成账号的 `proxy_url` 只能指向测试自己运行的本地代理；该代理记录 arrival 并对 CONNECT 返回 502，不转发、隧穿或解析商业服务请求正文。

验收必须覆盖以下可观察结果：

1. 实际生成 YAML 满足 S1/S2，两个实例不共享 auth-dir，路径中空格等合法字符经 YAML parser 正确还原。
2. 合成 home 中的启用账号和目标 alias 已存在，隔离实例的 `/v1/models` 不含该 ambient alias。未配置 mapper 的 `claude-opus` 请求稳定返回精确 HTTP 400/model_not_found；direct helper 的独立 alias 请求也返回对应 model-not-found 结果。
3. 未知 alias 请求不进入 fixture upstream，也不进入本地代理。计数使用与目标请求分开的基线，不能把 clean canary 或就绪探测算入目标结果。
4. 本用例声明的模型仍出现在模型表中，clean canary 正常返回 200，fixture arrival/request 增量正确，证明配置隔离没有关闭全部账号或破坏本地路由。
5. 对第一处保留插件启用和关闭对照，确认隔离不依赖 censorship 是否启用；合成账号的空 home、无关 alias、停用账号对照可复用已确认输入，避免扩充无关测试矩阵。

生成配置断言提供不依赖 watcher 时序的失败证据。行为断言仍须有界地观察认证加载后的模型表和路由：开发阶段用未隔离 helper 或临时单项变异确认该合成账号确实能够注册 alias、请求确实到达本地代理，再证明修复后排除同一输入。仅凭 HTTP readiness 后的一次立即请求，或固定短 `sleep` 后未观察到干扰，均不足以通过。若实现时有更简单的等价检查，必须保留实际 YAML、真实路由正负对照、加载时序证据及无商业请求这四项证明，并记录调整依据。

测试复用现有 `cpaInstance`、upstream capture、HTTP client、日志和进程清理。必要的局部断言可以共用，不引入测试框架，不复制完整启动 helper，不为测试增加产品配置或 runner 选项。

### S4. 既有集成语义不变

完整 integration 继续验证 HTTP、SSE、Responses WebSocket、native ABI、配置 watcher、生命周期恢复、模型映射及 direct-provider 路径。既有 clean canary、零 upstream 阻断、正文与机器字段字节、输入不变性、before/selected-auth after、输出 trace、终止与超时、进程树清理的断言保持完整。

不修改 `waitForCPA` 的通用 readiness 含义，不放宽错误码、计数或既有 fixture 的预期来适配账号污染。新测试的加载同步只服务于本次隔离回归。

### S5. 产品与兼容边界不变

固定 CLIProxyAPI v7.2.152 / `c76dfd4e0edabab9000628b1560ab8ab379eadb8`、native ABI 1、schema 5、model-mapper 固定 revision 和全部 Go 依赖。插件生产源码、C exports、ownership、pointer lifetime、同步 `C.GoBytes`、机器字段处理、显式文本路径、`block -> strip -> obfs`、filter/phase 及 request-only 注册均保持不变。

源码默认 `pluginVersion` 仍为 `0.0.0-dev`；仅正式构建通过现有 `VERSION` 注入 `0.3.13`。七个标准 `ConfigFields` 及 Object `words` 不变，不增加全局 `mode` 或自定义界面。

## 验收与发布条件

修复 owner 在同一独立 worktree 连续完成两个 helper 各自的 RED -> 最小 GREEN、版本 guard 的 RED/GREEN、完整回归、规格自审、修正、复审和英语提交。文档先经独立审查并合并 main，随后从该 SHA 启动唯一 fix worker；主会话只合并已经完成并通过独立审查的修改。

本地门槛包括 `make test`、`make race`、`make vet`、`make integration`、`make build`，以及显式 Windows packager test/race/vet、selected-platform 和 aggregate 两个 package 入口。正式 ZIP 核对 CRC、精确成员、LICENSE、lowercase SHA-256 sidecar 与 aggregate；从同包提取 DLL，实际读取 ABI 1、schema 5、Version 0.3.13、七个 `ConfigFields`、Object `words`、request-only 注册，并通过 active-after smoke。已完成的六个 bounded fuzz 基线及其适用范围在计划中记录；生产与对应 oracle 未变时不重复运行，不能写成本次新运行结果。

发布由主会话在发布授权范围内完成：最终 main CI 的 test、七平台 build、Windows amd64 ABI smoke 全部成功后才创建 v0.3.13 tag；tag CI 与 release 全部成功后，下载精确 15 资产，核对 checksum 及七个共享库的平台、`c-shared`、CGO、固定依赖和最终 revision。正式发行 Windows amd64 DLL 必须重新实际注册；其他平台没有执行 runtime 时明确记录 `not_run`，不以构建成功或本地 fixture 200 宣称跨平台运行或商业 API 接受。

本次文档任务只新增本设计和对应计划，不修改代码或发行文件，不合并 main，不 push，不 tag。验证证据保存在执行 owner 自己的专用忽略目录；清理只涉及本次已完成并合并的 worktree、分支和无需保留的临时内容，用户旧 worktree 保留。
