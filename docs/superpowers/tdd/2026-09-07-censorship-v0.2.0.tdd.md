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
