# 全面功能审计修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复五类已复现的 request transform、Interactions metadata、matcher 性能、aggregate package 和 GitHub release 错误，并发布 v0.2.5。

**Architecture:** 保持现有显式 provider selectors、`block -> strip -> obfs` pipeline、native ABI v1 和 package format。修复位于共同根因处：以最终 span 内容计算 mutation；只给 Interactions annotated text 增加 immutable metadata contract；让长 folded rewrite 始终走 KMP；让 aggregate output 与本轮 source artifact set 一致；只在 draft release 覆盖 assets，并在发布前验证。

**Tech Stack:** Go 1.26、CGO、`testing`、`gjson`、GitHub Actions Bash、GitHub CLI、SHA-256、ZIP。

**Spec:** `docs/superpowers/specs/2026-09-13-functional-audit-fixes-design.md`

## Global Constraints

- 只修改 censorship plugin 仓库，不修改 CLIProxyAPI core。
- 保持 pinned `github.com/router-for-me/CLIProxyAPI/v7` v7.2.152、host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8` 和 native ABI v1。
- 保持 ABI ownership、pointer lifetime 和 `cliproxyPluginCall` 的 `C.GoBytes`。
- `words` 仍为 strict Object；handwritten legacy sequence 行为不变；panel 不输出 global `mode`。
- 保持 `block -> strip -> obfs`、YAML rule order、once-only 和 leftmost non-overlapping 语义。
- 只选择明确 provider natural-language paths，不增加递归 string walker。
- 不手工修改 `.integration/` 或 `dist/`。
- 每个行为修复先运行 focused failing test，再写最小实现。
- 不实现 OpenAI reasoning rewrite、其他 provider annotation 推断、binary metadata validation 或无复现 hardening。

---

### Task 1: 以最终 span 内容决定 Changed

**Files:**
- Modify: `transform_test.go`
- Modify: `transform.go:55-88,235-271`

**Interfaces:**
- Consumes: `applyMode(spans []textSpan, cfg *configSnapshot) (*blockMatch, bool)`。
- Produces: `span.Changed` 和返回的 `changed` 都表示相对进入 `applyMode` 时的最终变化，而不是中间 rule 曾命中。

- [ ] **Step 1: 写 failing regression test**

在 `transform_test.go` 添加 `TestSignedRewriteCancellationLeavesRequestUntouched`。注册以下 config：

```yaml
words:
  strip: ["​"]
  obfs: [ab]
scope:
  roles: [assistant]
```

通过 `interceptRPC` 发送：

```json
{"contents":[{"role":"model","parts":[{"text":"a​b","thoughtSignature":"c2ln"}]}]}
```

断言 `Terminate == false`、`Body` 为空、`ResponseBody` 为空。

- [ ] **Step 2: 验证 RED**

Run：

```bash
go test . -run '^TestSignedRewriteCancellationLeavesRequestUntouched$' -count=1 -v
```

Expected：FAIL，当前实现返回 status 400 和 `censorship cannot rewrite signature-bound text`。

- [ ] **Step 3: 写最小根因修复**

在 `applyMode` 的 block phase 通过后、rewrite phase 前保存每个 `spans[i].Text`：

```go
original := make([]string, len(spans))
for i := range spans {
	original[i] = spans[i].Text
}
```

保留 strip 和 obfs 的现有文本赋值，但不要在 rule loop 中把最终 `changed` 永久置为 true。所有 rewrite 完成后统一计算：

```go
changed := false
for i := range spans {
	spans[i].Changed = spans[i].Text != original[i]
	changed = changed || spans[i].Changed
}
return nil, changed
```

不改变 block early return、rule order 或 matcher 调用。

- [ ] **Step 4: 验证 GREEN 和相邻 transform 行为**

Run：

```bash
go test . -run '^(TestSignedRewriteCancellationLeavesRequestUntouched|TestMixedRules|TestGeminiSignatureBoundVisibleText)' -count=1 -v
```

Expected：PASS。

- [ ] **Step 5: Commit**

```bash
git add transform.go transform_test.go
git commit -m "fix: track effective request rewrites"
```

---

### Task 2: 保护 Interactions annotations byte offsets

**Files:**
- Modify: `transform.go:9-20,73-79`
- Modify: `selectors_interactions.go`
- Modify: `selectors_interactions_test.go`
- Modify: `fuzz_test.go:35-39,540-680,1546-1665,2126-2245`

**Interfaces:**
- Consumes: `scanTextPart(part gjson.Result, requireTextType bool)` 和 `appendStringSpan`。
- Produces: Interactions part 中非空 `annotations` array 对应的 span 设置 `RequiresUnmodified`，并将 `UnmodifiedMessage` 设置为 `censorship cannot rewrite annotated text`。

- [ ] **Step 1: 写三个 focused tests**

在 `selectors_interactions_test.go` 添加：

1. `TestInteractionsAnnotatedModelOutputRejectsRewrite`，使用官方 request shape：

```json
{"input":[{"type":"model_output","content":[{"type":"text","text":"SECRET","annotations":[{"type":"url_citation","start_index":0,"end_index":6,"url":"https://example.com"}]}]}]}
```

assistant `obfs` 命中时断言 terminate、status 400、error code 为 `censorship_invalid_request`、message 为 `censorship cannot rewrite annotated text`。
2. `TestInteractionsAnnotatedModelOutputStillBlocks`，同一 body 使用 assistant `block`，断言仍以 term `SECRET` 和 role `assistant` 阻止请求。
3. `TestInteractionsEmptyAnnotationsRemainRewritable`，把 annotations 改为 `[]`，断言输出 text 为 `S​ECRET` 且不 terminate。

- [ ] **Step 2: 验证 RED**

Run：

```bash
go test . -run '^TestInteractions(AnnotatedModelOutputRejectsRewrite|AnnotatedModelOutputStillBlocks|EmptyAnnotationsRemainRewritable)$' -count=1 -v
```

Expected：第一项 FAIL，因为当前插件返回被改写 body；后两项用于锁定 block 和空 array 行为。

- [ ] **Step 3: 增加准确的 unmodified 错误消息**

给 `textSpan` 添加：

```go
UnmodifiedMessage string
```

在 `transformRequest` 的 `RequiresUnmodified` 分支中使用该字段；为空时保持原消息：

```go
message := span.UnmodifiedMessage
if message == "" {
	message = "censorship cannot rewrite signature-bound text"
}
return transformResult{Invalid: true, InvalidMessage: message}, nil
```

- [ ] **Step 4: 在 Interactions selector 的显式 part paths 设置约束**

在 `selectors_interactions.go` 增加本文件 helper：

```go
func appendInteractionTextPart(spans *[]textSpan, part gjson.Result, role string, roles scopeSet) {
	text, allowed, _ := scanTextPart(part, true)
	if !allowed {
		return
	}
	before := len(*spans)
	appendStringSpan(spans, text, role, roles)
	annotations := part.Get("annotations")
	if len(*spans) != before && annotations.IsArray() && annotations.Get("#").Int() > 0 {
		span := &(*spans)[len(*spans)-1]
		span.RequiresUnmodified = true
		span.UnmodifiedMessage = "censorship cannot rewrite annotated text"
	}
}
```

用它替换 `collectInteractions` 和 `collectInteractionItem` 中对 object/array text part 的重复 `scanTextPart` + `appendStringSpan`。scalar `content` 和 top-level string 路径不变。

- [ ] **Step 5: 同步 fuzz oracle**

扩展 `oracleProtocolSpan` 和 raw oracle span 的 metadata，使 Interactions part 的非空 annotations 被标记为 unmodified，并在 `checkProtocolResult` 中只接受 message 为 `censorship cannot rewrite annotated text` 且确有 selected annotated span 命中的 Interactions invalid result。保持 Gemini signature 和 Claude required-empty 分支不变。

- [ ] **Step 6: 验证 GREEN、selector contract 和 fuzz seeds**

Run：

```bash
go test . -run '^(TestInteractions|TestProtocolOracle|FuzzTransformRequest)' -count=1 -v
```

Expected：PASS。

- [ ] **Step 7: Commit**

```bash
git add transform.go selectors_interactions.go selectors_interactions_test.go fuzz_test.go
git commit -m "fix: preserve Interactions citation offsets"
```

---

### Task 3: 删除 folded KMP 的 naive fallback

**Files:**
- Modify: `matcher.go:325-380`
- Modify: `benchmark_test.go:1055-1139`

**Interfaces:**
- Consumes: `rewriteFoldedKMP(text string, rule compiledRule, char string, obfuscate bool) (string, bool)`。
- Produces: 长 text、长 folded pattern 始终在线性 KMP path 扫描；返回内容和 match bool 不变。

- [ ] **Step 1: 添加 adversarial benchmark 并记录基线**

给 `runBenchmarkFoldedRewriteStrategies` 的 holdout cases 添加专门 case，或增加 `BenchmarkFoldedRewriteAdjacentPrefixTail`。pattern 为 `strings.Repeat("a", 127) + "b"`，分别测试一个和两个 leading source matches，再追加 `strings.Repeat("a", 64<<10)`。

Run：

```bash
go test . -run '^$' -bench '^BenchmarkFoldedRewriteAdjacentPrefixTail$' -benchtime=500ms -count=5
```

Expected baseline：one-leading 约 0.25 ms/op；two-leading 约 21.5 ms/op，后者约慢 80 倍。保留原始输出用于优化后对比。

- [ ] **Step 2: 删除根因分支**

从 `rewriteFoldedKMP` 删除：

```go
if !matched && start == 0 && scan < len(text) {
	if _, adjacent := foldMatchEnd(text, scan, pattern); adjacent {
		return rewriteFolded(text, pattern, char, obfuscate)
	}
}
```

不改变 match 后的 `written = scan` 和 `state = 0`。

- [ ] **Step 3: 验证 matcher correctness**

Run：

```bash
go test . -run '^(TestFoldKMP|TestStripUsesLeftmostNonOverlappingOccurrences|TestMixedRules)' -count=1 -v
```

Expected：PASS。

- [ ] **Step 4: 记录优化后 benchmark**

Run：

```bash
go test . -run '^$' -bench '^BenchmarkFoldedRewriteAdjacentPrefixTail$' -benchtime=500ms -count=5
```

Expected：two-leading 不再进入逐起点 pattern scan，耗时接近 one-leading，输出与 baseline correctness 相同。

- [ ] **Step 5: Commit**

```bash
git add matcher.go benchmark_test.go
git commit -m "perf: keep folded rewrites on KMP path"
```

---

### Task 4: 删除 aggregate package 的同版本 stale outputs

**Files:**
- Modify: `.github/scripts/package-release.go:76-195`
- Modify: `.github/scripts/package-release_test.go`

**Interfaces:**
- Consumes: `artifactSpecs()` 和当前 normalized version。
- Produces: `packageExistingArtifacts` 成功返回后，output directory 只保留本轮 source set 对应的当前版本受支持平台 packages/checksums；其他版本和未知文件不变。

- [ ] **Step 1: 写 failing aggregate rerun test**

添加 `TestPackageExistingArtifactsRemovesMissingPlatformOutputs`：

1. 在 `t.TempDir()` 创建第一套 `dist/linux_amd64/censorship.so` 与 `dist/windows_amd64/censorship.dll`。
2. 调用 `packageExistingArtifacts("1.2.3", distA, out)`。
3. 创建只含 Linux source 的 `distB`，再次调用同版本和同一 `out`。
4. 断言 Windows ZIP 和 `.sha256` 均 `os.IsNotExist`。
5. 断言 Linux ZIP 与 checksum 存在，`checksums.txt` 只有 Linux 一行。
6. 在 `out` 预放 `censorship_1.2.2_windows_amd64.zip` 和 `keep.txt`，断言二者未删除。

- [ ] **Step 2: 验证 RED**

Run：

```bash
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^TestPackageExistingArtifactsRemovesMissingPlatformOutputs$' -count=1 -v
```

Expected：FAIL，当前同版本 Windows ZIP 和 `.sha256` 仍存在。

- [ ] **Step 3: 收集受限 stale path 并清理**

在 artifact enumeration 中，对 source 不存在的 spec 构造本版本 `zipPath` 和 `zipPath + ".sha256"` 并加入 `staleOutputs`。当前 packages 全部成功写完后，逐个执行 `removeOutputFile`；`os.IsNotExist` 忽略，其他错误带具体 path 返回。stale 清理成功后调用现有 `writeChecksums`。

不要 glob output directory，不删除其他版本或未知文件，不改变 direct package mode。

- [ ] **Step 4: 验证 GREEN 和全部 packager tests**

Run：

```bash
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1
```

Expected：PASS。

- [ ] **Step 5: Commit**

```bash
git add .github/scripts/package-release.go .github/scripts/package-release_test.go
git commit -m "fix: remove stale aggregate packages"
```

---

### Task 5: 让 release rerun 不删除公开 assets

**Files:**
- Modify: `.github/workflows/build.yml:194-218`
- Modify: `.github/scripts/package-release_test.go:1753-1873`

**Interfaces:**
- Consumes: downloaded build artifacts in `release/`、`GITHUB_REF_NAME`、`GH_TOKEN`。
- Produces: draft 在 assets 验证后发布；published release 只更新 notes 并验证，不执行 asset overwrite。

- [ ] **Step 1: 强化 failing workflow contract test**

在 `TestBuildWorkflowContract` 对 release run 增加以下必需片段：

```plaintext
--json isDraft --jq .isDraft
--draft
if [[ "${release_state}" == "true" ]]
gh release edit "$tag" --draft=false
gh release download "$tag"
diff -qr release
sha256sum --check checksums.txt
*.zip.sha256
```

同时检查 release shell 包含独立的 published branch，该 branch 调用 verification 而不调用 upload；`--clobber` 只位于 draft branch 的 exact block 中。删除原先只要求任意 `gh release upload` + `--clobber` 的宽松断言。

- [ ] **Step 2: 验证 RED**

Run：

```bash
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^TestBuildWorkflowContract$' -count=1 -v
```

Expected：FAIL，当前 workflow 不查询 `isDraft`、不发布 existing draft，也不下载验证。

- [ ] **Step 3: 实现三态 release flow**

在 release step 保留 aggregate manifest 生成，然后加入 `verify_release` shell function。该 function：

1. 清空本 job 新建的 `mktemp -d`。
2. `gh release download "$tag" --dir "$verify_dir"` 下载所有 assets。
3. `diff -qr release "$verify_dir"` 验证文件集合和字节内容。
4. 在 `verify_dir` 中逐个对 `*.zip.sha256` 执行 `sha256sum --check`，再对 `checksums.txt` 执行同一检查。

release branch 使用：

```bash
if release_state="$(gh release view "$tag" --json isDraft --jq .isDraft 2>/dev/null)"; then
  gh release edit "$tag" --notes-file RELEASE_NOTES.md
  if [[ "${release_state}" == "true" ]]; then
    gh release upload "$tag" release/*.zip release/*.sha256 release/checksums.txt --clobber
    verify_release
    gh release edit "$tag" --draft=false
  else
    verify_release
  fi
else
  gh release create "$tag" release/*.zip release/*.sha256 release/checksums.txt --draft --verify-tag --notes-file RELEASE_NOTES.md
  verify_release
  gh release edit "$tag" --draft=false
fi
test "$(gh release view "$tag" --json isDraft --jq .isDraft)" = "false"
```

用 `trap` 清理 verification temp directory。不要在 published branch 使用 `gh release upload` 或 `--clobber`。

- [ ] **Step 4: 验证 GREEN 和 YAML parse**

Run：

```bash
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^TestBuildWorkflowContract$' -count=1 -v
```

Expected：PASS。

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/build.yml .github/scripts/package-release_test.go
git commit -m "fix: verify releases before publication"
```

---

### Task 6: 更新 v0.2.5 文档和契约

**Files:**
- Modify: `README.md`
- Modify: `RELEASE_NOTES.md`
- Modify: `main_test.go:624-635`
- Existing: `docs/superpowers/specs/2026-09-13-functional-audit-fixes-design.md`
- Existing: `docs/superpowers/plans/2026-09-13-functional-audit-fixes.md`

**Interfaces:**
- Produces: v0.2.5 release notes 和 README contract，仍声明 CLIProxyAPI v7.2.152、schema 5、host commit 和 native ABI v1。

- [ ] **Step 1: 先更新 version contract test**

把 `TestReleaseNotesCompatibilityTargetsCurrentVersion` 的 current version 从 v0.2.4 改为 v0.2.5，并断言不再把 v0.2.4 写成 current target line。

- [ ] **Step 2: 验证 RED**

Run：

```bash
go test . -run '^TestReleaseNotesCompatibilityTargetsCurrentVersion$' -count=1 -v
```

Expected：FAIL，RELEASE_NOTES 仍以 v0.2.4 为 current release。

- [ ] **Step 3: 更新 README 和 RELEASE_NOTES**

将 current version 改为 v0.2.5。release notes 只记录本次已实现内容：effective rewrite state、Interactions citation protection、folded KMP tail、stale aggregate cleanup、draft/published release safety 和完整 remote checksum verification。保留 compatibility target、request-only、provider exclusions、ABI ownership 和 packaging contract。

README 在相关章节补充：

- annotated Interactions model-output text 可 block，但 length-changing strip/obfs 会被本地拒绝；空 annotations 不受此限制。
- 最终相消的 rewrite 不返回 body，也不触发 immutable-text 错误。
- aggregate packaging 删除当前版本缺失 source platform 的旧 ZIP/checksum。
- release 只在 draft 覆盖 assets，published rerun 验证而不覆盖。

- [ ] **Step 4: 验证 GREEN 和文档契约**

Run：

```bash
go test . -run '^(TestReleaseNotesCompatibilityTargetsCurrentVersion|TestDocumentation)' -count=1 -v
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^TestDocumentation' -count=1 -v
```

Expected：PASS。

- [ ] **Step 5: Commit**

```bash
git add README.md RELEASE_NOTES.md main_test.go docs/superpowers/specs/2026-09-13-functional-audit-fixes-design.md docs/superpowers/plans/2026-09-13-functional-audit-fixes.md
git commit -m "docs: prepare censorship v0.2.5"
```

---

### Task 7: 完整验证和 diff-only review

**Files:**
- Review: only `git diff 7840cf0...HEAD`
- Modify: 仅 review 中被独立验证为真实问题的本次文件

- [ ] **Step 1: 运行 focused 和 full unit tests**

```bash
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1
make test
```

Expected：全部 PASS。

- [ ] **Step 2: 运行静态和 race 验证**

```bash
make vet
make race
```

Expected：全部 PASS，无 race report。

- [ ] **Step 3: 运行 pinned CPA integration**

```bash
make integration
```

Expected：全部 integration tests PASS；不修改 CLIProxyAPI core 或 `.integration/` generated files。

- [ ] **Step 4: build 和 package**

```bash
make build
make package VERSION=v0.2.5
```

Expected：CGO shared library build 成功；本地 ZIP、各自 lowercase SHA-256 和 aggregate `checksums.txt` 一致。

- [ ] **Step 5: 检查工作树和 generated artifacts**

```bash
git status --short
git diff --check
git diff --stat 7840cf0...HEAD
git diff 7840cf0...HEAD -- . ':(exclude)docs/superpowers/specs/*' ':(exclude)docs/superpowers/plans/*'
```

确认 `.integration/` 和 `dist/` 未被纳入 commit，任务开始前已有 `.claude/worktrees/*` 未改动。

- [ ] **Step 6: 运行 Opus 1M xhigh diff-only review**

review scope 只允许 `7840cf0...HEAD` 的新 diff 和它直接调用的代码，不重新扫描已完成范围，不读取旧 spec/plan/TDD。按 correctness、provider contract、performance、package/release safety 四个互不重叠维度并行 review；每个 finding 再由独立 verifier 复现或反驳。

- [ ] **Step 7: 对 confirmed review findings 执行 TDD 修复**

每个 confirmed finding：先添加最小 failing test并运行 RED，再改根因并运行 GREEN。无法复现、符合既有契约、只能改 CPA core 或纯 hardening 的 finding 记录为排除，不改代码。

- [ ] **Step 8: 重新运行所有 verification**

```bash
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1
make test
make vet
make race
make integration
make build
make package VERSION=v0.2.5
git diff --check
```

Expected：全部 PASS。

---

### Task 8: 合并 main、发布 v0.2.5 并验证远端资产

**Files:**
- No source edits unless verification exposes a confirmed defect

- [ ] **Step 1: 确认 branch 和版本基线**

```bash
git status --short
git tag --list v0.2.5
git rev-parse main
git rev-parse origin/main
```

Expected：工作树干净，v0.2.5 不存在，main 和 origin/main 仍以任务基线 `7840cf0` 为祖先。

- [ ] **Step 2: 使用 finishing-a-development-branch 流程合并本地 main**

先同步 remote refs；若 origin/main 没有意外新提交，将开发 branch 以 non-destructive merge 合并到本地 main。不得 reset 或删除任务开始前 worktrees。

- [ ] **Step 3: 在 local main 重新验证关键 suite**

```bash
make test
make vet
make race
make integration
```

Expected：全部 PASS。

- [ ] **Step 4: 创建并 push patch tag**

```bash
git tag -a v0.2.5 -m "Censorship v0.2.5"
git push origin main
git push origin v0.2.5
```

- [ ] **Step 5: 观察 tag workflow**

使用 `gh run list` 找到由 v0.2.5 tag 触发的 `.github/workflows/build.yml` run，并用 `gh run watch --exit-status` 等待完成。若 API 或网络中断，恢复同一 run 的观察，不创建重复 tag/release。

- [ ] **Step 6: 验证 published release state 和资产集合**

```bash
gh release view v0.2.5 --json isDraft,isPrerelease,tagName,assets
```

Expected：`isDraft=false`、`isPrerelease=false`、tagName 为 v0.2.5，assets 精确包含 7 个 ZIP、7 个 `.zip.sha256` 和 `checksums.txt`。

- [ ] **Step 7: 下载并验证每个远端资产**

下载到新的系统临时目录；对每个 `*.zip.sha256` 和 aggregate `checksums.txt` 分别执行 `sha256sum --check`。比较 aggregate 行集合与 7 个 individual checksum 行集合完全相同，并确认每个 hash 为 lowercase 64 hex、两个空格和 archive basename。检查每个 ZIP 只含正确平台 library basename和可选 `LICENSE`，不含绝对路径或 parent traversal。

- [ ] **Step 8: 清理临时状态**

终止本次 Python REPL PID 9908；删除本次 overlay、fetch chunk 和 reproduction 临时目录；删除 memory `current-full-functional-audit.md` 及 `MEMORY.md` 对应索引行。保留任务开始前已有 `.claude/worktrees/*`。

- [ ] **Step 9: 最终状态检查**

```bash
git status --short
git log --oneline --decorate -8
gh release view v0.2.5 --json url,isDraft,assets
```

Expected：local main 与 origin/main 一致、工作树干净、tag 和 release 指向本次 merge commit、所有资产验证通过。
