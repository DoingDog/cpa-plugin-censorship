# Censorship v0.2.5

## v0.2.5 fixes

- Calculates rewrite state from final text. A rewrite that cancels back to the original bytes returns no replacement body and does not trigger an immutable-text error.
- Protects annotated Interactions `model_output.content` text: `block` remains allowed. A final text or byte change to annotated Interactions `model_output.content` with non-empty annotations returns local `censorship_invalid_request`; exact cancellation back to the original bytes is allowed. Empty annotations remain rewritable.
- Keeps long folded rewrites on the KMP path after adjacent leading matches instead of rescanning a near-miss tail.
- Deletes current-version stale ZIP and checksum sidecars when aggregate packaging reruns without a source platform, while retaining other-version and unknown output files.
- Overwrites release assets only while an existing release is draft. Published reruns update notes and verify rather than overwrite public assets.
- Downloads all remote release assets, compares their file set and bytes with `release/`, then verifies individual `.zip.sha256` files and aggregate `checksums.txt` before publication or on a published rerun.

Claims below are limited to verified behavior.

Censorship remains a pure CLIProxyAPI `RequestInterceptor`: it is request-only; does not inspect live model output, response bodies, SSE output, or server WebSocket output. Explicit historical output fields replayed as later request input may be inspected under assistant scope. It has no custom panel, menu, or Management API.

## Compatibility

v0.2.5 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+. The registered logo is `https://raw.githubusercontent.com/DoingDog/cpa-plugin-censorship/main/logo.png`.

## Object configuration panel

CPA's standard `ConfigFields` panel exposes `ignore_case`, `words`, `scope`, and `obfs`. `words` is the Object-only rule editor. It does not render or emit global `mode`.

```yaml
plugins:
  enabled: true
  configs:
    censorship:
      enabled: true
      ignore_case: false
      words:
        block: [example]
        strip: []
        obfs: []
      scope:
        formats: [openai, openai-response, claude, gemini, interactions]
        roles: [system, developer, user]
      obfs:
        char: "​"
```

Configuration types are strict and are not coerced. Unknown plugin keys, duplicate mapping keys, multiple YAML documents, invalid enum values, and incorrectly typed values reject the candidate configuration.

- `enabled`: boolean; host-owned. CPA uses it to enable this plugin.
- `ignore_case`: boolean; default `false`.
- `words`: Object with optional `block`, `strip`, and `obfs` arrays. Any subset of those strict bucket keys is valid; unknown keys reject the configuration. Each term is non-empty.
- `scope.formats`: sequence of strings; default all five formats. Allowed values are `openai`, `openai-response`, `claude`, `gemini`, and `interactions`; explicit `[]` disables all formats.
- `scope.roles`: sequence of strings; default `system`, `developer`, and `user`. Optional values are `assistant` and `tool`; explicit `[]` disables all roles.
- `obfs.char`: must be `U+200B` or `U+2060`; default `U+200B`.

Terms preserve YAML order, duplicates, case, whitespace, and newlines; they are not sorted, deduplicated, trimmed, normalized, or interpreted as regular expressions. `words.obfs` terms must have at least two Unicode scalars and omit the configured `obfs.char`; block and strip terms do not receive that obfs-only validation. words is the only term source; the plugin has no built-in terms, fallback terms, or online word-list download.

## Legacy handwritten YAML

For legacy handwritten YAML only, `words` can remain a list with global `mode`. The standard panel neither renders nor emits it.

```yaml
mode: strip
words: [example]
```

`mode`: string; default `block`. Allowed values are `block`, `strip`, and `obfs`; it chooses the action for a legacy list. With Object `words`, a supplied `mode` remains syntax-validated but is behaviorally ignored.

## Rule processing

Rules always run in fixed action order: block -> strip -> obfs. Eligible text nodes follow original JSON string-token byte order, matches never cross nodes, and each rule finds leftmost non-overlapping occurrences.

- block checks rules before document order and returns the YAML term with canonical role. The first matching word wins, and the earliest eligible node for that word supplies `role`; it terminates the request with HTTP 400.
- strip and obfs process all leftmost non-overlapping occurrences before the next rule. `strip` removes each actual source match from every eligible node.
- obfs preserves original case and inserts after the first Unicode scalar of every actual match.

Rules run once rather than to a fixed point; a later rule reads text produced by an earlier rule.

With `ignore_case: false`, matching is a case-sensitive literal substring operation. With `ignore_case: true`, the plugin uses Go `unicode.SimpleFold` equivalence over equal-length Unicode scalar windows while retaining original UTF-8 byte spans.

- Alpha/aLPHA matches.
- Greek sigma variants `Σ`, `σ`, and `ς` match one another.
- Kelvin sign `K` matches `K`.
- straße/STRASSE does not match because full case folding can change scalar count.

There is no Unicode normalization and no full case folding. `strip` removes the actual source bytes; obfs preserves original case and inserts after the first Unicode scalar. Long folded rewrites remain on the KMP scan after adjacent leading matches instead of rescanning a near-miss tail.

Rewrite state is calculated from each span's final text. A rewrite that cancels back to the original bytes returns no replacement body and does not trigger a signature-bound or annotated-text immutable-text error.

## Explicit provider paths

Selectors enter only documented text leaves; there is no recursive fallback string walker.

- OpenAI explicit text paths: Chat `messages[*]` string `content`, typed `text` content parts, `refusal` text, `prediction.content` only when `prediction.type == "content"`, documented `tool` and legacy `function` result text, legacy `functions[*]` descriptions/schema descriptions, `tools[*].function` and `tools[*].custom` descriptions/schema descriptions, and `response_format.json_schema` descriptions/schema descriptions. Chat prediction uses canonical `assistant`; Chat definitions use canonical `developer`. Responses top-level `instructions`, top-level string `input`, message `content`, `input_text`, `output_text`, and `refusal`, `prompt.variables` scalar or `input_text` values, `text.format` JSON-schema descriptions, top-level tool descriptions/schema descriptions, `additional_tools` definitions with only documented canonical roles, and `tool_search_output`/`mcp_list_tools` loaded definitions, plus documented `function`, `custom-tool`, `shell`, `apply-patch`, `MCP`, and `program` result-output text: `program_output.result`, `mcp_call.output`, `mcp_call.error.message` only for `mcp_protocol_error` and `http_error`, scalar `mcp_list_tools.error`, `file_search_call.results[*].text`, and `code_interpreter_call.outputs[type=logs].logs`. Responses prompt variables and local shell skill descriptions use canonical `user`; text format and top-level definitions use canonical `developer`; loaded definitions and result-text leaves use canonical `tool`; `additional_tools` uses only its exact documented canonical role. Legacy `/v1/completions` prompts arrive as canonical `user` messages after CPA conversion. Unsupported error/output variants and machine siblings remain unchanged.
- Claude explicit text paths: top-level string or typed-text `system`, enabled message string or typed-text content, direct user `search_result.title`, `document.title`, `document.context`, and scalar `document.source.content` when `source.type == "content"`, and a user `tool_result`'s string content or nested `text`, `search_result`, and `document` text. Direct user blocks use `user` and nested tool-result blocks use `tool`.
- Gemini explicit text paths: text parts in `systemInstruction` or `system_instruction` and `contents`, subject to the selected canonical role. The signature field and value remain excluded, but a non-null `thoughtSignature`, `thought_signature`, or `extra_content.google.thought_signature` does not exclude visible text in the same Part. A JSON `null` carrier is unset. A non-null function, media, file, or code carrier still excludes the whole Part. A matching `block` handles signed visible text normally; a matching `strip` or `obfs` never changes it and returns local `censorship_invalid_request` with `censorship cannot rewrite signature-bound text`.
- Interactions explicit text paths: documented `system_instruction` or fallback camel-case `systemInstruction`, recursive documented input text subsets, and direct input object or array items with exact `type: "text"` and text content. Annotated `model_output.content` text remains blockable. A final text or byte change to annotated Interactions `model_output.content` with non-empty annotations returns local `censorship_invalid_request`; exact cancellation back to the original bytes is allowed. Empty annotations add no restriction.

OpenAI Responses `output_text` and `refusal` leaves use canonical `assistant` scope regardless of the source item role. Missing Gemini roles follow CPA's user/model alternation. Invalid Gemini roles advance CPA's user/model alternation but remain unselected. Gemini `model` maps to `assistant`.

assistant is inspected only when explicitly listed in scope.roles. It applies to assistant history carried in a later request and never enables live output filtering. tool is inspected only for documented OpenAI and Claude result-text paths, including OpenAI string tool content and Claude selected tool_result text.

## Machine exclusions

Machine exclusions: tool calls, machine schema values, arguments, reasoning, thinking, JSON keys, machine JSON, binary uploads, and image/audio/video/file base64 are never changed. The selected natural-language tool and schema descriptions listed above are the only definition exception; text result fields listed above are the only tool/function result exception.

Machine arguments, grammar definitions, names, IDs, paths, schema values, and reasoning state remain excluded.

Tool names and IDs, protocol discriminators, model names, metadata, control fields, thought signatures, URL/media fields, multipart headers and boundaries, function-call arguments, Claude unselected tool-result fields, Gemini `functionResponse`, Interactions function/tool data, and every model response remain excluded. Gemini machine exclusions include camelCase and snake_case non-null function, media, file, and code carriers; signature values remain excluded without excluding same-Part visible text.

Interactions accepts camel-case `systemInstruction` when snake-case `system_instruction` is absent. Unknown SourceFormat, roles, item types, content blocks, and future protocol shapes are left unchanged. Enabled known formats reject JSON objects with duplicate member names at any nesting depth. JSON text inside a string remains ordinary text rather than a nested request object. Scalar Claude user content cannot be stripped to empty: a matching rewrite terminates locally with `censorship_invalid_request`: censorship rewrite would make a text field invalid. The same response applies if a rewrite empties a Claude `TextBlockParam.text`, present `document.title`, or present `document.context`.

## Reload, ABI, and integration

A non-Home CPA YAML update calls `plugin.reconfigure`, which publishes one complete immutable snapshot atomically. Poll a request containing a unique B-only term until it blocks. Therefore: valid non-Home YAML changes apply without restart after observing a snapshot-B sentinel. An invalid reconfiguration keeps the last-known-good snapshot. Home mode does not watch local YAML.

Bare `make build` and host-artifact `make package` use `0.0.0-dev`; explicit empty or unsafe `VERSION` remains invalid.

The native ABI remains v1 and validates native pointer/length descriptors. Production retains `C.GoBytes` for input requests, makes no no-copy performance claim, and does not claim pinned Windows host request-pointer liveness has been proven. After-auth intentionally does not read input. When censorship replaces a decoded request body, it clears `Content-Encoding`, `Content-Length`, and `Transfer-Encoding`; no-op requests preserve headers.

The integration harness verifies upstream arrival, HTTP/1.1 EOF, chunk/trailer handling, and exact provider paths. The integration oracle strictly validates complete, correctly typed HTTP and Responses WebSocket JSON.

## Accepted pure-plugin limits

1. hook is not raw ingress; document order follows current execution-body spans. The hook receives the body at CPA's request-execution boundary.
2. preprocessing can observe uncensored input. `ModelRouter` and some handler preprocessing run before this hook.
3. Responses WebSocket covers only model-executed turns.
4. `generate=false` prewarm bypasses the plugin.
5. `/v1/realtime`, Live, sideband, and DataChannel bypass the plugin.
6. Alpha Search bypasses the plugin.
7. WebSocket block events omit `term` and `role`. That path can emit status 400 and close, but cannot retain the custom HTTP fields.
8. RequestInterceptor failures are fail-open. This includes current CPA error, panic, and fuse handling.
9. BeforeAuth runs once per handler execution; AfterAuth can run zero, one, or multiple times; every call carries the full body and incurs full-body RPC encoding/copy cost. The host still encodes and carries the full body on every AfterAuth call, but after recognizing the fixed no-op method, the plugin does not perform a C-to-Go input copy.
10. Home mode does not watch local YAML.
11. unknown SourceFormat and future content types are not inspected; review schema drift when upgrading CPA.
12. Known SourceFormat JSON objects accept at most 1024 simultaneously open object or array levels, including the top-level object. Deeper requests return `censorship_invalid_request`.
13. Methods that read or copy request input reject lengths that exceed `C.int` with a non-zero ABI return code. After-auth intentionally does not read input and may accept a coherent non-nil oversized descriptor. Oversized host callback responses return a plugin error.
14. The Responses WebSocket integration test fails after 20 seconds without `response.completed`; it does not wait indefinitely.
15. A provider-controlled Claude signed-history prefix can bind preceding input. The BeforeAuth hook cannot reliably know final model/binding controls, so the plugin neither mutates thinking/signatures nor adds an overbroad runtime rejection.

## Artifacts

A complete GitHub release/workflow produces seven platform archives:

```plaintext
censorship_<version>_<goos>_<goarch>.zip
censorship_<version>_<goos>_<goarch>.zip.sha256
```

Supported tuples are `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`, `windows/arm64`, and `freebsd/amd64`. Raw tag, `VERSION`, and packager `-version` inputs must be safe ASCII filename components. One lowercase ASCII `v` is then removed exactly once.

Direct and aggregate packaging rejects library/archive/checksum input/output aliases and aliases of known release outputs before changing those paths. Direct `-library`/`-archive`/`-checksum` mode rejects archive/checksum symlinks, known hard links, junctions, and ASCII case aliases. An unknown external hard-link peer of an existing destination is not modified: packaging replaces only the destination directory entry. Packaging produces deterministic ZIP archives. Each ZIP contains the platform library and an optional repository `LICENSE` if one exists. Each `.zip.sha256` line contains 64 lowercase hex characters, two spaces, and the archive basename. An aggregate packaging run covers exactly its present source platforms; a complete GitHub release/workflow contains all seven supported platform lines and assets. Aggregate packaging deletes a current-version ZIP and sidecar checksum when its source platform is absent on a rerun; other-version and unknown output files remain unchanged.

The release workflow overwrites assets only for an existing draft. A published-release rerun updates notes, downloads the remote assets, compares their exact files and bytes with `release/`, and verifies every sidecar checksum plus aggregate `checksums.txt` without overwriting public assets.