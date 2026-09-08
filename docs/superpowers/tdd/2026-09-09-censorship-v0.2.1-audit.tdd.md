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

## Final verification
