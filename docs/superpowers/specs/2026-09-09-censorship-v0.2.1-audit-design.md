# CPA censorship plugin v0.2.1 audit design

**Date:** 2026-09-09
**Branch:** `feat/v0.2.1`
**Baseline:** `v0.2.0` at `1085eee`

## Goal

Fix every definite plugin, ABI, integration-harness, and release-tooling defect found by the v0.2.1 audit. Evaluate each runtime performance hypothesis with comparable before/after benchmarks and retain only changes that produce a repeatable improvement without correctness regressions.

## Audit boundary and evidence

The audit covered every tracked production, test, integration, build, packaging, and workflow source file. Eight mutually exclusive Opus xhigh partitions read the plugin, the directly relevant parts of CLIProxyAPI v7.2.152, gjson v1.18.0, sjson v1.2.5, and current official protocol specifications. The coverage pass reported no missing required file, no duplicate local read, and no missing protocol or dependency area.

Baseline `make test`, `make race`, `make vet`, and `make integration` passed. Six fuzz targets then ran for 30 seconds each and passed. Existing tests therefore do not detect the confirmed defects below.

Generated `.integration/` and `dist/`, old specs/plans, unrelated CLIProxyAPI core, `LICENSE`, and binary artwork were excluded from source review. Generated artifacts may be recreated only through their normal commands.

## Compatibility contracts

- Modify only this plugin repository. Do not patch CLIProxyAPI core.
- Preserve native ABI v1 ownership and synchronous call semantics.
- Keep `C.GoBytes` in `cliproxyPluginCall` unless both plugin-local and end-to-end benchmarks prove that a safe candidate removes the input-sized pre-auth allocation and improves throughput.
- Keep `words` as a strict Object with optional ordered `block`, `strip`, and `obfs` arrays. Preserve terms exactly.
- Keep the handwritten-YAML-only legacy sequence plus global `mode` behavior.
- Preserve matching order `block` -> `strip` -> `obfs`, rule order, document order, literal byte behavior, rewrite cascades, and duplicate-member rejection.
- Select only documented natural-language fields. Do not add a recursive string walker or inspect machine fields.
- Keep the plugin request-only. It must not advertise response, stream-chunk, or WebSocket output interception.

## Provider formats

The active request selectors remain:

1. OpenAI Chat Completions.
2. OpenAI Responses.
3. Anthropic Claude Messages.
4. Google Gemini `generateContent`.
5. Google Gemini Interactions.

The request-only capability means the plugin does not parse or rebuild provider response streams. HTTP/SSE/WebSocket output must continue passing through CPA unchanged; integration traces test that property. Provider-specific stream EOF and sentinel rules therefore stay in the host, not this plugin.

## Functional corrections

### 1. Gemini null union members

ProtoJSON treats `null` as an unset ordinary field. A Part containing natural-language `text` plus a machine-union key whose value is `null` must remain selectable. A non-null machine member and `thought: true` must continue suppressing selection.

Test every supported camelCase and snake_case machine discriminator as both `null` and non-null. Apply the same rule to `extra_content.google.thought_signature`.

### 2. Claude custom-content documents

For a Claude document source with `source.type == "content"`, select only `source.content[]` objects with `type == "text"`, field `text`. Preserve titles, URLs, citations, cache metadata, and non-text blocks. Propagate the role already assigned by the enclosing direct user message or nested tool result.

Test direct user content, nested tool content, enabled and disabled role scopes, ordered multiple text blocks, and preservation of non-text fields.

### 3. Native ABI pointer-length invariants

At the exported call boundary:

- `request == nil && requestLen == 0` remains a valid empty buffer.
- `request == nil && requestLen != 0` returns a nonzero ABI status with an empty output descriptor.
- A successful host callback may return `ptr == nil && len == 0` as empty output.
- `ptr == nil && len != 0` is malformed and returns an error.
- Every non-nil host buffer is copied before `free_buffer`, then freed exactly once.

Do not remove `C.GoBytes` from the production entrypoint in this correction.

### 4. Release VERSION handling

Treat `VERSION` as untrusted data at the shell boundary. It must never be expanded into shell recipe source before validation. Validate through a shell variable populated as data, using the existing allowed-character contract.

Normalize one leading lowercase `v` exactly once. Pass the raw version to the Go packager, which owns normalization for archive names and package metadata. Inputs such as `vv` normalize to `v`; command-substitution, quote, semicolon, newline, slash, and whitespace payloads are rejected before execution or filesystem use.

### 5. Direct packaging path safety

Before any write:

- Include the implicit repository `LICENSE` input in collision checks.
- Reject archive or checksum output leaves that are symlinks, including dangling symlinks.
- Reject writable outputs that alias the library, `LICENSE`, or each other.
- When two absent paths share an existing ancestor, conservatively treat ASCII case-only suffixes as aliases on every OS.
- On rejection, preserve every input and pre-existing output byte-for-byte.

No output may overwrite or truncate a package input, another output, or the completed ZIP.

### 6. Deterministic release archives

Set one fixed valid UTC ZIP timestamp on every library and `LICENSE` entry before writing headers. Identical filenames, bytes, modes, target metadata, and version must produce byte-identical ZIPs, per-archive lowercase SHA-256 files, and identical aggregate `checksums.txt` entries regardless of source mtimes.

### 7. Integration assertion precision

For Chat Completions strip and obfs tests, decode the captured JSON and assert the exact `messages[0].content` value. A decoy field containing the expected transformed spelling must not satisfy the assertion. Compare non-target fields with the disabled baseline where the existing test already has that evidence.

### 8. Upstream request arrival tracking

Record handler arrival before reading the request body. Record completed body capture separately. Every blocked-request assertion must require zero arrivals, not merely zero successfully read bodies. A deliberately truncated body must show one arrival and zero completed captures.

### 9. Raw HTTP/1.1 trace completeness

Preserve chunked trailers in `responseTrace`. Because the probe sends `Connection: close`, read once after the framed response and require EOF. Reject surplus bytes after either `Content-Length` bytes or the terminal chunk. Existing exact chunk boundaries, arrival timing, payload, status, and headers remain observable.

## Performance experiments

All experiments use existing dependencies and preserve current behavior. A production change is retained only when two independent before/after series, each with at least 10 process-level samples, agree under `benchstat`. Fix `GOMAXPROCS`, alternate baseline and candidate runs, use `-benchmem`, and run correctness, race, and holdout gates after each winner.

### A. Exact rewrite total-miss preflight

Compare current rule-major strip/obfs loops with one existing-matcher preflight per selected span. On total miss, return unchanged; on any hit, execute the current ordered loops unchanged.

Measure 20 MiB text with 1,024 rules for total miss, first/last sparse hit, dense hit, duplicate rules, span boundaries, and cascades, serial and parallel. Keep only if total-miss serial and parallel latency improves significantly, allocations do not increase, and no holdout regresses by more than 5%.

### B. Fused UTF-8 and nesting validation

Evaluate performing UTF-8 validation during the mandatory JSON nesting scan instead of making a second whole-body pass.

Measure 20 MiB ASCII/base64, 20 MiB multibyte text, invalid UTF-8 near the tail, depth boundaries, and ordinary small events, serial and parallel. Keep only if long ASCII and multibyte throughput both improve significantly, allocations do not increase, validity/error/span decisions remain identical, and no small-body case regresses by more than 5%.

### C. Inline duplicate-name storage

Evaluate a small fixed inline name buffer per JSON object, allocating a map only after the buffer fills.

Measure 100,000 small text-part objects, wide objects, nested tool schemas, escaped-equivalent duplicates, and late duplicates, serial and parallel. Keep only if latency and allocations both improve significantly for many-small-object workloads, bytes allocated do not increase, and ordinary-small or wide-object cases regress by no more than 5%.

### D. Folded KMP hybrid dispatch

Evaluate continuing KMP after two adjacent byte-zero matches instead of unconditionally switching the remaining suffix to candidate-by-candidate scanning.

Measure hybrid 64 KiB and 1 MiB inputs plus dense, sparse, miss, Unicode SimpleFold, and invalid-UTF-8 holdouts, for strip and obfs, serial and parallel. Keep only if both hybrid sizes improve significantly with unchanged allocation cost and no holdout regresses by more than 5%.

### E. Exact block matcher threshold

Measure direct matcher construction at 128, 192, and 255 rules against the current ordered scan for 64 KiB and 1 MiB text, with no/first/middle/last hits. Include snapshot build time and memory separately from request throughput.

Lower the threshold only for a measured rule range whose 1 MiB no-hit and late-hit serial and parallel throughput improves significantly, while construction cost, memory, all 64 KiB cases, and early hits regress by no more than 5%.

### F. Native input copy

Benchmark a test-only synchronous borrowed-input candidate against `C.GoBytes` at 1 KiB, 1 MiB, and 20 MiB, serial and parallel. Move host-side request cloning outside timed sections and validate the last response.

Production `C.GoBytes` may be removed only if the candidate is safe under race and strict checkptr tests, does not retain or mutate host memory, preserves all responses and ownership, removes about one input-sized allocation, and significantly improves both 1 MiB and 20 MiB end-to-end throughput in two 10-run comparisons. The pinned host's unresolved Windows request-liveness concern is a correctness blocker unless independently eliminated.

## Rejected performance scope

Do not optimize integration collector retention or release ZIP hashing as product runtime work. They do not improve plugin request throughput. A reduced-retention collector may be added only if needed to make a selected runtime stress benchmark valid; functional traces must continue retaining exact payload evidence.

Do not change CLIProxyAPI host cloning/locking, adopt a new JSON library, or add a dependency.

## TDD and verification

For each functional correction:

1. Add the smallest focused regression test.
2. Run it and record the expected failure.
3. Make the minimum production or harness change.
4. Run the focused test and adjacent package tests.

For each performance experiment:

1. Add a representative benchmark and correctness oracle before changing production code.
2. Capture baseline samples.
3. Implement one isolated candidate.
4. Capture interleaved candidate samples.
5. Keep or revert the candidate from measured criteria.
6. Record results, including rejected experiments, in the task TDD report.

Final verification must run formatting checks, `make test`, `make race`, `make vet`, `make integration`, `make build`, `make package`, active fuzzing of all six targets, relevant benchmark holdouts, and a clean-tree review. Generated artifacts are produced only by commands.

## Release

Update public version/release documentation only after implementation and verification. Merge `feat/v0.2.1` into local `main`, create an annotated `v0.2.1` tag matching the existing release convention, push `main` and the tag to `origin`, verify the GitHub build workflow is triggered, and confirm the release assets include matching lowercase SHA-256 files plus complete `checksums.txt` entries.
