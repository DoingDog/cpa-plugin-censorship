# Censorship v0.2.2 Functional Audit Fixes

## Status

Implementation-ready design for the verified findings from the 2026-09-11 whole-repository functional audit.

## Goal

Fix every verified plugin-side defect found by the audit without changing CLIProxyAPI core, native ABI v1, RPC schema v5, configuration semantics, response processing, or generic JSON traversal policy.

The release must continue to process rules in `block` -> `strip` -> `obfs` order and must select only explicit provider-documented natural-language leaves.

## Verified Problems

### R1. OpenAI Responses result items bypass filtering

The selector currently uses `program_result`, while the current Responses request contract uses `program_output`. Valid `program_output.result` text therefore bypasses `tool` scope.

The selector also treats `mcp_call.error` as a string. Current MCP protocol and HTTP errors are typed objects whose natural-language leaf is `error.message`. `mcp_list_tools.error` is a separate scalar error field and is not selected.

Two documented replayable built-in-tool results are absent from the selector:

- `file_search_call.results[*].text`
- `code_interpreter_call.outputs[*].logs` when the output discriminator is exactly `logs`

All these leaves use canonical `tool`. IDs, names, arguments, code, status, URLs, file metadata, scores, attributes, error types/codes, image outputs, schemas, and unknown item variants remain unchanged.

### R2. Claude document and search-result text bypass filtering

The bounded Claude selector omits these documented natural-language leaves:

- `search_result.title`
- `document.title`
- `document.context`
- scalar `document.source.content` when `source.type == "content"`

The leaves use the enclosing canonical role: direct blocks in a user message use `user`; blocks nested in `tool_result.content` use `tool`.

Existing explicit paths remain unchanged. Source URLs and identifiers, media types, base64 data, file IDs, citations, cache metadata, signatures, and unknown block variants remain excluded.

### R3. Claude strip can emit schema-invalid empty text

Anthropic requires `TextBlockParam.text`, and present `document.title` and `document.context`, to have a minimum length of one. A strip rule that removes the complete value currently emits an empty JSON string and CPA forwards a provider-invalid request.

The selector will mark only those documented minimum-length spans. After the existing single selection and rule pass, the transform will reject a changed marked span whose resulting text is empty. The plugin will return HTTP 400 with `Content-Type: application/json`, code `censorship_invalid_request`, and a specific message. It will not send a malformed provider request or perform a second JSON parse.

The plugin will not delete blocks or fields, insert replacement whitespace, or silently pass the original text. Those alternatives either require structural JSON editing, alter strip semantics, or fail open.

### R4. Aggregate packaging can overwrite its input

Direct packaging validates that the library, archive, checksum, and optional `LICENSE` do not alias. Aggregate packaging bypasses that existing validator. If an expected aggregate archive path is a hard link or symlink to the platform library, `os.Create` truncates the library before it is copied and emits an unusable archive.

Aggregate mode will call the existing path validator for each discovered platform library before creating that platform's archive or checksum. On rejection, the source library and all pre-existing paths remain unchanged; no checksum or aggregate checksum file is written for the failed run.

### R5. Bare `make build` and `make package` reject their intended default version

`unexport VERSION` creates a file-origin empty make variable before `PACKAGER_VERSION` checks `origin VERSION`. A missing `VERSION` is therefore misclassified as an explicit empty value, so the documented bare build and package commands fail validation.

The Makefile will capture `RAW_VERSION` and calculate `PACKAGER_VERSION` before `unexport VERSION`.

Required behavior:

- missing `VERSION` -> `PACKAGER_VERSION=0.0.0-dev`
- explicit empty `VERSION` -> validation failure
- explicit safe `VERSION` -> exact raw value reaches recursive make and the packager
- unsafe values remain rejected without shell or make expansion
- one leading lowercase `v` remains removed exactly once for artifact names and embedded version metadata

## Design

### Explicit selector additions

Extend only the existing provider-specific switch branches. Reuse `appendStringSpan`, `scanTextPart`, and the existing raw-offset reconstruction. Do not add a recursive string walker and do not unmarshal a provider body into a generic object graph.

For OpenAI Responses:

1. Replace `program_result` with `program_output` and select only `result`.
2. For `mcp_call`, continue selecting scalar `output`. Select `error.message` only when `error.type` is exactly `mcp_protocol_error` or `http_error`.
3. Add `mcp_list_tools` and select only scalar `error`.
4. Add `file_search_call`; iterate an array-valued `results` and select only each string-valued `text`.
5. Add `code_interpreter_call`; iterate an array-valued `outputs` and select only string-valued `logs` from objects whose `type` is exactly `logs`.

For Claude:

1. Select `search_result.title` before its typed-text content.
2. Select `document.title` and `document.context` before inspecting `source`.
3. For `source.type == "content"`, select a scalar `source.content`, then retain the existing typed-text array traversal.
4. Apply the same helper from direct user blocks and nested tool-result blocks so role mapping stays inherited and identical.

### Minimum-length metadata

Add one boolean to `textSpan` that means the provider contract rejects an empty resulting value. Add a small helper that appends a normal string span and marks the newly appended span when selection succeeded.

Use that helper only for Claude `TextBlockParam.text`, `document.title`, and `document.context`. The generic matching and rebuilding algorithms remain unchanged.

After `applyMode` returns a changed result and before `rebuildBody`, scan the already selected spans once. If a changed marked span is empty, return an invalid-transform result with the specific message. This adds no JSON parse and no traversal of unselected fields.

### Invalid-transform response

Extend `transformResult` with an optional invalid message. Existing malformed/deep/duplicate JSON continues to use `request body must be a JSON object`. An empty minimum-length Claude field uses `censorship rewrite would make a text field invalid`.

Both cases use the existing HTTP 400 `censorship_invalid_request` response path and JSON response header. No request headers are changed for successful rewrites; CPA remains responsible for recalculating `Content-Length` when it installs the returned body.

### Packaging and Makefile

Reuse `validateDirectPackagePaths` in aggregate mode. Do not add another canonicalization or alias implementation.

Reorder the three existing Make assignments. Do not weaken version validation, add a version source, or change release artifact naming.

## Tests

Use TDD for each behavior change.

### OpenAI Responses tests

Use complete current-contract fixtures and assert both selection and exclusion:

- `program_output.result`
- `mcp_call.output`
- `mcp_call.error.message` for `mcp_protocol_error` and `http_error`
- `mcp_list_tools.error`
- `file_search_call.results[*].text`
- `code_interpreter_call.outputs[type=logs].logs`

For every fixture, test `tool` enabled and disabled. Assert exact raw-body equality outside expected replacements. Include unknown error and output variants to prove they remain untouched.

### Claude tests

For direct user and nested tool-result placement, cover:

- `search_result.title`
- `document.title`
- `document.context`
- scalar content-document text

Test enabled and disabled roles and exact preservation of all machine siblings.

Add full-strip and partial-strip cases for typed `text`, `document.title`, and `document.context`. Full strip must terminate locally with `censorship_invalid_request`, the specific message, and no replacement body. Partial strip must produce valid JSON and change only the selected token.

### Packaging tests

Create a platform library and hard-link its expected aggregate archive path to it. Snapshot source and output state, call aggregate packaging, and assert rejection plus complete preservation. Use the existing snapshot and alias helpers.

### Make tests

Run the real Makefile with version-related environment variables removed and assert `validate-version` accepts the missing-version default. Assert explicit empty and unsafe values still fail. Existing dry-run tests continue to verify recursive propagation and target selection.

### Verification

Run, in order where dependencies require it:

```bash
make test
go test .github/scripts/package-release.go .github/scripts/package-release_test.go
make race
make vet
make integration
make build
make package
```

Use uncached focused tests during red/green work. The final integration run must continue to prove HTTP JSON EOF, SSE, request-header/body propagation, Responses WebSocket transparency, all selector behaviors, and ABI ownership.

A temporary pinned-CPA Interactions StepList executor probe already passed before this design: `system_instruction` and `user_input.content[type=text].text` were stripped, `model_output` remained excluded under default roles, the exact rewritten body reached `/v1beta/interactions`, and the JSON response reached client EOF unchanged. The temporary test was removed because retaining it would duplicate the existing CPA-start harness without fixing a defect.

## Documentation and Release

Update `README.md` and `RELEASE_NOTES.md` to list the exact added OpenAI and Claude leaves, the Claude empty-text rejection, aggregate alias protection, the restored default build version, and the unchanged request-only/stream-transparent behavior.

Release as patch version `v0.2.2`. The tag-triggered workflow must build all seven platform archives, write matching lowercase SHA-256 files, aggregate `checksums.txt`, and publish or update the GitHub release.

## Explicit Exclusions

### Claude signed-history prefix rewriting

The audit confirmed that Claude Fable 5.1 can bind a thinking block to preceding system, tools, and messages under provider-controlled conditions. BeforeAuth does not reliably know the final provider model or binding controls, and model aliases can hide them. Always rejecting any signed history would break unaffected models; skipping matching prefix text would violate configured censorship. There is no minimal plugin-side behavior that is both policy-correct and provider-correct with the available hook contract.

The plugin continues to leave thinking, redacted thinking, encrypted continuation state, and signatures byte-identical. The limitation will be documented, not converted into an overbroad runtime guard.

### Inputs larger than `C.int`

The native ABI continues to return a non-zero code for methods that must copy an input longer than `C.int`. This is an existing documented pure-plugin limit and preserves the required `C.GoBytes` ownership contract. Synthesizing a method-specific RPC response before reading the RPC envelope would broaden ABI behavior for an impractical descriptor and is outside this patch.

### Additional Interactions traversal

The current Interactions selector and the pinned-CPA executor probe passed for the documented current system instruction, TextContent, user-input, model-output, body replacement, response, and EOF path. Thought summaries, signed history, tool results, tool descriptions, and query strings lack an approved censorship-role policy or mutation contract and remain excluded.

### Response filtering and CPA core

The plugin remains request-only. It does not register response, stream-chunk, or WebSocket-response hooks. No CLIProxyAPI core source or behavior will be modified. Response transparency is verified relative to pinned CPA; the plugin does not promise to undo CPA's own protocol translation or header normalization.

## Completion Criteria

The change is complete only when:

1. Every included reproduction fails on v0.2.1 and passes after its focused fix.
2. No selected machine field changes in exact-body tests.
3. No added selector performs generic recursion or a second JSON parse.
4. Successful rewrites return only a body; terminal invalid requests return HTTP 400 JSON headers and never reach upstream execution.
5. Existing unit, race, vet, integration, build, and package checks pass from a clean source tree.
6. The implementation diff contains no CPA core edits and no hand-edited `.integration/` or `dist/` artifacts.
7. Local `main` contains the verified commits, tag `v0.2.2` is pushed, and the GitHub tag workflow completes successfully with all release artifacts and checksums.
