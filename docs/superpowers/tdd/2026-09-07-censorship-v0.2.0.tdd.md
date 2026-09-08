# Censorship v0.2.0 TDD Record

## Baseline, 2026-09-07

- `make integration`: PASS; the pinned CPA HTTP, SSE, reload, Responses WebSocket, and ABI scenarios completed.
- `go vet ./...`: PASS.
- `make test`: FAIL only at `TestDocumentationListsConfigAndLimits`; `RELEASE_NOTES.md` lacks the exact expected configuration block.
- `make race`: FAIL only at the same documentation assertion.
- Baseline benchmark output: `%TEMP%/censorship-v020-before-bench.txt`.

Every task below records its literal RED, GREEN, and REFACTOR command, exit code, and decisive output.

## Task 2: Upgrade CPA SDK and Integration Host Atomically, 2026-09-07

### RED

- `go test . -run '^TestSupportedPluginSchema$' -count=1`
  - exit code: `1`
  - decisive output: `main_test.go:21: plugin schema = 4, want 5`
- `go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -run '^TestPinnedCPARevision$' -count=1`
  - exit code: `1`
  - decisive output: `integration-runner_test.go:44: cpaSHA = "81e1b5374f99c212f196f34956eeed964a46b8fa", want "c76dfd4e0edabab9000628b1560ab8ab379eadb8"`

### GREEN

- `go get github.com/router-for-me/CLIProxyAPI/v7@v7.2.152`
  - exit code: `0`
  - decisive output: `go: upgraded github.com/router-for-me/CLIProxyAPI/v7 v7.2.147-0.20260831020448-81e1b5374f99 => v7.2.152`
- `go mod download github.com/gorilla/websocket@v1.5.3`
  - exit code: `0`
  - decisive output: no output.
- `go mod tidy`
  - exit code: `0`
  - decisive output: no output.
- `go test . -run '^TestSupportedPluginSchema$' -count=1`
  - exit code: `0`
  - decisive output: `ok   github.com/DoingDog/cpa-plugin-censorship  0.050s`
- `go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -run '^TestPinnedCPARevision$' -count=1`
  - exit code: `0`
  - decisive output: `ok   command-line-arguments  0.034s`
- `go list -m -json github.com/router-for-me/CLIProxyAPI/v7`
  - exit code: `0`
  - decisive output: `"Version": "v7.2.152"`, `"Sum": "h1:FkvGzpOCvuDGswaOyoVfbY5Ua7OlP/wMXw3agiNMUQI="`, and `"GoModSum": "h1:lTHwMAGajc1wKGQiRtDvYbwV0FWsM7sy+N0ZU5/gxJQ="`.
- `go mod verify`
  - exit code: `0`
  - decisive output: `all modules verified`
- `go test . -run '^(TestPluginRegistration|TestHandlePluginRegister|TestHandlePluginReconfigure)' -count=1`
  - exit code: `0`
  - decisive output: `ok   github.com/DoingDog/cpa-plugin-censorship  0.054s [no tests to run]`
- `go vet ./...`
  - exit code: `0`
  - decisive output: no output.

### REFACTOR

- No production refactor was needed.
- `git diff --check`
  - exit code: `0`
  - decisive output: no whitespace errors; Git printed only CRLF conversion warnings.

## Task 3: Compile Per-Action Rules and Publish Object-Only Metadata, 2026-09-07

### RED

The original Task 3A RED output was lost when its first agent process crashed. After recovery, the current tests were run against the old production files in isolated temporary checkouts; these are real reconstructed RED runs, not inferred output.

- `go test . -run '^(TestParseConfigYAMLWordsObject|TestParseConfigYAMLWordsObjectIgnoresMappingOrder|TestParseConfigYAMLRejectsInvalidWordsObject|TestParseConfigYAMLLegacyWordsUseFinalMode|TestParseConfigYAMLValidatesOnlyEffectiveObfsTerms)$' -count=1` with `config.go` restored from `7eeab1b`
  - exit code: `1`
  - decisive output: build failed because the old `configSnapshot` had no `BlockEnd` or `StripEnd` fields referenced by the new tests.
- `go test . -run '^(TestRegistrationDeclaresOnlyRequestInterceptor|TestRegistrationExposesEditableConfigFields|TestRegistrationUsesBuildVersion)$' -count=1` with `main.go` restored from `b069d08`
  - exit code: `1`
  - decisive output: registration still had `Logo:""`, five fields including global `mode`, and `words` type `array`; `config field count = 5, want 4`.

### GREEN

- `go test . -run '^(TestParseConfig|TestCompileSnapshot|TestSyntheticSnapshots|TestConcurrentReconfigure)' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 0.640s`.
- `go test -race . -run '^TestConcurrentReconfigureObservesOnlyWholeSnapshot$' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 4.564s`.
- `go test . -run '^(TestRegistration|TestHandlePlugin|TestConcurrentReconfigure)' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 1.041s`.
- `go test -race . -run '^(TestRegistration|TestHandlePlugin|TestConcurrentReconfigure)' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 5.403s`.
- `go vet ./...`
  - exit code: `0`
  - decisive output: no output.

### REFACTOR

- Kept one flat `Rules` slice with `BlockEnd` and `StripEnd`; retained only the temporary `Mode` compatibility bridge needed until Task 4.
- `gofmt -w config.go config_test.go main.go main_test.go`
  - exit code: `0`
  - decisive output: no output.
- `git diff --check`
  - exit code: `0`
  - decisive output: no whitespace errors; Git printed only LF/CRLF conversion warnings during the config-file check.

## Task 4: Execute Mixed Rules with Existing Matchers, 2026-09-08

### RED

The implementing agents stalled before returning their terminal records. The final tests were therefore paired with pre-task production/oracle code in isolated temporary checkouts to obtain literal RED evidence.

- `go test . -run '^(Test.*Mixed|Test.*Block.*Boundary|Test.*CrossPhase|Test.*Legacy|Test.*Matcher)' -count=1` with current core tests and production code from `ee64e13`
  - exit code: `1`
  - decisive output: `matcher_test.go:27:42: too many arguments in call to newFoldMatcher; have ([]compiledRule, number), want ([]compiledRule)`; package build failed.
- `go test . -run '^TestOracleApplyMixedRules$' -count=1` with the new mixed test and oracle from `247083f`
  - exit code: `1`
  - decisive output: production returned rewritten `"x​y"` with no block, while the old oracle kept `"ABxy"` unchanged and reported a block.

### GREEN

- `go test . -run '^(Test.*Mixed|Test.*Block|Test.*Rewrite|Test.*Legacy|Test.*Matcher)' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 0.090s`.
- `go test -race . -run '^(Test.*Mixed|Test.*Block|Test.*Rewrite|Test.*Legacy|Test.*Matcher|TestConcurrentReconfigure)' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 5.633s`.
- `go test . -count=1`
  - exit code: `1`
  - decisive output: only the known `TestDocumentationListsConfigAndLimits` legacy documentation fixture failed; all code tests passed.
- `go test . -run '^TestOracleApplyMixedRules$' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 0.074s`.
- `go test . -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=30s`
  - exit code: `0`
  - decisive output: 2,401,467 executions from 522 baseline cases with no failure; `ok` in `30.440s`.
- `go test . -run '^$' -bench 'Mixed|FoldedRewriteStrategies|ExactBlock' -benchmem -count=3`
  - exit code: `0`
  - decisive output: PASS; `BenchmarkMixedTransformScenario` used `264 B/op` and `7 allocs/op`; complete run finished in `307.843s`.
- `go test . -run '^TestOracleApplyMixedRules$' -count=1 && go test . -run '^$' -bench '^BenchmarkMixedTransformScenario$' -benchtime=1x -benchmem`
  - exit code: `0`
  - decisive output: valid two-rune obfs follow-up passed; benchmark retained `264 B/op` and `7 allocs/op`.

### REFACTOR

- Kept rule-major rewrite loops, the existing matcher nodes/KMP algorithms, and one flat rule slice. `RewriteMatcher` is boolean-only and block matchers are bounded to the block prefix.
- `gofmt -d matcher.go matcher_test.go transform.go transform_test.go`
  - exit code: `0`
  - decisive output: no output.
- `go vet ./...`
  - exit code: `0`
  - decisive output: no output.
- `git diff --check`
  - exit code: `0`
  - decisive output: no whitespace errors; only LF/CRLF conversion warnings.

## Task 5: Harden Shared JSON and Span Selection, 2026-09-08

### RED

The implementing agent committed before returning its terminal record. Current tests were paired with `selectors.go` from `d4d2a40` in an isolated temporary checkout for literal RED evidence.

- `go test . -run '^(TestSelectorRejectsDeepJSONBeforeGJSONValidation|TestSelectorRejectsInvalidUTF8|TestSelectorSkipsEmptyStringSpans|TestScanTextPartIgnoresNullMachineDiscriminators|TestSelectorHasEnabledRoleByFormat)$' -count=1`
  - exit code: `1`
  - decisive output: `openai-response` rejected `tool`; invalid UTF-8 returned nil error; 2048 decoded empty strings produced 2048 spans; and `functionCall:null` returned `allowed=false`.

### GREEN

- `go test . -run '^(TestSelectorRejectsDeepJSONBeforeGJSONValidation|TestSelectorRejectsInvalidUTF8|TestSelectorSkipsEmptyStringSpans|TestScanTextPartIgnoresNullMachineDiscriminators|TestSelectorHasEnabledRoleByFormat)$' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 0.129s`.
- `go test -race . -run '^(TestSelectorRejectsDeepJSONBeforeGJSONValidation|TestSelectorSkipsEmptyStringSpans)$' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 2.181s`.
- `go test . -run '^$' -bench 'Empty|DisabledRoleSelectors|TextPartScanning' -benchmem -count=3`
  - exit code: `0`
  - decisive output: PASS; `BenchmarkEmptyStringSpans` measured about `615-686 µs/op`, one allocation, and zero selected spans for 1000 empty contents.
- `go test . -run '^TestSelectorSkipsEmptyStringSpans$' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 0.080s`.
- `go test . -count=1`
  - exit code: `1`
  - decisive output: the known Task 15 documentation fixture and five provider-owned stale null-discriminator expectations failed; Tasks 8 and 9 explicitly own those test reversals. No other test failed.

### REFACTOR

- Removed the unused `bytes` import and reused the existing nesting, span, role, and benchmark helpers.
- `gofmt -w selectors.go selectors_role_gate_test.go benchmark_test.go`
  - exit code: `0`
  - decisive output: no output.
- `go vet ./...`
  - exit code: `0`
  - decisive output: no output.
- `git diff --check`
  - exit code: `0`
  - decisive output: no whitespace errors; only LF/CRLF conversion warnings.

## Task 6: Cover OpenAI Tool, Refusal, Function, and Result Text, 2026-09-08

### RED

The implementing agents were interrupted repeatedly by API EOF failures, so the final tests were paired with `selectors_openai.go` from `c20b99a` in a temporary detached worktree for literal RED evidence.

- `go test . -run '^(TestOpenAI.*Tool|TestOpenAI.*Refusal|TestOpenAI.*Function|TestOpenAIResponses.*Output|TestOpenAIRoleGate)' -count=1`
  - exit code: `1`
  - decisive output: `TestOpenAIChatToolTextArrayRoleGate/tool_enabled`, `TestOpenAIChatRefusalRoleGate/assistant_enabled`, and `TestOpenAIChatFunctionRoleGate/tool_enabled` returned an empty body; all seven `TestOpenAIResponsesOutputRoleGate/*/tool_enabled` rows returned an empty body; `TestOpenAIRoleGateUsesCanonicalToolRole/function_message_enabled` and `/function_output_enabled` returned zero spans instead of one.

### GREEN

- `go test . -run '^(TestOpenAI.*Tool|TestOpenAI.*Refusal|TestOpenAI.*Function|TestOpenAIResponses.*Output|TestOpenAIRoleGate)' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 0.115s`.
- `go test . -run '^(TestOpenAI|TestSelector.*OpenAI)' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 1.530s`.
- `go test -race . -run '^(TestOpenAI|TestSelector.*OpenAI)' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 15.604s`.
- Role-off integration cases assert an empty response `Body`, which is the existing interceptor representation for zero selected spans; enabled cases compare the complete rebuilt body and keep IDs, names, calls, arguments, URLs, media, files, status, and unknown output types byte-identical.

### REFACTOR

- Kept explicit Chat roles and exact Responses result-type switches; no default recursion or new shared helper was added.
- `gofmt -w selectors_openai.go selectors_openai_test.go selectors_role_gate_test.go`
  - exit code: `0`
  - decisive output: no output.
- `go vet .`
  - exit code: `0`
  - decisive output: no output.
- `git diff --check HEAD^ HEAD`
  - exit code: `0`
  - decisive output: no whitespace errors.

## Task 7: Cover Claude Result Text, 2026-09-08

### RED

- `go test . -run '^TestClaude' -count=1`
  - exit code: `1`
  - decisive output: `TestClaudeResultText/string_tool_result`, `/direct_user_search_result`, `/nested_tool_search_result`, `/direct_user_text_document`, and `/nested_tool_text_document` returned an empty body; `TestClaudeResultTextPreservesMachineFields` reported `body differs outside selected Claude result text`.

### GREEN

- `gofmt -w selectors_claude.go selectors_claude_test.go && go test . -run '^TestClaude' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 0.074s`.
- `go test -race . -run '^TestClaude' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 1.133s`.

### REFACTOR

- Direct search/document blocks use canonical `user`; nested `tool_result` content uses canonical `tool`; arbitrary string recursion remains excluded.
- `gofmt -d selectors_claude.go selectors_claude_test.go`
  - exit code: `0`
  - decisive output: no output.
- `go vet .`
  - exit code: `0`
  - decisive output: no output.
- `git diff --check`
  - exit code: `0`
  - decisive output: no whitespace errors; only LF/CRLF conversion warnings.

## Task 8: Handle Omitted Gemini Roles, 2026-09-08

### RED

- `go test . -run '^(TestGemini.*Role|TestGemini.*Null|TestGemini.*Disabled)' -count=1`
  - exit code: `1`
  - decisive output: `TestGeminiOmittedRoleFormsAlternateCanonicalRoles/null_first_only`, `/null_after_explicit_user`, `/empty_first_only`, and `/empty_after_explicit_user` returned an empty body.
- `go test . -run '^$' -bench '^BenchmarkGeminiSystemOnly' -benchmem -count=3`
  - exit code: `0`
  - decisive output: `17786572 ns/op`, `15789600 ns/op`, and `21565395 ns/op`; approximately `729176-729267 B/op` and `3 allocs/op`.

### GREEN

- `gofmt -w selectors_gemini.go selectors_gemini_test.go benchmark_test.go && go test . -run '^(TestGemini|TestScanTextPart)' -count=1`
  - exit code: `1`
  - decisive output: three provider-owned Task 9 fixtures in `TestScanTextPartPreservesProtocolRules` still expected null `functionCall` and `function_call` fields to be present; the focused Gemini tests below passed.
- `go test . -run '^TestGemini' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 0.128s`.
- `go test -race . -run '^TestGemini' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 1.375s`.
- `go test . -run '^$' -bench '^BenchmarkGeminiSystemOnly' -benchmem -count=5`
  - exit code: `0`
  - decisive output: `10164182`, `10884959`, `10903388`, `10188495`, and `9933489 ns/op`; approximately `729177-729236 B/op` and `3 allocs/op`.

### REFACTOR

- Missing, null, and empty Gemini roles share the existing omitted-role alternation; system-only and tool-only scopes return before traversing `contents`.
- `gofmt -w selectors_gemini.go selectors_gemini_test.go benchmark_test.go && go vet . && git diff --check`
  - exit code: `0`
  - decisive output: no vet or whitespace errors; only LF/CRLF conversion warnings.

## Task 9: Select Direct Interactions Text, 2026-09-08

### RED

- `go test . -run '^TestInteractions' -count=1`
  - exit code: `1`
  - decisive output: `TestInteractionsDirectTextContent/direct_object` and `/direct_array_element` returned an empty body; `TestInteractionsDirectTextContentKeepsProtocolExclusions` also returned an empty body.

### GREEN

- `go test . -run '^TestInteractions' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 0.088s`.
- `go test -race . -run '^TestInteractions' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 1.146s`.

### REFACTOR

- Direct Interactions objects and array items select only exact `type:"text"` text; media, tool, thought, control fields, and unknown nesting remain excluded.
- `gofmt -d selectors_interactions.go selectors_interactions_test.go`
  - exit code: `0`
  - decisive output: no output.
- `go vet .`
  - exit code: `0`
  - decisive output: no output.
- `git diff --check`
  - exit code: `0`
  - decisive output: no whitespace errors; only LF/CRLF conversion warnings.

## Task 10: Integrate Provider Selectors, 2026-09-08

### Integration gate

- Cherry-picked Task 6 `49687d9` as `afb4181`, Task 7 `a5b6f0d` as `dfc6187`, Task 8 `0a7a502` as `0fd7b96`, and Task 9 `08ab48d` as `9bdc857`, in numeric order and without conflicts.
- First `go test . -run '^(TestSelector|TestOpenAI|TestClaude|TestGemini|TestInteractions|TestScanTextPart|TestInvalidUTF8|TestEmptyString)' -count=1`
  - exit code: `1`
  - decisive output: only `TestScanTextPartPreservesProtocolRules/escaped_machine_key` failed with `allowed = true, want false`.
- First `go test -race . -run '^(TestOpenAI|TestClaude|TestGemini|TestInteractions)' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 16.419s`.
- The escaped key decodes to `functionCall`; its null value must follow the same null-as-absent contract as the unescaped key. Commit `409f3e9` changes only that stale expectation.

### Final GREEN

- `go test . -run '^(TestSelector|TestOpenAI|TestClaude|TestGemini|TestInteractions|TestScanTextPart|TestInvalidUTF8|TestEmptyString)' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 1.475s`.
- `go test -race . -run '^(TestOpenAI|TestClaude|TestGemini|TestInteractions)' -count=1`
  - exit code: `0`
  - decisive output: `ok github.com/DoingDog/cpa-plugin-censorship 15.970s`.

## Task 11: Tagged Integration and ABI Benchmark Seam, 2026-09-08

### Raw request ownership call graph

- `cliproxyPluginCall` receives a host-owned `(request, requestLen)` pair and calls `handleMethod` synchronously. Before the Task 11 change, every method except after-auth reaches `handleMethod` through `C.GoBytes`; after-auth already passes a nil slice.
- `plugin_register` routes through `handlePluginRegister` -> `parseLifecycleSnapshot` -> `json.Unmarshal`. The decoded `ConfigYAML` and the compiled snapshot are separate Go-owned values; neither stores the raw ABI slice.
- `plugin_reconfigure` follows the same parsing path. Its failure log contains only `err.Error()`; it does not retain raw input.
- `request_intercept_before_auth` routes through `interceptBeforeAuth` -> `json.Unmarshal` -> `transformRequest`. `RequestInterceptRequest.Body` is decoded from the envelope into a separate value, and result bytes are JSON-encoded into a new response before `cliproxyPluginCall` copies them to plugin-owned `malloc` memory.
- `request_intercept_after_auth` ignores its raw request and returns an empty response envelope.
- Unknown methods ignore their raw request and construct an error envelope from the method string only.

No branch stores the raw slice, aliases it into a surviving string, closes over it, starts asynchronous work with it, or returns it as output. The host may therefore retain ownership only until this synchronous ABI call returns.

### Tagged-build and runner seam GREEN

- `go test -mod=readonly -tags=integration ./integration -run '^TestReadUntilCompletedReturnsOnDeadline$' -count=1` passed: `ok github.com/DoingDog/cpa-plugin-censorship/integration 0.163s`.
- `go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -run 'BenchmarkPlacement|CopyIntegration' -count=1` passed: `ok command-line-arguments 0.164s`.
- The runner copies `abi_benchmark_test.go` into CPA's `integration/censorshipplugin` package and invokes only `BenchmarkDynamicABIRequestInterceptors` with `-benchmem`.

### Borrowing safety checks

- `go test . -run '^(TestBorrowedRequest|TestChecked.*Length|Test.*ABI|TestHandleMethod.*Ownership)' -count=1` passed: `ok github.com/DoingDog/cpa-plugin-censorship 0.118s`.
- `go test -race . -run '^(TestBorrowedRequest|Test.*ABI)' -count=1` passed: `ok github.com/DoingDog/cpa-plugin-censorship 1.199s`.
- The tests cover zero length, one byte, nil pointer with nonzero length, Go-`int` overflow, matching and nonmatching before-auth responses, and post-return poisoning. The ownership assertion compares the complete success envelope byte-for-byte and rejects output containing the `0xa5` poison byte.

### Benchmark gate and fallback

- Baseline copied-input `go run ./.github/scripts/integration-runner.go -bench-abi` results, `ns/op; B/op; allocs/op`:
  - before-auth, 1 KiB: `182346; 11163; 31`; after-auth, 1 KiB: `64738; 6293; 29`.
  - before-auth, 1 MiB: `59886155; 10223071; 38`; after-auth, 1 MiB: `3580756; 4225242; 32`.
  - before-auth, 20 MiB: `1278921100; 223730848; 43`; after-auth, 20 MiB: `50177007; 100989973; 35`.
- Borrowed-input candidate results:
  - before-auth, 1 KiB: `149485; 11150; 31`; after-auth, 1 KiB: `66118; 6290; 29`.
  - before-auth, 1 MiB: `59098655; 10082715; 38`; after-auth, 1 MiB: `3351113; 4315911; 32`.
  - before-auth, 20 MiB: `1116459600; 223730864; 43`; after-auth, 20 MiB: `39955114; 88055905; 33`.
- The candidate did not remove an input-sized allocation: before-auth allocation counts were unchanged, and 20 MiB before-auth bytes rose by 16. The benchmark gate failed, so `cliproxyPluginCall` retains its input-side `C.GoBytes` path; after-auth still performs no input read. The borrowing helper and coverage remain for the documented boundary contract, but no no-copy performance claim is made.

### Final verification

- The restored-copy benchmark passed. Its 20 MiB before-auth result was `1202176600 ns/op; 223730848 B/op; 43 allocs/op`, consistent with retaining the copy.
- `go vet ./...` passed.
- `go test ./...` and `go test -race ./...` both stopped at the pre-existing `TestDocumentationListsConfigAndLimits` contract failure: unchanged `README.md` and `RELEASE_NOTES.md` lack the expected `plugins.configs.censorship` block. Task 11 changes do not modify either file.


## Task 12: Harden CPA Integration Coverage, 2026-09-08

### RED

- `TestWatcherReloadLinearizesAtObservedSnapshotB` before the bounded handshake failed twice after about 23 seconds. Literal decisive output: `config watcher not observed, last status=200 body={"id":"chatcmpl-censorship-fixture"...}`.

### GREEN

- `$env:CPA_INTEGRATION_BIN = 'C:\Users\user\AppData\Local\Temp\cpa-c76.exe'; $env:CENSORSHIP_PLUGIN_DIR = Join-Path (Get-Location) '.integration\run\plugins\windows\amd64'; go test -mod=readonly -tags=integration ./integration -run '^TestWatcherReloadLinearizesAtObservedSnapshotB$' -count=1 -v`
  - exit code: `0`
  - literal output: `--- PASS: TestWatcherReloadLinearizesAtObservedSnapshotB (4.88s)` and `ok      github.com/DoingDog/cpa-plugin-censorship/integration  4.930s`.
- The full worker command `go test -mod=readonly -tags=integration ./integration -count=1 -v` passed. Literal output: `--- PASS: TestWatcherReloadLinearizesAtObservedSnapshotB (4.86s)`, `--- PASS: TestHTTPAndSSEOutputTraceUnaffected (21.35s)`, `--- PASS: TestResponsesWebSocketOutputMessagesUnaffected (15.95s)`, and `ok      github.com/DoingDog/cpa-plugin-censorship/integration  83.917s`.
- `go test -race -mod=readonly -tags=integration ./integration -run '^(TestResolve|TestRevision|TestReadiness|Test.*Timeout)' -count=1`
  - exit code: `0`
  - literal output: `ok      github.com/DoingDog/cpa-plugin-censorship/integration  16.114s`.
- Worker commit: source `a3845852031be421bf9c2f1ef876937524003e82`; integrated as `db6ec74da3870cbb3432fdb676be35efa1fa8159` with subject `test: harden CPA integration coverage`.

### REFACTOR

- The watcher writes the same path and probes a B-only marker under a bounded deadline. Windows cleanup waits for CPA process exit before returning.

## Task 13: Reuse CPA Integration Checkout, 2026-09-08

### GREEN

- `go -C "C:/Users/user/Downloads/cpa-plugin-censorship" test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go`
  - exit code: `0`
  - literal report output: `Passed: command-line-arguments in 1.983s.`
- Source commits `8d43b7e3b4b4714a9246d435fd2f68cf0544392a` and `f81f1e397fd9353f241f344861cc56b8b444b3c4` were integrated as `4dd60496f0ed98359a0167c78633d39f5fb0c60a`, subject `perf: reuse CPA integration checkout`.

### REFACTOR

- The integration runner reuses the existing CPA checkout. No second checkout is created.

## Task 14: Harden Release Packaging, 2026-09-08

### GREEN

- `go -C "C:/Users/user/Downloads/cpa-plugin-censorship" test .github/scripts/package-release.go .github/scripts/package-release_test.go`
  - exit code: `0`
  - literal report output: `Passed: command-line-arguments in 8.113s.`
- Source commit `f99188837a1520e2c1221a30fc8291456c8197b4` was integrated as `ca7f15c76c10d2400155abb8b86352132803310b`, subject `build: harden release packaging`.

### REFACTOR

- Packaging validates normalized versions, produces matching checksums, and includes the repository `LICENSE` only when it exists.

## Tooling-wave combined tagged integration, 2026-09-08

- `$env:CPA_INTEGRATION_BIN = "C:/Users/user/Downloads/cpa-plugin-censorship/.integration/bin.exe"; $env:CENSORSHIP_PLUGIN_DIR = "C:/Users/user/Downloads/cpa-plugin-censorship/.integration/run/plugins"; go -C "C:/Users/user/Downloads/cpa-plugin-censorship/.integration/cpa" test -tags=integration -count=1 -v ./integration/censorshipplugin`
  - exit code: `0`
  - literal report output: `Passed: github.com/router-for-me/CLIProxyAPI/v7/integration/censorshipplugin in 88.571s.`
- `git -C "C:/Users/user/Downloads/cpa-plugin-censorship" diff --check`
  - exit code: `0`
  - literal report output: no output.

## Task 15: Integrate Tooling Wave and Publish Accurate Documentation, 2026-09-08

### RED

- `go -C "C:/Users/user/Downloads/cpa-plugin-censorship" test . -run '^TestDocumentationListsConfigAndLimits$' -count=1`
  - exit code: `1`
  - decisive output: both `README.md` and `RELEASE_NOTES.md` were missing the canonical Object `words` configuration block. The updated contract also reported missing Object-only panel, legacy-mode compatibility, fixed phase order, provider-path, ABI, packaging, and integration documentation.

### GREEN

- `go -C "C:/Users/user/Downloads/cpa-plugin-censorship" test . -run '^TestDocumentationListsConfigAndLimits$' -count=1`
  - exit code: `0`
  - literal output: `ok      github.com/DoingDog/cpa-plugin-censorship  0.064s`.
- `go -C "C:/Users/user/Downloads/cpa-plugin-censorship" test -race . -run '^TestDocumentationListsConfigAndLimits$' -count=1`
  - exit code: `0`
  - literal output: `ok      github.com/DoingDog/cpa-plugin-censorship  1.102s`.
- `go -C "C:/Users/user/Downloads/cpa-plugin-censorship" test . -run '^(TestDocumentation|TestRegistration|TestParseConfig)' -count=1`
  - exit code: `0`
  - literal output: `ok      github.com/DoingDog/cpa-plugin-censorship  0.060s`.
- `git -C "C:/Users/user/Downloads/cpa-plugin-censorship" diff --check`
  - exit code: `0`
  - output: only Git LF-to-CRLF warnings for `README.md`, `RELEASE_NOTES.md`, and `main_test.go`; no whitespace error.

### REFACTOR

- Replaced the stale list-plus-mode primary fixture with the canonical Object block in the contract test and both documents. The global `mode` example is retained only in the explicit handwritten-YAML compatibility subsection.

## Task 16: Run the Full Verification Matrix and Package v0.2.0, 2026-09-08

### Partition verdicts

| Partition | Evidence | Verdict |
| --- | --- | --- |
| Static, unit, race, and vet | `task-16-static-round-2.md` | PASS |
| Integration and checkout reuse | `task-16-integration-round-2.md` | PASS |
| Fuzz repair and post-repair fuzz runs | `task-16-repair-report.md` | PASS |
| Historical benchmark baseline | `task-16-baseline-report.md` | PASS |
| Paired performance gate | `task-16-performance-adjudication.md` | PASS, adjudication `NO_REGRESSION` |
| Windows package, checksum, and ZIP | Package evidence consolidated in `task-16-performance-adjudication.md` | PASS |

All counted verification commands ran in `C:/Users/user/Downloads/cpa-plugin-censorship`. The static partition confirmed branch `feat/v0.2.0`. The paired performance evidence used baseline `da6e91262f1faaf82cacc5425eaf4a7f1b911042` and current `448c85200f33391811edab4237b6884a4ad105c8`.

### Root, formatting, static, unit, race, and vet commands

- `Set-Location -LiteralPath 'C:/Users/user/Downloads/cpa-plugin-censorship'; (Get-Location).Path; git rev-parse --show-toplevel`
  - exit code: `0`
  - decisive output: both path checks returned exact root `C:/Users/user/Downloads/cpa-plugin-censorship`.
- `git rev-parse --show-toplevel`
  - exit code: `0`
  - decisive output: `C:/Users/user/Downloads/cpa-plugin-censorship`.
- `git branch --show-current`
  - exit code: `0`
  - decisive output: `feat/v0.2.0`.
- `git status --short` before formatting
  - exit code: `0`
  - decisive output: 20 retained tracked `M` paths and untracked `task-12-full.log`.
- `gofmt -w` over every path returned by `git ls-files -- '*.go'`
  - exit code: `0`
  - decisive output: no formatter output.
- `git diff --check` immediately after formatting
  - exit code: `0`
  - decisive output: no whitespace errors; Git emitted LF-to-CRLF warnings only.
- `git status --short` after formatting
  - exit code: `0`
  - decisive output: the same 20 retained tracked `M` paths and untracked log.
- `make test`
  - exit code: `0`
  - decisive output: `go test ./...` passed; `integration` reported `[no test files]`.
- `go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -count=1`
  - exit code: `0`
  - decisive output: `ok command-line-arguments 1.675s`.
- `go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1`
  - exit code: `0`
  - decisive output: `ok command-line-arguments 6.839s`.
- `go -C C:/Users/user/Downloads/cpa-plugin-censorship test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -run '^TestPinnedCPARevision$' -count=1`
  - exit code: `0`
  - decisive output: `ok command-line-arguments 0.039s`.
- `make race`
  - exit code: `0`
  - decisive output: `go test -race ./...` passed; `integration` reported `[no test files]`.
- `make vet`
  - exit code: `0`
  - decisive output: `go vet ./...` passed.
- Final `git diff --check`
  - exit code: `0`
  - decisive output: no whitespace errors; Git emitted LF-to-CRLF warnings only.
- Final `git status --short` and `git diff --numstat`
  - exit code: `0`
  - decisive output: the same retained status and no textual numstat rows. The 20 tracked status entries predated round 2 and did not contain a gofmt-produced content diff, so none qualifies for staging as a formatter change.

The Task 2 lifecycle guard used `^TestPinnedCPARevision$` exactly once. No wrapper with `[no tests to run]` ran in static round 2. The partition did not run standalone integration, fuzz, benchmark, or package commands.

### Integration commands

| Order | Literal command | Exit code | Actual duration | Result |
| ---: | --- | ---: | ---: | --- |
| 1 | `make integration` | `0` | `00:01:38.6893060` | PASS |
| 2 | `go test -mod=readonly -tags=integration ./integration -count=1` | `0` | `00:01:24.2863915` | PASS |
| 3 | `make integration` | `0` | `00:01:34.4425159` | PASS |

Before command 2, `CPA_INTEGRATION_BIN` was set to `C:/Users/user/Downloads/cpa-plugin-censorship/.integration/bin.exe` and `CENSORSHIP_PLUGIN_DIR` was set to `C:/Users/user/Downloads/cpa-plugin-censorship/.integration/run/plugins`. An output-forwarding setup error happened before any `make` process started and is not a counted command.

The second `make integration` reused the existing checkout. `.git` remained the Git directory and CPA `HEAD` remained `448c85200f33391811edab4237b6884a4ad105c8` before and after. Its restricted VCS-token scan found zero matches. The 20 tracked status entries and `task-12-full.log` remained unchanged.

### Fuzz-oracle repair and fuzz commands

- `go -C C:/Users/user/Downloads/cpa-plugin-censorship test . -run '^TestProtocolOracleClassifiesInvalidUTF8AsInvalidRequest$' -count=1`
  - RED exit code: `1`; decisive output: `valid JSON object marked invalid`.
  - GREEN exit code: `0`; `validJSONObject` now requires `utf8.Valid(body)` as well as JSON validity.
- `go -C C:/Users/user/Downloads/cpa-plugin-censorship test . -run '^TestProtocolOracleTreatsEmptyGeminiRoleAsUser$' -count=1`
  - RED exit code: `1`; decisive output: `request without eligible match was rebuilt`.
  - GREEN exit code: `0`; both oracle Gemini walkers now treat absent, `null`, and empty roles like production.
- `go -C C:/Users/user/Downloads/cpa-plugin-censorship test . -run '^TestProtocolOracle(ClassifiesInvalidUTF8AsInvalidRequest|TreatsEmptyGeminiRoleAsUser)$' -count=1`
  - exit code: `0`.
- `go -C C:/Users/user/Downloads/cpa-plugin-censorship test . -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=30s`
  - exit code: `0`; PASS in `30.307s`.
- `go -C C:/Users/user/Downloads/cpa-plugin-censorship test . -run '^$' -fuzz '^FuzzProtocolTransform$' -fuzztime=30s`
  - exit code: `0`; PASS in `31.326s`.
- `go -C C:/Users/user/Downloads/cpa-plugin-censorship test . -run '^$' -fuzz '^FuzzRebuildBodyAgainstMarshalOracle$' -fuzztime=30s`
  - exit code: `0`; PASS in `31.180s`.
- `git -C C:/Users/user/Downloads/cpa-plugin-censorship diff --check`
  - exit code: `0`; no whitespace errors, with pre-existing LF-to-CRLF warnings only.

The generated failing corpora were deleted only after focused regressions covered them. Only `fuzz_test.go` was staged in repair commit `448c852 fix: repair v0.2.0 verification failure`.

### Historical baseline command

```powershell
$ErrorActionPreference = 'Stop'; $repo = 'C:\Users\user\Downloads\cpa-plugin-censorship'; $rawOutput = 'C:\Users\user\Downloads\cpa-plugin-censorship\.superpowers\sdd\2026-09-07-censorship-v0.2.0\censorship-v020-before-bench.txt'; $benchmarkPattern = '^(BenchmarkTransformMatrix|BenchmarkFoldStripDense|BenchmarkFoldObfuscateDense|BenchmarkFoldRootTransitions|BenchmarkFoldedRewriteStrategies|BenchmarkExactBlockStrategies)$'; $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ('cpa-plugin-censorship-task16-da6e912-clean-' + [guid]::NewGuid().ToString('N')); $archivePath = Join-Path $tempDir 'source.tar'; $resolved = (& git -C $repo rev-parse 'da6e912^{commit}'); $resolveExit = $LASTEXITCODE; if ($resolveExit -ne 0) { throw "git rev-parse failed with exit code $resolveExit" }; New-Item -ItemType Directory -Path $tempDir -ErrorAction Stop | Out-Null; $archiveExit = $null; $extractExit = $null; $benchmarkExit = $null; $cleanupExit = 0; try { & git -C $repo archive --format=tar da6e912 > $archivePath; $archiveExit = $LASTEXITCODE; if ($archiveExit -ne 0) { throw "git archive failed with exit code $archiveExit" }; & tar.exe -xf $archivePath -C $tempDir; $extractExit = $LASTEXITCODE; if ($extractExit -ne 0) { throw "tar extraction failed with exit code $extractExit" }; Remove-Item -LiteralPath $archivePath -Force -Confirm:$false; Push-Location -LiteralPath $tempDir; try { & go test . -run '^$' -bench $benchmarkPattern -benchmem -count=5 2>&1 | Tee-Object -FilePath $rawOutput; $benchmarkExit = $LASTEXITCODE } finally { Pop-Location } } finally { try { Remove-Item -LiteralPath $tempDir -Recurse -Force -Confirm:$false } catch { $cleanupExit = 1; Write-Error $_ } }; Write-Output "resolved_commit=$resolved"; Write-Output "temporary_directory=$tempDir"; Write-Output "archive_exit=$archiveExit"; Write-Output "extract_exit=$extractExit"; Write-Output "benchmark_exit=$benchmarkExit"; Write-Output "cleanup_exit=$cleanupExit"; if ($null -eq $benchmarkExit) { exit 1 }; if ($cleanupExit -ne 0) { exit $cleanupExit }; exit $benchmarkExit
```

- wrapper exit code: `0`.
- component exit codes: `archive_exit=0`, `extract_exit=0`, `benchmark_exit=0`, `cleanup_exit=0`.
- decisive output: `resolved_commit=da6e91262f1faaf82cacc5425eaf4a7f1b911042` and `PASS`.
- comparable benchmarks: `BenchmarkTransformMatrix`, `BenchmarkFoldStripDense`, `BenchmarkFoldObfuscateDense`, `BenchmarkFoldRootTransitions`, `BenchmarkFoldedRewriteStrategies`, and `BenchmarkExactBlockStrategies`, each with `-benchmem -count=5`.
- post-only benchmarks excluded from the historical run: `BenchmarkMixedTransformScenario`, `BenchmarkEmptyStringSpans`, and `BenchmarkGeminiSystemOnly`.
- the source was obtained with `git archive`; no `checkout`, `switch`, `reset`, `restore`, or `stash` command ran, and the temporary extraction directory was deleted.

### Paired benchmark commands and exits

Execution order was `baseline-1`, `current-1`, `current-2`, `baseline-2`. Every command below ran once in each output with exit vector `0/0/0/0`, `-cpu=16`, and three samples per output.

1. `go test . -run '^$' -bench '^BenchmarkExactBlockStrategies$/^impl=(baseline|production)$/^mode=block$/^rules=256$/^text=65536$/^pattern=literal$/^match=last$/^set=calibration$' -benchmem -count=3 -cpu=16`
2. `go test . -run '^$' -bench '^(BenchmarkFoldObfuscateDense|BenchmarkFoldStripDense)$' -benchmem -count=3 -cpu=16`
3. `go test . -run '^$' -bench '^BenchmarkFoldedRewriteStrategies$/^impl=(baseline|production)$/^mode=obfs$/^rules=1$/^text=65536$/^pattern=invalid-utf8-16scalars$/^match=sparse$/^set=holdout$' -benchmem -count=3 -cpu=16`
4. `go test . -run '^$' -bench '^BenchmarkFoldedRewriteStrategies$/^impl=(baseline|production)$/^mode=strip$/^rules=1$/^text=65536$/^pattern=ascii-16scalars$/^match=overlap$/^set=holdout$' -benchmem -count=3 -cpu=16`
5. `go test . -run '^$' -bench '^BenchmarkFoldedRewriteStrategies$/^impl=(baseline|kmp|production)$/^mode=strip$/^rules=1$/^text=16384$/^pattern=ascii-8scalars$/^match=none$/^set=calibration$' -benchmem -count=3 -cpu=16`
6. `go test . -run '^$' -bench '^BenchmarkFoldedRewriteStrategies$/^impl=(baseline|production)$/^mode=obfs$/^rules=1$/^text=65536$/^pattern=ascii-16scalars$/^match=dense$/^set=holdout$' -benchmem -count=3 -cpu=16`
7. `go test . -run '^$' -bench '^BenchmarkFoldedRewriteStrategies$/^impl=(baseline|production)$/^mode=obfs$/^rules=1$/^text=65536$/^pattern=ascii-16scalars$/^match=none$/^set=holdout$' -benchmem -count=3 -cpu=16`
8. `go test . -run '^$' -bench '^BenchmarkTransformMatrix$/^body=1048576$/^words=1024$/^fold=false$' -benchmem -count=3 -cpu=16`

Each paired output contained eight `PASS` sections, eight `ok` package footers, command exits `0,0,0,0,0,0,0,0`, and `block_exit_code=0`:

| Output | Ref | Elapsed | Samples |
| --- | --- | ---: | --- |
| `performance-paired-baseline-1.txt` | `da6e91262f1faaf82cacc5425eaf4a7f1b911042` | `89.337s` | 3 each |
| `performance-paired-current-1.txt` | `448c85200f33391811edab4237b6884a4ad105c8` | `88.843s` | 3 each |
| `performance-paired-current-2.txt` | `448c85200f33391811edab4237b6884a4ad105c8` | `86.374s` | 3 each |
| `performance-paired-baseline-2.txt` | `da6e91262f1faaf82cacc5425eaf4a7f1b911042` | `87.407s` | 3 each |

The comparison uses each output's median of three `ns/op` samples. Interleaved medians are the medians of the two baseline or current run medians. Absolute ratio is current divided by baseline. For `impl=production`, normalized values are production divided by the matching same-run `impl=baseline`; paired ratios test direction stability.

| # | Candidate | Baseline median ns/op | Current median ns/op | Absolute ratio | Paired absolute ratios | Normalized ratio | Paired normalized ratios | Verdict |
| ---: | --- | ---: | ---: | ---: | --- | ---: | --- | --- |
| 1 | `BenchmarkExactBlockStrategies/impl=baseline/mode=block/rules=256/text=65536/pattern=literal/match=last/set=calibration-16` | `195,754.0` | `204,559.5` | `1.0450x`, `+4.50%` | `1.2236x`, `0.8871x` | n/a | n/a | reference-only |
| 2 | `BenchmarkFoldObfuscateDense-16` | `17,077,636.5` | `19,014,975.5` | `1.1134x`, `+11.34%` | `1.0054x`, `1.2202x` | n/a | n/a | noise |
| 3 | `BenchmarkFoldStripDense-16` | `9,141,449.5` | `10,021,218.0` | `1.0962x`, `+9.62%` | `1.0003x`, `1.1927x` | n/a | n/a | noise |
| 4 | `BenchmarkFoldedRewriteStrategies/impl=baseline/mode=obfs/rules=1/text=65536/pattern=invalid-utf8-16scalars/match=sparse/set=holdout-16` | `5,172,956.5` | `5,227,829.5` | `1.0106x`, `+1.06%` | `1.0160x`, `1.0052x` | n/a | n/a | reference-only |
| 5 | `BenchmarkFoldedRewriteStrategies/impl=baseline/mode=strip/rules=1/text=65536/pattern=ascii-16scalars/match=overlap/set=holdout-16` | `224,841.0` | `213,289.0` | `0.9486x`, `-5.14%` | `1.1742x`, `0.7734x` | n/a | n/a | reference-only |
| 6 | `BenchmarkFoldedRewriteStrategies/impl=kmp/mode=strip/rules=1/text=16384/pattern=ascii-8scalars/match=none/set=calibration-16` | `61,670.5` | `70,632.0` | `1.1453x`, `+14.53%` | `1.1298x`, `1.1611x` | n/a | n/a | reference-only |
| 7 | `BenchmarkFoldedRewriteStrategies/impl=production/mode=obfs/rules=1/text=65536/pattern=ascii-16scalars/match=dense/set=holdout-16` | `263,225.5` | `233,844.5` | `0.8884x`, `-11.16%` | `0.8053x`, `0.9900x` | `0.8168x`, `-18.32%` | `0.8134x`, `0.8209x` | noise |
| 8 | `BenchmarkFoldedRewriteStrategies/impl=production/mode=obfs/rules=1/text=65536/pattern=ascii-16scalars/match=none/set=holdout-16` | `292,541.5` | `245,391.5` | `0.8388x`, `-16.12%` | `0.8927x`, `0.7910x` | `0.9173x`, `-8.27%` | `1.0947x`, `0.7893x` | noise |
| 9 | `BenchmarkFoldedRewriteStrategies/impl=production/mode=obfs/rules=1/text=65536/pattern=invalid-utf8-16scalars/match=sparse/set=holdout-16` | `1,035,427.0` | `979,883.0` | `0.9464x`, `-5.36%` | `0.9838x`, `0.9118x` | `0.9364x`, `-6.36%` | `0.9684x`, `0.9070x` | noise |
| 10 | `BenchmarkTransformMatrix/body=1048576/words=1024/fold=false-16` | `5,313,800.0` | `4,941,321.5` | `0.9299x`, `-7.01%` | `0.8845x`, `0.9715x` | n/a | n/a | noise |

The only stable material direct-strategy slowdown is `impl=kmp` in case 6. Its production sibling is `1.1170x` slower absolutely and `1.0610x` slower after same-run normalization, but normalized pairs reverse at `0.8829x` and `1.2410x`. Dense cases 2 and 3 changed only in the second current run, while the first pair was within `0.54%`; matching controls identify run-specific variation. No production candidate is slower in a stable direction after same-run normalization. Final performance status: `NO_REGRESSION`.

The benchmark-definition command was:

```powershell
git diff --unified=0 da6e91262f1faaf82cacc5425eaf4a7f1b911042 448c85200f33391811edab4237b6884a4ad105c8 -- benchmark_test.go
```

- exit code: `0`.
- decisive output: all selected benchmark definitions are unchanged. Only three post-only benchmarks and unselected `runBenchmarkRewritePreflightStrategies` code differ. The related `matcher.go` diff changes only `foldMatcher` rule-offset handling; `rewriteFolded`, `rewriteFoldedKMP`, `stripRule`, and `obfuscateRule` are unchanged.

No benchmark was rerun for adjudication. The gate reused the four completed interleaved outputs once and introduced no unrequested materiality threshold.

### ABI benchmark evidence

- `go run ./.github/scripts/integration-runner.go -bench-abi`
  - exit code: `0`; PASS in `9.845s`.

| Payload | Before-auth -> after-auth ns/op | Before-auth -> after-auth B/op | Before-auth -> after-auth allocs/op |
| --- | ---: | ---: | ---: |
| 1 KiB | `56,386 -> 22,681` | `11,159 -> 6,302` | `31 -> 29` |
| 1 MiB | `27,803,588 -> 1,848,338` | `10,291,138 -> 4,512,906` | `38 -> 32` |
| 20 MiB | `511,753,550 -> 17,537,427` | `223,729,748 -> 94,153,355` | `42 -> 34` |

Task 11's borrowing candidate did not remove the input-sized before-auth allocation, so production retains the safe input-side `C.GoBytes` copy. No no-copy benefit is claimed.

### Windows package, checksum, and ZIP evidence

- `make package VERSION=v0.2.0 GOOS=windows GOARCH=amd64`
  - exit code: `0`; PASS.
  - historical digest for that then-current ZIP: `f650928db5cb2a9efd8b85f45b2c1099d2dae642010fbc83e5e188ef67e60209`.

The artifact was later rebuilt, so the historical digest does not describe the current ZIP. The permitted checksum and archive recheck did not rebuild it and found:

- ZIP: `dist/censorship_0.2.0_windows_amd64.zip`, `2,136,591` bytes.
- DLL: `dist/windows_amd64/censorship.dll`, `4,896,256` bytes.
- current calculated SHA-256: `2fdd8f5d53691a4a98cf8e70a731fa8fcec01f832d5cd9efc7ca37976e877e2e`.
- checksum file content: `2fdd8f5d53691a4a98cf8e70a731fa8fcec01f832d5cd9efc7ca37976e877e2e  censorship_0.2.0_windows_amd64.zip`.
- ZIP entries: exactly `censorship.dll` and `LICENSE`.

The current artifact and checksum agree. `dist/`, raw logs, and temporary files remain outside the verification commit.

### Active constraints

- Active module: `github.com/router-for-me/CLIProxyAPI/v7 v7.2.152`. Active module files contain neither revision `81e1b537` nor pin `v7.2.147`.
- Active registration fields are `ignore_case`, `words`, `scope`, and `obfs`; `main.go` has no active `ConfigField.*mode` registration.
- `words` remains an Object with ordered `block`, `strip`, and `obfs` arrays. The panel emits no global `mode`; legacy global `mode` remains handwritten-YAML compatibility only.
- Matching order remains `block` -> `strip` -> `obfs`.
- Provider selection remains limited to documented natural-language paths; no generic recursive string walker was added.
- The exact required Logo URL is present in `main.go`, `main_test.go`, `README.md`, and `RELEASE_NOTES.md`.
- `abi_cgo.go` intentionally retains two `C.GoBytes` calls under the Task 11 safety ruling.
- A literal repository-wide old-reference or `C.GoBytes` grep is intentionally not clean because historical specs and plans retain old references and the production ABI retains the two required copies. Active dependency and metadata checks are clean.
- Integration run 2 reused its checkout and did not synchronize a remote or initialize another checkout.
- Release packages require lowercase SHA-256 files, aggregate `checksums.txt` entries, and the repository `LICENSE` when present.

## Repository Guide: Add Project CLAUDE.md, 2026-09-08

- Source commit `ddb4b38fb440540a562ef3aa065b56e74189ef1d`, subject `docs: add repository guidance`, changed only `CLAUDE.md`.
- The source commit was not previously an ancestor of `feat/v0.2.0`.
- The add/add conflict was resolved using the source commit's guide. The control-branch commit is `0a413d870c6700c2e651bd01cc82f25515300c1d`, subject `docs: add repository guidance`, and its committed path list contains only `CLAUDE.md`.
- `CLAUDE.md` exists and contains the plugin-only scope, smallest relevant verification commands, Object `words` and legacy-YAML rules, fixed phase order, explicit provider-path boundary, ABI ownership and `C.GoBytes` constraints, generated-artifact rules, and surgical TDD discipline.
- The guide contains no absolute-path pattern and no private model preference.
- `git diff --check`
  - exit code: `0`.
- The guide integration did not stage, modify, or overwrite the pre-existing Task 16 tracked working-tree status.

## Completion ledger, Tasks 12 through 16 and repository guide

- [x] Task 12: hardened CPA integration coverage; focused watcher, full tagged integration, and race coverage passed; integrated as `db6ec74da3870cbb3432fdb676be35efa1fa8159`.
- [x] Task 13: reused the existing CPA integration checkout; runner tests passed; integrated as `4dd60496f0ed98359a0167c78633d39f5fb0c60a`.
- [x] Task 14: hardened normalized release packaging, checksums, and conditional `LICENSE`; script tests passed; integrated as `ca7f15c76c10d2400155abb8b86352132803310b`.
- [x] Task 15: published canonical Object configuration and accurate limits; documentation test, race test, and focused contract tests passed.
- [x] Task 16: static, unit, race, vet, repaired fuzz, integration, baseline benchmark, paired performance, ABI benchmark, and Windows package evidence passed; performance adjudication is `NO_REGRESSION`; verification commit subject is `test: verify censorship v0.2.0`.
- [x] Repository guide: project `CLAUDE.md` integrated alone as `0a413d870c6700c2e651bd01cc82f25515300c1d`; content and privacy constraints verified.


## Task 17: Partitioned Review Fixes and Final Gate, 2026-09-08

### Canonical review outcomes

| Partition | Confirmed | Rejected | Outcome |
| --- | ---: | ---: | --- |
| A | 2 | 2 | Fixed A-001 and A-002. |
| B | 0 | 0 | PASS, no finding survived evidence gating. |
| C | 5 | 2 | Fixed C-01 through C-05. |
| D | 5 | 0 | Fixed D-001 through D-005. |

The coordinator used the one canonical result from each disjoint partition. It excluded a duplicate partition-D result and an orphan retry candidate because neither was the canonical correct-checkout review.

### Evidence-gate dispositions

- A-001, confirmed: `applyMode` reset `SkipFoldRewrite` for every selected span before deciding whether folded rewrite could use it. Exact and block-only requests made an unconditional extra pass. The fix gates reset and preflight bookkeeping on the folded-rewrite path.
- A-002, confirmed: the mixed holdout benchmark sent a block rule through `stripRule` in baseline while preflight used `applyMode`, measuring different semantics. Both variants now use `applyMode`; baseline disables only `RewriteMatcher`.
- A-R01, rejected: `borrowedRequest` is called only by ABI tests. Production still uses `C.GoBytes`, so this does not establish an ABI lifetime violation.
- A-R02, rejected: a registration test's literal `schema_version: 4` does not establish that the plugin must reject mismatched incoming lifecycle schema values.
- B, no confirmed finding: selector review and assigned selector tests supplied no concrete wrong result.
- C-01, confirmed: `taskkill /PID <pid> /T` without `/F` can fail and leave console children alive. Cleanup now uses `/F`.
- C-02, confirmed: a CPA build with the pinned revision and `vcs.modified=true` could pass host validation. The resolver now rejects modified builds.
- C-03, confirmed: the five-second integration limit did not bound an accepted TCP connection that stalls during WebSocket upgrade. The handshake now runs under a timeout context.
- C-04, confirmed: the ABI benchmark could measure an error, no-match, or already-mutated request. Its oracle now requires exact successful strip behavior and fresh request bytes each iteration.
- C-05, confirmed: four timeout tests serially waited for five seconds each, producing a deterministic twenty-second floor. Tests inject a short timeout while production helpers retain their default.
- C-R01 and C-R02, rejected: output-fixture matching and non-atomic watcher-write candidates concerned pre-review-range code; no in-range defect was proved.
- D-001, confirmed: lexical containment allowed cleanup through a junction. `requireContained` now rejects symlink and junction ancestors before cleanup.
- D-002, confirmed: archive and checksum paths under aliases of an absent file could refer to the same target, allowing checksum write to truncate the ZIP. The packager compares deepest-existing-ancestor filesystem identities and remaining suffixes.
- D-003, confirmed: unsafe `VERSION` values were rejected only after library creation. `validate-version` now runs before build targets.
- D-004, confirmed: README and RELEASE_NOTES omitted active Claude and OpenAI tool-result text paths, contradicting actual selector behavior. Both documents now state the exact selected fields.
- D-005, confirmed: README and RELEASE_NOTES said no `LICENSE` exists even though package inspection includes it conditionally. The false statement was deleted.

### Fix RED and GREEN evidence

- A-001: direct changed-flow proof established the unnecessary reset before fix. GREEN: `go test . -run '^TestFoldRewritePreflight' -count=1` exited `0`.
- A-002: direct benchmark-control-flow proof established unequal block semantics before fix. GREEN: `go test . -run '^TestBenchmarkFoldRewriteStrategyPreservesMixedBlockSemantics$' -count=1` exited `0`.
- C-01: RED `TestTerminateCPAStopsWindowsProcessTree` failed with `taskkill` exit `128`; GREEN exited `0` in `0.514s`.
- C-02: RED `TestRevisionRejectsModifiedBuild` failed with `expected modified build rejection`; GREEN exited `0` in `0.052s`.
- C-03: RED failed to compile because the timeout helper did not exist; GREEN `TestResponsesWebSocketDialTimesOutDuringStalledUpgrade` exited `0` in `0.158s`.
- C-04: the worker added exact response, termination, and request-immutability assertions, then passed `make integration` and one dynamic ABI benchmark run.
- C-05: RED timeout group failed after `20.060s`; GREEN exited `0` in `0.469s`.
- D-001: RED accepted a symlinked checkout; GREEN `TestPrepareCheckoutRejectsSymlinkedCheckoutBeforeRemovingGeneratedTests` exited `0` in `0.396s`.
- D-002: RED accepted absent output aliases; GREEN `TestValidateDirectPackagePathsRejectsAbsentOutputAliasesThroughJunction` exited `0` in `0.139s`.
- D-003: RED accepted `foo/bar` and delayed validation; GREEN `TestMakeVersionValidationContract` exited `0` in `0.816s`.
- D-004: RED documentation contract reported omitted tool-result paths; GREEN exited `0` in `0.044s`.
- D-005: RED reported a false optional-`LICENSE` denial; GREEN exited `0` in `0.041s`.
- The round-1 final gate found a real Windows CRLF comparison failure in the document contract. Commit `4e6b41b` normalizes CRLF to LF before comparison. Its focused test, `make test`, and `make race` passed before the fresh round-2 gate.

### Fresh final gate, round 2

Control checkout: `C:/Users/user/Downloads/cpa-plugin-censorship`, branch `feat/v0.2.0`, HEAD `4e6b41bc7fca2d1612b011a263ae30cd093f6201`.

Every command began with:

```powershell
Set-Location -LiteralPath 'C:/Users/user/Downloads/cpa-plugin-censorship'
$env:GOPATH='C:/Users/user/go'
$env:GOMODCACHE='C:/Users/user/go/pkg/mod'
$root = git rev-parse --show-toplevel
$root
if ($root -ne 'C:/Users/user/Downloads/cpa-plugin-censorship') { throw "Unexpected repository root: $root" }
```

`make test`

```plaintext
C:/Users/user/Downloads/cpa-plugin-censorship
go test ./...
ok  	github.com/DoingDog/cpa-plugin-censorship	(cached)
?   	github.com/DoingDog/cpa-plugin-censorship/integration	[no test files]
```

Exit code: `0`.

`go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go -count=1`

```plaintext
C:/Users/user/Downloads/cpa-plugin-censorship
ok  	command-line-arguments	2.049s
```

Exit code: `0`.

`go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1`

```plaintext
C:/Users/user/Downloads/cpa-plugin-censorship
ok  	command-line-arguments	7.599s
```

Exit code: `0`.

`make race`

```plaintext
C:/Users/user/Downloads/cpa-plugin-censorship
go test -race ./...
ok  	github.com/DoingDog/cpa-plugin-censorship	(cached)
?   	github.com/DoingDog/cpa-plugin-censorship/integration	[no test files]
```

Exit code: `0`.

`make vet`

```plaintext
C:/Users/user/Downloads/cpa-plugin-censorship
go vet ./...
```

Exit code: `0`.

`make integration` built current-HEAD `.integration/bin.exe` and `.integration/run/plugins` before the direct tagged test.

```plaintext
C:/Users/user/Downloads/cpa-plugin-censorship
go test .github/scripts/integration-runner.go .github/scripts/integration-runner_test.go
ok  	command-line-arguments	2.001s
go run ./.github/scripts/integration-runner.go
=== RUN   TestDynamicABIResponseOracle
=== RUN   TestDynamicABIResponseOracle/successful_strip
=== RUN   TestDynamicABIResponseOracle/terminated_response
=== RUN   TestDynamicABIResponseOracle/empty_response
=== RUN   TestDynamicABIResponseOracle/mutated_request
--- PASS: TestDynamicABIResponseOracle (0.00s)
    --- PASS: TestDynamicABIResponseOracle/successful_strip (0.00s)
    --- PASS: TestDynamicABIResponseOracle/terminated_response (0.00s)
    --- PASS: TestDynamicABIResponseOracle/empty_response (0.00s)
    --- PASS: TestDynamicABIResponseOracle/mutated_request (0.00s)
=== RUN   TestResolveCPAPathsNormalizesRelativePaths
--- PASS: TestResolveCPAPathsNormalizesRelativePaths (0.00s)
=== RUN   TestRevisionRejectsWrongAndMissingVCSRevision
=== RUN   TestRevisionRejectsWrongAndMissingVCSRevision/wrong
=== RUN   TestRevisionRejectsWrongAndMissingVCSRevision/missing
--- PASS: TestRevisionRejectsWrongAndMissingVCSRevision (0.00s)
    --- PASS: TestRevisionRejectsWrongAndMissingVCSRevision/wrong
    --- PASS: TestRevisionRejectsWrongAndMissingVCSRevision/missing
```
```plaintext
=== RUN   TestRevisionRejectsModifiedBuild
--- PASS: TestRevisionRejectsModifiedBuild (0.00s)
=== RUN   TestTerminateCPAStopsWindowsProcessTree
--- PASS: TestTerminateCPAStopsWindowsProcessTree (0.51s)
=== RUN   TestTerminateCPAProcessTreeHelper
--- PASS: TestTerminateCPAProcessTreeHelper (0.00s)
=== RUN   TestReadinessUsesAuthenticatedModelsEndpoint
--- PASS: TestReadinessUsesAuthenticatedModelsEndpoint (0.01s)
=== RUN   TestHTTPClientTimeout
--- PASS: TestHTTPClientTimeout (0.10s)
=== RUN   TestTCPTimeoutAfterConnection
--- PASS: TestTCPTimeoutAfterConnection (0.10s)
=== RUN   TestHTTPBlockIncludesTermAndRole
--- PASS: TestHTTPBlockIncludesTermAndRole (0.79s)
=== RUN   TestHTTPRejectsDuplicateJSONMembers
--- PASS: TestHTTPRejectsDuplicateJSONMembers (0.60s)
=== RUN   TestLegacyCompletionsPromptUsesConvertedUserRole
--- PASS: TestLegacyCompletionsPromptUsesConvertedUserRole (0.61s)
=== RUN   TestWatcherReloadLinearizesAtObservedSnapshotB
--- PASS: TestWatcherReloadLinearizesAtObservedSnapshotB (2.77s)
=== RUN   TestHTTPResponsesBlockStringInput
--- PASS: TestHTTPResponsesBlockStringInput (0.59s)
=== RUN   TestHTTPResponsesTransformsStringInput
--- PASS: TestHTTPResponsesTransformsStringInput (0.66s)
=== RUN   TestHTTPResponsesTransformsStructuredInputText
--- PASS: TestHTTPResponsesTransformsStructuredInputText (0.58s)
=== RUN   TestHTTPAndSSEOutputTraceUnaffected
=== RUN   TestHTTPAndSSEOutputTraceUnaffected/nonmatching/stream=false
=== RUN   TestHTTPAndSSEOutputTraceUnaffected/nonmatching/stream=true
=== RUN   TestHTTPAndSSEOutputTraceUnaffected/strip
=== RUN   TestHTTPAndSSEOutputTraceUnaffected/obfs
--- PASS: TestHTTPAndSSEOutputTraceUnaffected (4.61s)
    --- PASS: TestHTTPAndSSEOutputTraceUnaffected/nonmatching/stream=false (1.23s)
    --- PASS: TestHTTPAndSSEOutputTraceUnaffected/nonmatching/stream=true (1.16s)
    --- PASS: TestHTTPAndSSEOutputTraceUnaffected/strip (1.10s)
    --- PASS: TestHTTPAndSSEOutputTraceUnaffected/obfs (1.13s)
=== RUN   TestResponsesWebSocketModelTurnUsesResponsesSelector
--- PASS: TestResponsesWebSocketModelTurnUsesResponsesSelector (0.61s)
=== RUN   TestResponsesWebSocketDialTimesOutDuringStalledUpgrade
--- PASS: TestResponsesWebSocketDialTimesOutDuringStalledUpgrade (0.10s)
=== RUN   TestReadUntilCompletedReturnsOnDeadline
--- PASS: TestReadUntilCompletedReturnsOnDeadline (0.01s)
=== RUN   TestResponsesWebSocketReadTimesOutAfterDial
--- PASS: TestResponsesWebSocketReadTimesOutAfterDial (0.10s)
=== RUN   TestTerminalWebSocketTimeoutIsNotPeerClose
--- PASS: TestTerminalWebSocketTimeoutIsNotPeerClose (0.10s)
=== RUN   TestResponsesWebSocketBlockReturnsStatus400ThenCloses
--- PASS: TestResponsesWebSocketBlockReturnsStatus400ThenCloses (0.59s)
=== RUN   TestResponsesWebSocketOutputMessagesUnaffected
=== RUN   TestResponsesWebSocketOutputMessagesUnaffected/strip
=== RUN   TestResponsesWebSocketOutputMessagesUnaffected/obfs
--- PASS: TestResponsesWebSocketOutputMessagesUnaffected (3.42s)
    --- PASS: TestResponsesWebSocketOutputMessagesUnaffected/strip (1.16s)
    --- PASS: TestResponsesWebSocketOutputMessagesUnaffected/obfs (1.12s)
PASS
ok  	github.com/router-for-me/CLIProxyAPI/v7/integration/censorshipplugin	16.961s
```

`make integration` exit code: `0`.

The direct tagged package command set:

```powershell
$env:CPA_INTEGRATION_BIN='C:/Users/user/Downloads/cpa-plugin-censorship/.integration/bin.exe'
$env:CENSORSHIP_PLUGIN_DIR='C:/Users/user/Downloads/cpa-plugin-censorship/.integration/run/plugins'
go test -tags=integration ./integration -count=1
```

```plaintext
C:/Users/user/Downloads/cpa-plugin-censorship
ok  	github.com/DoingDog/cpa-plugin-censorship/integration	16.591s
```

Exit code: `0`.

Final `git diff --check; git status --short` output:

```plaintext
C:/Users/user/Downloads/cpa-plugin-censorship
```

Exit code: `0`. `git diff --check` produced no output and `git status --short` was empty before review evidence was written.

Round-2 result: **PASS**. No source or test file changed during this gate. The final report is `.superpowers/sdd/2026-09-07-censorship-v0.2.0/task-17-final-gate-round-2.md`.
