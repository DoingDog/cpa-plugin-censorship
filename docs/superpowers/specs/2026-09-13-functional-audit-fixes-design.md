# 全面功能审计修复设计

日期：2026-09-13

## 目标

修复审计中已由真实插件路径、独立 benchmark 或发布脚本复现的五类问题，同时保持 CLIProxyAPI v7.2.152、native ABI v1、现有 provider 选择范围和处理顺序不变。所有改动限于 censorship plugin 仓库，不修改 CLIProxyAPI core，不手工修改 `.integration/` 或 `dist/`。

## 已确认问题

1. Gemini signature-bound 文本经过 `strip` 后又被 `obfs` 恢复为原字节时，`textSpan.Changed` 仍保留中间阶段的 `true`。插件因此对最终未改动的请求返回 `censorship_invalid_request`。
2. Gemini Interactions 的 `model_output.content[].annotations` 使用按 UTF-8 字节计数的 `start_index` 和 exclusive `end_index`。插件当前可在不更新 annotations 的情况下向对应 `text` 插入 U+200B，使 citation 指向错误字节范围。
3. `rewriteFoldedKMP` 在文本开头检测到两个相邻 match 后回退到逐起点执行 `foldMatchEnd`。128-scalar pattern 加 64 KiB near-miss tail 的实测从约 0.25 ms 退化到约 21.5 ms，吞吐从约 255 MB/s 降到约 3 MB/s。
4. aggregate packager 重用 output directory 时，只为本轮存在的 source binary 重建 ZIP 和 checksum，却不删除同版本、本轮 source 已不存在的平台产物。成功返回后，旧 ZIP 和 `.sha256` 仍存在，但新的 `checksums.txt` 不再列出它们。
5. release workflow 对已存在的 release 无条件执行 `gh release upload --clobber`。已存在的 draft 不会因为 `gh release edit --notes-file` 而发布；已发布 release 的同名资产会在新上传前删除，上传失败时原资产丢失。

## 设计

### 1. 以最终文本决定 rewrite 状态

`applyMode` 在 rewrite 前保存每个 span 的原始 string，并保持 `block -> strip -> obfs`、规则顺序和单次处理语义不变。所有 rewrite 完成后，以 `span.Text != original[i]` 重新计算每个 `span.Changed` 和整体 `changed`。

这样只有最终请求字节需要重建时才返回 body，也只有最终文本确实变化时才触发 `RequiresUnmodified` 和 `RequiresNonEmpty` 校验。中间阶段仍按当前顺序影响后续规则，不改成 fixed point，也不改变 leftmost non-overlapping 语义。

### 2. 保护 Interactions citation-bound 文本

只在 `selectors_interactions.go` 的明确 natural-language text paths 处理 annotations，不扩展到其他 provider，也不增加递归 string walker。

新增一个本文件内的小 helper，继续通过 `scanTextPart` 验证 text part，然后调用现有 `appendStringSpan`。当 part 含非空 `annotations` array 时，把刚追加的 span 标为 `RequiresUnmodified`，并提供准确的本地拒绝消息 `censorship cannot rewrite annotated text`。`textSpan` 增加可选的 unmodified 错误消息；未设置时继续使用现有 signature-bound 消息，因此 Gemini signed-part 行为和 API 文案保持不变。

`block` 仍可检查 annotated text。只有 `strip` 或 `obfs` 导致最终文本变化时才 fail-closed；规则最终相消时不拒绝。

### 3. 保持 folded rewrite 为线性扫描

删除 `rewriteFoldedKMP` 中基于开头两个相邻 match 的 naive fallback。现有 KMP 在每次完整 match 后把 state 重置为 0，已经实现 leftmost non-overlapping 行为，并能直接处理相邻 match。

保留现有短文本和短 pattern 的 naive 路径；只修复已进入 KMP 路径后重新退化的问题。新增 adversarial benchmark 覆盖一个和两个 leading matches 后接 near-miss tail，使用现有 correctness tests 验证输出不变。

### 4. aggregate package 清除同版本 stale outputs

`packageExistingArtifacts` 在枚举支持平台时，同时记录本版本中 source binary 缺失的平台 ZIP 和 `.sha256` 路径。当前平台的新包和单独 checksum 全部成功生成后，删除这些同版本 stale outputs；忽略不存在，其他删除错误导致命令失败。随后通过现有原子写入路径更新 `checksums.txt`。

清理范围只包括 `artifactSpecs()` 可推导出的当前 normalized version 文件名，不删除其他版本、未知文件、source binary 或目录。直接 `-library/-archive/-checksum` 模式不变。

### 5. release 发布和验证

release job 先查询 existing release 的 `isDraft`：

- 不存在：使用 `gh release create --draft` 创建 release 并上传资产，在 draft 状态下验证全部 assets，验证通过后执行 `gh release edit --draft=false`。
- 已存在且是 draft：更新 notes，在 draft 状态下使用 `--clobber` 上传，然后在发布前下载并验证全部 assets，验证通过后执行 `gh release edit --draft=false`。
- 已存在且已发布：更新 notes，但绝不覆盖公开资产；直接下载并验证。若本地构建产物与公开 assets 不一致，workflow 失败并保留公开资产原状。

验证使用新的临时目录下载该 tag 的所有 release assets，要求其文件集合和字节内容与 `release/` 完全一致；随后分别执行每个 `*.zip.sha256` 和 aggregate `checksums.txt` 的 `sha256sum --check`。这同时验证 ZIP、每个单独 SHA-256、aggregate manifest 和 remote upload。draft 在验证前不可见；published rerun 变为只验证的幂等路径。

## 测试

按 TDD 添加以下 focused tests：

- Gemini signed text 经 `strip` 和 `obfs` 相消后不 terminate、不返回 rewrite body。
- Interactions 官方 `model_output` + `TextContent.annotations` 形状在 assistant rewrite 会破坏 offsets 时返回 400；block 仍能命中；空 annotations 不受额外限制。
- package aggregate 第二次只提供部分 source platforms 后，不再保留缺失平台的同版本 ZIP 或 `.sha256`，且 manifest 只列当前包。
- workflow contract 覆盖 `isDraft` 查询、draft 验证后发布、published release 不使用 `--clobber`、remote 文件集合和两层 checksum 验证。
- adversarial folded rewrite benchmark 记录优化前后结果；现有 matcher equivalence、fuzz 和 full suite 保证输出语义不变。

验证命令包括 focused tests、`make test`、`make vet`、`make race`、`make integration`、`make build` 和 `make package`。

## 明确不做

- 不改变 overlapping occurrence、leftmost non-overlapping、单次处理或 rule order 契约。
- 不选择 OpenAI reasoning、Gemini thought/signature、tool arguments、schema 或其他 machine state。
- 不为 OpenAI、Claude 或其他 provider 推断 annotation 规则。
- 不验证 packager 输入的 binary target 或 embedded version；现有公开契约没有要求该 hardening。
- 不改变 ABI ownership、pointer lifetime、`C.GoBytes` 或 CPA fail-open 策略。
- 不尝试自动覆盖已发布 release 的不一致资产；GitHub CLI 的 `--clobber` 明确会先删除原资产，安全行为是验证失败并保留公开文件。
