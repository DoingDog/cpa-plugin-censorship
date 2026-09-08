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

## Performance experiments

## Final verification
