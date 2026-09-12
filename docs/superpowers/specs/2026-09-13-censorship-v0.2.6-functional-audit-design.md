# Censorship Plugin v0.2.6 Functional Audit Fixes

## Goal

Close the confirmed provider-contract gaps found by the 2026-09-13 functional audit without changing the plugin ABI, configuration model, matcher semantics, or CLIProxyAPI core. The selector must inspect only current provider-documented natural-language leaves and must leave identifiers, signatures, schemas other than `description`, arbitrary objects, and other machine fields byte-for-byte unchanged.

## Invariants

- Keep native ABI v1, CGO ownership, pointer lifetimes, and the pinned `github.com/router-for-me/CLIProxyAPI/v7` integration unchanged.
- Keep `words` as the strict ordered `block`, `strip`, and `obfs` object, including the handwritten-YAML-only legacy sequence form.
- Keep rule execution order `block` -> `strip` -> `obfs`.
- Keep global UTF-8, JSON-object, nesting-depth, and duplicate-member validation unchanged.
- Use exact provider discriminators and named text leaves. Do not add a recursive string walker.
- Preserve raw request bytes outside selected string tokens.
- Never select IDs, names, URLs, signatures, booleans, enum values, schema property names, `required`, `enum`, `default`, tool arguments, arbitrary result objects, media payloads, or unknown future variants.
- Keep `assistant` and `tool` opt-in through `scope.roles`; the default roles remain `system`, `developer`, and `user`.

## Confirmed defects and behavior

### Gemini Interactions API

#### Annotation ownership

`TextContent.annotations` is citation metadata for model-generated content. The current shared helper treats every selected annotated `TextContent` as immutable, including caller-authored direct input text. This incorrectly rejects otherwise valid `strip` and `obfs` requests.

Annotation-based immutability applies only to text selected from the `content` field of an exact `type: "model_output"` step. A non-empty annotations array on that text permits `block`, permits a rewrite that cancels back to the original bytes, and returns local `censorship_invalid_request` with `censorship cannot rewrite annotated text` for a final byte change. Annotations on direct input, system input, `user_input`, tool-result text, or other caller-authored text impose no rewrite restriction. Empty annotations impose no restriction.

#### Tool-result steps

Add exact handling for these documented step variants in `input` and nested `steps` arrays:

- `type: "function_result"`: select `result` when it is a string. When `result` is an array, select only each object whose discriminator is exactly `type: "text"`, using its string `text` leaf. Map the leaves to canonical role `tool`.
- `type: "mcp_server_tool_result"`: apply the same string and exact typed-text-array rules to `result`, mapped to `tool`.
- `type: "code_execution_result"`: select a string `result`, mapped to `tool`. If sibling `signature` is present and non-null, mark the selected result signature-bound. `block` still reports the match. A final `strip` or `obfs` byte change returns local `censorship_invalid_request` with the existing `censorship cannot rewrite signature-bound text` message. Exact cancellation to the original bytes remains allowed.

For these variants, preserve `call_id`, `name`, `server_name`, `is_error`, `signature`, and all other siblings. Do not inspect object-valued `result`, image entries, text-looking unknown entries, or the obsolete/non-contract `content` field on result steps.

#### Function declarations

For each top-level `tools` array member with exact `type: "function"`:

- Select a string `description` as canonical role `developer`.
- Walk `parameters` only as a JSON Schema and select string `description` leaves using the existing finite JSON Schema keyword traversal.

Do not select function names, schema keys, enum/default values, or descriptions on non-function tool variants.

#### Response schema

`response_format` may be one object or an array. For each exact `type: "text"` member, walk `schema` with the existing JSON Schema description selector and map selected descriptions to canonical role `developer`. Do not inspect schemas on audio, image, video, unknown, missing, or non-string discriminators. Preserve MIME types and all non-description schema data.

The Interactions selector's cheap source-format role gate must include `developer` and `tool` now that documented leaves use those roles. Its internal input gate must also admit `tool`, so a tool-only scope reaches result steps; a developer-only scope may return after root tool/response-schema collection.

### OpenAI Responses API

#### Local skills inside `additional_tools`

The local-shell collector currently hard-codes skill descriptions to canonical role `user`. Keep top-level local shell skill descriptions mapped to `user`, as already documented. When the same local shell declaration appears inside an `input` item of exact `type: "additional_tools"`, map each skill `description` to the item's already validated canonical role. Current official requests use `role: "developer"`; therefore a developer-only policy must inspect that description, while a user-only policy must not.

Keep the existing accepted canonical-role compatibility behavior for `additional_tools`, and continue rejecting missing, null, non-string, and unknown roles. Preserve skill names, paths, IDs, versions, archives, environment fields, and commands.

#### MCP approval reason

For an `input` object with exact `type: "mcp_approval_response"`, select an optional string `reason` as canonical role `user`. Preserve `approval_request_id`, `approve`, `id`, and every other member. Do not treat the item as a message or inspect unknown values.

### Anthropic Messages API

#### Compaction instructions

Inspect `context_management.edits` independently of whether `messages` is present. For each object with exact string discriminator `type: "compact_20260112"`, select a string `instructions` as canonical role `system`. Ignore null or non-string instructions and all unknown edit variants. Preserve `trigger`, `pause_after_compaction`, replayed compaction blocks, thinking blocks, and every other control or machine field.

#### Beta MCP tool results

For a message content block with exact `type: "mcp_tool_result"`, map its model-visible result text to canonical role `tool`:

- Select `content` when it is a string.
- When `content` is an array, select only exact `type: "text"` objects with a string `text` leaf.

Preserve tool-use IDs, names, flags, image/document blocks, arbitrary objects, unknown typed blocks, and all other fields. Do not recursively inspect their strings. Keep existing stable `tool_result` behavior unchanged, including its documented search-result and document subtypes.

## Selector structure

Make surgical extensions to the existing provider collectors:

- Let the Interactions text-part helper receive an explicit annotation-protection flag. Only the exact `model_output.content` caller enables it.
- Dispatch exact Interactions tool-result types before the user/model content discriminator rejects them. Use small provider-local helpers for the documented result unions and signature marking.
- Reuse `appendJSONSchemaDescriptions` for Interactions function parameters and response schemas.
- Let the OpenAI tool declaration collector use its supplied canonical role for local skills, while the top-level caller explicitly supplies `user` for local shell declarations.
- Scan Anthropic compaction edits before the current early return for absent/non-array `messages`. Add an exact Beta MCP result branch without widening stable result traversal.

No selector change requires an ABI, configuration, matcher, transformer, or CLIProxyAPI-core modification.

## TDD and oracle requirements

Add focused tests before production changes. The red tests must demonstrate:

- Annotated direct/user Interactions text rewrites successfully while annotated `model_output.content` remains protected.
- Interactions function and MCP result string/typed-text leaves use `tool`; object/image/unknown/obsolete fields stay unchanged.
- Unsigned code execution result text can be rewritten; signed text can be blocked but rejects a final rewrite; signature and machine siblings stay unchanged.
- Interactions function and response JSON Schema descriptions use `developer` and preserve all machine fields and unsupported variants.
- OpenAI additional-tools local skills obey the item role while top-level local skills remain `user`.
- OpenAI MCP approval `reason` uses `user` and preserves decision fields.
- Anthropic compaction instructions use `system` even without `messages`, and unknown edit/control fields stay unchanged.
- Anthropic Beta MCP result string and typed-text content use `tool`, while non-text content and machine fields stay unchanged.

Update both independent protocol oracles in `fuzz_test.go` to mirror only the new documented paths and narrowed annotation rule. Add deterministic seeds for the new provider shapes, including signed Interactions code output. The differential fuzz harness must continue checking role assignment, raw token boundaries, immutability metadata, and byte preservation.

## Documentation and release

Update `README.md` to list the exact new paths, canonical roles, annotation ownership, and signed code-result behavior. Amend only current contract prose. Add a new `v0.2.6` entry to `RELEASE_NOTES.md`; do not rewrite historical releases.

Do not hand-edit `.integration/` or `dist/`. Generated artifacts may be created only by verification/package commands and must not be committed.

## Acceptance criteria

- Every focused regression test fails against `v0.2.5` behavior for the intended reason and passes after the selector change.
- Existing tests, race tests, vet, integration tests, native build, and packaging all pass with the required Windows/MSYS environment forwarded.
- Changed-code review finds no unresolved functional defect, contract overreach, generic recursive scan, machine-field mutation, role regression, or signed-state rewrite.
- The final commit contains only intended source, tests, README, release notes, and this task's spec/plan documents. The temporary task-memory file and generated artifacts are absent.
- Local `main` receives the verified change, patch version `v0.2.6` is tagged, `main` and the tag are pushed, and the actual GitHub build/release workflow is monitored to a terminal result.
