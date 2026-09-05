# Task 6 protocol transform oracle report

## Baseline

- Worktree HEAD: `dd6352d744a8fc42e342c98a473b36d260877a91`
- Code file changed: `fuzz_test.go`
- The required report is this file.

## RED

Command:

```bash
go test . -run '^(TestProtocolOracleRequiresEarliestBlockRole|TestProtocolOracleRejectsOutsideSpanByteMutation|TestExactBlockAcrossSpans|TestFoldMatcherMatchesRuleMajorOracle)$' -count=1
```

Result: exit code 1, as expected. The old oracle accepted both regression cases:

```text
--- FAIL: TestProtocolOracleRequiresEarliestBlockRole (0.00s)
    fuzz_test.go:76: oracle accepted later eligible role
--- FAIL: TestProtocolOracleRejectsOutsideSpanByteMutation (0.00s)
    fuzz_test.go:86: oracle accepted a number spelling mutation outside the eligible span
FAIL
github.com/DoingDog/cpa-plugin-censorship 0.085s
```

## GREEN and verification

Focused regression command after the implementation:

```bash
go test . -run '^(TestProtocolOracleRequiresEarliestBlockRole|TestProtocolOracleRejectsOutsideSpanByteMutation|TestExactBlockAcrossSpans|TestFoldMatcherMatchesRuleMajorOracle)$' -count=1
```

```text
ok  github.com/DoingDog/cpa-plugin-censorship  0.095s
```

Protocol oracle regression suite:

```bash
go test . -run '^(TestProtocolOracle|TestExactBlockAcrossSpans|TestFoldMatcher)' -count=1
```

```text
ok  github.com/DoingDog/cpa-plugin-censorship  0.091s
```

Required fuzz seeds:

```bash
go test . -run '^$' -fuzz '^FuzzRuleEngineAgainstOracle$' -fuzztime=1x
```

```text
fuzz: elapsed: 0s, gathering baseline coverage: 0/430 completed
fuzz: elapsed: 0s, gathering baseline coverage: 1/430 completed
PASS
ok  github.com/DoingDog/cpa-plugin-censorship  0.234s
```

```bash
go test . -run '^$' -fuzz '^FuzzProtocolTransform$' -fuzztime=1x
```

```text
fuzz: elapsed: 0s, gathering baseline coverage: 0/792 completed
fuzz: elapsed: 0s, gathering baseline coverage: 1/792 completed
PASS
ok  github.com/DoingDog/cpa-plugin-censorship  0.299s
```

Full suite:

```bash
go test ./...
```

Result: exit code 1. The package tests ran, but the existing documentation assertion failed:

```text
--- FAIL: TestDocumentationListsConfigAndLimits (0.00s)
    main_test.go:522: README.md missing "plugins:\n  enabled: true\n  configs:\n    censorship:\n      enabled: true\n      mode: block\n      ignore_case: false\n      words:\n        - example\n      scope:\n        formats: [openai, openai-response, claude, gemini, interactions]\n        roles: [system, developer, user]\n      obfs:\n        char: \"​\""
    main_test.go:522: RELEASE_NOTES.md missing "plugins:\n  enabled: true\n  configs:\n    censorship:\n      enabled: true\n      mode: block\n      ignore_case: false\n      words:\n        - example\n      scope:\n        formats: [openai, openai-response, claude, gemini, interactions]\n        roles: [system, developer, user]\n      obfs:\n        char: \"​\""
FAIL
github.com/DoingDog/cpa-plugin-censorship 2.252s
?    github.com/DoingDog/cpa-plugin-censorship/integration [no test files]
FAIL
```

The requested scope prohibits changing those documentation files, so this concern remains outside Task 6.

## Changes

- Added `TestProtocolOracleRequiresEarliestBlockRole` and `TestProtocolOracleRejectsOutsideSpanByteMutation`.
- Block validation now chooses the first matching eligible `SECRET` span by raw document offset and requires the reported role to match it. It no longer accepts any role in a matching-role set.
- Added a test-local raw JSON parser that records exact string-token byte offsets while preserving the existing semantic protocol oracle.
- Transform validation compares the output and input byte-for-byte outside the selected eligible raw string token ranges. Prefix, every gap, suffix, whitespace, number spelling, member order, and unknown or hard-excluded strings are therefore required to remain unchanged.
- The raw mask and offset logic does not call production selector, matcher, canonicalization, or body rebuild helpers.
- Ran `gofmt -w fuzz_test.go`; `git diff --check` passed.

## Concern

`go test ./...` remains red only because `main_test.go` expects a configuration example missing from `README.md` and `RELEASE_NOTES.md`. Those files were not read or modified, per task scope.
