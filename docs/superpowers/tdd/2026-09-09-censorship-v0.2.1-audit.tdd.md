# CPA censorship plugin v0.2.1 TDD and benchmark evidence

Baseline: `v0.2.0` (`1085eee`)
Branch: `feat/v0.2.1`
Toolchain: Go 1.26.5 windows/amd64, CGO enabled, GCC, GNU Make 4.4.1

## Baseline verification

- `make test`: pass
- `make race`: pass
- `make vet`: pass
- `make integration`: pass
- `FuzzRuleEngineAgainstOracle`, 30 seconds: pass
- `FuzzFoldKMPAgainstOracle`, 30 seconds: pass
- `FuzzDuplicateWalkerAgainstOracle`, 30 seconds: pass
- `FuzzProtocolTransform`, 30 seconds: pass
- `FuzzRebuildBodyAgainstMarshalOracle`, 30 seconds: pass
- `FuzzByteMatcherAgainstOrderedContains`, 30 seconds: pass

## Functional red/green cycles

### SEL-001: Gemini null machine-union members

- Mutation caught: treating a present JSON `null` machine discriminator as a non-null machine Part.
- RED: `go test . -run '^(TestGeminiNullMachineDiscriminatorsRemainSelectable|TestScanTextPartPreservesProtocolRules)$' -count=1` failed for the twelve previously unconditional direct keys and `extra_content.google.thought_signature: null`; `functionCall` and `function_call` already passed.
- GREEN: the same command passed after all direct machine keys used `value.Type != gjson.Null` and the nested signature applied the same rule.
- Adjacent: `go test . -run '^(TestGemini|TestInteractions|TestClaude|TestScanTextPart)' -count=1` passed.

### SEL-002: Claude custom-content documents

- Mutation caught: omitting `source.type == "content"` traversal or selecting non-text document metadata.
- RED: `go test . -run '^(TestClaudeResultText|TestClaudeResultTextPreservesMachineFields)$' -count=1` failed for direct-user and nested-tool custom-content documents and preservation evidence.
- GREEN: the same command passed after selecting only ordered `source.content[]` objects with `type == "text"`, field `text`.
- Coverage: direct user and nested tool roles enabled/disabled; title, citation, URL, `cache_control`, and image data remain unchanged.
- Fuzz corpus: `go test . -run '^FuzzProtocolTransform$' -count=1` passed.

### IT-005: exact Chat Completions target assertion

- Mutation caught: replacing exact `messages.0.content` validation with whole-body substring search when a decoy contains the expected transformed spelling.
- RED: `go test -tags=integration ./integration -run '^TestCapturedChatContentValidatorRejectsTransformedDecoy$' -count=1` failed to compile because the wished-for validator did not exist.
- GREEN: the same command passed after adding the exact-path validator; its retained literal fixture has unchanged target content and an expected transformed decoy, so a body-wide implementation fails the test.
- Ruling: retained behavioral mutation coverage replaces the plan's temporary deliberately false expectation; no false test state is committed.

### IT-009: upstream arrival accounting

- Mutation caught: incrementing arrival only after `io.ReadAll` succeeds.
- RED: `go test -tags=integration ./integration -run '^TestUpstreamCaptureCountsArrivalBeforeTruncatedBody$' -count=1` failed because `arrivalCount` did not exist.
- GREEN: the same command passed with one handler arrival and zero completed captures for a request that declared three body bytes but sent two.
- Blocked HTTP and WebSocket assertions now require zero arrivals; successful-request arithmetic still uses completed captures.

### IT-012: complete raw HTTP/1.1 trace

- Mutation caught: dropping terminal trailers or accepting a byte after complete fixed-length or chunked framing.
- RED: `go test -tags=integration ./integration -run '^TestReadHTTP11Trace$' -count=1` failed because the error-returning parser did not exist.
- GREEN: `go test -tags=integration ./integration -run '^(TestReadHTTP11Trace|TestUpstreamCaptureCountsArrivalBeforeTruncatedBody)$' -count=1` passed for fixed length, fixed-length surplus, chunked trailers, and chunked surplus fixtures.
- CPA gate: `make integration` passed after all Task 1-4 initial changes, including exact HTTP/SSE output trace and WebSocket pass-through checks.

### ABI-001: native request pointer-length invariant

- Mutation caught: moving nil/nonzero request validation inside the copying branch, allowing `request_intercept_after` to bypass it.
- Initial RED: `go test . -run '^(TestValidPluginRequest|TestCopyPluginRequest|TestCopyHostResponse)' -count=1` failed because all three helpers were undefined.
- Wiring RED: after temporarily moving validation into the copy branch, `go test . -run '^TestCliproxyPluginCallRejectsNilNonzeroAfterAuthRequest$' -count=1` failed with status 0 instead of 1.
- GREEN: the direct exported-entrypoint test passes with nil pointer, length 1, non-copying after-auth method, status 1, and cleared output.
- Production path validates every descriptor before `shouldCopyPluginRequest`; before-auth still copies through `C.GoBytes`.

### ABI-002: native host response pointer-length invariant

- Mutation caught: treating `ptr == nil && len != 0` as empty success or returning host-owned memory.
- GREEN after initial helper RED: nil/zero returns empty, nil/one errors, nonnil/one copies; mutating the source after return leaves the copied byte unchanged.
- Exact focused normal and race runs execute `TestCopyHostResponseRejectsNilNonzeroLength`, both `TestBorrowedABI...` tests, and the request descriptor tests; both passed.
- Every non-nil callback buffer retains the existing deferred host `free_buffer` call exactly once, including callback error and zero-length cases.

### BUILD-001 and VERSION-001: VERSION shell safety and one-time normalization

- Mutation caught: expanding untrusted command-line `VERSION` in recipe source, GNU Make automatically exporting recursive command-line payloads, or passing a pre-normalized value to the Go packager.
- RED: focused Make tests accepted both `v1";touch VERSION_INJECTION_SENTINEL;version="1` and `$(shell touch VERSION_MAKE_SENTINEL)` and failed the raw-version dry-run assertion.
- Root cause evidence: simple derived variables alone did not stop GNU Make from automatically exporting recursive command-line `VERSION`; `unexport VERSION` did.
- GREEN: executable tests reject quote/semicolon, Make-function, newline, whitespace, and slash values without creating sentinels. Recipes read exported `NORMALIZED_VERSION` and `PACKAGER_VERSION` as shell data.
- `make validate-version VERSION=vv` passes; platform and aggregate `make -n package VERSION=vv` send raw `vv` to Go while artifact names use the once-normalized `v`.

### PACKAGE-001: repository LICENSE collision

- RED: direct package cases using repository `LICENSE` as archive or checksum unexpectedly succeeded.
- GREEN: existing `LICENSE` is canonicalized and compared with both writable outputs before packaging; rejection preserves input/output bytes and mtimes.
- Fix-round RED/GREEN: a dangling `LICENSE -> archive.zip` initially allowed archive creation, then passed after `Lstat` distinguished a dangling input link and rejected it before any write.

### PACKAGE-002: dangling output symlinks

- RED: a dangling checksum symlink to the absent archive unexpectedly succeeded and could replace the completed ZIP with checksum text.
- GREEN: archive and checksum leaves are checked with `Lstat` and rejected when symlinks, including dangling links, before canonicalization or writes.

### PACKAGE-003: case-only absent output aliases

- Regression fixture: absent `out/Archive.zip` and `out/archive.zip` under one existing ancestor are treated as aliases on every OS. The former Windows-only equal-fold condition was removed while existing-ancestor identity and hardlink/junction checks remain.
- The case-only row already failed safely on the Windows development host before the change; cross-platform behavior is protected by host-independent production logic and the executable fixture.

### PACKAGE-004: deterministic ZIP metadata

- RED: `TestPackageLibraryIsDeterministicAcrossSourceMtimes` reported changed ZIP bytes when only library and LICENSE mtimes changed by six seconds.
- GREEN: both entry headers use `1980-01-01T00:00:00Z`; archive bytes and lowercase checksum-line bytes are identical across source mtime changes.
- Existing entry basenames, Deflate method, library mode `0755`, and optional LICENSE behavior remain unchanged.

### Functional combined gate

- `go test ./... -count=1`: pass.
- `go test .github/scripts/package-release.go .github/scripts/package-release_test.go -count=1`: pass.
- Focused ABI `go test -race` with exact complete test names: pass.
- `make integration`: pass against pinned CLIProxyAPI after all functional fixes.

## Performance experiments

### A. Exact rewrite total-miss preflight

## Decision

REJECT production candidate. `config.go`, `transform.go`, and `config_test.go` remain unchanged. Only benchmark/oracle coverage changed in `benchmark_test.go`.

## Correctness oracle

Command:

```powershell
go test . -run '^TestBenchmarkExactRewritePreflightMatchesBaseline$' -count=1
```

Result: PASS. The oracle compares benchmark-only preflight with current `applyMode` across duplicate rules, multiple spans, none/first/middle/last/dense hits, cross-span boundaries, strip and obfs cascades, every final `Text`, every `Changed` bit, and aggregate changed flag.

## Measurement

- Two opposite-order series.
- 10 independent process samples per implementation in each series.
- `-benchmem`, `-benchtime=500ms`, and fixed `-cpu=1,16` values.
- 56 CPU/workload variants per repetition and 560 rows per implementation per series.
- Rules, matcher, 20 MiB inputs, expected results, and correctness checks were constructed outside timed regions.
- Raw forward series: `C:\Users\user\AppData\Local\Temp\cpa-v021-exact-rewrite-forward.txt`.
- Raw reverse series: `C:\Users\user\AppData\Local\Temp\cpa-v021-exact-rewrite-reverse.txt`.
- Full forward and reverse `benchstat -col /impl` output: `C:\Users\user\AppData\Local\Temp\cpa-v021-exact-rewrite-benchstat.txt`.
- Every collection command exited 0; logs contain no FAIL, panic, or fatal error.

## Results and rejection gate

Total-miss latency improved 94.16% to 98.59% in forward order and 94.14% to 98.68% in reverse order, all `p=0.000`. The candidate is rejected because hit/cascade holdouts exceeded the permitted 5% regression in both independent series.

The initial summary listed examples only. The complete original-table holdout violations are appended in Fix round 1 below.

No production candidate was applied, so production race/holdout gates do not apply. The retained benchmark code contains only the correctness oracle and isolated strategy measurement.

## Concern

The implementing worker did not write this report because it misread its report-file instruction. The controller reconstructed the report from the workflow's structured result; the complete statistical tables remain in the named benchmark file for scoped review and later TDD-record append.

## Fix round 1

`BenchmarkExactRewriteStrategies` and `runBenchmarkExactRewriteStrategies` were restored from `c295ec3`, including the ordinary 32, 128, 256, and 1,024 rule matrix over 4 KiB, 64 KiB, and 1 MiB request bodies. Experiment A remains isolated under `BenchmarkExactRewritePreflight` and `runBenchmarkExactRewritePreflight`.

The preflight oracle now has a `later span hit` fixture with `[]bool{false, true}` span expectations and an aggregate changed expectation of `true`. The pure rewrite helper no longer has the unexercised mixed block fallback or block-result return value.

Commands:

```powershell
go test . -run '^TestBenchmarkExactRewritePreflightMatchesBaseline$' -count=1
```

Result: PASS after adding the fixture.

A temporary first-span-only mutation changed the helper loop to `for i := 0; i < 1 && i < len(spans); i++`. The same command failed for both `strip/later_span_hit` and `obfs/later_span_hit` with `changed = false, want true`. The loop was restored to `for i := range spans`, and the same command passed again.

### Original 500 ms holdout violations

This table enumerates every literal result above the 5% cap from both original `sec/op` benchstat tables. Percentages are preflight `sec/op` relative to baseline. `*` means benchstat printed `~`, so the percentage was computed from its printed rounded medians.

| Series | Mode | Match | Variant | Regression | p |
|---|---|---|---|---:|---:|
| forward | strip | last-sparse | serial | +5.53% | 0.000 |
| forward | strip | last-sparse | parallel-16 | +5.60%* | 0.089 |
| forward | strip | duplicate | serial | +5.27%* | 0.143 |
| forward | strip | duplicate | serial-16 | +6.15% | 0.000 |
| forward | strip | duplicate | parallel-16 | +7.96% | 0.015 |
| forward | strip | cascade | serial | +5.36% | 0.015 |
| forward | strip | cascade | serial-16 | +5.81% | 0.005 |
| forward | strip | cascade | parallel | +9.67% | 0.002 |
| forward | obfs | duplicate | serial | +14.78% | 0.004 |
| forward | obfs | duplicate | serial-16 | +7.03% | 0.009 |
| forward | obfs | duplicate | parallel | +5.96% | 0.005 |
| forward | obfs | cascade | serial | +6.89% | 0.000 |
| forward | obfs | cascade | serial-16 | +7.38% | 0.000 |
| forward | obfs | cascade | parallel | +8.86% | 0.000 |
| forward | obfs | cascade | parallel-16 | +5.42% | 0.004 |
| reverse | strip | dense | parallel-16 | +8.43%* | 0.436 |
| reverse | strip | duplicate | serial | +6.67% | 0.007 |
| reverse | strip | duplicate | serial-16 | +6.64% | 0.004 |
| reverse | strip | duplicate | parallel | +7.02% | 0.011 |
| reverse | strip | cascade | serial | +9.05% | 0.000 |
| reverse | strip | cascade | parallel | +9.40% | 0.000 |
| reverse | obfs | last-sparse | parallel | +11.65% | 0.029 |
| reverse | obfs | cascade | serial | +9.06% | 0.000 |
| reverse | obfs | cascade | serial-16 | +8.85% | 0.000 |
| reverse | obfs | cascade | parallel | +7.98% | 0.001 |

The original tables therefore contain 15 forward and 10 reverse literal holdout violations. The serial cascade violations reproduce in both orders, so the production decision remains REJECT.

### Saturated `RunParallel` rerun

Only Experiment A `execution=parallel` cases ran. Each raw file has 20 successful process blocks: 10 baseline and 10 preflight samples. Each process ran 14 benchmark variants at `-cpu=16`; all 280 benchmark rows in each file report `b.N=32`, which is greater than 16. Neither raw file contains `FAIL`, `panic:`, or `fatal error:`.

Raw files:

- `C:\Users\user\AppData\Local\Temp\cpa-v021-exact-rewrite-parallel-32x-forward.txt`
- `C:\Users\user\AppData\Local\Temp\cpa-v021-exact-rewrite-parallel-32x-reverse.txt`
- `C:\Users\user\AppData\Local\Temp\cpa-v021-exact-rewrite-parallel-32x-benchstat.txt`

Collection command:

```powershell
$forward = Join-Path $env:TEMP 'cpa-v021-exact-rewrite-parallel-32x-forward.txt'; $reverse = Join-Path $env:TEMP 'cpa-v021-exact-rewrite-parallel-32x-reverse.txt'; Remove-Item $forward,$reverse -ErrorAction SilentlyContinue; $baseline = '^BenchmarkExactRewritePreflight/impl=baseline/.*/.*/.*/.*/.*/.*/execution=parallel$'; $preflight = '^BenchmarkExactRewritePreflight/impl=preflight/.*/.*/.*/.*/.*/.*/execution=parallel$'; 1..10 | ForEach-Object { Add-Content $forward "=== sample $_ baseline ==="; & go test . -run '^$' -bench $baseline -benchtime=32x -benchmem -cpu=16 2>&1 | Tee-Object -FilePath $forward -Append; if ($LASTEXITCODE -ne 0) { throw "baseline sample $_ failed" }; Add-Content $forward "=== sample $_ preflight ==="; & go test . -run '^$' -bench $preflight -benchtime=32x -benchmem -cpu=16 2>&1 | Tee-Object -FilePath $forward -Append; if ($LASTEXITCODE -ne 0) { throw "preflight sample $_ failed" } }; 1..10 | ForEach-Object { Add-Content $reverse "=== sample $_ preflight ==="; & go test . -run '^$' -bench $preflight -benchtime=32x -benchmem -cpu=16 2>&1 | Tee-Object -FilePath $reverse -Append; if ($LASTEXITCODE -ne 0) { throw "preflight sample $_ failed" }; Add-Content $reverse "=== sample $_ baseline ==="; & go test . -run '^$' -bench $baseline -benchtime=32x -benchmem -cpu=16 2>&1 | Tee-Object -FilePath $reverse -Append; if ($LASTEXITCODE -ne 0) { throw "baseline sample $_ failed" } }
```

Benchstat command:

```powershell
$forward = Join-Path $env:TEMP 'cpa-v021-exact-rewrite-parallel-32x-forward.txt'; $reverse = Join-Path $env:TEMP 'cpa-v021-exact-rewrite-parallel-32x-reverse.txt'; $output = Join-Path $env:TEMP 'cpa-v021-exact-rewrite-parallel-32x-benchstat.txt'; $benchstat = Join-Path (go env GOPATH) 'bin\benchstat.exe'; Remove-Item $output -ErrorAction SilentlyContinue; Add-Content $output 'FORWARD: baseline then preflight'; & $benchstat -col /impl $forward 2>&1 | Tee-Object -FilePath $output -Append; if ($LASTEXITCODE -ne 0) { throw 'forward benchstat failed' }; Add-Content $output 'REVERSE: preflight then baseline'; & $benchstat -col /impl $reverse 2>&1 | Tee-Object -FilePath $output -Append; if ($LASTEXITCODE -ne 0) { throw 'reverse benchstat failed' }
```

Results from the saturated `sec/op` tables:

| Series | strip total-miss | obfs total-miss | Hit/cascade median above 5% |
|---|---:|---:|---|
| forward, baseline then preflight | -95.18% (p=0.000) | -95.92% (p=0.000) | obfs last-sparse: +17.73%* (p=0.579) |
| reverse, preflight then baseline | -95.88% (p=0.000) | -94.98% (p=0.000) | none |

`*` is computed from the printed medians, `140.8m / 119.6m - 1`; benchstat printed `~` because the result was not significant. The serial holdout failures above still decide the gate: REJECT.

#### Complete original forward/reverse benchstat output

```plaintext
FORWARD: baseline then preflight
goos: windows
goarch: amd64
pkg: github.com/DoingDog/cpa-plugin-censorship
cpu: AMD Ryzen 7 7840H with Radeon 780M Graphics
                                                                                                                               │    baseline    │              preflight               │
                                                                                                                               │     sec/op     │    sec/op     vs base                │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial           412.79m ±  6%   24.09m ±  1%  -94.16% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16        421.35m ±  3%   24.22m ±  1%  -94.25% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel         423.90m ±  4%   24.04m ±  1%  -94.33% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16     267.442m ± 12%   3.954m ±  9%  -98.52% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial          428.4m ±  3%   426.1m ±  3%        ~ (p=0.796 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16       427.1m ±  4%   422.3m ±  3%        ~ (p=0.739 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel        422.4m ±  2%   424.7m ±  2%        ~ (p=0.393 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16     370.0m ±  6%   374.0m ±  3%        ~ (p=0.190 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial           427.6m ±  4%   451.2m ±  3%   +5.53% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16        426.0m ±  6%   443.8m ±  4%   +4.16% (p=0.023 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel         433.3m ± 12%   447.6m ±  3%        ~ (p=0.165 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16      278.8m ± 15%   294.4m ±  5%        ~ (p=0.089 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial                 36.42m ±  2%   36.20m ±  1%        ~ (p=0.280 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16              36.25m ±  7%   36.01m ±  3%        ~ (p=0.393 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel               36.19m ± 20%   35.97m ±  1%        ~ (p=0.353 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16            6.392m ± 69%   6.289m ±  7%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial             426.8m ± 13%   449.3m ±  3%        ~ (p=0.143 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16          423.8m ±  4%   449.8m ±  3%   +6.15% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel           423.8m ±  3%   442.4m ±  4%   +4.40% (p=0.004 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16        316.9m ±  9%   342.1m ±  5%   +7.96% (p=0.015 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial           280.91m ±  4%   24.46m ±  1%  -91.29% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16        257.61m ±  8%   24.08m ±  1%  -90.65% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel         270.12m ±  7%   24.17m ±  2%  -91.05% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      54.817m ± 83%   3.979m ± 13%  -92.74% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial               426.4m ±  8%   449.3m ±  3%   +5.36% (p=0.015 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16            428.3m ±  4%   453.2m ±  2%   +5.81% (p=0.005 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel             414.6m ±  3%   454.7m ±  5%   +9.67% (p=0.002 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16          373.5m ±  4%   388.5m ±  9%        ~ (p=0.190 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial            417.79m ±  5%   24.32m ± 16%  -94.18% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16         417.72m ±  3%   24.22m ± 11%  -94.20% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel          418.70m ±  5%   24.32m ±  6%  -94.19% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16      286.478m ±  7%   4.039m ± 54%  -98.59% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial           436.8m ±  5%   428.1m ±  4%        ~ (p=0.529 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16        439.0m ± 11%   427.6m ±  6%        ~ (p=0.280 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel         446.0m ±  6%   420.0m ± 13%        ~ (p=0.143 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16      363.2m ±  6%   367.0m ±  6%        ~ (p=0.971 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial            441.0m ± 12%   458.3m ± 11%        ~ (p=0.143 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16         432.1m ± 10%   451.8m ± 15%        ~ (p=0.218 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel          445.8m ± 10%   451.9m ± 16%        ~ (p=0.247 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16       317.6m ± 14%   306.9m ± 30%        ~ (p=0.684 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial                  669.4m ± 12%   667.0m ± 13%        ~ (p=0.853 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16               685.1m ±  8%   657.4m ± 14%        ~ (p=0.739 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel                693.7m ±  9%   672.3m ± 10%        ~ (p=0.190 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16             679.0m ±  9%   667.2m ± 14%        ~ (p=0.739 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial              427.4m ±  7%   490.6m ± 10%  +14.78% (p=0.004 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16           428.1m ± 10%   458.2m ± 13%   +7.03% (p=0.009 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel            429.6m ±  3%   455.2m ± 12%   +5.96% (p=0.005 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16         329.0m ±  7%   329.9m ± 13%        ~ (p=0.481 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial            265.75m ±  7%   24.52m ± 13%  -90.77% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16         284.66m ± 10%   24.23m ±  7%  -91.49% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel          271.32m ± 10%   24.25m ±  8%  -91.06% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16       54.641m ±  7%   4.035m ± 49%  -92.62% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial                427.4m ±  5%   456.8m ± 13%   +6.89% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16             422.1m ±  3%   453.2m ±  1%   +7.38% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel              422.7m ±  3%   460.1m ±  9%   +8.86% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16           368.3m ±  3%   388.2m ±  4%   +5.42% (p=0.004 n=10)
geomean                                                                                                                            302.7m         137.3m        -54.63%

                                                                                                                               │   baseline    │                 preflight                 │
                                                                                                                               │      B/s      │       B/s        vs base                  │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial          48.45Mi ±  6%    830.11Mi ±  1%  +1613.45% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16       47.47Mi ±  3%    825.80Mi ±  1%  +1639.66% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel        47.18Mi ±  4%    831.85Mi ±  1%  +1663.02% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16     74.84Mi ± 11%   5059.29Mi ±  9%  +6659.75% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial        46.69Mi ±  4%     46.94Mi ±  3%          ~ (p=0.796 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16     46.83Mi ±  4%     47.37Mi ±  3%          ~ (p=0.739 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel      47.35Mi ±  2%     47.10Mi ±  2%          ~ (p=0.393 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16   54.05Mi ±  6%     53.47Mi ±  3%          ~ (p=0.190 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial         46.77Mi ±  4%     44.32Mi ±  3%     -5.24% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16      46.95Mi ±  6%     45.07Mi ±  4%     -4.00% (p=0.023 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel       46.16Mi ± 11%     44.68Mi ±  3%          ~ (p=0.165 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16    71.78Mi ± 13%     67.93Mi ±  5%          ~ (p=0.089 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial               549.2Mi ±  2%     552.4Mi ±  1%          ~ (p=0.289 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16            551.8Mi ±  7%     555.5Mi ±  3%          ~ (p=0.393 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel             552.7Mi ± 17%     556.1Mi ±  1%          ~ (p=0.353 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16          3.067Gi ± 41%     3.106Gi ±  6%          ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial           46.87Mi ± 11%     44.51Mi ±  3%          ~ (p=0.143 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16        47.20Mi ±  4%     44.46Mi ±  3%     -5.81% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel         47.20Mi ±  3%     45.21Mi ±  4%     -4.21% (p=0.004 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16      63.11Mi ± 10%     58.46Mi ±  5%     -7.37% (p=0.015 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial          71.20Mi ±  5%    817.69Mi ±  1%  +1048.42% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16       77.63Mi ±  7%    830.73Mi ±  1%   +970.06% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel        74.04Mi ±  6%    827.54Mi ±  2%  +1017.65% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16     364.8Mi ± 45%    5027.2Mi ± 14%  +1277.89% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial             46.90Mi ±  8%     44.51Mi ±  2%     -5.08% (p=0.015 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16          46.70Mi ±  4%     44.13Mi ±  2%     -5.50% (p=0.005 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel           48.24Mi ±  3%     43.99Mi ±  5%     -8.81% (p=0.002 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16        53.56Mi ±  4%     51.48Mi ± 10%          ~ (p=0.190 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial           47.87Mi ±  5%    822.34Mi ± 14%  +1617.71% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16        47.88Mi ±  4%    825.63Mi ± 10%  +1624.40% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel         47.77Mi ±  5%    822.26Mi ±  6%  +1621.30% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16      69.82Mi ±  6%   4951.17Mi ± 35%  +6991.01% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial         45.79Mi ±  5%     46.72Mi ±  3%          ~ (p=0.529 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16      45.55Mi ± 10%     46.79Mi ±  5%          ~ (p=0.271 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel       44.85Mi ±  6%     47.63Mi ± 11%          ~ (p=0.143 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16    55.07Mi ±  6%     54.50Mi ±  6%          ~ (p=0.971 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial          45.36Mi ± 11%     43.64Mi ± 10%          ~ (p=0.143 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16       46.29Mi ±  9%     44.33Mi ± 13%          ~ (p=0.218 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel        44.86Mi ±  9%     44.26Mi ± 14%          ~ (p=0.247 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16     62.99Mi ± 14%     65.16Mi ± 29%          ~ (p=0.684 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial                29.88Mi ± 11%     29.99Mi ± 11%          ~ (p=0.836 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16             29.20Mi ±  7%     30.43Mi ± 13%          ~ (p=0.739 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel              28.83Mi ±  9%     29.75Mi ±  9%          ~ (p=0.190 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16           29.45Mi ±  8%     29.98Mi ± 12%          ~ (p=0.671 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial            46.80Mi ±  6%     40.78Mi ± 11%    -12.86% (p=0.004 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16         46.72Mi ±  9%     43.66Mi ± 12%     -6.55% (p=0.009 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel          46.55Mi ±  3%     43.94Mi ± 11%     -5.62% (p=0.005 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16       60.81Mi ±  8%     60.63Mi ± 12%          ~ (p=0.481 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial           75.26Mi ±  6%    815.53Mi ± 11%   +983.63% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16        70.26Mi ± 11%    825.45Mi ±  7%  +1074.82% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel         73.72Mi ±  9%    824.62Mi ±  8%  +1018.53% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      366.0Mi ±  8%    4956.8Mi ± 33%  +1254.21% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial              46.80Mi ±  5%     43.78Mi ± 11%     -6.45% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16           47.40Mi ±  4%     44.13Mi ±  1%     -6.91% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel            47.32Mi ±  3%     43.47Mi ±  8%     -8.12% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16         54.31Mi ±  3%     51.52Mi ±  4%     -5.14% (p=0.004 n=10)
geomean                                                                                                                          66.09Mi           145.7Mi         +120.41%

                                                                                                                               │     baseline     │                preflight                 │
                                                                                                                               │       B/op       │      B/op       vs base                  │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial            0.000 ±   0%       0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16         0.000 ±   0%       0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel         144.00 ±   0%       12.00 ±   8%  -91.67% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16     4864.00 ±  75%       61.00 ±  20%  -98.75% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial        20.00Mi ±   0%     20.00Mi ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16     20.00Mi ±   0%     20.00Mi ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel      20.00Mi ±   0%     20.00Mi ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16   20.00Mi ±   0%     20.00Mi ±   0%        ~ (p=0.148 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial         20.00Mi ±   0%     20.00Mi ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16      20.00Mi ±   0%     20.00Mi ±   0%        ~ (p=0.204 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel       20.00Mi ±   0%     20.00Mi ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16    20.00Mi ±   0%     20.00Mi ±   0%        ~ (p=0.136 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial                 0.000 ±   0%       0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16              0.000 ±    ?       0.000 ±   0%        ~ (p=0.582 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel               19.00 ±   5%       18.00 ±   6%        ~ (p=0.116 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16            41.00 ± 102%       41.50 ± 117%        ~ (p=0.778 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial           20.00Mi ±   0%     20.00Mi ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16        20.00Mi ±   0%     20.00Mi ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel         20.00Mi ±   0%     20.00Mi ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16      20.00Mi ±   0%     20.00Mi ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial            0.000 ±   0%       0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16         0.000 ±   0%       0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel         144.00 ±   0%       11.00 ±   9%  -92.36% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      491.00 ±  57%       11.00 ± 482%  -97.76% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial             40.00Mi ±   0%     40.00Mi ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16          40.00Mi ±   0%     40.00Mi ±   0%        ~ (p=0.628 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel           40.00Mi ±   0%     40.00Mi ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16        40.00Mi ±   0%     40.00Mi ±   0%        ~ (p=0.459 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial             0.000 ±   0%       0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16          0.000 ±   0%       0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel          144.00 ±   0%       12.00 ±   8%  -91.67% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16       688.00 ±   8%       37.50 ±  65%  -94.55% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial         20.01Mi ±   0%     20.01Mi ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16      20.01Mi ±   0%     20.01Mi ±   0%        ~ (p=0.303 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel       20.01Mi ±   0%     20.01Mi ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16    20.01Mi ±   0%     20.01Mi ±   0%        ~ (p=0.290 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial          20.01Mi ±   0%     20.01Mi ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16       20.01Mi ±   0%     20.01Mi ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel        20.01Mi ±   0%     20.01Mi ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16     20.01Mi ±   0%     20.01Mi ±   0%        ~ (p=0.352 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial                27.50Mi ±   0%     27.50Mi ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16             27.50Mi ±   0%     27.50Mi ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel              27.50Mi ±   0%     27.50Mi ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16           27.50Mi ±   0%     27.50Mi ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial            20.01Mi ±   0%     20.01Mi ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16         20.01Mi ±   0%     20.01Mi ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel          20.01Mi ±   0%     20.01Mi ±   0%        ~ (p=0.474 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16       20.01Mi ±   0%     20.01Mi ±   0%        ~ (p=0.866 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial             0.000 ±   0%       0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16          0.000 ±   0%       0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel          144.00 ±   0%       13.00 ±  15%  -90.97% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16       686.00 ±  73%       13.00 ± 331%  -98.10% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial              40.02Mi ±   0%     40.02Mi ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16           40.02Mi ±   0%     40.02Mi ±   0%   -0.00% (p=0.033 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel            40.02Mi ±   0%     40.02Mi ±   0%        ~ (p=0.474 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16         40.02Mi ±   0%     40.02Mi ±   0%        ~ (p=0.070 n=10)
geomean                                                                                                                                         ²                   -36.06%                ²
¹ all samples are equal
² summaries must be >0 to compute geomean

                                                                                                                               │    baseline    │                 preflight                 │
                                                                                                                               │   allocs/op    │  allocs/op    vs base                     │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial          0.000 ±   0%     0.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16       0.000 ±   0%     0.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel        3.000 ±   0%     0.000 ±   0%  -100.00% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16     26.00 ±  27%      0.00 ±   0%  -100.00% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial        1.000 ±   0%     1.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16     1.000 ±   0%     1.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel      4.000 ±   0%     4.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16   24.50 ±  18%     21.00 ±  29%         ~ (p=0.320 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial         1.000 ±   0%     1.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16      1.000 ± 100%     1.000 ±   0%         ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel       4.000 ±   0%     4.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16    19.00 ±  32%     19.00 ±   5%         ~ (p=0.587 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial               0.000 ±   0%     0.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16            0.000 ±   0%     0.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel             0.000 ±   0%     0.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16          0.000 ±   0%     0.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial           1.000 ±   0%     1.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16        1.000 ±   0%     1.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel         4.000 ±   0%     4.000 ±   0%         ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16      19.00 ±   5%     19.00 ±   5%         ~ (p=0.721 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial          0.000 ±   0%     0.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16       0.000 ±   0%     0.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel        3.000 ±   0%     0.000 ±   0%  -100.00% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16     4.000 ±  75%     0.000 ±   0%  -100.00% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial             2.000 ±   0%     2.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16          2.000 ±   0%     2.000 ±   0%         ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel           5.000 ±   0%     5.000 ±   0%         ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16        20.00 ±   5%     21.00 ±   5%         ~ (p=0.465 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial           0.000 ±   0%     0.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16        0.000 ±   0%     0.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel         3.000 ±   0%     0.000 ±   0%  -100.00% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16      18.00 ±   0%      0.00 ±   0%  -100.00% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial         1.000 ±   0%     1.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16      1.000 ±   0%     1.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel       4.000 ±   0%     4.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16    19.00 ±   5%     19.00 ±   0%         ~ (p=0.582 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial          1.000 ±   0%     1.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16       1.000 ±   0%     1.000 ±   0%         ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel        4.000 ±   0%     4.000 ±   0%         ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16     19.00 ±   5%     19.00 ±  26%         ~ (p=0.880 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial                1.000 ±   0%     1.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16             1.000 ±   0%     1.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel              38.00 ±   0%     38.00 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16           38.00 ±   0%     38.00 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial            1.000 ±   0%     1.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16         1.000 ±   0%     1.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel          4.000 ±   0%     4.000 ± 850%         ~ (p=0.474 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16       19.00 ±   5%     19.00 ±   5%         ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial           0.000 ±   0%     0.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16        0.000 ±   0%     0.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel         3.000 ±   0%     0.000 ±   0%  -100.00% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      4.000 ±  25%     0.000 ±   0%  -100.00% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial              2.000 ±   0%     2.000 ±   0%         ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16           2.000 ±   0%     2.000 ±   0%         ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel            5.000 ±   0%     5.000 ± 680%         ~ (p=0.474 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16         21.00 ±   5%     20.00 ±   5%         ~ (p=0.070 n=10)
geomean                                                                                                                                       ²                 ?                       ² ³
¹ all samples are equal
² summaries must be >0 to compute geomean
³ ratios must be >0 to compute geomean
REVERSE: preflight then baseline
goos: windows
goarch: amd64
pkg: github.com/DoingDog/cpa-plugin-censorship
cpu: AMD Ryzen 7 7840H with Radeon 780M Graphics
                                                                                                                               │  preflight   │                 baseline                 │
                                                                                                                               │    sec/op    │     sec/op      vs base                  │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial          24.62m ±  3%    420.27m ±  6%  +1606.87% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16       24.39m ±  2%    423.26m ±  6%  +1635.56% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel        24.09m ±  1%    435.28m ± 11%  +1706.83% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16     3.849m ±  9%   290.322m ± 17%  +7443.58% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial        438.8m ±  5%     438.5m ±  5%          ~ (p=0.912 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16     431.6m ±  8%     438.2m ± 11%          ~ (p=0.481 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel      429.8m ±  2%     433.7m ±  8%          ~ (p=0.529 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16   374.7m ±  3%     371.4m ±  9%          ~ (p=0.481 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial         455.4m ±  2%     442.2m ± 15%          ~ (p=0.684 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16      455.5m ±  2%     461.8m ±  9%          ~ (p=0.739 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel       456.7m ±  8%     453.4m ±  7%          ~ (p=0.684 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16    300.0m ± 70%     317.4m ± 13%          ~ (p=0.684 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial               36.15m ± 19%     36.78m ±  9%          ~ (p=0.436 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16            36.51m ± 19%     36.74m ± 10%          ~ (p=0.853 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel             36.30m ± 15%     36.30m ± 29%          ~ (p=0.971 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16          6.589m ± 71%     6.077m ± 76%          ~ (p=0.436 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial           465.7m ± 13%     436.6m ± 12%     -6.25% (p=0.007 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16        458.0m ± 12%     429.5m ±  5%     -6.23% (p=0.004 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel         456.1m ±  3%     426.2m ±  8%     -6.58% (p=0.011 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16      342.8m ±  9%     330.4m ±  3%          ~ (p=0.075 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial          24.42m ±  4%    312.71m ±  4%  +1180.55% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16       24.35m ± 29%    298.68m ±  5%  +1126.85% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel        24.34m ± 16%    308.08m ±  7%  +1165.86% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16     3.725m ± 63%    73.637m ± 17%  +1876.90% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial             477.4m ±  9%     437.8m ±  3%     -8.29% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16          466.4m ± 12%     444.8m ±  5%     -4.62% (p=0.035 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel           460.8m ± 18%     421.2m ±  3%     -8.59% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16        392.1m ± 30%     374.7m ±  6%     -4.45% (p=0.011 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial           24.30m ± 21%    428.98m ±  5%  +1665.21% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16        24.38m ± 12%    432.38m ±  4%  +1673.24% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel         24.46m ± 18%    441.38m ±  9%  +1704.78% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16      4.095m ± 54%   277.689m ± 32%  +6681.96% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial         431.0m ± 14%     443.1m ± 11%          ~ (p=0.393 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16      431.9m ±  4%     441.5m ±  9%          ~ (p=0.165 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel       423.9m ±  8%     438.4m ±  8%          ~ (p=0.529 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16    378.0m ±  7%     368.2m ±  4%          ~ (p=0.579 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial          455.1m ±  6%     435.0m ± 11%          ~ (p=0.315 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16       459.2m ±  6%     449.7m ±  9%          ~ (p=0.436 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel        463.9m ± 12%     415.5m ± 18%    -10.44% (p=0.029 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16     301.8m ± 20%     291.9m ± 28%          ~ (p=0.739 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial                691.9m ±  4%     695.7m ±  8%          ~ (p=0.315 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16             665.7m ±  5%     646.8m ± 16%          ~ (p=0.631 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel              659.1m ±  3%     679.2m ± 11%          ~ (p=0.089 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16           667.5m ±  3%     692.4m ±  8%     +3.74% (p=0.043 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial            450.2m ±  3%     431.9m ± 12%          ~ (p=0.353 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16         452.2m ±  2%     436.6m ± 10%          ~ (p=0.280 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel          450.1m ±  5%     437.1m ± 12%          ~ (p=0.481 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16       345.5m ±  5%     330.8m ±  7%          ~ (p=0.123 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial           24.40m ±  1%    304.84m ± 10%  +1149.38% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16        24.32m ±  2%    298.56m ±  9%  +1127.73% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel         24.24m ± 10%    312.77m ±  7%  +1190.31% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      3.707m ± 69%    73.146m ± 15%  +1873.41% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial              475.3m ±  9%     435.8m ±  3%     -8.29% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16           464.9m ±  9%     427.1m ±  5%     -8.13% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel            461.3m ± 15%     427.2m ±  3%     -7.38% (p=0.001 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16         389.7m ± 30%     379.0m ±  2%          ~ (p=0.190 n=10)
geomean                                                                                                                          138.3m           313.8m         +126.85%

                                                                                                                               │    preflight    │               baseline                │
                                                                                                                               │       B/s       │      B/s       vs base                │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial           812.28Mi ±  3%   47.59Mi ±  6%  -94.14% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16        820.09Mi ±  2%   47.25Mi ±  6%  -94.24% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel         830.19Mi ±  1%   45.95Mi ± 10%  -94.47% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16     5199.91Mi ±  8%   68.89Mi ± 15%  -98.68% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial          45.58Mi ±  5%   45.61Mi ±  5%        ~ (p=0.912 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16       46.34Mi ±  7%   45.65Mi ± 10%        ~ (p=0.481 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel        46.54Mi ±  2%   46.11Mi ±  7%        ~ (p=0.529 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16     53.38Mi ±  3%   53.85Mi ±  8%        ~ (p=0.481 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial           43.93Mi ±  2%   45.24Mi ± 13%        ~ (p=0.670 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16        43.91Mi ±  2%   43.31Mi ± 10%        ~ (p=0.699 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel         43.80Mi ±  7%   44.12Mi ±  7%        ~ (p=0.699 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16      66.66Mi ± 41%   63.24Mi ± 12%        ~ (p=0.684 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial                 553.3Mi ± 16%   543.7Mi ±  8%        ~ (p=0.436 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16              547.9Mi ± 16%   544.3Mi ±  9%        ~ (p=0.853 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel               551.0Mi ± 13%   551.0Mi ± 22%        ~ (p=0.971 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16            2.964Gi ± 42%   3.214Gi ± 43%        ~ (p=0.436 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial             42.97Mi ± 11%   45.81Mi ± 10%   +6.61% (p=0.006 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16          43.66Mi ± 10%   46.57Mi ±  5%   +6.66% (p=0.004 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel           43.85Mi ±  3%   46.94Mi ±  8%   +7.05% (p=0.011 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16        58.34Mi ±  9%   60.53Mi ±  3%        ~ (p=0.075 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial           819.01Mi ±  4%   63.97Mi ±  4%  -92.19% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16        821.52Mi ± 23%   66.96Mi ±  6%  -91.85% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel         821.77Mi ± 14%   64.93Mi ±  8%  -92.10% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      5369.3Mi ± 39%   271.7Mi ± 16%  -94.94% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial               41.91Mi ±  8%   45.68Mi ±  3%   +9.00% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16            42.88Mi ± 11%   44.96Mi ±  5%   +4.85% (p=0.035 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel             43.41Mi ± 15%   47.48Mi ±  3%   +9.39% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16          51.01Mi ± 23%   53.37Mi ±  6%   +4.64% (p=0.011 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial            822.98Mi ± 18%   46.63Mi ±  5%  -94.33% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16         820.23Mi ± 11%   46.26Mi ±  4%  -94.36% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel          817.79Mi ± 15%   45.31Mi ±  9%  -94.46% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16      4884.59Mi ± 35%   72.02Mi ± 24%  -98.53% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial           46.41Mi ± 12%   45.15Mi ± 10%        ~ (p=0.393 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16        46.31Mi ±  4%   45.30Mi ±  8%        ~ (p=0.165 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel         47.19Mi ±  7%   45.64Mi ±  7%        ~ (p=0.529 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16      52.90Mi ±  7%   54.33Mi ±  4%        ~ (p=0.579 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial            43.95Mi ±  6%   45.98Mi ± 10%        ~ (p=0.315 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16         43.55Mi ±  5%   44.51Mi ±  9%        ~ (p=0.436 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel          43.12Mi ± 10%   48.14Mi ± 15%  +11.66% (p=0.029 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16       66.27Mi ± 17%   68.59Mi ± 22%        ~ (p=0.739 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial                  28.91Mi ±  4%   28.75Mi ±  7%        ~ (p=0.315 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16               30.04Mi ±  5%   30.92Mi ± 14%        ~ (p=0.617 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel                30.35Mi ±  3%   29.44Mi ± 10%        ~ (p=0.089 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16             29.96Mi ±  3%   28.89Mi ±  7%   -3.58% (p=0.046 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial              44.42Mi ±  3%   46.31Mi ± 11%        ~ (p=0.353 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16           44.23Mi ±  2%   45.81Mi ±  9%        ~ (p=0.280 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel            44.45Mi ±  6%   45.76Mi ± 11%        ~ (p=0.481 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16         57.88Mi ±  6%   60.47Mi ±  7%        ~ (p=0.123 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial            819.70Mi ±  1%   65.61Mi ±  9%  -92.00% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16         822.42Mi ±  2%   66.99Mi ±  8%  -91.85% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel          825.10Mi ±  9%   63.95Mi ±  6%  -92.25% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16       5395.8Mi ± 41%   273.4Mi ± 18%  -94.93% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial                42.09Mi ±  9%   45.89Mi ±  3%   +9.02% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16             43.02Mi ±  8%   46.82Mi ±  5%   +8.85% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel              43.36Mi ± 13%   46.81Mi ±  3%   +7.95% (p=0.001 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16           51.33Mi ± 23%   52.77Mi ±  2%        ~ (p=0.190 n=10)
geomean                                                                                                                            144.6Mi         63.75Mi        -55.91%

                                                                                                                               │    preflight     │                 baseline                  │
                                                                                                                               │       B/op       │     B/op       vs base                    │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial            0.000 ±   0%       0.000 ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16         0.000 ±   0%       0.000 ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel          12.00 ±   8%      144.00 ±  0%  +1100.00% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16       52.50 ±  22%     3401.00 ± 71%  +6378.10% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial        20.00Mi ±   0%     20.00Mi ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16     20.00Mi ±   0%     20.00Mi ±  0%          ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel      20.00Mi ±   0%     20.00Mi ±  0%          ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16   20.00Mi ±   0%     20.00Mi ±  0%     +0.01% (p=0.049 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial         20.00Mi ±   0%     20.00Mi ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16      20.00Mi ±   0%     20.00Mi ±  0%          ~ (p=0.543 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel       20.00Mi ±   0%     20.00Mi ±  0%          ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16    20.00Mi ±   0%     20.00Mi ±  0%          ~ (p=0.780 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial                 0.000 ±   0%       0.000 ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16              0.000 ±    ?       0.000 ±  0%          ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel               19.00 ±   5%       18.00 ± 33%          ~ (p=0.379 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16            24.00 ± 288%       67.50 ± 76%          ~ (p=0.618 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial           20.00Mi ±   0%     20.00Mi ±  0%          ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16        20.00Mi ±   0%     20.00Mi ±  0%          ~ (p=0.474 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel         20.00Mi ±   0%     20.00Mi ±  0%          ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16      20.00Mi ±   0%     20.00Mi ±  0%          ~ (p=0.358 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial            0.000 ±   0%       0.000 ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16         0.000 ±   0%       0.000 ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel          12.00 ±   8%      144.00 ±  0%  +1100.00% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16       51.00 ±  80%      755.50 ± 67%  +1381.37% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial             40.00Mi ±   0%     40.00Mi ±  0%          ~ (p=0.211 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16          40.00Mi ±   0%     40.00Mi ±  0%          ~ (p=0.889 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel           40.00Mi ±   0%     40.00Mi ±  0%          ~ (p=0.211 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16        40.00Mi ±   0%     40.00Mi ±  0%          ~ (p=0.620 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial             0.000 ±   0%       0.000 ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16          0.000 ±   0%       0.000 ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel           12.00 ±  25%      144.00 ±  0%  +1100.00% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16        34.00 ±  74%      744.00 ±  8%  +2088.24% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial         20.01Mi ±   0%     20.01Mi ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16      20.01Mi ±   0%     20.01Mi ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel       20.01Mi ±   0%     20.01Mi ±  0%          ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16    20.01Mi ±   0%     20.01Mi ±  0%          ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial          20.01Mi ±   0%     20.01Mi ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16       20.01Mi ±   0%     20.01Mi ±  0%          ~ (p=0.363 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel        20.01Mi ±   0%     20.01Mi ±  0%          ~ (p=0.526 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16     20.01Mi ±   0%     20.01Mi ±  0%          ~ (p=0.670 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial                27.50Mi ±   0%     27.50Mi ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16             27.50Mi ±   0%     27.50Mi ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel              27.50Mi ±   0%     27.50Mi ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16           27.50Mi ±   0%     27.50Mi ±  0%          ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial            20.01Mi ±   0%     20.01Mi ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16         20.01Mi ±   0%     20.01Mi ±  0%          ~ (p=0.582 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel          20.01Mi ±   0%     20.01Mi ±  0%          ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16       20.01Mi ±   0%     20.01Mi ±  0%          ~ (p=0.191 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial             0.000 ±   0%       0.000 ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16          0.000 ±   0%       0.000 ±  0%          ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel           12.00 ±  33%      144.00 ±  0%  +1100.00% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16        9.500 ± 184%     684.000 ± 87%  +7100.00% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial              40.02Mi ±   0%     40.02Mi ±  0%          ~ (p=0.211 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16           40.02Mi ±   0%     40.02Mi ±  0%          ~ (p=0.127 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel            40.02Mi ±   0%     40.02Mi ±  0%          ~ (p=0.737 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16         40.02Mi ±   0%     40.02Mi ±  0%     -0.00% (p=0.011 n=10)
geomean                                                                                                                                         ²                    +56.69%                ²
¹ all samples are equal
² summaries must be >0 to compute geomean

                                                                                                                               │   preflight    │                baseline                │
                                                                                                                               │   allocs/op    │  allocs/op    vs base                  │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial          0.000 ±   0%     0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16       0.000 ±   0%     0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel        0.000 ±   0%     3.000 ±   0%        ? (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16      0.00 ±   0%     23.00 ±  22%        ? (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial        1.000 ±   0%     1.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16     1.000 ±   0%     1.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel      4.000 ±   0%     4.000 ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16   20.00 ±   5%     22.50 ±  20%  +12.50% (p=0.030 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial         1.000 ±   0%     1.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16      1.000 ± 100%     1.000 ± 100%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel       4.000 ±   0%     4.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16    19.50 ±  95%     20.00 ±  25%        ~ (p=0.889 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial               0.000 ±   0%     0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16            0.000 ±   0%     0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel             0.000 ±   0%     0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16          0.000 ±   0%     0.000 ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial           1.000 ±   0%     1.000 ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16        1.000 ±   0%     1.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel         4.000 ±   0%     4.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16      20.00 ±  30%     19.00 ±  16%        ~ (p=0.319 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial          0.000 ±   0%     0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16       0.000 ±   0%     0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel        0.000 ±   0%     3.000 ±   0%        ? (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16     0.000 ±   0%     6.000 ±  17%        ? (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial             2.000 ±  50%     2.000 ±   0%        ~ (p=0.211 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16          2.000 ±  50%     2.000 ±   0%        ~ (p=0.211 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel           5.000 ± 700%     5.000 ±   0%        ~ (p=0.211 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16        20.50 ±  95%     20.50 ±   2%        ~ (p=0.745 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial           0.000 ±   0%     0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=serial-16        0.000 ±   0%     0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel         0.000 ±   0%     3.000 ±   0%        ? (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16       0.00 ±   0%     18.00 ±   0%        ? (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial         1.000 ±   0%     1.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=serial-16      1.000 ±   0%     1.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel       4.000 ±   0%     4.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16    19.00 ±   0%     19.00 ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial          1.000 ±   0%     1.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=serial-16       1.000 ±   0%     1.000 ± 100%        ~ (p=0.582 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel        4.000 ±   0%     4.000 ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16     19.00 ±   0%     19.00 ±  32%        ~ (p=0.458 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial                1.000 ±   0%     1.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=serial-16             1.000 ±   0%     1.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel              38.00 ±   0%     38.00 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16           38.00 ±   0%     38.00 ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial            1.000 ±   0%     1.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=serial-16         1.000 ±   0%     1.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel          4.000 ±   0%     4.000 ±   0%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16       19.00 ±   0%     19.00 ±   5%        ~ (p=0.582 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial           0.000 ±   0%     0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=serial-16        0.000 ±   0%     0.000 ±   0%        ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel         0.000 ±   0%     3.000 ±   0%        ? (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      0.000 ±   0%     5.500 ±  27%        ? (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial              2.000 ±  50%     2.000 ±   0%        ~ (p=0.211 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=serial-16           2.500 ±  20%     2.000 ±   0%        ~ (p=0.119 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel            5.000 ± 680%     5.000 ±   0%        ~ (p=0.474 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16         21.00 ±  90%     20.00 ±   5%   -4.76% (p=0.011 n=10)
geomean                                                                                                                                       ²                 ?                      ²
¹ all samples are equal
² summaries must be >0 to compute geomean
```

#### Complete saturated RunParallel benchstat output

```plaintext
FORWARD: baseline then preflight
goos: windows
goarch: amd64
pkg: github.com/DoingDog/cpa-plugin-censorship
cpu: AMD Ryzen 7 7840H with Radeon 780M Graphics
                                                                                                                               │    baseline    │              preflight               │
                                                                                                                               │     sec/op     │    sec/op     vs base                │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16      96.959m ± 14%   4.676m ± 24%  -95.18% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16     461.4m ±  0%   464.2m ±  1%   +0.62% (p=0.043 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16      140.3m ± 20%   140.5m ± 17%        ~ (p=0.853 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16            11.33m ± 23%   10.32m ± 37%        ~ (p=0.393 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16        307.4m ±  4%   306.5m ±  5%        ~ (p=0.684 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      80.992m ± 46%   4.343m ±  4%  -94.64% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16          460.5m ±  0%   463.2m ±  1%   +0.60% (p=0.009 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16      143.389m ± 30%   5.848m ± 25%  -95.92% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16      463.9m ±  1%   469.3m ±  1%        ~ (p=0.105 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16       119.6m ± 38%   140.8m ± 28%        ~ (p=0.579 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16             643.5m ±  1%   635.8m ±  1%   -1.20% (p=0.029 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16         336.7m ± 11%   327.1m ± 20%        ~ (p=0.796 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16       98.929m ± 35%   6.155m ± 17%  -93.78% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16           460.6m ±  1%   464.6m ±  1%   +0.88% (p=0.009 n=10)
geomean                                                                                                                            187.0m         80.08m        -57.18%

                                                                                                                               │   baseline    │                preflight                 │
                                                                                                                               │      B/s      │      B/s        vs base                  │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16     206.3Mi ± 12%   4276.9Mi ± 19%  +1973.12% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16   43.35Mi ±  0%    43.08Mi ±  1%     -0.62% (p=0.041 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16    142.6Mi ± 25%    142.4Mi ± 17%          ~ (p=0.853 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16          1.724Gi ± 29%    1.896Gi ± 59%          ~ (p=0.393 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16      65.06Mi ±  3%    65.26Mi ±  5%          ~ (p=0.699 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16     247.0Mi ± 31%   4606.5Mi ±  4%  +1764.99% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16        43.43Mi ±  0%    43.18Mi ±  1%     -0.58% (p=0.011 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16      139.5Mi ± 42%   3464.3Mi ± 31%  +2382.80% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16    43.12Mi ±  1%    42.61Mi ±  1%          ~ (p=0.101 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16     167.6Mi ± 28%    142.1Mi ± 39%          ~ (p=0.579 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16           31.08Mi ±  2%    31.46Mi ±  1%     +1.21% (p=0.022 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16       59.40Mi ± 10%    61.14Mi ± 16%          ~ (p=0.796 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      202.2Mi ± 53%   3249.6Mi ± 21%  +1506.74% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16         43.42Mi ±  1%    43.04Mi ±  1%     -0.87% (p=0.009 n=10)
geomean                                                                                                                          107.0Mi          250.0Mi         +133.74%

                                                                                                                               │    baseline    │               preflight               │
                                                                                                                               │      B/op      │      B/op       vs base               │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16     1.431Ki ±   9%   1.509Ki ±  10%  +5.42% (p=0.013 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16   20.00Mi ±   0%   20.00Mi ±   0%       ~ (p=0.516 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16    20.00Mi ±   0%   20.00Mi ±   0%       ~ (p=0.700 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16            185.5 ±  77%     223.0 ±  81%       ~ (p=0.748 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16      20.00Mi ±   0%   20.00Mi ±   0%       ~ (p=0.238 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      142.00 ±  91%     58.00 ± 259%       ~ (p=0.300 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16        40.00Mi ±   0%   40.00Mi ±   0%       ~ (p=0.853 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16        95.50 ± 134%    110.50 ± 102%       ~ (p=0.987 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16    20.01Mi ±   0%   20.01Mi ±   0%       ~ (p=0.592 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16     20.01Mi ±   0%   20.01Mi ±   0%       ~ (p=0.616 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16           27.50Mi ±   0%   27.50Mi ±   0%       ~ (p=0.492 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16       20.01Mi ±   0%   20.01Mi ±   0%       ~ (p=0.402 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16        59.50 ± 255%     61.00 ± 315%       ~ (p=0.774 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16         40.02Mi ±   0%   40.02Mi ±   0%       ~ (p=0.118 n=10)
geomean                                                                                                                          362.6Ki          350.2Ki         -3.42%

                                                                                                                               │  baseline   │              preflight               │
                                                                                                                               │  allocs/op  │  allocs/op   vs base                 │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16     2.500 ± 20%   3.000 ±  0%       ~ (p=0.141 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16   2.000 ± 50%   2.000 ±  0%       ~ (p=0.474 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16    2.500 ± 20%   3.000 ± 33%       ~ (p=0.350 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16          1.000 ±  0%   1.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16      2.000 ±  0%   2.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16     1.000 ±  0%   1.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16        3.000 ±  0%   3.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16      1.000 ±  0%   1.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16    2.000 ±  0%   2.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16     2.000 ±  0%   2.000 ±  0%       ~ (p=1.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16           2.000 ±  0%   2.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16       2.000 ±  0%   2.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      1.000 ±  0%   1.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16         3.000 ±  0%   3.000 ±  0%       ~ (p=1.000 n=10) ¹
geomean                                                                                                                          1.795         1.842        +2.64%
¹ all samples are equal
REVERSE: preflight then baseline
goos: windows
goarch: amd64
pkg: github.com/DoingDog/cpa-plugin-censorship
cpu: AMD Ryzen 7 7840H with Radeon 780M Graphics
                                                                                                                               │  preflight   │                 baseline                 │
                                                                                                                               │    sec/op    │     sec/op      vs base                  │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16     4.264m ± 51%   103.355m ± 14%  +2323.87% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16   463.9m ±  1%     462.2m ±  1%          ~ (p=0.247 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16    149.0m ± 21%     145.3m ± 24%          ~ (p=0.684 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16          11.32m ± 37%     10.89m ± 33%          ~ (p=0.393 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16      317.0m ±  4%     313.8m ±  8%          ~ (p=0.436 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16     4.369m ±  7%    79.261m ± 20%  +1714.34% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16        466.3m ±  2%     464.7m ±  1%          ~ (p=0.315 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16      6.375m ± 10%   127.089m ± 68%  +1893.65% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16    470.4m ±  1%     467.3m ±  2%     -0.65% (p=0.015 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16     117.9m ± 32%     135.3m ± 23%          ~ (p=0.529 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16           637.8m ±  1%     643.3m ±  1%          ~ (p=0.190 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16       320.3m ± 16%     320.5m ±  8%          ~ (p=0.796 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      6.267m ± 20%   103.793m ± 37%  +1556.11% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16         468.8m ±  1%     462.6m ±  0%     -1.31% (p=0.000 n=10)
geomean                                                                                                                          80.22m           188.1m         +134.51%

                                                                                                                               │   preflight    │               baseline                │
                                                                                                                               │      B/s       │      B/s       vs base                │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16     4695.0Mi ± 34%   193.5Mi ± 12%  -95.88% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16    43.12Mi ±  1%   43.27Mi ±  1%        ~ (p=0.209 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16     134.3Mi ± 27%   137.7Mi ± 31%        ~ (p=0.684 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16           1.726Gi ± 59%   1.794Gi ± 48%        ~ (p=0.393 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16       63.10Mi ±  4%   63.74Mi ±  9%        ~ (p=0.436 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16     4578.3Mi ±  8%   252.9Mi ± 25%  -94.48% (p=0.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16         42.89Mi ±  2%   43.04Mi ±  1%        ~ (p=0.305 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16      3138.2Mi ± 11%   158.5Mi ± 41%  -94.95% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16     42.52Mi ±  1%   42.80Mi ±  2%   +0.65% (p=0.015 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16      169.9Mi ± 25%   148.3Mi ± 29%        ~ (p=0.529 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16            31.36Mi ±  1%   31.09Mi ±  1%        ~ (p=0.159 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16        62.47Mi ± 14%   62.41Mi ±  7%        ~ (p=0.796 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      3192.1Mi ± 16%   192.8Mi ± 59%  -93.96% (p=0.000 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16          42.67Mi ±  1%   43.23Mi ±  0%   +1.33% (p=0.000 n=10)
geomean                                                                                                                           249.4Mi         106.4Mi        -57.33%

                                                                                                                               │   preflight   │                baseline                │
                                                                                                                               │     B/op      │      B/op       vs base                │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16     1.516Ki ± 10%   1.506Ki ±   7%        ~ (p=0.052 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16   20.00Mi ±  0%   20.00Mi ±   0%        ~ (p=0.289 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16    20.00Mi ±  0%   20.00Mi ±   0%        ~ (p=0.796 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16            194.5 ± 78%     200.5 ±  79%        ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16      20.00Mi ±  0%   20.00Mi ±   0%        ~ (p=0.952 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      202.00 ± 79%     43.00 ± 384%        ~ (p=0.077 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16        40.00Mi ±  0%   40.00Mi ±   0%        ~ (p=0.924 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16        157.0 ± 73%     170.5 ±  73%        ~ (p=0.867 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16    20.01Mi ±  0%   20.01Mi ±   0%        ~ (p=0.670 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16     20.01Mi ±  0%   20.01Mi ±   0%        ~ (p=0.725 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16           27.50Mi ±  0%   27.50Mi ±   0%        ~ (p=0.060 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16       20.01Mi ±  0%   20.01Mi ±   0%        ~ (p=0.286 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16        185.5 ± 77%     140.5 ±  82%        ~ (p=0.808 n=10)
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16         40.02Mi ±  0%   40.02Mi ±   0%        ~ (p=0.060 n=10)
geomean                                                                                                                          421.1Ki         372.4Ki         -11.55%

                                                                                                                               │  preflight  │               baseline               │
                                                                                                                               │  allocs/op  │  allocs/op   vs base                 │
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16     3.000 ±  0%   3.000 ± 33%       ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16   2.000 ±  0%   2.000 ±  0%       ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16    3.000 ± 33%   3.000 ± 33%       ~ (p=1.000 n=10)
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16          1.000 ±  0%   1.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16      2.000 ±  0%   2.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16     1.000 ±  0%   1.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=strip/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16        3.000 ±  0%   3.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=total-miss/set=holdout/execution=parallel-16      1.000 ±  0%   1.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=first-sparse/set=holdout/execution=parallel-16    2.000 ±  0%   2.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=last-sparse/set=holdout/execution=parallel-16     2.000 ±  0%   2.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=dense/set=holdout/execution=parallel-16           2.000 ±  0%   2.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=duplicate/set=holdout/execution=parallel-16       2.000 ±  0%   2.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cross-span/set=holdout/execution=parallel-16      1.000 ±  0%   1.000 ±  0%       ~ (p=1.000 n=10) ¹
ExactRewritePreflight/mode=obfs/rules=1024/text=20971520/pattern=literal/match=cascade/set=holdout/execution=parallel-16         3.000 ±  0%   3.000 ±  0%       ~ (p=1.000 n=10) ¹
geomean                                                                                                                          1.842         1.842        +0.00%
¹ all samples are equal
```

### B. Fused UTF-8 and nesting validation

#### Decision

`REJECT`。候选将 `jsonNestingWithin` 与 `utf8.Valid` 合并为一次扫描，但 long ASCII、`ordinary-1k`、边界输入和 multibyte `parallel-16` 均违反性能 gate。`selectors.go` 保持原有两次独立校验。

#### Correctness oracle

先对 fused oracle 注入未闭合 string 处理错误，focused test 按预期失败：

```plaintext
--- FAIL: TestJSONNestingAndUTF8WithinMatchesCurrentValidation (0.02s)
    selectors_gemini_test.go:267: corpus-6-byte-0: fused validation = true, current validation = false for bytes 7a 22 75 6e 74 65 72 6d 69 6e 61 74 65 64 22 3a 22 76 61 6c 75 65 7d
FAIL
FAIL    github.com/DoingDog/cpa-plugin-censorship    0.079s
FAIL
```

恢复候选后运行：

```powershell
go test . -run '^TestJSONNestingAndUTF8WithinMatchesCurrentValidation$' -count=1
```

结果：

```plaintext
ok   github.com/DoingDog/cpa-plugin-censorship  0.135s
```

oracle 将候选与 `jsonNestingWithin(body, maxJSONNestingDepth) && utf8.Valid(body)` 逐项比较。corpus 覆盖 ASCII、有效 multibyte、转义 string、混合 scalar/object/array、1023/1024/1025 层、1 KiB、closure 错误和 invalid UTF-8 tail，并对每个 corpus body 的每个 byte 执行确定性单字节 mutation。

#### Measurement

- workload：20 MiB ASCII、20 MiB multibyte、invalid UTF-8 tail、depth boundary、escaped strings 和 1 KiB ordinary body。
- execution：serial、parallel、serial-16 和 parallel-16。
- 每种实现按 forward 和 reverse 两种相反顺序各采集 10 个独立 process sample。
- calibrated raw data 中每个统计 `RunParallel` sample 都满足 `b.N > active GOMAXPROCS`；smoke 的 `b.N=1` 未进入统计。
- 所有 serial rows 都是 `0 B/op, 0 allocs/op`。parallel `B/op` 差异来自 `RunParallel` 固定启动 allocation 按不同 `b.N` 摊销，不作为 candidate allocation 证据。

完整 `benchstat -col /impl` 输出：

```plaintext
FORWARD: baseline then fused
goos: windows
goarch: amd64
pkg: github.com/DoingDog/cpa-plugin-censorship
cpu: AMD Ryzen 7 7840H with Radeon 780M Graphics
                                                                │   baseline   │                 fused                 │
                                                                │    sec/op    │    sec/op      vs base                │
SelectorValidation/body=ascii-20m/execution=serial                19.93m ±  1%    24.09m ±  2%  +20.87% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=serial-16             19.92m ±  2%    23.90m ±  1%  +20.01% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=parallel              19.93m ±  1%    23.71m ±  1%  +18.98% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=parallel-16           2.607m ± 10%    3.310m ± 19%  +26.98% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=serial            30.14m ±  1%    27.33m ±  1%   -9.33% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=serial-16         30.18m ±  0%    27.57m ±  3%   -8.65% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=parallel          30.32m ±  2%    27.50m ±  1%   -9.30% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=parallel-16       4.191m ± 11%    4.589m ±  5%   +9.50% (p=0.029 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=serial        955.0n ±  1%   1202.0n ±  3%  +25.87% (p=0.000 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=serial-16     954.6n ±  1%   1204.0n ±  2%  +26.13% (p=0.000 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=parallel      954.0n ±  2%   1195.5n ±  2%  +25.31% (p=0.000 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=parallel-16   138.9n ±  8%    171.0n ±  9%  +23.11% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=serial           1.912µ ±  2%    2.331µ ±  2%  +21.95% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=serial-16        1.903µ ±  1%    2.327µ ±  2%  +22.28% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=parallel         1.898µ ±  1%    2.344µ ±  1%  +23.47% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=parallel-16      268.4n ± 10%    331.4n ±  5%  +23.51% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=serial         944.4n ±  2%   1165.5n ±  1%  +23.41% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=serial-16      936.7n ±  2%   1164.5n ±  2%  +24.32% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=parallel       929.4n ±  1%   1170.5n ±  2%  +25.95% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=parallel-16    127.8n ±  8%    172.5n ±  6%  +35.03% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=serial          40.60n ±  3%    58.53n ±  2%  +44.16% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=serial-16       40.23n ±  1%    58.77n ±  2%  +46.07% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=parallel        40.35n ±  3%    57.88n ±  1%  +43.46% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=parallel-16     5.836n ± 11%    8.412n ± 12%  +44.14% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=serial              946.5n ±  1%   1191.0n ±  2%  +25.83% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=serial-16           949.1n ±  1%   1194.5n ±  1%  +25.86% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=parallel            956.2n ±  1%   1206.0n ±  1%  +26.12% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=parallel-16         135.6n ±  5%    175.4n ± 12%  +29.39% (p=0.000 n=10)
geomean                                                           7.437µ          9.112µ        +22.52%

                                                                │    baseline    │                 fused                 │
                                                                │      B/s       │      B/s       vs base                │
SelectorValidation/body=ascii-20m/execution=serial                1003.6Mi ±  1%   830.3Mi ±  2%  -17.26% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=serial-16             1004.2Mi ±  2%   836.8Mi ±  1%  -16.67% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=parallel              1003.7Mi ±  1%   843.6Mi ±  1%  -15.95% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=parallel-16            7.493Gi ±  9%   5.902Gi ± 16%  -21.23% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=serial             663.6Mi ±  1%   731.9Mi ±  1%  +10.29% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=serial-16          662.7Mi ±  0%   725.5Mi ±  3%   +9.47% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=parallel           659.6Mi ±  2%   727.3Mi ±  1%  +10.26% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=parallel-16        4.661Gi ± 10%   4.257Gi ±  5%   -8.66% (p=0.029 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=serial        1023.6Mi ±  1%   813.1Mi ±  3%  -20.57% (p=0.000 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=serial-16     1024.1Mi ±  1%   811.8Mi ±  2%  -20.73% (p=0.000 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=parallel      1024.7Mi ±  2%   817.7Mi ±  2%  -20.20% (p=0.000 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=parallel-16    6.874Gi ±  8%   5.587Gi ±  9%  -18.73% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=serial           1022.4Mi ±  2%   838.3Mi ±  2%  -18.01% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=serial-16        1027.0Mi ±  1%   839.8Mi ±  2%  -18.22% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=parallel         1029.6Mi ±  1%   833.9Mi ±  1%  -19.01% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=parallel-16       7.112Gi ±  9%   5.757Gi ±  4%  -19.06% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=serial          2.023Gi ±  2%   1.639Gi ±  1%  -18.97% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=serial-16       2.039Gi ±  2%   1.640Gi ±  2%  -19.60% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=parallel        2.055Gi ±  1%   1.632Gi ±  2%  -20.60% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=parallel-16     14.96Gi ±  7%   11.07Gi ±  6%  -25.95% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=serial           869.1Mi ±  3%   602.9Mi ±  2%  -30.63% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=serial-16        877.2Mi ±  1%   600.5Mi ±  2%  -31.54% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=parallel         874.6Mi ±  3%   609.6Mi ±  1%  -30.30% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=parallel-16      5.905Gi ± 10%   4.096Gi ± 11%  -30.62% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=serial              1031.7Mi ±  1%   820.0Mi ±  2%  -20.52% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=serial-16           1028.9Mi ±  1%   817.6Mi ±  1%  -20.54% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=parallel            1021.3Mi ±  1%   809.7Mi ±  1%  -20.72% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=parallel-16          7.033Gi ±  6%   5.435Gi ± 11%  -22.72% (p=0.000 n=10)
geomean                                                            1.660Gi         1.355Gi        -18.38%

                                                                │    baseline    │                 fused                  │
                                                                │      B/op      │     B/op      vs base                  │
SelectorValidation/body=ascii-20m/execution=serial                0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=ascii-20m/execution=serial-16             0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10)
SelectorValidation/body=ascii-20m/execution=parallel              9.500 ±  16%     12.000 ±  8%  +26.32% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=parallel-16           38.50 ±   6%      52.00 ± 27%  +35.06% (p=0.008 n=10)
SelectorValidation/body=multibyte-20m/execution=serial            0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=multibyte-20m/execution=serial-16         0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=multibyte-20m/execution=parallel          16.00 ±   0%      14.00 ±  7%  -12.50% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=parallel-16       30.50 ± 130%      63.50 ± 51%        ~ (p=0.183 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=serial        0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=serial-16     0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=parallel      0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=parallel-16   0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=serial           0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=serial-16        0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=parallel         0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=parallel-16      0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=serial         0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=serial-16      0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=parallel       0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=parallel-16    0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=serial          0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=serial-16       0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=parallel        0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=parallel-16     0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=serial              0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=serial-16           0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=parallel            0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=parallel-16         0.000 ±   0%      0.000 ±  0%        ~ (p=1.000 n=10) ¹
geomean                                                                        ²                  +4.13%                ²
¹ all samples are equal
² summaries must be >0 to compute geomean

                                                                │   baseline   │                fused                │
                                                                │  allocs/op   │ allocs/op   vs base                 │
SelectorValidation/body=ascii-20m/execution=serial                0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ascii-20m/execution=serial-16             0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ascii-20m/execution=parallel              0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ascii-20m/execution=parallel-16           0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=multibyte-20m/execution=serial            0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=multibyte-20m/execution=serial-16         0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=multibyte-20m/execution=parallel          0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=multibyte-20m/execution=parallel-16       0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=serial        0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=serial-16     0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=parallel      0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=parallel-16   0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=serial           0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=serial-16        0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=parallel         0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=parallel-16      0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=serial         0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=serial-16      0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=parallel       0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=parallel-16    0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=serial          0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=serial-16       0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=parallel        0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=parallel-16     0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=serial              0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=serial-16           0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=parallel            0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=parallel-16         0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
geomean                                                                      ²               +0.00%                ²
¹ all samples are equal
² summaries must be >0 to compute geomean
REVERSE: fused then baseline
goos: windows
goarch: amd64
pkg: github.com/DoingDog/cpa-plugin-censorship
cpu: AMD Ryzen 7 7840H with Radeon 780M Graphics
                                                                │     fused     │               baseline               │
                                                                │    sec/op     │    sec/op     vs base                │
SelectorValidation/body=ascii-20m/execution=serial                 23.74m ±  2%   19.98m ±  2%  -15.84% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=serial-16              23.62m ±  2%   20.02m ±  1%  -15.22% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=parallel               23.86m ±  1%   19.96m ±  2%  -16.35% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=parallel-16            3.467m ± 10%   2.808m ±  7%  -19.01% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=serial             27.68m ±  1%   30.25m ±  1%   +9.28% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=serial-16          27.66m ±  1%   30.19m ±  1%   +9.17% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=parallel           27.56m ±  1%   30.17m ±  1%   +9.45% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=parallel-16        4.902m ± 12%   4.361m ±  9%  -11.03% (p=0.019 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=serial        1209.0n ±  2%   961.8n ±  2%  -20.45% (p=0.000 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=serial-16     1196.0n ±  1%   961.0n ±  1%  -19.64% (p=0.000 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=parallel      1205.0n ±  1%   956.8n ±  1%  -20.60% (p=0.000 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=parallel-16    174.7n ±  8%   136.8n ±  6%  -21.69% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=serial            2.339µ ±  1%   1.908µ ±  1%  -18.43% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=serial-16         2.325µ ±  1%   1.909µ ±  1%  -17.93% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=parallel          2.315µ ±  1%   1.895µ ±  1%  -18.14% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=parallel-16       366.6n ± 12%   273.2n ±  8%  -25.47% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=serial         1176.5n ±  1%   940.4n ±  1%  -20.07% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=serial-16      1162.5n ±  1%   939.3n ±  1%  -19.20% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=parallel       1171.0n ±  1%   940.5n ±  1%  -19.68% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=parallel-16     174.7n ± 10%   139.5n ±  8%  -20.15% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=serial           58.45n ±  1%   40.21n ±  3%  -31.20% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=serial-16        58.45n ±  1%   40.14n ±  2%  -31.33% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=parallel         58.20n ±  2%   39.92n ±  3%  -31.41% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=parallel-16      9.133n ± 10%   6.472n ± 10%  -29.14% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=serial              1198.5n ±  2%   961.6n ±  1%  -19.77% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=serial-16           1195.5n ±  1%   951.1n ±  1%  -20.45% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=parallel            1206.5n ±  2%   949.3n ±  1%  -21.32% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=parallel-16          178.2n ±  7%   131.2n ±  8%  -26.33% (p=0.000 n=10)
geomean                                                            9.228µ         7.517µ        -18.54%

                                                                │     fused     │                baseline                │
                                                                │      B/s      │      B/s        vs base                │
SelectorValidation/body=ascii-20m/execution=serial                842.4Mi ±  2%   1001.0Mi ±  2%  +18.82% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=serial-16             846.8Mi ±  2%    998.8Mi ±  1%  +17.95% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=parallel              838.3Mi ±  1%   1002.2Mi ±  2%  +19.55% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=parallel-16           5.633Gi ±  9%    6.957Gi ±  7%  +23.49% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=serial            722.5Mi ±  1%    661.1Mi ±  1%   -8.49% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=serial-16         723.2Mi ±  1%    662.5Mi ±  1%   -8.40% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=parallel          725.6Mi ±  1%    662.9Mi ±  1%   -8.64% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=parallel-16       3.984Gi ± 13%    4.480Gi ± 10%  +12.44% (p=0.019 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=serial        808.3Mi ±  2%   1016.4Mi ±  2%  +25.74% (p=0.000 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=serial-16     817.5Mi ±  1%   1017.1Mi ±  1%  +24.42% (p=0.000 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=parallel      811.3Mi ±  2%   1021.7Mi ±  1%  +25.93% (p=0.000 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=parallel-16   5.466Gi ±  8%    6.978Gi ±  6%  +27.67% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=serial           835.5Mi ±  1%   1024.2Mi ±  1%  +22.59% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=serial-16        840.2Mi ±  1%   1023.8Mi ±  1%  +21.85% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=parallel         844.1Mi ±  1%   1031.2Mi ±  1%  +22.15% (p=0.000 n=10)
SelectorValidation/body=depth-at-limit/execution=parallel-16      5.206Gi ± 14%    6.990Gi ±  8%  +34.26% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=serial         1.623Gi ±  1%    2.031Gi ±  2%  +25.15% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=serial-16      1.643Gi ±  1%    2.034Gi ±  1%  +23.81% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=parallel       1.631Gi ±  1%    2.031Gi ±  1%  +24.49% (p=0.000 n=10)
SelectorValidation/body=depth-over-limit/execution=parallel-16    10.94Gi ± 11%    13.70Gi ±  9%  +25.20% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=serial          603.7Mi ±  1%    877.4Mi ±  2%  +45.34% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=serial-16       603.6Mi ±  1%    879.1Mi ±  3%  +45.64% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=parallel        606.3Mi ±  2%    883.9Mi ±  3%  +45.80% (p=0.000 n=10)
SelectorValidation/body=escaped-strings/execution=parallel-16     3.774Gi ± 11%    5.325Gi ± 11%  +41.08% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=serial              815.0Mi ±  2%   1015.6Mi ±  1%  +24.61% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=serial-16           816.8Mi ±  1%   1026.8Mi ±  1%  +25.71% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=parallel            809.7Mi ±  2%   1028.7Mi ±  1%  +27.05% (p=0.000 n=10)
SelectorValidation/body=ordinary-1k/execution=parallel-16         5.353Gi ±  8%    7.266Gi ±  8%  +35.74% (p=0.000 n=10)
geomean                                                           1.337Gi          1.642Gi        +22.76%

                                                                │     fused      │               baseline                │
                                                                │      B/op      │    B/op      vs base                  │
SelectorValidation/body=ascii-20m/execution=serial                0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=ascii-20m/execution=serial-16             0.000 ±   0%     0.000 ±   ?        ~ (p=0.582 n=10)
SelectorValidation/body=ascii-20m/execution=parallel              12.00 ±   8%     10.00 ± 10%  -16.67% (p=0.000 n=10)
SelectorValidation/body=ascii-20m/execution=parallel-16           53.00 ±   6%     40.00 ± 15%  -24.53% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=serial            0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=multibyte-20m/execution=serial-16         0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=multibyte-20m/execution=parallel          15.00 ±   7%     16.00 ±  0%   +6.67% (p=0.000 n=10)
SelectorValidation/body=multibyte-20m/execution=parallel-16       16.00 ± 331%     39.50 ± 80%        ~ (p=0.540 n=10)
SelectorValidation/body=invalid-utf8-tail/execution=serial        0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=serial-16     0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=parallel      0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=parallel-16   0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=serial           0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=serial-16        0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=parallel         0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=parallel-16      0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=serial         0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=serial-16      0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=parallel       0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=parallel-16    0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=serial          0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=serial-16       0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=parallel        0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=parallel-16     0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=serial              0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=serial-16           0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=parallel            0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=parallel-16         0.000 ±   0%     0.000 ±  0%        ~ (p=1.000 n=10) ¹
geomean                                                                        ²                 +1.82%                ²
¹ all samples are equal
² summaries must be >0 to compute geomean

                                                                │    fused     │              baseline               │
                                                                │  allocs/op   │ allocs/op   vs base                 │
SelectorValidation/body=ascii-20m/execution=serial                0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ascii-20m/execution=serial-16             0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ascii-20m/execution=parallel              0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ascii-20m/execution=parallel-16           0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=multibyte-20m/execution=serial            0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=multibyte-20m/execution=serial-16         0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=multibyte-20m/execution=parallel          0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=multibyte-20m/execution=parallel-16       0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=serial        0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=serial-16     0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=parallel      0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=invalid-utf8-tail/execution=parallel-16   0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=serial           0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=serial-16        0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=parallel         0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-at-limit/execution=parallel-16      0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=serial         0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=serial-16      0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=parallel       0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=depth-over-limit/execution=parallel-16    0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=serial          0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=serial-16       0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=parallel        0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=escaped-strings/execution=parallel-16     0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=serial              0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=serial-16           0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=parallel            0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
SelectorValidation/body=ordinary-1k/execution=parallel-16         0.000 ± 0%     0.000 ± 0%       ~ (p=1.000 n=10) ¹
geomean                                                                      ²               +0.00%                ²
¹ all samples are equal
² summaries must be >0 to compute geomean
```

#### Rejection gate

- forward long ASCII 四种模式慢约 19% 到 27%，均为 `p=0.000`；reverse 方向一致。
- multibyte serial、serial-16 和 parallel 改善约 9%，但 `parallel-16` forward 慢 9.50%（`p=0.029`），reverse 方向一致。
- 1 KiB 四种模式 forward 慢约 25.8% 到 29.4%，reverse 方向一致。
- invalid UTF-8、depth boundary 和 escaped-string holdout 也出现显著回退。

候选及 benchmark-only oracle 已删除，没有 production 或 test diff。

### C. Inline duplicate-name storage

#### Decision

`REJECT`。`[4]string` 加 overflow map 在部分 many-small-object timing 上有局部收益，但 many-small allocation 没有改善，wide object 和 nested tool schema 明确增加 allocation，并违反 5% holdout gate。`hasDuplicateJSONMembers` 保持 object-local map。

#### Correctness oracle

普通 duplicate mutation 能被 oracle 捕获：

```plaintext
--- FAIL: TestInlineDuplicateWalkerMatchesMapWalker (0.00s)
    selectors_gemini_test.go:540: duplicate member: inline walker = false, production map walker = true
FAIL
FAIL    github.com/DoingDog/cpa-plugin-censorship    0.053s
FAIL
```

旧 benchmark 的 raw string 使用两个反斜杠，`case=escaped-equivalent-duplicate` 实际不是 Unicode-escaped equivalent key。下方原始完整 `benchstat` 中该 case 的历史 rows 只为保留审计记录，不参与任何 gate。

修正 fixture 后，第二个 member name bytes 为一个 `0x5C` 后接 ASCII `u0078`，JSON 解码后与第一个 key `x` 重复。clean oracle 覆盖该 key、普通与嵌套 duplicate、arrays、root/nested early termination、1023/1024/1025 深度 fixture，以及 10,000 个确定性 generated object/array input：

```powershell
go test . -run '^TestInlineDuplicateWalkerMatchesMapWalker$' -count=1
```

结果：

```plaintext
ok   github.com/DoingDog/cpa-plugin-censorship  0.155s
```

#### Original measurement

每种实现按 forward 和 reverse 两种相反顺序各采集 10 个独立 process sample，覆盖 serial、parallel、serial-16 和 parallel-16。除已明确排除的旧 escaped-equivalent rows 外，其余结果用于 gate。完整原始 `benchstat -col /impl` 输出：

```plaintext
goos: windows
goarch: amd64
pkg: github.com/DoingDog/cpa-plugin-censorship
cpu: AMD Ryzen 7 7840H with Radeon 780M Graphics
                                                                             │     map      │                inline                │
                                                                             │    sec/op    │    sec/op     vs base                │
DuplicateJSONMembers/case=two-field-100000/execution=serial                    18.78m ± 18%   16.68m ± 18%  -11.17% (p=0.023 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=serial-16                 18.67m ±  9%   16.29m ± 12%  -12.78% (p=0.011 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=parallel                  18.22m ± 14%   16.55m ± 10%   -9.17% (p=0.023 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=parallel-16               3.494m ± 24%   2.999m ± 27%        ~ (p=0.247 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=serial                   31.92m ±  9%   27.50m ± 14%  -13.85% (p=0.005 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=serial-16                32.83m ±  9%   28.30m ±  9%  -13.81% (p=0.002 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=parallel                 31.22m ± 17%   29.30m ± 12%   -6.15% (p=0.035 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=parallel-16              6.598m ± 31%   5.671m ± 27%        ~ (p=0.143 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial                         17.14µ ± 13%   18.29µ ± 24%        ~ (p=0.481 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial-16                      17.40µ ± 11%   17.59µ ± 12%        ~ (p=0.529 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel                       18.63µ ± 15%   20.18µ ± 21%        ~ (p=0.353 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel-16                    5.507µ ± 29%   5.584µ ± 23%        ~ (p=0.912 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial                 150.6µ ±  9%   142.1µ ± 10%        ~ (p=0.481 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial-16              146.6µ ± 14%   142.6µ ± 14%        ~ (p=0.436 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel               146.4µ ± 19%   158.3µ ± 12%        ~ (p=0.481 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel-16            30.59µ ± 23%   34.87µ ± 21%        ~ (p=0.123 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        415.2n ± 13%   360.1n ±  7%  -13.27% (p=0.003 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     424.2n ± 14%   405.5n ± 11%        ~ (p=0.393 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      393.9n ± 14%   362.0n ± 18%        ~ (p=0.631 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   118.8n ± 25%   108.1n ± 19%        ~ (p=0.481 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=serial                      17.91µ ± 11%   18.79µ ± 11%        ~ (p=0.631 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=serial-16                   20.17µ ± 16%   22.16µ ± 22%        ~ (p=0.165 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel                    18.06µ ± 12%   17.93µ ± 14%        ~ (p=1.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel-16                 5.395µ ± 23%   5.431µ ± 23%        ~ (p=0.739 n=10)
geomean                                                                        106.2µ         102.1µ         -3.88%

                                                                             │      map      │                inline                 │
                                                                             │      B/s      │      B/s       vs base                │
DuplicateJSONMembers/case=two-field-100000/execution=serial                    71.12Mi ± 15%   80.04Mi ± 15%  +12.54% (p=0.023 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=serial-16                 71.51Mi ±  9%   81.99Mi ± 11%  +14.66% (p=0.011 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=parallel                  73.29Mi ± 12%   80.71Mi ± 11%  +10.12% (p=0.023 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=parallel-16               382.5Mi ± 31%   446.3Mi ± 36%        ~ (p=0.247 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=serial                   77.68Mi ±  8%   90.17Mi ± 12%  +16.08% (p=0.005 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=serial-16                75.52Mi ± 10%   87.62Mi ± 10%  +16.02% (p=0.002 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=parallel                 79.52Mi ± 15%   84.62Mi ± 14%   +6.42% (p=0.035 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=parallel-16              375.8Mi ± 44%   437.4Mi ± 38%        ~ (p=0.143 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial                         66.17Mi ± 12%   61.98Mi ± 20%        ~ (p=0.481 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial-16                      65.18Mi ± 12%   64.48Mi ± 11%        ~ (p=0.529 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel                       60.98Mi ± 18%   56.20Mi ± 26%        ~ (p=0.353 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel-16                    206.0Mi ± 41%   203.1Mi ± 29%        ~ (p=0.912 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial                 135.8Mi ± 10%   143.9Mi ±  9%        ~ (p=0.494 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial-16              139.5Mi ± 12%   143.4Mi ± 12%        ~ (p=0.436 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel               139.7Mi ± 16%   129.2Mi ± 14%        ~ (p=0.481 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel-16            668.4Mi ± 25%   586.6Mi ± 26%        ~ (p=0.123 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        43.66Mi ± 15%   50.33Mi ±  8%  +15.29% (p=0.003 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     42.73Mi ± 16%   44.68Mi ± 13%        ~ (p=0.393 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      46.03Mi ± 16%   50.05Mi ± 15%        ~ (p=0.631 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   152.4Mi ± 34%   167.7Mi ± 23%        ~ (p=0.481 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=serial                      63.89Mi ± 10%   60.92Mi ± 12%        ~ (p=0.631 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=serial-16                   56.76Mi ± 20%   51.64Mi ± 29%        ~ (p=0.165 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel                    63.37Mi ± 11%   63.85Mi ± 13%        ~ (p=1.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel-16                 212.2Mi ± 29%   210.8Mi ± 29%        ~ (p=0.739 n=10)
geomean                                                                        101.8Mi         105.9Mi         +4.02%

                                                                             │       map        │                 inline                  │
                                                                             │       B/op       │      B/op       vs base                 │
DuplicateJSONMembers/case=two-field-100000/execution=serial                      0.000 ±   0%       0.000 ±   0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=two-field-100000/execution=serial-16                   0.000 ±   0%       0.000 ±   0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=two-field-100000/execution=parallel                   10.000 ±  20%       9.000 ±  11%       ~ (p=0.207 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=parallel-16                 42.50 ±  27%       35.00 ±  66%       ~ (p=0.184 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=serial                     0.000 ±   0%       0.000 ±   0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=four-field-100000/execution=serial-16                  0.000 ±   0%       0.000 ±   0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=four-field-100000/execution=parallel                   17.00 ±  24%       15.50 ±  16%       ~ (p=0.108 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=parallel-16                24.50 ± 320%       17.00 ± 371%       ~ (p=0.423 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial                         12.95Ki ±   0%     13.20Ki ±   0%  +1.93% (p=0.000 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial-16                      12.95Ki ±   0%     13.20Ki ±   0%  +1.93% (p=0.000 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel                       12.95Ki ±   0%     13.20Ki ±   0%  +1.93% (p=0.000 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel-16                    12.95Ki ±   0%     13.20Ki ±   0%  +1.93% (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial                  0.00Ki ±   0%     16.00Ki ±   0%       ? (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial-16               0.00Ki ±   0%     16.00Ki ±   0%       ? (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel                0.00Ki ±   0%     16.00Ki ±   0%       ? (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel-16             0.00Ki ±   0%     16.00Ki ±   0%       ? (p=0.000 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial          208.0 ±   0%       208.0 ±   0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16       208.0 ±   0%       208.0 ±   0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel        208.0 ±   0%       208.0 ±   0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16     208.0 ±   0%       208.0 ±   0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=late-duplicate/execution=serial                      12.95Ki ±   0%     13.20Ki ±   0%  +1.93% (p=0.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=serial-16                   12.95Ki ±   0%     13.20Ki ±   0%  +1.93% (p=0.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel                    12.95Ki ±   0%     13.20Ki ±   0%  +1.93% (p=0.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel-16                 12.95Ki ±   0%     13.20Ki ±   0%  +1.93% (p=0.000 n=10)
geomean                                                                                       ²                   ?                     ²
¹ all samples are equal
² summaries must be >0 to compute geomean

                                                                             │     map      │                inline                │
                                                                             │  allocs/op   │ allocs/op   vs base                  │
DuplicateJSONMembers/case=two-field-100000/execution=serial                    0.000 ± 0%     0.000 ± 0%        ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=two-field-100000/execution=serial-16                 0.000 ± 0%     0.000 ± 0%        ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=two-field-100000/execution=parallel                  0.000 ± 0%     0.000 ± 0%        ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=two-field-100000/execution=parallel-16               0.000 ± 0%     0.000 ± 0%        ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=four-field-100000/execution=serial                   0.000 ± 0%     0.000 ± 0%        ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=four-field-100000/execution=serial-16                0.000 ± 0%     0.000 ± 0%        ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=four-field-100000/execution=parallel                 0.000 ± 0%     0.000 ± 0%        ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=four-field-100000/execution=parallel-16              0.000 ± 0%     0.000 ± 0%        ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=wide-object/execution=serial                         11.00 ± 0%     13.00 ± 0%  +18.18% (p=0.000 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial-16                      11.00 ± 0%     13.00 ± 0%  +18.18% (p=0.000 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel                       11.00 ± 0%     13.00 ± 0%  +18.18% (p=0.000 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel-16                    11.00 ± 0%     13.00 ± 0%  +18.18% (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial                   0.0 ± 0%     128.0 ± 0%        ? (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial-16                0.0 ± 0%     128.0 ± 0%        ? (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel                 0.0 ± 0%     128.0 ± 0%        ? (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel-16              0.0 ± 0%     128.0 ± 0%        ? (p=0.000 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        6.000 ± 0%     6.000 ± 0%        ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     6.000 ± 0%     6.000 ± 0%        ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      6.000 ± 0%     6.000 ± 0%        ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   6.000 ± 0%     6.000 ± 0%        ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=late-duplicate/execution=serial                      11.00 ± 0%     13.00 ± 0%  +18.18% (p=0.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=serial-16                   11.00 ± 0%     13.00 ± 0%  +18.18% (p=0.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel                    11.00 ± 0%     13.00 ± 0%  +18.18% (p=0.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel-16                 11.00 ± 0%     13.00 ± 0%  +18.18% (p=0.000 n=10)
geomean                                                                                   ²               ?                      ²
¹ all samples are equal
² summaries must be >0 to compute geomean

Reverse order: inline first, map second

goos: windows
goarch: amd64
pkg: github.com/DoingDog/cpa-plugin-censorship
cpu: AMD Ryzen 7 7840H with Radeon 780M Graphics
                                                                             │    inline    │                 map                  │
                                                                             │    sec/op    │    sec/op     vs base                │
DuplicateJSONMembers/case=two-field-100000/execution=serial                    16.40m ± 13%   19.91m ± 12%  +21.41% (p=0.001 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=serial-16                 16.32m ± 11%   19.56m ±  8%  +19.84% (p=0.001 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=parallel                  16.37m ±  8%   19.07m ±  9%  +16.53% (p=0.002 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=parallel-16               3.025m ± 14%   3.552m ± 32%        ~ (p=0.089 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=serial                   30.78m ± 14%   31.73m ± 20%        ~ (p=0.052 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=serial-16                29.35m ±  8%   34.15m ± 11%  +16.36% (p=0.000 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=parallel                 28.00m ±  5%   32.62m ±  9%  +16.53% (p=0.000 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=parallel-16              6.072m ± 20%   6.841m ± 37%        ~ (p=0.105 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial                         18.28µ ± 10%   17.70µ ± 15%        ~ (p=0.631 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial-16                      18.52µ ±  7%   17.96µ ±  9%        ~ (p=0.436 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel                       18.93µ ± 11%   19.50µ ± 20%        ~ (p=0.971 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel-16                    5.843µ ± 16%   5.936µ ± 29%        ~ (p=0.796 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial                 152.5µ ± 16%   156.5µ ± 21%        ~ (p=0.853 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial-16              153.4µ ±  8%   154.8µ ± 10%        ~ (p=0.853 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel               153.1µ ±  8%   157.2µ ± 11%        ~ (p=0.481 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel-16            39.39µ ± 21%   33.19µ ± 11%  -15.75% (p=0.029 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        373.1n ±  6%   414.7n ± 10%  +11.15% (p=0.002 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     398.9n ±  9%   416.7n ±  9%        ~ (p=0.393 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      394.3n ± 16%   426.4n ± 14%   +8.13% (p=0.043 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   122.3n ± 17%   123.6n ±  8%        ~ (p=0.280 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=serial                      18.53µ ±  6%   18.74µ ±  9%        ~ (p=0.481 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=serial-16                   22.29µ ± 10%   21.52µ ± 14%        ~ (p=0.393 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel                    19.89µ ±  9%   19.27µ ± 11%        ~ (p=0.353 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel-16                 5.926µ ± 27%   5.466µ ± 10%        ~ (p=0.165 n=10)
geomean                                                                        105.7µ         110.7µ         +4.77%

                                                                             │    inline     │                  map                  │
                                                                             │      B/s      │      B/s       vs base                │
DuplicateJSONMembers/case=two-field-100000/execution=serial                    81.41Mi ± 11%   67.05Mi ± 11%  -17.64% (p=0.001 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=serial-16                 81.83Mi ± 10%   68.27Mi ±  8%  -16.57% (p=0.001 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=parallel                  81.66Mi ±  7%   70.00Mi ± 10%  -14.28% (p=0.002 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=parallel-16               441.6Mi ± 13%   376.2Mi ± 47%        ~ (p=0.089 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=serial                   80.55Mi ± 16%   78.14Mi ± 17%        ~ (p=0.052 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=serial-16                84.49Mi ±  8%   72.63Mi ± 12%  -14.04% (p=0.000 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=parallel                 88.57Mi ±  5%   76.01Mi ± 10%  -14.18% (p=0.000 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=parallel-16              408.8Mi ± 25%   362.9Mi ± 57%        ~ (p=0.105 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial                         62.07Mi ±  9%   64.07Mi ± 15%        ~ (p=0.631 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial-16                      61.23Mi ±  7%   63.15Mi ±  9%        ~ (p=0.436 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel                       59.91Mi ± 10%   58.22Mi ± 25%        ~ (p=0.971 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel-16                    194.1Mi ± 19%   191.0Mi ± 41%        ~ (p=0.796 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial                 134.1Mi ± 14%   130.8Mi ± 17%        ~ (p=0.853 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial-16              133.3Mi ±  8%   132.2Mi ± 11%        ~ (p=0.853 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel               133.6Mi ±  8%   130.1Mi ± 13%        ~ (p=0.481 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel-16            519.1Mi ± 27%   616.2Mi ± 13%  +18.70% (p=0.029 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        48.57Mi ±  6%   43.71Mi ±  9%  -10.01% (p=0.002 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     45.43Mi ±  9%   43.49Mi ± 10%        ~ (p=0.393 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      45.95Mi ± 19%   42.51Mi ± 17%   -7.49% (p=0.043 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   148.2Mi ± 21%   146.5Mi ±  7%        ~ (p=0.280 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=serial                      61.76Mi ±  5%   61.08Mi ±  8%        ~ (p=0.481 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=serial-16                   51.41Mi ±  9%   53.22Mi ± 16%        ~ (p=0.393 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel                    57.54Mi ± 10%   59.38Mi ± 13%        ~ (p=0.325 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel-16                 193.1Mi ± 36%   209.4Mi ±  9%        ~ (p=0.165 n=10)
geomean                                                                        102.3Mi         97.64Mi         -4.54%

                                                                             │     inline      │                    map                     │
                                                                             │      B/op       │     B/op       vs base                     │
DuplicateJSONMembers/case=two-field-100000/execution=serial                      0.000 ±  0%       0.000 ±  0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=two-field-100000/execution=serial-16                   0.000 ±  0%       0.000 ±  0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=two-field-100000/execution=parallel                    9.000 ± 11%      10.000 ± 20%   +11.11% (p=0.042 n=10)
DuplicateJSONMembers/case=two-field-100000/execution=parallel-16                 40.50 ± 19%       52.00 ± 44%         ~ (p=0.086 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=serial                     0.000 ±  0%       0.000 ±  0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=four-field-100000/execution=serial-16                  0.000 ±  0%       0.000 ±  0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=four-field-100000/execution=parallel                   16.00 ± 38%       17.50 ± 20%    +9.38% (p=0.035 n=10)
DuplicateJSONMembers/case=four-field-100000/execution=parallel-16                79.50 ± 80%       78.00 ± 76%         ~ (p=0.684 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial                         13.20Ki ±  0%     12.95Ki ±  0%    -1.89% (p=0.000 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial-16                      13.20Ki ±  0%     12.95Ki ±  0%    -1.89% (p=0.000 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel                       13.20Ki ±  0%     12.95Ki ±  0%    -1.89% (p=0.000 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel-16                    13.20Ki ±  0%     12.95Ki ±  0%    -1.89% (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial                 16.00Ki ±  0%      0.00Ki ±  0%  -100.00% (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial-16              16.00Ki ±  0%      0.00Ki ±  0%  -100.00% (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel               16.00Ki ±  0%      0.00Ki ±  0%  -100.00% (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel-16            16.00Ki ±  0%      0.00Ki ±  0%  -100.00% (p=0.000 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial          208.0 ±  0%       208.0 ±  0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16       208.0 ±  0%       208.0 ±  0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel        208.0 ±  0%       208.0 ±  0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16     208.0 ±  0%       208.0 ±  0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=late-duplicate/execution=serial                      13.20Ki ±  0%     12.95Ki ±  0%    -1.89% (p=0.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=serial-16                   13.20Ki ±  0%     12.95Ki ±  0%    -1.89% (p=0.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel                    13.20Ki ±  0%     12.95Ki ±  0%    -1.89% (p=0.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel-16                 13.20Ki ±  0%     12.95Ki ±  0%    -1.89% (p=0.000 n=10)
geomean                                                                                      ²                  ?                       ² ³
¹ all samples are equal
² summaries must be >0 to compute geomean
³ ratios must be >0 to compute geomean

                                                                             │    inline    │                   map                   │
                                                                             │  allocs/op   │ allocs/op   vs base                     │
DuplicateJSONMembers/case=two-field-100000/execution=serial                    0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=two-field-100000/execution=serial-16                 0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=two-field-100000/execution=parallel                  0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=two-field-100000/execution=parallel-16               0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=four-field-100000/execution=serial                   0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=four-field-100000/execution=serial-16                0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=four-field-100000/execution=parallel                 0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=four-field-100000/execution=parallel-16              0.000 ± 0%     0.000 ± 0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=wide-object/execution=serial                         13.00 ± 0%     11.00 ± 0%   -15.38% (p=0.000 n=10)
DuplicateJSONMembers/case=wide-object/execution=serial-16                      13.00 ± 0%     11.00 ± 0%   -15.38% (p=0.000 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel                       13.00 ± 0%     11.00 ± 0%   -15.38% (p=0.000 n=10)
DuplicateJSONMembers/case=wide-object/execution=parallel-16                    13.00 ± 0%     11.00 ± 0%   -15.38% (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial                 128.0 ± 0%       0.0 ± 0%  -100.00% (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=serial-16              128.0 ± 0%       0.0 ± 0%  -100.00% (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel               128.0 ± 0%       0.0 ± 0%  -100.00% (p=0.000 n=10)
DuplicateJSONMembers/case=nested-tool-schemas/execution=parallel-16            128.0 ± 0%       0.0 ± 0%  -100.00% (p=0.000 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        6.000 ± 0%     6.000 ± 0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     6.000 ± 0%     6.000 ± 0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      6.000 ± 0%     6.000 ± 0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   6.000 ± 0%     6.000 ± 0%         ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=late-duplicate/execution=serial                      13.00 ± 0%     11.00 ± 0%   -15.38% (p=0.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=serial-16                   13.00 ± 0%     11.00 ± 0%   -15.38% (p=0.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel                    13.00 ± 0%     11.00 ± 0%   -15.38% (p=0.000 n=10)
DuplicateJSONMembers/case=late-duplicate/execution=parallel-16                 13.00 ± 0%     11.00 ± 0%   -15.38% (p=0.000 n=10)
geomean                                                                                   ²               ?                       ² ³
¹ all samples are equal
² summaries must be >0 to compute geomean
³ ratios must be >0 to compute geomean
```

#### Corrected escaped-equivalent measurement

只重跑已证明无效的 escaped-equivalent workload。forward 的 10 个 process pair 都是 `map` 后 `inline`；reverse 的 10 个 process pair 都是 `inline` 后 `map`。每个 process 使用 `-cpu=1,16`。80 条统计 parallel row 全部满足 `b.N > active GOMAXPROCS`。

Forward：

```plaintext
order: map then inline
command: go test . -run '^$' -bench '^BenchmarkDuplicateJSONMembers/impl=<impl>/' -benchtime=1s -count=1 -cpu=1,16
goos: windows
goarch: amd64
pkg: github.com/DoingDog/cpa-plugin-censorship
cpu: AMD Ryzen 7 7840H with Radeon 780M Graphics
                                                                             │     map      │                inline                │
                                                                             │    sec/op    │    sec/op     vs base                │
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        747.6n ± 17%   646.7n ± 25%        ~ (p=0.165 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     781.3n ± 18%   686.3n ± 13%        ~ (p=0.063 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      809.6n ± 19%   733.8n ± 26%        ~ (p=0.165 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   223.4n ± 15%   208.5n ± 14%   -6.65% (p=0.015 n=10)
geomean                                                                        570.1n         510.5n        -10.46%

                                                                             │      map      │                inline                 │
                                                                             │      B/s      │      B/s       vs base                │
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        22.97Mi ± 21%   26.55Mi ± 20%        ~ (p=0.165 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     22.07Mi ± 21%   25.01Mi ± 11%        ~ (p=0.063 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      21.22Mi ± 23%   23.40Mi ± 35%        ~ (p=0.165 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   76.84Mi ± 18%   82.31Mi ± 17%   +7.11% (p=0.015 n=10)
geomean                                                                        30.15Mi         33.63Mi        +11.51%

                                                                             │    map     │               inline                │
                                                                             │    B/op    │    B/op     vs base                 │
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        200.0 ± 0%   200.0 ± 0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     200.0 ± 0%   200.0 ± 0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      200.0 ± 0%   200.0 ± 0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   200.0 ± 0%   200.0 ± 0%       ~ (p=1.000 n=10) ¹
geomean                                                                        200.0        200.0       +0.00%
¹ all samples are equal

                                                                             │    map     │               inline                │
                                                                             │ allocs/op  │ allocs/op   vs base                 │
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        5.000 ± 0%   5.000 ± 0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     5.000 ± 0%   5.000 ± 0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      5.000 ± 0%   5.000 ± 0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   5.000 ± 0%   5.000 ± 0%       ~ (p=1.000 n=10) ¹
geomean                                                                        5.000        5.000       +0.00%
¹ all samples are equal
```

Reverse：

```plaintext
order: inline then map
command: go test . -run '^$' -bench '^BenchmarkDuplicateJSONMembers/impl=<impl>/' -benchtime=1s -count=1 -cpu=1,16
goos: windows
goarch: amd64
pkg: github.com/DoingDog/cpa-plugin-censorship
cpu: AMD Ryzen 7 7840H with Radeon 780M Graphics
                                                                             │    inline    │                 map                 │
                                                                             │    sec/op    │    sec/op     vs base               │
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        736.1n ± 54%   761.0n ± 53%       ~ (p=0.739 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     731.9n ± 57%   659.9n ± 50%       ~ (p=0.353 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      742.1n ± 57%   652.4n ± 51%       ~ (p=0.684 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   210.5n ± 50%   220.6n ± 50%       ~ (p=0.184 n=10)
geomean                                                                        538.6n         518.5n        -3.74%

                                                                             │     inline     │                  map                  │
                                                                             │      B/s       │      B/s        vs base               │
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        23.46Mi ± 117%   22.56Mi ± 111%       ~ (p=0.739 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     23.56Mi ± 133%   26.10Mi ± 100%       ~ (p=0.353 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      23.17Mi ± 130%   26.38Mi ± 101%       ~ (p=0.684 n=10)
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   81.54Mi ± 100%   77.82Mi ± 102%       ~ (p=0.184 n=10)
geomean                                                                        31.96Mi          33.16Mi         +3.74%

                                                                             │   inline   │                 map                 │
                                                                             │    B/op    │    B/op     vs base                 │
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        200.0 ± 0%   200.0 ± 0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     200.0 ± 0%   200.0 ± 0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      200.0 ± 0%   200.0 ± 0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   200.0 ± 0%   200.0 ± 0%       ~ (p=1.000 n=10) ¹
geomean                                                                        200.0        200.0       +0.00%
¹ all samples are equal

                                                                             │   inline   │                 map                 │
                                                                             │ allocs/op  │ allocs/op   vs base                 │
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial        5.000 ± 0%   5.000 ± 0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=serial-16     5.000 ± 0%   5.000 ± 0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel      5.000 ± 0%   5.000 ± 0%       ~ (p=1.000 n=10) ¹
DuplicateJSONMembers/case=escaped-equivalent-duplicate/execution=parallel-16   5.000 ± 0%   5.000 ± 0%       ~ (p=1.000 n=10) ¹
geomean                                                                        5.000        5.000       +0.00%
¹ all samples are equal
```

corrected workload 只有 forward `parallel-16` 显著改善，reverse 四种模式均不显著；两种顺序所有模式都是 `200 B/op, 5 allocs/op`。该结果不改变 `REJECT`。

#### Rejection gate

1. many-small-object timing 没有在全部执行方式和两个顺序中显著改善。two-field 与 four-field 的 `parallel-16` 均不显著，four-field reverse serial 为 `p=0.052`。
2. many-small-object allocation 没有改善。two-field 与 four-field 的全部执行方式均为 `0 -> 0 allocs/op`。
3. memory gate 失败。wide object 约为 `12.95 KiB -> 13.20 KiB`、`11 -> 13 allocs/op`；nested tool schemas 为 `0 B/op, 0 allocs/op -> 16 KiB/op, 128 allocs/op`。这些回退出现在 serial rows。
4. 5% holdout gate 失败。forward wide serial 约回退 6.7%，forward wide parallel 约回退 8.3%。

#### Review and cleanup

初始 scoped review 的两项 Important 和两项 Minor finding 已在 fix round 1 中处理。Opus xhigh scoped re-review 的 spec verdict 和 quality verdict 均为 `PASS`。临时 candidate、oracle、fixture helper 和 benchmark 均已删除；Task 7 没有保留 production 或 test change。

## Final verification
