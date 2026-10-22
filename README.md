# cpa-plugin-censorship

`censorship` is a pure dynamic plugin for [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI). It examines selected text leaves in model request bodies. When filter.models is empty, model-free filters process BeforeAuth. When filter.models is non-empty, model-bearing filters skip BeforeAuth and process only selected-auth `RequestAfterAuthInterceptor` calls. The plugin is request-only; does not inspect live model output, response bodies, SSE output, or server WebSocket output. Explicit historical output fields replayed as later request input may be inspected under assistant scope.

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

v0.3.11 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+. The registered logo is `https://raw.githubusercontent.com/DoingDog/cpa-plugin-censorship/main/logo.png`.

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

Full-string, case-sensitive glob matching uses `*` for zero or more Unicode scalars and `?` for exactly one Unicode scalar. When filter.models is empty, the model-free filter gate and transforms process at BeforeAuth. When filter.models is non-empty, every BeforeAuth call returns no-op before request-envelope or body parsing; only a selected-auth `RequestAfterAuthInterceptor` call processes. A non-empty, non-whitespace string in either `selected_auth_id` or `selected_auth_index` marks a selected-auth attempt. `selected_auth_id` and `selected_auth_index` are CLIProxyAPI v7.2.152 implementation-level phase signals, not a cross-version stable `FinalModelResolved` API. A pinned host change requires re-verification of their semantics. That attempt uses `request.Model` as its only model subject. The plugin does not parse request-body model fields or implement CPA alias routing. It does not use `RequestedModel`, URL, headers, callback `source`, or other Metadata as model fallbacks. The filter gate is evaluated independently for each valid selected-auth AfterAuth invocation; an ordinary AuthManager attempt receives at most one transform. `api-keys` matches authenticated `caller_scope`. `Principal` and literal credentials, including credentials carried in headers, do not match `caller_scope` when they differ.

All five supported `SourceFormat` values keep the same provider selectors and use the stage source above. When filter.models is empty, exact API-key filters process at BeforeAuth using `Metadata["caller_scope"]`. When filter.models is non-empty, model+API combinations move together on a valid selected-auth AfterAuth call: model matching uses `request.Model`, exact API-key matching uses `Metadata["caller_scope"]`, and the documented wildcard credential carriers inspect headers visible at AfterAuth. The supported mapped scope is the non-stream host.model.execute callback that returns to ordinary AuthManager. Wildcard credential carriers are scanned in this exact order: `Authorization`, `X-Goog-Api-Key`, then `X-Api-Key`. `Authorization` accepts case-insensitive `Bearer <credential>` or a raw credential. Every wildcard candidate is trimmed and scope-bound before glob matching. Other headers and query-only credentials are unsupported wildcard carriers.

A mapped nonexcluded block on the supported callback path remains zero-upstream, but pinned CLIProxyAPI v7.2.152 loses the nested terminal response and surfaces HTTP 500 to the client. This is a pinned-host limitation, not the intended censorship response. The selected-auth `request.Model` can differ from the final wire model after Executor-side payload rewrites. Mapped `host.model.execute_stream`, opaque terminal Executor, and Antigravity credits fallback paths are excluded from this release's post-route model-filter guarantee.

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

Selectors enter only the explicit text leaves listed here. There is no recursive fallback string walker. Selected escaped JSON string tokens use Go `encoding/json.Unmarshal` semantics, including replacement of unpaired UTF-16 surrogates with `U+FFFD`; unescaped tokens retain their direct text path. No-op or cancelled rewrites preserve the original string tokens and surrounding request bytes.

- OpenAI explicit text paths: Chat `messages[*]` string `content`, typed `text` content parts, `refusal` text, `prediction.content` only when `prediction.type == "content"`, documented `tool` and legacy `function` result text, legacy `functions[*]` descriptions/schema descriptions, `tools[*].function` descriptions/schema descriptions, nested `tools[*].custom.description`, flat custom tool `tools[*].description`, and `response_format.json_schema` descriptions/schema descriptions. Chat prediction uses canonical `assistant`; Chat definitions use canonical `developer`. Chat function tools inspect `parametersJsonSchema` only when `parameters` is absent, and inspect `output_schema` description leaves independently; legacy `functions[*].output_schema` and Responses function `output_schema` descriptions are also selected. Responses top-level `instructions`, top-level string `input`, message `content`, `input_text`, `output_text`, and `refusal`, `prompt.variables` scalar or `input_text` values, `text.format` JSON-schema descriptions, top-level tool descriptions/schema descriptions, `additional_tools` definitions with only documented canonical roles, and `tool_search_output`/`mcp_list_tools` loaded definitions, plus documented `function`, `custom-tool`, `shell`, `apply-patch`, `MCP`, and `program` result-output text: `program_output.result`, `mcp_call.output`, `mcp_call.error.message` only for `mcp_protocol_error` and `http_error`, scalar `mcp_list_tools.error`, `file_search_call.results[*].text`, and `code_interpreter_call.outputs[type=logs].logs`. Responses prompt variables and local shell skill descriptions use canonical `user`; OpenAI `mcp_approval_response.reason` is selected as `user`; `additional_tools` local skills inherit the validated item role; top-level local skills remain `user`; text format and top-level definitions use canonical `developer`; loaded definitions and result-text leaves use canonical `tool`; `additional_tools` uses only its exact documented canonical role. Legacy `/v1/completions` prompts arrive as canonical `user` messages after CPA conversion. Unsupported error/output variants and machine siblings remain unchanged.
  Chat flat custom tool descriptions require an Object tool, string `type: "custom"`, a non-empty string top-level `name`, a non-empty string top-level `description`, and enabled `developer` scope. Names are not trimmed or matched. Nested `custom.description` remains independent of the flat name gate; missing, empty, null, or safe nested descriptions do not hide an eligible flat description. When both descriptions qualify, both are inspected in original token order. Full strip may leave the flat description empty; Chat custom tools do not use Responses description fallback or dormant activation.
  Responses top-level `instructions` uses canonical `system` when `scope.roles` enables `system`; otherwise it uses canonical `developer` when `scope.roles` enables `developer`. Function and custom tool descriptions fall back to `function.description` when flat `description` is empty. Function parameter schema descriptions come from the first present of `parameters`, `parametersJsonSchema`, `input_schema`, `function.parameters`, and `function.parametersJsonSchema`. If `strip` empties an active flat description, the newly effective nested string description is filtered before forwarding; a newly effective non-string description rejects the rewrite locally.
- Claude explicit text paths: top-level string or typed-text `system`, enabled message string or typed-text content, direct user `search_result.title` and `search_result.content[*].text` for exact typed-text blocks, `document.title`, `document.context`, `document.source.data` when `source.type == "text"`, and scalar `document.source.content` or its exact typed-text array form when `source.type == "content"`, and a user `tool_result`'s string content or nested `text`, `search_result`, and `document` text. Anthropic exact compaction instructions use canonical `system`; Beta `mcp_tool_result` string and exact typed-text results inside user or assistant messages use canonical `tool` independently of the enclosing message's own role scope. Ordinary `tool_result` remains user-message-only. Direct user blocks use `user` and nested tool-result blocks use `tool`. Top-level user-defined `tools[*].description`, `tools[*].input_schema` JSON Schema description leaves, and `output_config.format.schema` description leaves when `format.type == "json_schema"`, plus legacy beta `output_format.schema` description leaves when `output_format.type == "json_schema"`, use canonical `system` scope; tool names, schema keys, non-description schema values, and `input_examples` remain excluded.
- Gemini explicit text paths: text parts in `systemInstruction` or `system_instruction` and `contents`, subject to the selected canonical role. `tools[*].functionDeclarations[*].description` and `tools[*].function_declarations[*].description`, `description` leaves within `parameters`, `parametersJsonSchema`, `parameters_json_schema`, `response`, `responseJsonSchema`, and `response_json_schema`, and schema `description` leaves at `generationConfig.responseSchema`, `generationConfig.responseJsonSchema`, and `generationConfig.responseFormat.text.schema` use canonical `developer`. The four additional `generationConfig` schema aliases `response_schema`, `_responseJsonSchema`, `response_json_schema`, and `response_json_schema_ordered` use the same scope; function names, enum values, call arguments, and signatures remain excluded. The signature field and value remain excluded, but a non-null `thoughtSignature`, `thought_signature`, or `extra_content.google.thought_signature` does not exclude visible text in the same Part. A JSON `null` carrier is unset. A non-null function, media, file, or code carrier still excludes the whole Part. A matching `block` handles signed visible text normally; a matching `strip` or `obfs` never changes it and returns local `censorship_invalid_request` with `censorship cannot rewrite signature-bound text`.
- Interactions explicit text paths: documented `system_instruction` or fallback camel-case `systemInstruction`, documented input text subsets, direct input object or array items with exact `type: "text"` and text content, function/MCP result strings and exact typed-text array items, code execution result strings, function declaration descriptions, parameter JSON Schema description leaves, and exact text response schema description leaves. Function/MCP result strings and exact typed-text array items use canonical `tool`. Code execution result strings use canonical `tool`; a non-null sibling signature for those code execution result strings permits `block` but rejects a final rewrite. Function declaration descriptions, parameter JSON Schema description leaves, and exact text response schema description leaves use canonical `developer`. `response_format` accepts an exact `type: "text"` object or an array of those objects; only their schema description leaves are selected. Only non-empty annotations on exact `model_output.content` text protect rewrites; caller-authored annotated text remains rewritable. Annotated `model_output.content` text remains blockable. A final text or byte change to annotated Interactions `model_output.content` with non-empty annotations returns local `censorship_invalid_request`; exact cancellation back to the original bytes is allowed. Empty annotations add no restriction.

OpenAI Responses `output_text` and `refusal` leaves use canonical `assistant` scope regardless of the source item role. Missing Gemini roles follow CPA's user/model alternation, except that a content containing `functionResponse` or `function_response` makes a missing, null, or empty role `user` and resets the next inferred role. Invalid Gemini roles remain unselected; those containing a function response reset the next inferred role to `user`, while other invalid roles advance the usual alternation. Gemini `model` maps to `assistant`.

assistant is inspected only when explicitly listed in scope.roles. This applies only to assistant history carried in a later request and never to live output. tool is inspected only for documented OpenAI, Claude, and Interactions result-text paths, including OpenAI string tool content, Claude selected tool_result text, and Interactions selected function/MCP/code execution result text.

OpenAI Chat tool/legacy function string `content` and Responses `function_call_output.output`/`custom_tool_call_output.output` recognize a JSON-encoded array as a multimodal carrier only when it contains an `image_url` object with a non-empty string `url` or `file_id`, or an `input_image` with a non-empty string `image_url` or `file_id`. Within that carrier, only string `text` leaves of exact `text`, `input_text`, and `output_text` items are selected with `tool` scope. Image URLs, file IDs, base64, keys, and machine siblings retain their bytes; image-only carriers are no-op. Matching keeps rule order and outer token order, with no matches across inner text nodes. Other JSON strings, including arrays without those image shapes and other provider/result paths, remain whole natural-language strings.

Final rewritten text is measured in Unicode code points. OpenAI Responses limits `function_call_output.output` strings and typed `input_text` leaves, `shell_call_output.output[*].stdout`/`stderr`, `apply_patch_call_output.output`, and `program_output.result` to 10,485,760 code points per selected field. For an encoded `function_call_output.output` carrier, the limit applies to the complete rebuilt inner JSON string. Claude `document.title` is limited to 500 code points in direct user documents and nested user `tool_result` documents. Limits apply only to changed final text after all rules; unchanged or exactly cancelled fields are preserved. A final rewrite exceeding a limit returns local `censorship_invalid_request` without truncation. Custom tool outputs, local shell outputs, and unrelated text fields do not inherit these limits.

## Machine exclusions

Machine exclusions: tool calls, machine schema values, arguments, reasoning, thinking, JSON keys, machine JSON, binary uploads, and image/audio/video/file base64 are never changed. The selected natural-language tool and schema descriptions listed above are the only definition exception; text result fields listed above are the only tool/function result exception.

Machine arguments, grammar definitions, names, IDs, paths, schema values, and reasoning state remain excluded.

This also excludes tool names and IDs, protocol discriminators, model names, metadata, control fields, thought signatures, URL/media fields, multipart headers and boundaries, function-call arguments, Claude unselected tool-result fields, Gemini `functionResponse`, Interactions function/tool machine fields and unsupported variants, and every live/current model response and response event. Explicit historical output fields replayed inside a later request remain governed by the request selectors and assistant scope already documented.

Gemini machine exclusions include camelCase and snake_case non-null function, media, file, and code carriers; signature values remain excluded without excluding same-Part visible text. Interactions accepts camel-case `systemInstruction` when snake-case `system_instruction` is absent. Unknown SourceFormat, roles, item types, content blocks, and future protocol shapes are left unchanged.

Enabled known formats reject JSON objects with duplicate member names at any nesting depth. The plugin returns `censorship_invalid_request`; JSON text inside a string remains ordinary text except for the explicit OpenAI multimodal carriers above. Scalar Claude user content cannot be stripped to empty: a changed final rewrite terminates locally with `censorship_invalid_request`: censorship rewrite would make a text field invalid. This restriction also applies to non-final assistant scalar history and `messages[*]` system scalar content without a per-message `output_config.effort` field, including `output_config: {}`. The same invalid-text response applies if a rewrite empties a Claude `TextBlockParam.text` (including final assistant typed text), present `document.title`, or present `document.context`.

Top-level `system` scalar, system scalar with an existing per-message `output_config.effort` field, final assistant scalar prefill, and `tool_result`/`mcp_tool_result` scalar retain their existing full-strip behavior. Effort-only `content: []` remains untouched. Preserving effort-bearing scalar behavior does not establish provider acceptance of empty scalar content or validate the effort value.

## Configuration reload

In non-Home local-config mode, CPA watches its YAML and invokes `plugin.reconfigure`. The plugin parses and compiles a complete immutable snapshot, then publishes it with one atomic store. Each request loads exactly one snapshot. A same-instance config-only reconfigure may update only within one `filter.models` phase class: empty-to-empty and non-empty-to-non-empty are allowed; a candidate that changes the phase class is rejected and keeps the last-known-good snapshot.

A filter.models phase change requires restart: drain in-flight requests, then fully restart before a phase-class, plugin binary, or enabled-status change. The plugin cannot drain requests or preserve at-most-once filtering across binary replacement, disable/enable, or restart.

For an observable allowed rollout, write configuration A, then change to configuration B with a unique term such as `snapshot-b-sentinel`. Poll a request containing only that term until B blocks it. Requests started after that observation use the complete B snapshot rather than a mix of A and B.

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

The ABI boundary remains v1 and validates native pointer/length descriptors. Production synchronously uses `C.GoBytes` for every request input it reads, including selected-auth AfterAuth, so AfterAuth incurs an input-sized C-to-Go copy and does not retain the host pointer. It makes no no-copy performance claim and does not claim pinned Windows host request-pointer liveness has been proven. When censorship replaces a decoded request body, it clears `Content-Encoding`, `Content-Length`, and `Transfer-Encoding`; no-op requests preserve headers.

## Accepted pure-plugin limits

1. hook is not raw ingress; document order follows current execution-body spans. The hook receives the body at CPA's request-execution boundary.
2. preprocessing can observe uncensored input. `ModelRouter` and some handler preprocessing run before this hook.
3. Responses WebSocket covers only model-executed turns. Raw ingress and unrelated WebSocket traffic are outside the plugin.
4. `generate=false` prewarm bypasses the plugin.
5. `/v1/realtime`, Live, sideband, and DataChannel bypass the plugin.
6. Alpha Search bypasses the plugin.
7. WebSocket block events omit `term` and `role`. CPA can emit status 400 and close, but that path cannot retain the plugin's custom HTTP error fields.
8. RequestInterceptor failures are fail-open. This includes current CPA error, panic, and fuse handling.
9. BeforeAuth runs once per handler execution; AfterAuth can run zero, one, or multiple times; every AfterAuth call that reads an input body performs synchronous `C.GoBytes` and incurs input-sized copy. The host still encodes and carries the full body on every AfterAuth call.
10. Home mode does not watch local YAML. Use a configuration path that CPA watches or another host-supported reconfiguration mechanism.
11. unknown SourceFormat and future content types are not inspected; review schema drift when upgrading CPA.
12. Known SourceFormat JSON objects accept at most 1024 simultaneously open object or array levels, including the top-level object. Deeper requests return `censorship_invalid_request`.
13. Methods that read or copy request input, including selected-auth AfterAuth, reject lengths that exceed `C.int` with a non-zero ABI return code. Oversized host callback responses return a plugin error.
14. The Responses WebSocket integration test fails after 20 seconds without `response.completed`; it does not wait indefinitely.
15. A provider-controlled Claude signed-history prefix can bind preceding input. The BeforeAuth hook cannot reliably know final model/binding controls, so the plugin neither mutates thinking/signatures nor adds an overbroad runtime rejection.
16. A filter bypass returns no replacement body or header changes before JSON validation.

## Release artifacts

Releases contain `censorship_<version>_<goos>_<goarch>.zip` and `censorship_<version>_<goos>_<goarch>.zip.sha256` for each supported tuple. Raw tag, `VERSION`, and packager `-version` inputs must be safe ASCII filename components. One lowercase ASCII `v` is then removed exactly once.

Direct and aggregate packaging rejects library/archive/checksum input/output aliases and aliases of known release outputs before changing those paths. Direct `-library`/`-archive`/`-checksum` mode rejects archive/checksum symlinks, known hard links, junctions, and ASCII case aliases. The existing conservative ASCII alias rule applies on all platforms. Windows additionally checks non-ASCII names using the actual existing ancestor directory's case-sensitivity attribute and native `CompareStringOrdinal` comparison; absent basenames and parent directories are covered. Case-sensitive directories retain distinct Unicode names. This comparison follows Windows behavior, with no general Unicode case folding applied to other platforms. An unknown external hard-link peer of an existing destination is not modified: packaging replaces only the destination directory entry. Packaging produces deterministic ZIP archives. Each ZIP contains the platform library and an optional repository `LICENSE` if one exists. Each `.zip.sha256` line contains 64 lowercase hex characters, two spaces, and the archive basename. An aggregate packaging run covers exactly its present source platforms; a complete GitHub release/workflow contains all seven supported platform lines and assets. Aggregate packaging deletes a current-version ZIP and sidecar checksum when its source platform is absent on a rerun; other-version and unknown output files remain unchanged.

Direct CLI calls pass the main source and exactly one helper for the running platform. Windows uses:

```powershell
go run ./.github/scripts/package-release.go ./.github/scripts/package-release_windows.go -version v0.3.11 -library dist/windows_amd64/censorship.dll -archive dist/censorship_0.3.11_windows_amd64.zip -checksum dist/censorship_0.3.11_windows_amd64.zip.sha256
```

Unix uses:

```bash
go run ./.github/scripts/package-release.go ./.github/scripts/package-release_other.go -version v0.3.11 -library dist/linux_amd64/censorship.so -archive dist/censorship_0.3.11_linux_amd64.zip -checksum dist/censorship_0.3.11_linux_amd64.zip.sha256
```

Choose the helper by the running host, independently of the library target. Explicit Go source lists do not filter the two helpers by build tags or platform filenames. From a cwd outside the repository, use absolute paths to both source files in their shared repository directory and keep the original cwd: relative flags, output paths, optional `LICENSE`, and Git version lookup still resolve from the caller cwd. The packager uses only the standard library and requires no CGO. Both Make package recipes compile and run it with `GOHOSTOS`/`GOHOSTARCH`, while the library build keeps its selected target.

The release workflow overwrites assets only for an existing draft. A published-release rerun updates notes, downloads the remote assets, compares their exact files and bytes with `release/`, and verifies every sidecar checksum plus aggregate `checksums.txt` without overwriting public assets.

v0.3.11 local verification scope includes Windows amd64 tests, race, vet, independent packager checks, bounded fuzz, both package recipes, fixed-host integration, and formal DLL ABI registration. Chat->Codex fixtures verify selection, rewriting, converted Responses tool descriptions, and preserved machine fields using a local upstream. New main/tag Linux and Windows CI checks, seven-platform builds, Linux `.so` registration, and release asset acceptance remain pending; required main CI jobs must succeed before creating the final tag. Commercial provider API acceptance and native loading on every platform are outside this local verification scope.