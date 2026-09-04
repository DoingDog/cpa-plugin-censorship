# cpa-plugin-censorship

`censorship` is a pure dynamic plugin for [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI). It examines selected text leaves in model request bodies before authentication. The plugin is request-only; never inspects or changes model output, response bodies, SSE chunks, or server WebSocket events.

The plugin exposes only `RequestInterceptor`. It has no custom panel, menu, or Management API. Configuration lives in CPA YAML, and words is the only term source; the plugin has no built-in terms, fallback list, or online download.

## Installation

Download the archive for the CPA host and copy the library to its platform plugin directory:

| Platform | CPA plugin path |
|---|---|
| Windows amd64 | `<CPA directory>/plugins/windows/amd64/censorship.dll` |
| Windows arm64 | `<CPA directory>/plugins/windows/arm64/censorship.dll` |
| Linux amd64 | `<CPA directory>/plugins/linux/amd64/censorship.so` |
| Linux arm64 | `<CPA directory>/plugins/linux/arm64/censorship.so` |
| macOS Intel | `<CPA directory>/plugins/darwin/amd64/censorship.dylib` |
| macOS Apple silicon | `<CPA directory>/plugins/darwin/arm64/censorship.dylib` |
| FreeBSD amd64 | `<CPA directory>/plugins/freebsd/amd64/censorship.so` |

Start or restart CPA to load a newly installed library. Once loaded, valid non-Home configuration changes can apply without another restart as described under [Configuration reload](#configuration-reload).

## Configuration

CPA management clients can edit the five plugin-owned top-level fields exposed through standard `ConfigFields` metadata. `mode` is an enum, `ignore_case` is a boolean, `words` is a JSON array, and `scope` and `obfs` are JSON objects. The saved values remain ordinary CPA YAML configuration.

```yaml
plugins:
  enabled: true
  configs:
    censorship:
      enabled: true
      mode: block
      ignore_case: false
      words:
        - example
      scope:
        formats: [openai, openai-response, claude, gemini, interactions]
        roles: [system, developer, user]
      obfs:
        char: "​"
```

Configuration types are strict and are not coerced. Unknown plugin keys, duplicate mapping keys, multiple YAML documents, invalid enum values, and incorrectly typed values reject the candidate configuration.

| Field | Contract |
|---|---|
| `enabled` | `enabled`: boolean; host-owned. CPA uses it to enable this plugin. |
| `mode` | `mode`: string; default `block`. Allowed values are `block`, `strip`, and `obfs`. |
| `ignore_case` | `ignore_case`: boolean; default `false`. |
| `words` | `words`: sequence of strings; default `[]`. Entries must be non-empty. |
| `scope.formats` | `scope.formats`: sequence of strings; default all five formats. Allowed values are `openai`, `openai-response`, `claude`, `gemini`, and `interactions`; explicit `[]` disables all formats. |
| `scope.roles` | `scope.roles`: sequence of strings; default `system`, `developer`, and `user`. Optional values are `assistant` and `tool`; explicit `[]` disables all roles. |
| `obfs.char` | `obfs.char`: must be `U+200B` or `U+2060`; default `U+200B`. |

The parser also accepts host-owned `priority` and `store` keys. It preserves each word's YAML order, duplicates, case, leading/trailing whitespace, and newlines; it does not sort, deduplicate, trim, normalize, or interpret regular expressions. In `obfs` mode every word must contain at least two Unicode scalars and must not already contain the configured `obfs.char`.

## Modes and rule ordering

Eligible text nodes follow the original JSON string-token byte order in the current execution body. Matches never cross nodes. Each rule finds leftmost non-overlapping occurrences.

- `block`: block checks rules before document order and returns the YAML term with canonical role. The first matching YAML word wins, and the earliest eligible node for that word supplies `role`. The request stops with HTTP 400 and is not sent upstream.
- `strip`: strip and obfs process all leftmost non-overlapping occurrences before the next rule. `strip` removes each actual source match from every eligible node.
- `obfs`: uses the same rule-major order; obfs preserves original case and inserts after the first Unicode scalar of every actual match.

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

Rules run once rather than to a fixed point. A later rule reads text produced by earlier rules. For example, `words: [ab, bc]` with `strip` transforms `abc` to `c`; reversing the word order transforms it to `a`. An identical request body, immutable configuration snapshot, and plugin version produce identical bytes, but repeated processing of already transformed content is not required to be idempotent.

## Case-insensitive matching

With `ignore_case: false`, matching is a case-sensitive literal substring operation. With `ignore_case: true`, the plugin compares equal-length Unicode scalar windows using Go `unicode.SimpleFold` equivalence while retaining byte spans from the original UTF-8 text.

- Alpha/aLPHA matches.
- Greek sigma variants `Σ`, `σ`, and `ς` match one another.
- Kelvin sign `K` matches `K`.
- straße/STRASSE does not match because full case folding can change scalar count.

There is no Unicode normalization and no full case folding. `strip` removes the actual source bytes; obfs preserves original case and inserts after the first Unicode scalar.

## Source formats and selected text

Selectors enter only the documented text leaves. There is no recursive fallback string walker.

| `SourceFormat` | Eligible request text |
|---|---|
| `openai` | String message content and typed `text` parts for enabled `system`, `developer`, `user`, or opt-in `assistant`; opt-in string tool message content. Legacy `/v1/completions` prompts arrive here as canonical `user` messages after CPA conversion. |
| `openai-response` | Top-level `instructions` as `system`, top-level string `input` as `user`, and documented message `content`, `input_text`, `output_text`, or `refusal` leaves. `output_text` and `refusal` use canonical `assistant` scope. |
| `claude` | Top-level string/typed-text `system`, enabled message string/typed-text content, and the narrow opt-in typed-text tool result path. |
| `gemini` | Text parts in `systemInstruction`/`system_instruction` and `contents`, excluding thought, signature, and machine-discriminator parts; Gemini `model` maps to `assistant`. |
| `interactions` | Documented system instruction and recursive input text subsets; `model` and `model_output` map to opt-in `assistant`; unknown and tool item types are excluded. |

OpenAI Responses `output_text` and `refusal` leaves use canonical `assistant` scope regardless of the source item role. Missing Gemini roles follow CPA's user/model alternation. Invalid Gemini roles advance CPA's user/model alternation but remain unselected. Interactions accepts camel-case `systemInstruction` when snake-case `system_instruction` is absent. Gemini machine exclusions include camelCase and snake_case function, signature, media, and code carriers.

assistant is inspected only when explicitly listed in scope.roles. This applies only to assistant history carried in a later request and never to live output.

tool is inspected only for OpenAI string tool content and Claude typed text tool_result content. More precisely:

- OpenAI requires `messages[*].role == "tool"` and JSON-string `content`.
- Claude requires a user message `tool_result` block whose `content[]` member has `type == "text"` and JSON-string `text`.

Unknown `SourceFormat`, roles, item types, content blocks, and future protocol shapes are left unchanged.

Enabled known formats reject JSON objects with duplicate member names at any nesting depth. The plugin returns `censorship_invalid_request`; JSON text inside a string remains ordinary text rather than a nested request object.

## Hard exclusions

Never changes tool calls, tool schemas, arguments, reasoning, thinking, other tool/function results, JSON keys, machine JSON, binary uploads, or image/audio/video/file base64.

This also excludes tool names and IDs, protocol discriminators, model names, metadata, control fields, thought signatures, URL/media fields, multipart headers and boundaries, Responses function/custom-tool outputs, Claude non-typed-text tool results, Gemini `functionResponse`, Interactions function/tool data, and every model response.

## Configuration reload

In non-Home local-config mode, CPA watches its YAML and invokes `plugin.reconfigure`. The plugin parses and compiles a complete immutable snapshot, then publishes it with one atomic store. Each request loads exactly one snapshot.

For an observable rollout, write configuration A, then change to configuration B with a unique term such as `snapshot-b-sentinel`. Poll a request containing only that term until B blocks it. Therefore: valid non-Home YAML changes apply without restart after observing a snapshot-B sentinel. Requests started after that observation use the complete B snapshot rather than a mix of A and B.

If B is malformed, it is logged and invalid reconfiguration keeps the last-known-good snapshot. Direct edits to Home-mode local YAML do not trigger `plugin.reconfigure`.

## Build and test

Go 1.26 and a working CGO compiler for the target are required.

```bash
make test
make race
make vet
make integration
```

`make integration` builds the plugin and fixed CPA commit `81e1b5374f99c212f196f34956eeed964a46b8fa`, then runs HTTP, SSE, watcher, Responses WebSocket, and ABI checks.

Build the current host or a selected host-supported target:

```bash
make build VERSION=v0.1.0
make build-platform VERSION=v0.1.0 GOOS=linux GOARCH=amd64
make package VERSION=v0.1.0 GOOS=windows GOARCH=amd64
```

`build-platform` and `package` require a suitable native or cross CGO compiler. GitHub Actions builds all seven supported tuples; the local `make build` convenience target always selects `GOHOSTOS` and `GOHOSTARCH`.

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

## Release artifacts

Releases contain these files for each supported tuple:

```plaintext
censorship_<version>_<goos>_<goarch>.zip
censorship_<version>_<goos>_<goarch>.zip.sha256
```

One lowercase ASCII `v` is removed from a tag, `VERSION`, or packager `-version` input. Each ZIP contains the platform library and an optional repository `LICENSE` if one exists. This repository does not add a license file.

Each `.zip.sha256` line contains 64 lowercase hex characters, two spaces, and the archive basename. `checksums.txt` aggregates the seven per-platform checksum lines.
