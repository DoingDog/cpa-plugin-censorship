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
