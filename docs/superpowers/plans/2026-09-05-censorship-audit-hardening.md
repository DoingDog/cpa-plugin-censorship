# censorship 插件审计 hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不修改 CLIProxyAPI 的前提下，修复已确认的 selector、JSON 深度、C ABI、release packaging 和 integration 稳定性缺陷，补强独立测试 oracle，并用数据决定是否保留 exact rewrite 全 miss 优化。

**Architecture:** 保留现有 `gjson` raw span 和显式五协议 selector。known format 在 selector 进入 duplicate walker 前增加一次原始 JSON nesting 检查；Interactions object-level system text 复用现有 `scanTextPart`。C ABI 只增加长度边界检查和安全 output copy。release aggregator 与 direct packager 共享版本校验，Makefile/cross workflow 把 normalized version 传给 direct mode。性能候选只在 case-sensitive rewrite 的规则快照中增加预编译 `byteMatcher`，无初始命中时跳过不会产生 cascade 的逐规则重写。

**Tech Stack:** Go 1.26、CGO C ABI、`github.com/tidwall/gjson`、`gopkg.in/yaml.v3`、Go `encoding/json`/`strings`/`unicode` 标准库、Gorilla WebSocket integration harness、GitHub Actions artifact packager。

**Spec:** `docs/superpowers/specs/2026-09-05-censorship-audit-hardening-design.md`

## Global Constraints

- 只修改插件仓库；禁止读取、修改或提交 `.integration/cpa` 下与插件加载/运行无关的 CPA 源码。
- 所有生产代码保持 `package main`；不增加 provider factory、通用递归 JSON walker、content hash cache、conversation state 或新依赖。
- 保留五种 `SourceFormat` 的既有 selector 合同、hard exclusions、rule-major 处理顺序、span 外 byte 保持和 request-only capability。
- JSON nesting 深度定义为同时打开的 object/array container 数量，包含顶层 object；`1024` 合法，`1025` 返回 `errInvalidRequest`，公开 code 仍为 `censorship_invalid_request`。
- `scope.roles: []` 的 `transformRequest` 早退、`schema_version` 向后兼容协商、PR 七平台 build 策略、临时端口理论争用均不改。
- C ABI 传给 `C.GoBytes` 的长度必须先通过 `uint64` 到 `int32` 的 checked conversion；AfterAuth 不复制 request；host/plugin buffer ownership 不变。
- normalized release version 只允许单个 ASCII 文件名组件；dist mode、Makefile `package-platform` 和 cross workflow direct mode 都必须在创建 output path 前使用同一校验。
- TDD 顺序固定为 RED -> 观察失败 -> 最小 GREEN -> 定向测试 -> `go test ./...`；每个任务完成后才提交。
- 审计、plan 和最终 review 使用 Opus；编码代理使用 Sonnet；不得让两个代理编辑同一文件。
- 所有 shell 命令从仓库根目录运行，commit 不使用 `--no-verify`。

## 文件地图与所有权

| 文件 | 本轮责任 | 所属任务 |
|---|---|---|
| `selectors.go` | nesting scan、selector 入口边界 | Task 2 |
| `selectors_interactions.go` | system instruction direct text predicate | Task 1 |
| `selectors_interactions_test.go` | Interactions RED/GREEN 回归 | Task 1 |
| `selectors_role_gate_test.go` | 深度边界与 string brace 回归 | Task 2 |
| `abi_cgo.go` | checked C length、response 初始化、output copy | Task 3 |
| `abi_cgo_test.go` | 不分配大 buffer 的长度 helper 测试 | Task 3 |
| `.github/scripts/package-release.go` | version validator 与 direct mode 校验 | Task 4 |
| `.github/scripts/package-release_test.go` | version table、CLI、Makefile/workflow contract | Task 4 |
| `Makefile` | direct package 传 `-version` | Task 4 |
| `.github/workflows/build.yml` | 两个 cross direct package 传 `-version` | Task 4 |
| `integration/websocket_test.go` | completion read deadline helper 与 timeout test | Task 5 |
| `fuzz_test.go` | 独立 protocol/folded/external-byte oracle | Task 6 |
| `matcher_test.go` | 脱离生产 matcher/canonicalizer 的 folded expected | Task 6 |
| `transform_test.go` | exact block 多 span ordering | Task 6 |
| `config.go` | exact rewrite matcher snapshot（仅性能候选通过时） | Task 7 |
| `transform.go` | exact rewrite preflight（仅性能候选通过时） | Task 7 |
| `benchmark_test.go` | sink、fixture preflight、depth和rewrite benchmark | Task 7 |
| `README.md` | 新增深度、ABI、release/direct packaging 限制 | Task 8 |
| `docs/superpowers/tdd/2026-09-05-censorship-audit-hardening.tdd.md` | RED/GREEN/benchmark evidence | Task 8 |

并行关系：

- Wave 1：Task 1、Task 2、Task 3、Task 4、Task 5 可并行，文件不重叠。
- Wave 2：Task 6 在 Wave 1 完成后执行，独占所有 oracle 测试文件，避免 `fuzz_test.go` 与其他任务冲突。
- Wave 3：Task 7 在 Task 6 完成后执行，独占 `config.go`、`transform.go`、`benchmark_test.go`，因为性能候选需要用修正后的独立 differential test 验证。
- Wave 4：Task 8 文档与最终 review；之后执行完整 verification。

---

### Task 1：修复 Interactions system instruction hard exclusion

**Files:**
- Modify: `selectors_interactions.go:5-27`
- Test: `selectors_interactions_test.go`

**Interfaces:**
- Consumes: `scanTextPart(part gjson.Result, requireTextType bool)`、`appendStringSpan`。
- Produces: object-level `system_instruction` 和 `systemInstruction` 的 direct `.text` 只在协议允许时产生 system span；`parts[]` 的逐 part 语义不变。

- [ ] **Step 1: 写 RED 测试**

在 `selectors_interactions_test.go` 增加一个 table test，使用公开 `interceptRPC`，不要直接调用 collector：

```go
func TestInteractionsSystemInstructionObjectTextHonorsMachineExclusions(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system]\n")
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unknown image type",
			body: `{"system_instruction":{"type":"image","text":"SECRET"}}`,
			want: `{"system_instruction":{"type":"image","text":"SECRET"}}`,
		},
		{
			name: "text with inline data",
			body: `{"system_instruction":{"type":"text","text":"SECRET","inlineData":{"data":"SECRET"}}}`,
			want: `{"system_instruction":{"type":"text","text":"SECRET","inlineData":{"data":"SECRET"}}}`,
		},
		{
			name: "plain text object remains eligible",
			body: `{"system_instruction":{"type":"text","text":"SECRET"}}`,
			want: `{"system_instruction":{"type":"text","text":""}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := interceptRPC(t, "interactions", []byte(tc.body))
			if resp.Terminate || string(resp.Body) != tc.want {
				t.Fatalf("response = %#v, body = %s, want %s", resp, resp.Body, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: 运行 RED 测试**

运行：

```bash
go test . -run '^TestInteractionsSystemInstructionObjectTextHonorsMachineExclusions$' -count=1
```

预期：失败。当前 object-level `text` 在 `type=image` 和 `inlineData` 两个 case 中被 strip。

- [ ] **Step 3: 写最小 GREEN 实现**

在 `collectInteractions` 的 `systemInstruction.IsObject()` 分支，把直接 `appendStringSpan` 替换为共享 predicate，保留后面的 `parts` 遍历：

```go
case systemInstruction.IsObject():
	text, allowed := scanTextPart(systemInstruction, true)
	if allowed {
		appendStringSpan(spans, text, "system", roles)
	}
	parts := systemInstruction.Get("parts")
```

不要把 `parts` 作为 container 整体禁用。该修改只让 container direct `.text` 遵守 `type`、thought 和 machine discriminator。

- [ ] **Step 4: 运行定向和全套测试**

```bash
go test . -run '^TestInteractions' -count=1
go test ./...
```

预期：新增反例通过，既有 system parts、camelCase fallback 和 item content tests 全部通过。

- [ ] **Step 5: 提交**

```bash
git add selectors_interactions.go selectors_interactions_test.go
git commit -m "fix: exclude machine interactions system text"
```

---

### Task 2：增加 JSON nesting 资源边界

**Files:**
- Modify: `selectors.go:43-79`
- Test: `selectors_role_gate_test.go`

**Interfaces:**
- Consumes: 已有 `gjson.ValidBytes`、`gjson.ParseBytes` 和 `errInvalidRequest`。
- Produces: `const maxJSONNestingDepth = 1024`、`jsonNestingWithin(body []byte, maxDepth int) bool`，并在 duplicate walker/selector recursion 前拒绝超限 body。

- [ ] **Step 1: 写 RED 测试**

在 `selectors_role_gate_test.go` 增加只构造 bytes 的 helper 和边界测试：

```go
func nestedObjectBody(depth int) []byte {
	body := []byte(`0`)
	for i := 0; i < depth; i++ {
		body = append([]byte(`{"a":`), append(body, '}')...)
	}
	return body
}

func TestSelectorRejectsJSONBeyondConfiguredNestingDepth(t *testing.T) {
	roles := scopeSet{"user": {}}
	if _, err := selectTextSpans(nestedObjectBody(1024), "openai", roles); err != nil {
		t.Fatalf("depth 1024 error = %v, want nil", err)
	}
	if _, err := selectTextSpans(nestedObjectBody(1025), "openai", roles); err != errInvalidRequest {
		t.Fatalf("depth 1025 error = %v, want %v", err, errInvalidRequest)
	}
}

func TestJSONNestingScanIgnoresBracketsInsideStrings(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"{ [ ] } \\"quoted\\""}]}`)
	if _, err := selectTextSpans(body, "openai", scopeSet{"user": {}}); err != nil {
		t.Fatalf("string brackets caused error = %v", err)
	}
}
```

- [ ] **Step 2: 运行 RED 测试**

```bash
go test . -run '^(TestSelectorRejectsJSONBeyondConfiguredNestingDepth|TestJSONNestingScanIgnoresBracketsInsideStrings)$' -count=1
```

预期：失败或无法编译，因为 production 没有 depth check 和 `maxJSONNestingDepth`。

- [ ] **Step 3: 写最小 GREEN 实现**

在 `selectors.go` 增加：

```go
const maxJSONNestingDepth = 1024

func jsonNestingWithin(body []byte, maxDepth int) bool {
	depth := 0
	inString := false
	escaped := false
	for _, b := range body {
		if inString {
			switch {
			case escaped:
				escaped = false
			case b == '\\':
				escaped = true
			case b == '"':
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > maxDepth {
				return false
			}
		case '}', ']':
			depth--
		}
	}
	return depth == 0 && !inString
}
```

在 `selectTextSpans` 中保持 `gjson.ValidBytes` 和 root object 判断，然后在 `hasDuplicateJSONMembers(root)` 前调用：

```go
root := gjson.ParseBytes(body)
if !root.IsObject() || !jsonNestingWithin(body, maxJSONNestingDepth) || hasDuplicateJSONMembers(root) {
	return nil, errInvalidRequest
}
```

这一步只统计 string 外的 container；JSON 合法性已经由 `gjson.ValidBytes` 负责，所以不在 scan 中重写语法校验。

- [ ] **Step 4: 运行边界、现有 duplicate 和全套测试**

```bash
go test . -run '^(TestSelectorRejectsJSONBeyondConfiguredNestingDepth|TestJSONNestingScanIgnoresBracketsInsideStrings|TestDuplicate)' -count=1
go test ./...
```

预期：1024 层通过，1025 层返回 `errInvalidRequest`，escaped key、nested duplicate 和 JSON string 中的 duplicate 既有行为不变。

- [ ] **Step 5: 提交**

```bash
git add selectors.go selectors_role_gate_test.go
git commit -m "fix: bound JSON selector nesting"
```

---

### Task 3：保护 CGO ABI 长度转换和 output copy

**Files:**
- Modify: `abi_cgo.go:56-126`
- Test: `abi_cgo_test.go`

**Interfaces:**
- Consumes: existing `cliproxy_host_api`/`cliproxy_buffer` callbacks and `shouldCopyPluginRequest`。
- Produces: `checkedCIntLength(length uint64) (int, bool)` and safe ABI error paths without changing exported C signatures。

- [ ] **Step 1: 写 RED boundary test**

在 `abi_cgo_test.go` 增加：

```go
func TestCheckedCIntLength(t *testing.T) {
	max := uint64(^uint32(0) >> 1)
	for _, tc := range []struct {
		name string
		in   uint64
		want int
		ok   bool
	}{
		{name: "zero", in: 0, want: 0, ok: true},
		{name: "max", in: max, want: int(max), ok: true},
		{name: "overflow", in: max + 1, want: 0, ok: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := checkedCIntLength(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("checkedCIntLength(%d) = %d, %t; want %d, %t", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}
```

- [ ] **Step 2: 运行 RED 测试**

```bash
go test . -run '^TestCheckedCIntLength$' -count=1
```

预期：编译失败，因为 helper 不存在。

- [ ] **Step 3: 写最小 GREEN 实现**

在 `abi_cgo.go` 的 Go 部分加入：

```go
func checkedCIntLength(length uint64) (int, bool) {
	const max = uint64(^uint32(0) >> 1)
	if length > max {
		return 0, false
	}
	return int(length), true
}
```

在 host callback closure 中，`response.ptr` 的 free defer 必须先注册，再检查长度：

```go
if response.ptr != nil {
	defer C.cliproxy_host_free(host, response.ptr, response.len)
}
if response.ptr == nil || response.len == 0 {
	return nil, nil
}
	responseLen, ok := checkedCIntLength(uint64(response.len))
	if !ok {
		return nil, fmt.Errorf("host callback response too large: %d", uint64(response.len))
	}
return C.GoBytes(response.ptr, C.int(responseLen)), nil
```

在 `cliproxyPluginCall` 中，确认 `response != nil` 后立刻清零 `response.ptr`/`response.len`；然后对需要复制的 request length 做 checked conversion，超限直接返回非零 rc，不调用 `C.GoBytes`。output allocation 后用：

```go
copy(unsafe.Slice((*byte)(ptr), len(payload)), payload)
```

替换 `(*[1 << 30]byte)(ptr)`，避免固定 1 GiB slice ceiling。所有 early return 都不能留下未初始化的 response buffer。

- [ ] **Step 4: 运行 ABI 定向、race、vet 和动态库编译**

```bash
go test . -run '^(TestCheckedCIntLength|TestShouldCopyPluginRequest)$' -count=1
go test -race . -run '^(TestCheckedCIntLength|TestShouldCopyPluginRequest)$' -count=1
go vet ./...
go build -trimpath -buildmode=c-shared -o dist/test/censorship.dll .
```

再用搜索确认不存在未检查的 conversion：

```bash
rg 'C\.GoBytes|C\.int\(response\.len\)|C\.int\(requestLen\)|1 << 30' abi_cgo.go
```

预期：两处 `C.GoBytes` 都先经过 checked helper，固定数组表达式不存在；已有 dynamic ABI integration 路径仍可构建。

- [ ] **Step 5: 提交**

```bash
git add abi_cgo.go abi_cgo_test.go
git commit -m "fix: bound cgo ABI buffer lengths"
```

---

### Task 4：拒绝不安全 release version 并保护 direct packaging

**Files:**
- Modify: `.github/scripts/package-release.go:23-53,113-133`
- Test: `.github/scripts/package-release_test.go`
- Modify: `Makefile:42-43`
- Modify: `.github/workflows/build.yml:127-132,173-178`

**Interfaces:**
- Consumes: existing `normalizeReleaseVersion`, `resolveVersion`, direct `-library/-archive/-checksum` mode。
- Produces: `validateReleaseVersion(version string) error`，aggregate mode 和显式 `-version` direct mode 共用安全校验；Makefile/cross workflow 传入 normalized version。

- [ ] **Step 1: 写 RED table 和 CLI tests**

在 packager tests 增加：

```go
func TestValidateReleaseVersion(t *testing.T) {
	valid := []string{"1.2.3", "0.1.0-rc.1+build", "0.0.0-dev", "v1"}
	for _, version := range valid {
		if err := validateReleaseVersion(normalizeReleaseVersion(version)); err != nil {
			t.Errorf("validateReleaseVersion(%q) = %v", version, err)
		}
	}
	invalid := []string{"", ".", "..", "foo/bar", `foo\bar`, "foo bar", "foo\x00bar", "foo:bar", "foo*bar", "foo?bar", "foo<bar", "foo>bar", "foo|bar"}
	for _, version := range invalid {
		if err := validateReleaseVersion(normalizeReleaseVersion(version)); err == nil {
			t.Errorf("validateReleaseVersion(%q) = nil", version)
		}
	}
}
```

在既有 CLI fixture 中增加 `TestPackagerRejectsUnsafeVersionBeforeCreatingOutput`：创建一个有效 `dist/linux_amd64/censorship.so`，运行 `go run package-release.go -version foo/bar -dist <dist> -out <out>`，断言非零退出码，`out` 不存在或为空，且不存在任何 `censorship_foo` 路径。另用 direct mode 传 `-version foo/bar -library ... -archive ... -checksum ...`，断言同样失败且 archive/checksum 不存在。

更新 workflow/Makefile contract test，断言两个 cross package command 和 `package-platform` command 都包含 `-version`，但不改变既有 `buildCondition`。

- [ ] **Step 2: 运行 RED 测试**

```bash
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -run '^(TestValidateReleaseVersion|TestPackagerRejectsUnsafeVersionBeforeCreatingOutput|TestWorkflow|TestMakeBuild)' -count=1
```

预期：编译失败或非法 version 当前被接受并产生嵌套 output。

- [ ] **Step 3: 写最小 GREEN 实现**

增加 ASCII validator：

```go
func validateReleaseVersion(version string) error {
	if version == "" {
		return fmt.Errorf("release version is empty")
	}
	for i := 0; i < len(version); i++ {
		c := version[i]
		if i == 0 {
			if !isASCIIAlpha(c) && !isASCIIDigit(c) {
				return fmt.Errorf("release version %q is not a safe filename component", version)
			}
			continue
		}
		if !isASCIIAlpha(c) && !isASCIIDigit(c) && c != '.' && c != '_' && c != '+' && c != '-' {
			return fmt.Errorf("release version %q is not a safe filename component", version)
		}
	}
	return nil
}
```

`resolveVersion` 对 flag、`VERSION` 和 exact tag 分别 normalize 后调用 validator；不要把合法 `v1` 的现有“移除一个小写前导 v”语义改成大小写不敏感。`run` 的 direct mode 在 `versionFlag != ""` 时先 normalize/validate，再调用 `packageLibrary`；未提供 `-version` 的通用 direct mode保持兼容，因为 archive path 是调用方完整传入的。

修改 Makefile：

```make
package-platform: build-platform
	$(GO) run ./.github/scripts/package-release.go -version "$(NORMALIZED_VERSION)" -library "$(LIBRARY)" -archive "$(ARCHIVE)" -checksum "$(CHECKSUM)"
```

修改两个 cross workflow direct command，在 `-library` 前加入：

```yaml
-version "${VERSION}"
```

不改变 job trigger、matrix、upload path 或 release metadata 逻辑。

- [ ] **Step 4: 运行 packager/Makefile/workflow 全套测试**

```bash
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1
go test .github/scripts/package-release.go .github/scripts/package-release_test.go -race -count=1
go vet ./...
```

预期：普通 SemVer、`vX`、direct archive/checksum、Makefile contract 和 workflow contract 全部通过；非法 version 在任何 output path 创建前失败。

- [ ] **Step 5: 提交**

```bash
git add .github/scripts/package-release.go .github/scripts/package-release_test.go Makefile .github/workflows/build.yml
git commit -m "fix: validate release artifact versions"
```

---

### Task 5：为 WebSocket completion reader 增加 deadline

**Files:**
- Modify: `integration/websocket_test.go:106-131`

**Interfaces:**
- Consumes: existing Gorilla `*websocket.Conn` and `wsMessage`。
- Produces: `readUntilCompletedWithTimeout(conn *websocket.Conn, timeout time.Duration) ([]wsMessage, error)`；生产测试 wrapper 固定使用 20 秒。

- [ ] **Step 1: 写 RED timeout test**

将现有 helper 拆成可返回 error 的函数，并先添加 test server fixture。测试 server 使用 `httptest.NewServer`、`websocket.Upgrader` 完成 upgrade 后不发送任何 frame；client 用短 timeout 调 helper：

```go
func TestReadUntilCompletedReturnsOnDeadline(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	start := time.Now()
	_, err = readUntilCompletedWithTimeout(conn, 10*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "i/o timeout") {
		t.Fatalf("error = %v, want read timeout", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("timeout helper took %v", time.Since(start))
	}
}
```

如平台包装 error 文本不同，测试改为 `net.Error` 的 `Timeout() == true`，不要比较完整错误字符串。

- [ ] **Step 2: 运行 RED 测试**

```bash
go test -tags=integration ./integration -run '^TestReadUntilCompletedReturnsOnDeadline$' -count=1
```

预期：编译失败，因为 helper 当前没有 timeout-return 版本；不要运行完整 integration 等待 20 秒。

- [ ] **Step 3: 写最小 GREEN 实现**

增加常量和 helper：

```go
const websocketCompletionReadTimeout = 20 * time.Second

func readUntilCompleted(t *testing.T, conn *websocket.Conn) []wsMessage {
	t.Helper()
	messages, err := readUntilCompletedWithTimeout(conn, websocketCompletionReadTimeout)
	if err != nil {
		t.Fatal(err)
	}
	return messages
}

func readUntilCompletedWithTimeout(conn *websocket.Conn, timeout time.Duration) ([]wsMessage, error) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	defer conn.SetReadDeadline(time.Time{})
	var messages []wsMessage
	for {
		opcode, payload, err := conn.ReadMessage()
		if err != nil {
			return nil, err
		}
		messages = append(messages, wsMessage{Opcode: opcode, Payload: bytes.Clone(payload)})
		if gjson.GetBytes(payload, "type").String() == "response.completed" {
			return messages, nil
		}
	}
}
```

- [ ] **Step 4: 运行 integration 定向测试和 helper test**

```bash
go test -tags=integration ./integration -run '^TestReadUntilCompletedReturnsOnDeadline$' -count=1
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -count=1
go run ./.github/scripts/integration-runner.go
```

预期：短 fixture 在毫秒级返回 timeout，既有 completion/block/output tests 的读取语义不变；runner 负责在固定 CPA checkout 中执行完整 integration。

- [ ] **Step 5: 提交**

```bash
git add integration/websocket_test.go
git commit -m "test: bound websocket completion wait"
```

---

### Task 6：修正独立 fuzz/test oracle 和 exact multi-span 覆盖

**Files:**
- Modify: `fuzz_test.go`
- Modify: `matcher_test.go`
- Modify: `transform_test.go`

**Interfaces:**
- Consumes: production `transformResult` 作为被测输入，但 oracle 只使用 test-local `strings`、`unicode`、`encoding/json` 和固定协议枚举。
- Produces: block 最早 role、span 外 bytes、folded independent expected、exact multi-span rule-major differential tests。

- [ ] **Step 1: 写 RED oracle tests**

在 `fuzz_test.go` 增加 block role regression：

```go
func TestProtocolOracleRequiresEarliestBlockRole(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"SECRET"},{"role":"developer","content":"SECRET"}]}`)
	got := transformResult{Blocked: &blockMatch{Term: "SECRET", Role: "developer"}}
	if err := checkProtocolResult("openai", body, modeBlock, false, got); err == nil {
		t.Fatal("oracle accepted later eligible role")
	}
}
```

增加固定 fixture 的 span-gap mutation test，使用能在当前 oracle 中复现共同失效的具体输入：

```go
func TestProtocolOracleRejectsOutsideSpanByteMutation(t *testing.T) {
	body := []byte(" \n{\"messages\":[{\"role\":\"user\",\"content\":\"SECRET\"}],\"n\":1e+03,\"unknown\":\"KEEP\"} \n")
	want := bytes.Replace(body, []byte(`"SECRET"`), []byte(`""`), 1)
	mutated := bytes.Replace(want, []byte("1e+03"), []byte("1000"), 1)
	got := transformResult{Body: mutated}
	if err := checkProtocolResult("openai", body, modeStrip, false, got); err == nil {
		t.Fatal("oracle accepted a number spelling mutation outside the eligible span")
	}
}
```

实现时让 `checkProtocolResult` 使用 test-local raw token mask：只对协议枚举出来且 text 等于当前 rule 的 selected raw token 区间允许变更；prefix、gap、suffix 和所有非 selected raw token 必须逐 byte 相等。mask helper 只能接收该 fixture 的明确 selected raw token ranges，不调用 `selectTextSpans` 或 `rebuildBody`。

在 `transform_test.go` 增加 exact matcher 跨 span 表格测试：构造 256 条规则、3 个带不同 role 的 spans，覆盖 rule 0/127/255、同 rule 出现在多个 span、无命中；expected 严格执行：

```go
wantRule, wantRole, wantMatched := -1, "", false
for ruleIndex, rule := range rules {
	for _, span := range spans {
		if strings.Contains(span.Text, rule.Term) {
			wantRule, wantRole, wantMatched = ruleIndex, span.Role, true
			break
		}
	}
	if wantMatched { break }
}
```

调用 `matchExactBlock(spans, cfg)`，不调用 `containsRule`、`matchExactBlock` 以外的 production expected helper。

在 `matcher_test.go` 把 `TestFoldMatcherMatchesRuleMajorOracle` 的 expected 改为 test-local `independentFoldContains(text, term)`：从每个 UTF-8 rune boundary 建窗口，要求窗口 scalar 数与 term 相同，逐 rune 用 test-local `unicode.SimpleFold` cycle 比较。`forcedFoldedKMPRule` 同样调用 test-local `independentFoldClassRune`，不调用 production `foldClassRune`。

- [ ] **Step 2: 运行 RED 测试**

```bash
go test . -run '^(TestProtocolOracleRequiresEarliestBlockRole|TestProtocolOracleRejectsOutsideSpanByteMutation|TestExactBlockAcrossSpans|TestFoldMatcherMatchesRuleMajorOracle)$' -count=1
```

预期：role mutation 和 span-gap mutation 被当前 oracle 错误接受，独立 folded expected 至少证明测试不再调用 production helper；exact table test 可先通过，但它必须在 production candidate 改动前固定为回归 oracle。

- [ ] **Step 3: 写最小 GREEN oracle 实现**

`checkProtocolResult` 的 block 分支不再构造 `matchingRoles` 集合作为正确性判断。对当前单个 YAML `SECRET`，通过 oracle spans 的 raw document order 找第一个匹配 span：

```go
wantRole := ""
for _, span := range before {
	if oracleContains(span.Text, "SECRET", fold) {
		wantRole = span.Role
		break
	}
}
if got.Blocked == nil || got.Blocked.Role != wantRole {
	return fmt.Errorf("blocked role %q, want earliest role %q", got.Blocked.Role, wantRole)
}
```

对 span 外 bytes，增加 test-local `oracleRawStringTokens`/mask 逻辑：只对协议枚举出来且 text 等于当前 rule 的 selected raw token区间允许变更；prefix、gap、suffix 和所有非 selected raw token 必须逐 byte 相等。不要把整个 body `json.Marshal` 后比较，因为那会错误允许 whitespace、member order 和 number spelling 改变。

- [ ] **Step 4: 运行独立 oracle、fuzz seed 和全套测试**

```bash
go test . -run '^(TestProtocolOracle|TestExactBlockAcrossSpans|TestFoldMatcher)' -count=1
go test . -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=1x
go test . -run '^$' -fuzz '^FuzzProtocolTransform$' -fuzztime=1x
go test ./...
```

预期：注入的晚 role、gap mutation、共同 folded helper mutation 都能被测试拒绝；五种协议的既有 seed 仍通过。`maxFuzzJSONDepth` 仍可作为 fuzz 输入筛选，但深度边界由 Task 2 的生产测试覆盖，不把 fuzz skip 当资源安全证据。

- [ ] **Step 5: 提交**

```bash
git add fuzz_test.go matcher_test.go transform_test.go
git commit -m "test: strengthen independent censorship oracles"
```

---

### Task 7：用 benchmark 验证并实现 exact rewrite all-miss preflight

**Files:**
- Modify: `config.go`
- Modify: `transform.go`
- Modify: `benchmark_test.go`
- Test: `config_test.go`, `transform_test.go`（仅需要新增 regression 时）

**Interfaces:**
- Consumes: `byteMatcher`, immutable `configSnapshot` compiler、Task 6 的 exact multi-span oracle。
- Produces: rewrite snapshot 中唯一的 `ExactRewriteMatcher *byteMatcher`（candidate 保留时）、`useExactRewritePreflight(ruleCount, textBytes) bool`、`textSpan.SkipExactRewrite`，以及可靠的 sink/preflight benchmark。block snapshot 继续只使用既有 `ExactBlockMatcher`，不在同一 snapshot 中重复构造 matcher。

- [ ] **Step 1: 先修 benchmark harness 并加入 baseline matrix**

`BenchmarkTransformMatrix`、`BenchmarkTransformScenarios` 和 `BenchmarkBeforeAuthRPCEnvelope` 在 timer 外先执行一次 transform/RPC 并验证 `Invalid`、`Blocked`、Body/Envelope。增加一个普通 test，固定 benchmark fixture 的公开 RPC 结果合同：

```go
func TestBeforeAuthBenchmarkFixtureResult(t *testing.T) {
	old := loadedSnapshot()
	t.Cleanup(func() { installSnapshot(old) })
	installSnapshot(mustBenchmarkConfig(t, "mode: block\nwords: [NEVER-MATCH]\n"))
	request, err := json.Marshal(pluginapi.RequestInterceptRequest{
		RequestID: "benchmark",
		SourceFormat: "openai",
		Body: benchmarkScenarioBody("plain", "", 1000, ""),
	})
	if err != nil { t.Fatal(err) }
	raw, err := handleMethod(pluginabi.MethodRequestInterceptBefore, request)
	if err != nil { t.Fatal(err) }
	var envelope pluginabi.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.OK {
		t.Fatalf("benchmark envelope = %s, %v", raw, err)
	}
}
```

timer 内把返回值写入已有 package sink：

```go
benchmarkTransformSink, benchmarkErrorSink = transformRequest(body, "openai", cfg)
```

`BenchmarkBeforeAuthRPCEnvelope` 先 `json.Unmarshal` 到 `pluginabi.Envelope`，确认 `OK` 且 result 是预期 no-op response，再在 timer 内写：

```go
benchmarkEnvelopeSink, benchmarkErrorSink = handleMethod(pluginabi.MethodRequestInterceptBefore, request)
```

`BenchmarkConcurrentSnapshotSwap` 在 `RunParallel` 内使用局部 result 并 `runtime.KeepAlive(result)`，不要让多个 goroutine 写同一个非原子全局 sink；reconfigure goroutine 的错误仍通过 channel 传回。

扩展 `BenchmarkExactRewriteStrategies`：规则数量为 32、128、256、1024，text 为 4 KiB、64 KiB、1 MiB，覆盖 all-miss、first、middle、last、sparse、dense、cascade，strip 和 obfs 都要有 none/hit 样本。每个 sub-benchmark timer 前验证 baseline output；expected 用 test-local ordered `strings.ReplaceAll` rule loop，不调用 production transform 生成 expected。

- [ ] **Step 2: 运行 harness RED/基线 benchmark**

```bash
go test . -run '^TestBeforeAuthBenchmarkFixtureResult$' -count=1
go test -run '^$' -bench '^Benchmark(TransformMatrix|TransformScenarios|BeforeAuthRPCEnvelope|ExactRewriteStrategies)' -benchmem -count=5 .
```

预期：新的 fixture assertion 在实现前至少能捕捉故意错误 envelope；baseline matrix 通过，并记录 all-miss 与 hit 的 `ns/op`、`B/op`、`allocs/op` 到 `docs/superpowers/tdd/2026-09-05-censorship-audit-hardening.tdd.md`。不要把单台机器的绝对时间写成单元测试阈值。

- [ ] **Step 3: 写 candidate RED regression**

增加 threshold RED 和 cascade regression：

```go
func TestExactRewritePreflightThreshold(t *testing.T) {
	if !useExactRewritePreflight(128, 16<<10) {
		t.Fatal("128 rules and 16 KiB did not enable exact rewrite preflight")
	}
	if useExactRewritePreflight(127, 16<<10) || useExactRewritePreflight(128, (16<<10)-1) {
		t.Fatal("preflight enabled below configured thresholds")
	}
}

func TestExactRewritePreflightKeepsOrderedCascade(t *testing.T) {
	cfg := mustConfig(t, "mode: strip\nwords: [X, ab]\n")
	body := []byte(`{"messages":[{"role":"user","content":"aXb"}]}`)
	got, err := transformRequest(body, "openai", cfg)
	if err != nil || string(got.Body) != `{"messages":[{"role":"user","content":""}]}` {
		t.Fatalf("transformRequest() = %#v, %v", got, err)
	}
	miss := mustConfig(t, "mode: strip\nwords: [X, ab]\n")
	unchanged, err := transformRequest([]byte(`{"messages":[{"role":"user","content":"plain"}]}`), "openai", miss)
	if err != nil || len(unchanged.Body) != 0 {
		t.Fatalf("all-miss transform = %#v, %v; want no replacement", unchanged, err)
	}
}
```

这保证“初始没有任意规则命中”才可跳过，且不会错误跳过删除后产生的 cascade，因为有初始 `X` 命中时走完整规则循环。

- [ ] **Step 4: 运行 candidate RED**

```bash
go test . -run '^(TestExactRewritePreflightThreshold|TestExactRewritePreflightKeepsOrderedCascade)$' -count=1
```

预期：`TestExactRewritePreflightThreshold` 在 `useExactRewritePreflight` 尚未定义时编译失败；该 RED 固定阈值合同。实现 helper 后，cascade test 仍应保持通过，证明 candidate 不会跳过会产生后续 cascade 的 span。

- [ ] **Step 5: 写最小 candidate GREEN**

在 `textSpan` 增加 `SkipExactRewrite bool`；在 `configSnapshot` 增加 `ExactRewriteMatcher *byteMatcher`。定义：

```go
const (
	exactRewritePreflightMinRules = 128
	exactRewritePreflightMinTextBytes = 16 << 10
)

func useExactRewritePreflight(ruleCount, textBytes int) bool {
	return ruleCount >= exactRewritePreflightMinRules && textBytes >= exactRewritePreflightMinTextBytes
}
```

`compileSnapshot` 只在 `!IgnoreCase` 且 mode 为 `strip`/`obfs` 且规则数达到阈值时构造：

```go
cfg.ExactRewriteMatcher = newByteMatcher(cfg.Rules, 0)
```

`applyMode` 在 strip/obfs rule loop 前按 span 做一次：

```go
if !cfg.IgnoreCase && cfg.ExactRewriteMatcher != nil {
	for i := range spans {
		spans[i].SkipExactRewrite = useExactRewritePreflight(len(cfg.Rules), len(spans[i].Text))
		if spans[i].SkipExactRewrite {
			_, matched := cfg.ExactRewriteMatcher.match(spans[i].Text)
			spans[i].SkipExactRewrite = !matched
		}
	}
}
```

在每个 rule loop 中先跳过 `SkipExactRewrite` span。规则一旦在原始 span 命中，candidate 不跳过，仍执行现有 `strings.ReplaceAll` rule-major cascade。folded path、block path 和 no-op body rebuild 不改变。

- [ ] **Step 6: 运行 candidate differential、全套测试和 benchmark holdout**

```bash
go test . -run '^(TestExactRewritePreflightKeepsOrderedCascade|TestExactBlockAcrossSpans)' -count=1
go test ./...
go test -race ./...
go test -run '^$' -bench '^BenchmarkExactRewriteStrategies' -benchmem -count=5 .
```

保留 candidate 的硬门槛：

1. 128/256/1024 rules 的 all-miss holdout 中，代表性 `ns/op` 至少比 baseline 改善 20%。
2. first/middle/last/sparse/dense/cascade 的 hit holdout 没有稳定超过 10% 回退。
3. `B/op` 和 `allocs/op` 不高于 baseline。
4. Task 6 的 independent expected、cascade test、fuzz seed 全部通过。

把 baseline/candidate 的实际输出和是否满足门槛写入 TDD 文档。若门槛不满足，删除 `ExactRewriteMatcher`、`SkipExactRewrite`、preflight 常量和 candidate benchmark，只保留可靠 benchmark harness；不得为了保留优化放宽门槛。

- [ ] **Step 7: 提交**

```bash
git add config.go transform.go config_test.go transform_test.go benchmark_test.go
git commit -m "perf: skip exact rewrite scans on total misses"
```

---

### Task 8：同步文档、TDD 证据并做最终审阅

**Files:**
- Modify: `README.md`
- Create/Modify: `docs/superpowers/tdd/2026-09-05-censorship-audit-hardening.tdd.md`

**Interfaces:**
- Consumes: 已通过的生产行为、packager grammar、integration timeout、benchmark raw output。
- Produces: 用户可操作的限制说明和可审计的 RED/GREEN/verification 记录。

- [ ] **Step 1: 写 README 回归检查**

在 README 的 hard exclusions/accepted limits/release sections 增加以下事实，不声称覆盖 CPA 非插件入口：

```plaintext
- known SourceFormat 的 JSON object nesting 最大为 1024 层（包含顶层 object）；超过后返回 censorship_invalid_request。
- C ABI 对超出 `C.int` 的 request length 返回非零 ABI rc；超大的 host callback response 返回 plugin error；两者都不尝试执行截断转换。
- release version 去除一个前导小写 v 后必须是安全 ASCII 文件名组件；direct cross packaging 也传入同一 version 校验。
- Responses WebSocket integration test 在 20 秒未收到 response.completed 时失败，不会无限等待。
```

保留 README 已有的 raw ingress、prewarm、Realtime、fail-open、Home mode 和未知 SourceFormat 限制。

- [ ] **Step 2: 写 TDD evidence**

TDD 文档按以下结构记录每个 slice 的实际结果：

```markdown
# censorship audit hardening TDD evidence

## Slice 1 ...
### RED
命令、退出码、关键失败输出。
### GREEN
命令、通过结果、行为断言。

## Performance
baseline/candidate command、GOOS/GOARCH、ns/op、B/op、allocs/op、是否达到保留门槛。

## Verification blockers
只记录可在仓库外最小复现的环境故障；不得把未运行的命令写成通过。
```

把 Task 7 的 candidate 删除或保留决定明确写出。

- [ ] **Step 3: 运行文档与 diff 检查**

```bash
git diff --check
git status --short
```

预期：README 和 TDD 只描述已经实现并验证的行为，不出现待办标记、未决阈值或“完整覆盖 CPA”表述。

- [ ] **Step 4: 提交**

```bash
git add README.md docs/superpowers/tdd/2026-09-05-censorship-audit-hardening.tdd.md
git commit -m "docs: record censorship hardening limits"
```

---

### Task 9：最终 cross-module review 与 verification

**Files:**
- Review current diff since `6cd6cec`，不新增未验证功能。

**Interfaces:**
- Consumes: Task 1-8 的 committed changes 和 TDD evidence。
- Produces: 只报告已复现的残留问题；最终命令输出作为完成依据。

- [ ] **Step 1: 使用 Opus review 变更**

启动一个只读 Opus reviewer，范围限定为本轮变更文件，按以下顺序检查：

1. `selectTextSpans` 是否在 duplicate/selector recursion 前执行 depth bound，1024/1025 边界是否与 README 一致。
2. Interactions direct text 是否只改 predicate，parts/item/steps 未扩大选择范围。
3. 每个 `C.GoBytes` 前是否 checked，host buffer 是否在 error 前释放，plugin response 是否先清零，`unsafe.Slice` 长度是否来自已分配 payload。
4. version validator 是否覆盖 aggregate、Makefile direct 和两个 cross direct 入口，非法版本是否在输出目录创建前失败。
5. WebSocket deadline 是否只改变 test helper，正常 completion/error message 顺序不变。
6. independent oracle 是否没有调用被测 matcher/selector/canonicalizer，exact multi-span expected 是否 rule-major/document-minor。
7. candidate benchmark 是否使用 sink、timer 前 fixture assertion，candidate 是否满足 TDD 记录的硬门槛。

只报告可由代码或命令复现的问题，不报告 workflow 策略偏好。

- [ ] **Step 2: 运行 gofmt 和完整单元 gate**

```bash
gofmt -w abi_cgo.go abi_cgo_test.go selectors.go selectors_interactions.go selectors_interactions_test.go selectors_role_gate_test.go fuzz_test.go matcher_test.go transform_test.go config.go transform.go benchmark_test.go .github/scripts/package-release.go .github/scripts/package-release_test.go integration/websocket_test.go
go test ./...
go test -race ./...
go vet ./...
```

- [ ] **Step 3: 运行 fuzz seed、相关 benchmark 和 packager tests**

```bash
go test . -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=1x
go test . -run '^$' -fuzz '^FuzzProtocolTransform$' -fuzztime=1x
go test -run '^$' -bench '^Benchmark(ExactRewriteStrategies|TransformMatrix|TransformScenarios|BeforeAuthRPCEnvelope|JSONNestingDepth)' -benchmem -count=5 .
go test -run '^$' -bench . -benchmem ./...
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
```

- [ ] **Step 4: 运行固定 integration runner**

```bash
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
go run ./.github/scripts/integration-runner.go
```

如果需要性能 integration evidence，再单独运行：

```bash
$env:BENCH="1"; go run ./.github/scripts/integration-runner.go
```

不直接在仓库根目录读取或测试 `.integration/cpa` 源码；runner 的 checkout/copy 行为是唯一集成入口。

- [ ] **Step 5: 复核状态和完成标准**

```bash
git diff --check
git status --short
git log -12 --oneline --decorate
```

完成前必须确认：所有 intended changes 已 commit；无 generated artifact 被误加入；README、spec、plan、TDD evidence 与实际行为一致；没有任何 CLIProxyAPI 源码变更。若任一 gate 失败，记录完整命令和可复现错误，不宣称完成。
