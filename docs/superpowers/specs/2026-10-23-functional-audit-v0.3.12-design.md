# v0.3.12 功能修复设计

日期：2026-10-23。基线：`main@23fe07f1c4ba2ec23879ec2addc8015b4d7acc6d`（v0.3.11）。目标：v0.3.12。

## 范围与依据

本规格定义已独立复现的 lifecycle 修复：首次注册因配置无效而失败后，合法配置必须能够通过固定 host 随后发送的 `plugin.reconfigure` 成功启用规则。本规格不代表其余全面扫描已经完成；其他最终确认问题统一增补到本规格与[实施计划](../plans/2026-10-23-functional-audit-v0.3.12.md)。

固定 CLIProxyAPI v7.2.152、commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`、schema 5、native ABI v1，版本依据为 [go.mod](../../../go.mod) 与 [integration runner](../../../.github/scripts/integration-runner.go)。仅修改 censorship plugin，不修改 CPA core，不执行网络安全扫描，不做无关重构。

## 已确认缺陷

两次独立验证从当前源码构建 Windows amd64 DLL，使用固定 `Host.ApplyConfig` 复现了以下调用链；其中一次同时运行真实 CPA 配置 watcher 与 HTTP 请求。

1. [config.go 的 `init`](../../../config.go) 预先存入默认空 snapshot，其中 `Rules` 与 `Filter.Models` 均为空。
2. 首次 `plugin.register` 因 `words.block` 含空词失败，未安装任何成功解析的配置。固定 host 保留已加载实例，并将其内部 `lp.registered` 设为 true，后续 `ApplyConfig` 调用 `plugin.reconfigure`。
3. 把空词改为 `BLOCKME`、保留合法非空 `filter.models` 后，[`handlePluginReconfigure`](../../../main.go) 把默认空 snapshot 当成旧有效 phase，记录 `filter.models phase change requires restart`，保留默认 snapshot，却返回成功 registration。
4. host 随后报告 `PluginRegistered("censorship") == true`。before 与 selected-auth after 均不拦截；真实 watcher 场景返回 HTTP 200，upstream 新收到一次包含 `BLOCKME` 的请求。重复应用同一合法非空 models 配置仍不能恢复。

独立新进程首次注册合法非空 models 能正常拦截；首次失败后改成合法空 models 也能恢复。缺陷来自默认 snapshot 被当成已安装配置。

## 初始化与 phase 设计

`activeConfig` 的原子指针初始保持 nil，表示尚未成功安装配置。删除启动时预装默认 snapshot 的 `init()`；`defaultSnapshot()` 继续提供成功解析配置所需的默认值，包括合法空 YAML 与零规则配置。

`plugin.register` 继续先完整解析、校验和编译 candidate，再通过现有 `installSnapshot` 原子发布。`plugin.reconfigure` 仅在 `current != nil` 时比较 old 与 candidate 的 models phase。尚无有效 snapshot 时，合法空 models 与非空 models candidate 均可成为首次有效配置。

| 当前 snapshot | 合法 candidate 的 models | 结果 |
|---|---|---|
| nil | 空 | 安装 candidate，使用 before 阶段 |
| nil | 非空 | 安装 candidate，使用 selected-auth after 阶段 |
| 非 nil，models 为空 | 空 | 原子更新 candidate |
| 非 nil，models 非空 | 非空 | 原子更新 candidate |
| 非 nil，models 为空 | 非空 | 整体拒绝，保留旧 snapshot 与原拒绝日志 |
| 非 nil，models 非空 | 空 | 整体拒绝，保留旧 snapshot 与原拒绝日志 |

成功安装的零规则 snapshot 同样属于非 nil 状态，继续锁定 phase。不新增初始化 boolean，不用 `len(Rules)` 判断是否成功初始化，不增加配置键或请求关联状态。

[2026-09-15 phase 规格](2026-09-15-model-filter-post-route-execution-design.md) 对已有有效 snapshot 的双向 phase 拒绝、selected-auth marker、auth-selected attempt `Model`、同 phase callback 边界及 drain/restart 限制继续适用。两阶段现有 nil gate 已在 envelope decode 前返回 no-op，保留原实现。

## 修改位置与兼容边界

| 位置 | 本次责任 |
|---|---|
| `config.go` | 仅调整启动时 active snapshot 的状态；保留 parser、默认值与 atomic 发布方式 |
| `main.go` | 仅调整未安装 snapshot 时的 models phase guard；保留注册、已安装配置拒绝与请求处理行为 |
| `main_test.go` | 在 lifecycle/RPC 行为测试处增加恢复、nil gate 与零规则 phase 回归；复用现有 helpers |
| 新增 `integration/lifecycle_test.go` | 专属 native `Host.ApplyConfig` 恢复矩阵及真实 watcher HTTP 回归 |
| `config_test.go`、`abi_cgo_test.go` | 运行既有并发、input copy、pointer 与 buffer 回归，无须为本设计改写这些文件 |
| `integration/harness_test.go`、`.github/scripts/integration-runner.go`、`.github/scripts/testdata/abi_benchmark_test.go` | 复用启动、配置写入、fixture 复制及 active-after oracle，不修改 CPA 生成目录 |

生产 caller 为 `handleMethod` 分派的 register/reconfigure 与 before/after RPC。固定 host 是 lifecycle caller，保持只读；不改变其内部状态处理。`withDynamicABIHost` 要求首次注册成功，不能直接承担首次失败的 native case；新 fixture 在自己的子进程直接创建 host 并调用 `ApplyConfig`。

以下边界保持不变：

- `words` Object、手写 YAML legacy sequence、七个 `ConfigFields`、request-only capability、schema 5 与 ABI v1。
- `block -> strip -> obfs`、自然语言 selectors、机器字段与 provider/filter 语义。
- metadata 与 headers 的传递、正文改写时的 `ClearHeaders`、原请求字节不变性。
- `cliproxyPluginCall` 同步 `C.GoBytes`、host pointer 生命周期、response buffer ownership 与 `free_buffer`。
- 全部现有 C exports、依赖版本、配置默认值；不增加依赖、重试、fallback、cache 或额外初始化状态。

## 验收条件

| ID | 场景与可观察结果 |
|---|---|
| L1 | 首次空词 register 返回 `plugin_error`，随后合法空 models reconfigure 成功；含 `BLOCKME` 的请求在 before 返回 HTTP 400 `censorship_blocked`，after 不重复处理 |
| L2 | 同样的首次失败后，合法非空 models reconfigure 成功；before 精确 no-op，有效 selected-auth after 使用匹配 `Model` 返回 400；重复应用该合法配置后仍拦截 |
| L3 | 独立新进程首次成功注册空 models、非空 models 均保留既有行为；`RequestedModel` 与 body model 使用相反 decoy，不影响匹配结果 |
| L4 | 已有有效 snapshot 后，empty -> non-empty 与 non-empty -> empty 继续整体拒绝；旧规则、旧处理阶段及不含配置内容的拒绝日志保持不变；成功安装零规则 snapshot 的两个方向也受此限制 |
| L5 | empty -> empty、non-empty -> non-empty 更新生效；已有 snapshot 时，malformed lifecycle envelope、缺失/null `config_yaml`、无效 YAML candidate 均保留 last-good 与原响应行为 |
| L6 | nil snapshot 下，两个拦截阶段对 malformed raw envelope 均返回精确 no-op，且不修改原输入；无效注册不得安装 snapshot |
| L7 | atomic 发布只暴露完整 snapshot；既有并发、metadata、headers、输入不变性、ABI copy/free 与 active-after 回归通过 |
| L8 | native 矩阵从真实 DLL 和固定 `Host.ApplyConfig` 验证首次失败恢复、已有 phase 保护、同 phase 更新与无效配置保留；watcher HTTP 从首次无效配置修正到合法配置后返回 canonical 400，待验收请求的 upstream arrival/request 增量均为零 |

RPC no-op 是空 `RequestInterceptResponse`；固定 host adapter 对 no-op 保留原 body 与 headers。malformed lifecycle envelope 通过公开 RPC 测试，native 配置路径使用 host 实际可发送的输入。

native 矩阵每个独立初始状态必须使用独立子进程。同一场景的失败、修正与重复应用必须留在同一个 host/DLL 实例；不能用 `ShutdownAll` 或 `cliproxyPluginShutdown` 代替 Go DLL 静态状态重置。

watcher 等待必须有截止时间，并确认修正配置已经被观察；同步确认后再记录 upstream 基线并发送待验收请求。保留 clean canary 成功转发对照，排除 provider/auth/路由不可用造成的假通过。

## 回归与发布完成条件

实施按 TDD 在公开 RPC 与 native/watcher seam 留下真实 RED/GREEN 命令、退出状态和日志。新增测试归属 `main_test.go` 与专属 lifecycle fixture，避免复制通用 harness；不手改 `.integration/` 或 `dist/`。

所有已确认修复合并后，顺序更新英语 `README.md`、`RELEASE_NOTES.md` 及 `main_test.go` 的发行版本断言，目标 v0.3.12。完成全量本地回归、两种正式打包与同 DLL 实际注册后，仍须等待最终 main CI、tag CI 和七平台发行资产分别验收。具体命令、工作区责任、来源核对与发布依赖由[实施计划](../plans/2026-10-23-functional-audit-v0.3.12.md)规定。
