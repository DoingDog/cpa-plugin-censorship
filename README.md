# cpa-plugin-censorship

`censorship` is a pure dynamic plugin for [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI). It examines selected text leaves in model request bodies before authentication. The plugin is request-only; does not inspect live model output, response bodies, SSE output, or server WebSocket output. Explicit historical output fields replayed as later request input may be inspected under assistant scope.

The plugin exposes only `RequestInterceptor`. It has no custom panel, menu, or Management API. CPA's standard configuration UI supplies the Object-only rule editor described below; terms still live in CPA YAML. words is the only term source; the plugin has no built-in terms, fallback list, or online download.

## Installation and compatibility

Copy the native library to the CPA platform plugin directory, then start or restart CPA to load it.

| Platform | CPA plugin path |
|---|---|
| Windows amd64 | `<CPA directory>/plugins/windows/amd64/censorship.dll` |
| Windows arm64 | `<CPA directory>/plugins/windows/arm64/censorship.dll` |
| Linux amd64 | `<CPA directory>/plugins/linux/amd64/censorship.so` |
| Linux arm64 | `<CPA directory>/plugins/linux/arm64/censorship.so` |
| macOS Intel | `<CPA directory>/plugins/darwin/amd64/censorship.dylib` |
| macOS Apple silicon | `<CPA directory>/plugins/darwin/arm64/censorship.dylib` |
| FreeBSD amd64 | `<CPA directory>/plugins/freebsd/amd64/censorship.so` |

Linux release libraries require glibc 2.34+.

v0.3.0 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. The registered logo is `https://raw.githubusercontent.com/DoingDog/cpa-plugin-censorship/main/logo.png`.

## Configuration

CPA management clients expose `ignore_case`, `words`, `scope`, `obfs`, `filter_mode`, `filter_logic`, and `filter` through standard `ConfigFields`. `words` is the Object-only rule editor: it has action buckets instead of a global selector. The panel does not render or emit global `mode`.

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
      filter_mode: exclude
      filter_logic: or
      filter: {}
```

Configuration types are strict and are not coerced. Unknown plugin keys, duplicate mapping keys, multiple YAML documents, invalid enum values, and incorrectly typed values reject the candidate configuration.

| Field | Contract |
|---|---|
| `enabled` | `enabled`: boolean; host-owned. CPA uses it to enable this plugin. |
| `ignore_case` | `ignore_case`: boolean; default `false`. |
| `words` | `words`: Object with optional `block`, `strip`, and `obfs` arrays. Bucket keys are strict; any subset is valid, while unknown keys reject the configuration. Every term is a non-empty string. |
| `scope.formats` | `scope.formats`: sequence of strings; default all five formats. Allowed values are `openai`, `openai-response`, `claude`, `gemini`, and `interactions`; explicit `[]` disables all formats. |
| `scope.roles` | `scope.roles`: sequence of strings; default `system`, `developer`, and `user`. Optional values are `assistant` and `tool`; explicit `[]` disables all roles. |
| `obfs.char` | `obfs.char`: must be `U+200B` or `U+2060`; default `U+200B`. |
| `filter_mode` | `filter_mode`: string; default `exclude`. Allowed values are `exclude` and `include`. |
| `filter_logic` | `filter_logic`: string; default `or`. Allowed values are `or` and `and`. |
| `filter` | `filter`: Object with optional `api-keys` and `models` arrays. |

The parser also accepts host-owned `priority` and `store` keys. It preserves each term's YAML order, duplicates, case, leading/trailing whitespace, and newlines; it does not sort, deduplicate, trim, normalize, or interpret regular expressions. `words.obfs` terms must contain at least two Unicode scalars and must not already contain the configured `obfs.char`. Block and strip terms do not receive that obfs-only validation.

## Request filtering

An empty `filter: {}` or a filter with both lists empty disables request filtering and processes all requests. `api-keys` and `models` each apply list-internal OR. Only non-empty dimensions participate. Across non-empty dimensions, `filter_logic: or` matches either dimension and `filter_logic: and` requires both dimensions.

| API-key list matches | Model list matches | `filter_logic: or` | `filter_logic: and` |
|---|---|---|---|
| yes | yes | match | match |
| yes | no | match | no match |
| no | yes | match | no match |
| no | no | no match | no match |

| `filter_mode` | Filter match | No filter match |
|---|---|---|
| `exclude` | bypass | process |
| `include` | process | bypass |

Full-string, case-sensitive glob matching uses `*` for zero or more Unicode scalars and `?` for exactly one Unicode scalar. `models` matches `RequestedModel`, never `Model` or a body model. `api-keys` matches authenticated `caller_scope`. `Principal` and literal credentials, including credentials carried in headers, do not match `caller_scope` when they differ.

All five supported `SourceFormat` values use the same request-filter sources: `Metadata["caller_scope"]`, `RequestedModel`, and wildcard credential carriers. Wildcard credential carriers are scanned in this exact order: `Authorization`, `X-Goog-Api-Key`, then `X-Api-Key`. `Authorization` accepts case-insensitive `Bearer <credential>` or a raw credential. Every wildcard candidate is trimmed and scope-bound before glob matching. Other headers and query-only credentials are unsupported wildcard carriers.

Query-only conditions support exact values only; wildcard query conditions are not supported. Management configuration readback returns filter values unchanged and does not mask secrets. Do not store secrets in these values.

A filter bypass returns no replacement body or header changes before JSON validation.

## Legacy handwritten YAML

For legacy handwritten YAML only, `words` may remain a legacy list together with global `mode`. The standard panel neither renders nor emits it.

```yaml
mode: strip
words: [example]
```

`mode`: string; default `block`. Allowed values are `block`, `strip`, and `obfs`; it selects the action for a legacy list. With Object `words`, a supplied `mode` remains syntax-validated but is behaviorally ignored.

## Rule order and matching

Eligible text nodes follow original JSON string-token byte order in the current execution body. Matches never cross nodes, and each rule finds leftmost non-overlapping occurrences.

Rules always run in fixed action order: block -> strip -> obfs. A block bucket match stops the request before rewriting. Strip bucket rules run next in their YAML order, then obfs bucket rules run in their YAML order. A later rule reads text produced by an earlier rule; processing is once rather than to a fixed point.

- block checks rules before document order and returns the YAML term with canonical role. The first matching word wins, and the earliest eligible node for that word supplies `role`. The request stops with HTTP 400 and is not sent upstream.
- strip and obfs process all leftmost non-overlapping occurrences before the next rule. `strip` removes each actual source match from every eligible node.
- obfs preserves original case and inserts after the first Unicode scalar of every actual match.

A block response has this shape:

```json
{
  "error": {
    "type": "invalid_request_error",
    "code": "censorship_blocked",
    "message": "request blocked by censorship rule",
    "term": "matched word",
    "role": "user"
  }
}
```

With `ignore_case: false`, matching is a case-sensitive literal substring operation. With `ignore_case: true`, the plugin compares equal-length Unicode scalar windows using Go `unicode.SimpleFold` equivalence while retaining byte spans from the original UTF-8 text.

- Alpha/aLPHA matches.
- Greek sigma variants `Σ`, `σ`, and `ς` match one another.
- Kelvin sign `K` matches `K`.
- straße/STRASSE does not match because full case folding can change scalar count.

There is no Unicode normalization and no full case folding. `strip` removes the actual source bytes; obfs preserves original case and inserts after the first Unicode scalar. Long folded rewrites remain on the KMP scan after adjacent leading matches instead of rescanning a near-miss tail.

Rewrite state is calculated from each span's final text. A rewrite that cancels back to the original bytes returns no replacement body and does not trigger a signature-bound or annotated-text immutable-text error.

## Source formats and selected text

Selectors enter only the explicit text leaves listed here. There is no recursive fallback string walker.

- OpenAI explicit text paths: Chat `messages[*]` string `content`, typed `text` content parts, `refusal` text, `prediction.content` only when `prediction.type == "content"`, documented `tool` and legacy `function` result text, legacy `functions[*]` descriptions/schema descriptions, `tools[*].function` and `tools[*].custom` descriptions/schema descriptions, and `response_format.json_schema` descriptions/schema descriptions. Chat prediction uses canonical `assistant`; Chat definitions use canonical `developer`. Responses top-level `instructions`, top-level string `input`, message `content`, `input_text`, `output_text`, and `refusal`, `prompt.variables` scalar or `input_text` values, `text.format` JSON-schema descriptions, top-level tool descriptions/schema descriptions, `additional_tools` definitions with only documented canonical roles, and `tool_search_output`/`mcp_list_tools` loaded definitions, plus documented `function`, `custom-tool`, `shell`, `apply-patch`, `MCP`, and `program` result-output text: `program_output.result`, `mcp_call.output`, `mcp_call.error.message` only for `mcp_protocol_error` and `http_error`, scalar `mcp_list_tools.error`, `file_search_call.results[*].text`, and `code_interpreter_call.outputs[type=logs].logs`. Responses prompt variables and local shell skill descriptions use canonical `user`; OpenAI `mcp_approval_response.reason` is selected as `user`; `additional_tools` local skills inherit the validated item role; top-level local skills remain `user`; text format and top-level definitions use canonical `developer`; loaded definitions and result-text leaves use canonical `tool`; `additional_tools` uses only its exact documented canonical role. Legacy `/v1/completions` prompts arrive as canonical `user` messages after CPA conversion. Unsupported error/output variants and machine siblings remain unchanged.
- Claude explicit text paths: top-level string or typed-text `system`, enabled message string or typed-text content, direct user `search_result.title`, `document.title`, `document.context`, and scalar `document.source.content` when `source.type == "content"`, and a user `tool_result`'s string content or nested `text`, `search_result`, and `document` text. Anthropic exact compaction instructions use canonical `system`; Beta MCP string and exact typed-text results use canonical `tool`. Direct user blocks use `user` and nested tool-result blocks use `tool`.
- Gemini explicit text paths: text parts in `systemInstruction` or `system_instruction` and `contents`, subject to the selected canonical role. The signature field and value remain excluded, but a non-null `thoughtSignature`, `thought_signature`, or `extra_content.google.thought_signature` does not exclude visible text in the same Part. A JSON `null` carrier is unset. A non-null function, media, file, or code carrier still excludes the whole Part. A matching `block` handles signed visible text normally; a matching `strip` or `obfs` never changes it and returns local `censorship_invalid_request` with `censorship cannot rewrite signature-bound text`.
- Interactions explicit text paths: documented `system_instruction` or fallback camel-case `systemInstruction`, documented input text subsets, direct input object or array items with exact `type: "text"` and text content, function/MCP result strings and exact typed-text array items, code execution result strings, function declaration descriptions, parameter JSON Schema description leaves, and exact text response schema description leaves. Function/MCP result strings and exact typed-text array items use canonical `tool`. Code execution result strings use canonical `tool`; a non-null sibling signature for those code execution result strings permits `block` but rejects a final rewrite. Function declaration descriptions, parameter JSON Schema description leaves, and exact text response schema description leaves use canonical `developer`. Only non-empty annotations on exact `model_output.content` text protect rewrites; caller-authored annotated text remains rewritable. Annotated `model_output.content` text remains blockable. A final text or byte change to annotated Interactions `model_output.content` with non-empty annotations returns local `censorship_invalid_request`; exact cancellation back to the original bytes is allowed. Empty annotations add no restriction.

OpenAI Responses `output_text` and `refusal` leaves use canonical `assistant` scope regardless of the source item role. Missing Gemini roles follow CPA's user/model alternation. Invalid Gemini roles advance CPA's user/model alternation but remain unselected. Gemini `model` maps to `assistant`.

assistant is inspected only when explicitly listed in scope.roles. This applies only to assistant history carried in a later request and never to live output. tool is inspected only for documented OpenAI, Claude, and Interactions result-text paths, including OpenAI string tool content, Claude selected tool_result text, and Interactions selected function/MCP/code execution result text.

## Machine exclusions

Machine exclusions: tool calls, machine schema values, arguments, reasoning, thinking, JSON keys, machine JSON, binary uploads, and image/audio/video/file base64 are never changed. The selected natural-language tool and schema descriptions listed above are the only definition exception; text result fields listed above are the only tool/function result exception.

Machine arguments, grammar definitions, names, IDs, paths, schema values, and reasoning state remain excluded.

This also excludes tool names and IDs, protocol discriminators, model names, metadata, control fields, thought signatures, URL/media fields, multipart headers and boundaries, function-call arguments, Claude unselected tool-result fields, Gemini `functionResponse`, Interactions function/tool machine fields and unsupported variants, and every live/current model response and response event. Explicit historical output fields replayed inside a later request remain governed by the request selectors and assistant scope already documented.

Gemini machine exclusions include camelCase and snake_case non-null function, media, file, and code carriers; signature values remain excluded without excluding same-Part visible text. Interactions accepts camel-case `systemInstruction` when snake-case `system_instruction` is absent. Unknown SourceFormat, roles, item types, content blocks, and future protocol shapes are left unchanged.

Enabled known formats reject JSON objects with duplicate member names at any nesting depth. The plugin returns `censorship_invalid_request`; JSON text inside a string remains ordinary text rather than a nested request object. Scalar Claude user content cannot be stripped to empty: a matching rewrite terminates locally with `censorship_invalid_request`: censorship rewrite would make a text field invalid. The same response applies if a rewrite empties a Claude `TextBlockParam.text`, present `document.title`, or present `document.context`.

## Configuration reload

In non-Home local-config mode, CPA watches its YAML and invokes `plugin.reconfigure`. The plugin parses and compiles a complete immutable snapshot, then publishes it with one atomic store. Each request loads exactly one snapshot.

For an observable rollout, write configuration A, then change to configuration B with a unique term such as `snapshot-b-sentinel`. Poll a request containing only that term until B blocks it. Therefore: valid non-Home YAML changes apply without restart after observing a snapshot-B sentinel. Requests started after that observation use the complete B snapshot rather than a mix of A and B.

If B is malformed, it is logged and invalid reconfiguration keeps the last-known-good snapshot. Direct edits to Home-mode local YAML do not trigger `plugin.reconfigure`.

## Build, ABI, and integration

Go 1.26 and a working native or cross CGO compiler for the selected target are required. `make integration` builds against the fixed CPA commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`; its harness verifies upstream arrival, HTTP/1.1 EOF, chunk/trailer handling, exact provider paths, SSE, watcher reload, Responses WebSocket, and ABI scenarios. The integration oracle strictly validates complete, correctly typed HTTP and Responses WebSocket JSON.

```bash
make test
make race
make vet
make integration
```

Bare `make build` and host-artifact `make package` use `0.0.0-dev`; explicit empty or unsafe `VERSION` remains invalid.

The ABI boundary remains v1 and validates native pointer/length descriptors. Production retains `C.GoBytes` for input requests, makes no no-copy performance claim, and does not claim pinned Windows host request-pointer liveness has been proven. After-auth intentionally does not read input. When censorship replaces a decoded request body, it clears `Content-Encoding`, `Content-Length`, and `Transfer-Encoding`; no-op requests preserve headers.

## Accepted pure-plugin limits

1. hook is not raw ingress; document order follows current execution-body spans. The hook receives the body at CPA's request-execution boundary.
2. preprocessing can observe uncensored input. `ModelRouter` and some handler preprocessing run before this hook.
3. Responses WebSocket covers only model-executed turns. Raw ingress and unrelated WebSocket traffic are outside the plugin.
4. `generate=false` prewarm bypasses the plugin.
5. `/v1/realtime`, Live, sideband, and DataChannel bypass the plugin.
6. Alpha Search bypasses the plugin.
7. WebSocket block events omit `term` and `role`. CPA can emit status 400 and close, but that path cannot retain the plugin's custom HTTP error fields.
8. RequestInterceptor failures are fail-open. This includes current CPA error, panic, and fuse handling.
9. BeforeAuth runs once per handler execution; AfterAuth can run zero, one, or multiple times; every call carries the full body and incurs full-body RPC encoding/copy cost. The host still encodes and carries the full body on every AfterAuth call, but after recognizing the fixed no-op method, the plugin does not perform a C-to-Go input copy.
10. Home mode does not watch local YAML. Use a configuration path that CPA watches or another host-supported reconfiguration mechanism.
11. unknown SourceFormat and future content types are not inspected; review schema drift when upgrading CPA.
12. Known SourceFormat JSON objects accept at most 1024 simultaneously open object or array levels, including the top-level object. Deeper requests return `censorship_invalid_request`.
13. Methods that read or copy request input reject lengths that exceed `C.int` with a non-zero ABI return code. After-auth intentionally does not read input and may accept a coherent non-nil oversized descriptor. Oversized host callback responses return a plugin error.
14. The Responses WebSocket integration test fails after 20 seconds without `response.completed`; it does not wait indefinitely.
15. A provider-controlled Claude signed-history prefix can bind preceding input. The BeforeAuth hook cannot reliably know final model/binding controls, so the plugin neither mutates thinking/signatures nor adds an overbroad runtime rejection.
16. A filter bypass returns no replacement body or header changes before JSON validation.

## Release artifacts

Releases contain `censorship_<version>_<goos>_<goarch>.zip` and `censorship_<version>_<goos>_<goarch>.zip.sha256` for each supported tuple. Raw tag, `VERSION`, and packager `-version` inputs must be safe ASCII filename components. One lowercase ASCII `v` is then removed exactly once.

Direct and aggregate packaging rejects library/archive/checksum input/output aliases and aliases of known release outputs before changing those paths. Direct `-library`/`-archive`/`-checksum` mode rejects archive/checksum symlinks, known hard links, junctions, and ASCII case aliases. An unknown external hard-link peer of an existing destination is not modified: packaging replaces only the destination directory entry. Packaging produces deterministic ZIP archives. Each ZIP contains the platform library and an optional repository `LICENSE` if one exists. Each `.zip.sha256` line contains 64 lowercase hex characters, two spaces, and the archive basename. An aggregate packaging run covers exactly its present source platforms; a complete GitHub release/workflow contains all seven supported platform lines and assets. Aggregate packaging deletes a current-version ZIP and sidecar checksum when its source platform is absent on a rerun; other-version and unknown output files remain unchanged.

The release workflow overwrites assets only for an existing draft. A published-release rerun updates notes, downloads the remote assets, compares their exact files and bytes with `release/`, and verifies every sidecar checksum plus aggregate `checksums.txt` without overwriting public assets.