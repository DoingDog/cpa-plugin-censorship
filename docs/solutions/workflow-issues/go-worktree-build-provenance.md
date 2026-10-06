---
title: "核查 nested worktree 中 Go 共享库的源码来源"
date: "2026-10-06"
category: workflow-issues
module: "CPA Censorship Plugin"
problem_type: workflow_issue
component: development_workflow
severity: low
applies_when:
  - "在父仓库内部的 linked Git worktree 构建 Go c-shared 库"
  - "go version -m 的 module 版本或 vcs.revision 与预期来源不同"
tags: ["go", "git-worktree", "cgo", "build-provenance"]
---

# 核查 nested worktree 中 Go 共享库的源码来源

## Context

Go 1.26.5 的一次 Windows amd64 `c-shared` 构建中，worktree 的实验 sourceHead 为 `20e93f65db747c051dbc0fc21d870abd4622308d`，`go version -m` 却记录 module 版本 `v0.3.8` 和 `vcs.revision=2d4dc5da26f4bbea574ad3008fd0943e9a510e23`。同一正式 DLL 的 ABI 注册报告注入版本 `0.3.9`。这次调查分别核查了编译源码与 VCS metadata 的来源。

历史证据目录为 `C:/Users/user/Downloads/cpa-plugin-censorship/.claude/worktrees/wf_b2ecfa23-d8d-1/.superpowers/sdd/2026-10-02-v039-functional-corrections/provenance/`。其中 `source-and-artifacts.json` 保存工具链、源码对象、构建命令、build info 和产物；`official-compiler-trace.log` 保存实际编译过程；`verification.log` 保存源码与产物核对结果；`commands.log:108-152` 保存 build info、ABI 注册和 PE export Name 的检查输出。这些文件是本地、未提交的历史实验记录，后续 checkout 可能不包含它们；以下保留关键结果和复核方法。

在该机器的源码 `F:/go-sdk/go1.26.5/src/cmd/go/internal/vcs/vcs.go:214-218` 中，`vcsGit.RootNames` 将 `.git` 根标记设为目录；`:636-644` 的 `isVCSRoot` 用 `fi.IsDir() == root.isDir` 核对类型；`:582-631` 的 `FromDir` 向父目录查找。此实验 worktree 的 `.git` 是文件，父仓库的 `.git` 是目录。实际 trace 的 VCS 查询使用父仓库目录，`compile` 和 `cgo` 输入使用实验 worktree 的源码。这一诊断限定于已观察的 Go 1.26.5 和该 nested worktree 布局。

## Guidance

遇到 build info 与预期提交不同，保留并分别检查三类证据：

1. 源码与编译输入。在实际构建 worktree 记录 HEAD、工作目录状态、源码及 `go.mod`/`go.sum` 的 Git blob 和文件 SHA-256、Go 版本与 `go env`。需要重建时，仅在已获授权、能够保留相同构建条件的新隔离 checkout 中按相同工具链、构建参数和正式相对输出路径执行 `go build -a -x`，保存真实退出状态和 trace，不覆盖待核查 DLL。另建 checkout 的建议仅适用于允许这样复核的环境，仍须遵守执行环境的 worktree、权限和 Git 结构限制。检查 trace 中实际 `compile`/`cgo` 输入的目录，并将这些输入与记录的源码对象核对。
2. VCS metadata。检查 trace 中 VCS 查询使用的目录，再读取共享库的 `go version -m`。单独记录 module 版本、`vcs.revision`、平台和依赖；metadata 有差异时，不能仅据此判定编译使用了旧源码。升级 Go 后，重新检查对应版本的 VCS 根识别代码和实际 trace。
3. 产物一致性。将重建库与待核查库逐字节比较并记录 SHA-256，保留相同输出 basename、正式相对路径和构建参数。该次改名为 `censorship-traced.dll` 的产物具有不同 PE export Name 和 hash，初次 byte equality 断言失败；这只是该次实验的观察。若构建失败或输入无法核对，保留未完成项，不用相同版本字符串代替来源证明。

版本注入独立核对。[Makefile 的 `build-platform`](../../../Makefile) 第 39-42 行使用 `-buildmode=c-shared` 和 `-X main.pluginVersion`；[main.go 的 `pluginRegistration`](../../../main.go) 第 41-46 行把 `pluginVersion` 写入注册 metadata。ABI 注册版本可以核对注入结果，不能独立证明源码提交。

远端产物仍须单独验收。[Build workflow](../../../.github/workflows/build.yml) 使用 `actions/checkout@v5`，目标为 `linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64`、`windows/amd64`、`windows/arm64`、`freebsd/amd64`。按 [Task G 的 main CI -> tag CI -> 发行资产验收顺序](../../superpowers/plans/2026-10-02-v039-functional-corrections.md)，对最终 main CI 产物及 tag 发行产物分别读取七个共享库的 build info，将每个 `vcs.revision` 与对应已通过 CI 的最终 main/tag SHA 独立比较。最终 SHA 包含后来加入的文档提交；本地实验 sourceHead 只用于复核历史证据。缺失或不同的 revision 保持未通过，版本注入、checksum、workflow `headSha` 和 job success 均不能替代该项比较。库的平台、`c-shared` 构建模式与 [go.mod 中固定的依赖](../../../go.mod) 也须核对。

## Why This Matters

该次 trace 同时呈现当前 worktree 的编译输入和父仓库的 VCS 查询。若仅凭旧 revision 判断源码错误，会重复调查已核实的编译过程；若仅凭 ABI 版本或 hash 接受发布，又会遗漏最终提交的独立验收。本地来源诊断解释该次 metadata 差异，远端 revision 检查验证实际交付提交，两者各自保留证据。

## When to Apply

- 在父仓库内部的 linked worktree 构建 Go 共享库，发现 build info 与预期源码来源不同。
- 比较重建库和既有 DLL 时，输出名称或参数发生变化。
- 按[当前设计的发布条件](../../superpowers/specs/2026-10-02-v039-functional-corrections-design.md)验收最终 main/tag 的七个平台产物。本机 worktree 的历史诊断不放宽远端 revision 要求，也不代表七个平台均已实际加载。

## Examples

该次正式路径重建的命令记录含 `-a -x -trimpath -buildmode=c-shared`、`-X main.pluginVersion=0.3.9` 和输出 `dist/windows_amd64/censorship.dll`，真实 build 退出 0。`verification.log:5-18` 记录 13 个源码及依赖文件的 blobs 与实验 HEAD 和当时 F BASE 相同，编译源码路径属于实验 worktree，VCS 查询目录为父仓库。正式重建库、当时 F 的正式库和 staged ABI smoke 库逐字节相同，SHA-256 为 `2808a809158cd7b82f0d7cef32ac43f9b5e025b130d42aab70d957e2f34949a4`。这个 hash 只标识该次历史产物，后续版本另行计算。

从实际构建 worktree 读取环境，在新下载目录中检查已有库。将示例中的 `$library` 设置为实际待验收文件；这些命令不重建或覆盖 DLL：

```powershell
git rev-parse HEAD
git status --short
go version
go env GOMOD GOROOT GOFLAGS GOOS GOARCH CGO_ENABLED CC
$library = "C:/verified-download/windows_amd64/censorship.dll"
go version -m $library
Get-FileHash $library -Algorithm SHA256
```
