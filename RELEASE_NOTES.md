# Censorship v0.1.2

## Performance

Folded `block` matching now uses a precompiled SimpleFold-aware trie with failure links, so each eligible span is scanned once instead of once per rule. The matcher preserves YAML rule priority, document-order role selection, Unicode scalar boundaries, and span isolation.

Folded `strip` and `obfs` rewrites now stream directly into a `strings.Builder` without retaining an occurrence list. Dense obfuscation reserves the maximum non-overlapping output capacity, avoiding repeated builder growth and copy operations. Fuzz snapshots reuse their precompiled matcher.

The focused TDD checks cover Kelvin sign canonicalization, failure suffixes, prefix and duplicate folded rules, Unicode cases, cross-span isolation, rule-major role selection, all non-overlapping rewrite occurrences, and source-case-preserving obfuscation.

## Visual configuration

This release exposes `mode`, `ignore_case`, `words`, `scope`, and `obfs` through standard CPA `ConfigFields` metadata. CPA Manager Plus renders an enum selector, a boolean switch, a JSON array editor, and JSON object editors, then saves the values into the existing CPA YAML structure.

Censorship is a pure CLIProxyAPI request-interceptor plugin. It is request-only; never inspects or changes model output, response bodies, SSE chunks, or server WebSocket events. It has no custom panel, menu, or Management API.

Configuration remains stored in CPA YAML: words is the only term source; the plugin has no built-in terms, fallback terms, or online word-list download.

## Configuration

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

Types are strict and are not coerced. Unknown plugin keys, duplicate mapping keys, multiple YAML documents, invalid enum values, and incorrectly typed values reject the candidate configuration.

- `enabled`: boolean; host-owned. CPA uses it to enable this plugin.
- `mode`: string; default `block`. Allowed values are `block`, `strip`, and `obfs`.
- `ignore_case`: boolean; default `false`.
- `words`: sequence of strings; default `[]`. Entries must be non-empty.
- `scope.formats`: sequence of strings; default all five formats. Allowed values are `openai`, `openai-response`, `claude`, `gemini`, and `interactions`; explicit `[]` disables every format.
- `scope.roles`: sequence of strings; default `system`, `developer`, and `user`. Optional values are `assistant` and `tool`; explicit `[]` disables every role.
- `obfs.char`: must be `U+200B` or `U+2060`; default `U+200B`.
- `priority` and `store` are also accepted as host-owned keys.

Words preserve YAML order, duplicates, case, whitespace, and newlines. They are not sorted, deduplicated, trimmed, normalized, or interpreted as regular expressions. In `obfs` mode every word must contain at least two Unicode scalars and must not contain the configured `obfs.char`.

## Modes and ordering

Eligible text nodes follow original JSON string-token byte order in the current execution body. Matches never cross nodes, and each rule finds leftmost non-overlapping occurrences.

- block checks rules before document order and returns the YAML term with canonical role. The first matching word wins, and the earliest eligible node for that word supplies `role`; the plugin returns HTTP 400 without sending the request upstream.
- strip and obfs process all leftmost non-overlapping occurrences before the next rule. Each rule finishes every eligible node before the next YAML word starts.
- obfs preserves original case and inserts after the first Unicode scalar of every actual match.

Rules run once, not to a fixed point. A later word reads text produced by earlier words. An identical request body, immutable configuration snapshot, and plugin version produce identical bytes; idempotence is not promised.

The HTTP block body is:

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

## Case-insensitive matching

With `ignore_case: false`, matching is a case-sensitive literal substring operation. With `ignore_case: true`, the plugin uses Go `unicode.SimpleFold` equivalence over equal-length Unicode scalar windows, with byte spans taken from the original UTF-8 text.

- Alpha/aLPHA matches.
- Greek sigma variants `Σ`, `σ`, and `ς` match one another.
- Kelvin sign `K` matches `K`.
- straße/STRASSE does not match because full case folding can change scalar count.

There is no Unicode normalization and no full case folding. `strip` removes actual source matches; obfs preserves original case and inserts after the first Unicode scalar.

## Request selectors and scope

The plugin supports `openai`, `openai-response`, `claude`, `gemini`, and `interactions`. It selects only documented system/developer/user request leaves, OpenAI Responses instructions/input messages, Claude text blocks, Gemini text parts, and Interactions text input. Legacy `/v1/completions` prompts are handled as canonical `user` messages after CPA conversion.

OpenAI Responses `output_text` and `refusal` leaves use canonical `assistant` scope regardless of the source item role. Missing Gemini roles follow CPA's user/model alternation. Invalid Gemini roles advance CPA's user/model alternation but remain unselected. Interactions accepts camel-case `systemInstruction` when snake-case `system_instruction` is absent. Gemini machine exclusions include camelCase and snake_case function, signature, media, and code carriers.

assistant is inspected only when explicitly listed in scope.roles. It covers assistant history carried in a later request and never enables live model-output filtering.

tool is inspected only for OpenAI string tool content and Claude typed text tool_result content. More precisely:

- OpenAI requires `messages[*].role == "tool"` and JSON-string `content`.
- Claude requires a user message `tool_result` block whose `content[]` member has `type == "text"` and JSON-string `text`.

Unknown `SourceFormat`, roles, item types, content blocks, and future protocol shapes are left unchanged. No fallback recursive string walker is used.

Enabled known formats reject JSON objects with duplicate member names at any nesting depth. The plugin returns `censorship_invalid_request`; JSON text inside a string remains ordinary text rather than a nested request object.

## Configuration reload

Changing a valid non-Home CPA YAML configuration triggers `plugin.reconfigure`; a complete immutable snapshot changes atomically without restarting CPA. Poll a request containing a unique B-only term until it is blocked before declaring the rollout observed: valid non-Home YAML changes apply without restart after observing a snapshot-B sentinel.

An invalid candidate is logged, and invalid reconfiguration keeps the last-known-good snapshot. Direct edits to Home-mode local YAML do not trigger reconfiguration.

## Hard exclusions

Never changes tool calls, tool schemas, arguments, reasoning, thinking, other tool/function results, JSON keys, machine JSON, binary uploads, or image/audio/video/file base64.

This also excludes tool names and IDs, protocol discriminators, model names, metadata, control fields, thought signatures, URL/media fields, multipart headers and boundaries, Responses function/custom-tool outputs, Claude non-typed-text tool results, Gemini `functionResponse`, Interactions function/tool data, and every response body, SSE chunk, and server WebSocket event.

## Accepted pure-plugin limits

1. hook is not raw ingress; document order follows current execution-body spans. The hook receives the body at CPA's request-execution boundary.
2. preprocessing can observe uncensored input. `ModelRouter` and some handler preprocessing run before this hook.
3. Responses WebSocket covers only model-executed turns.
4. `generate=false` prewarm bypasses the plugin.
5. `/v1/realtime`, Live, sideband, and DataChannel bypass the plugin.
6. Alpha Search bypasses the plugin.
7. WebSocket block events omit `term` and `role`. That path can emit status 400 and close, but cannot retain the custom HTTP fields.
8. RequestInterceptor failures are fail-open. This includes current CPA error, panic, and fuse handling.
9. BeforeAuth runs once per handler execution; AfterAuth can run zero, one, or multiple times; every call carries the full body and incurs full-body RPC encoding/copy cost. AfterAuth is always a no-op in this plugin, but transport cost remains.
10. Home mode does not watch local YAML.
11. unknown SourceFormat and future content types are not inspected; review schema drift when upgrading CPA.

## Artifacts

Seven platform archives are produced:

```plaintext
censorship_<version>_<goos>_<goarch>.zip
censorship_<version>_<goos>_<goarch>.zip.sha256
```

Supported tuples are `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`, `windows/arm64`, and `freebsd/amd64`. One lowercase ASCII `v` is removed from tag, `VERSION`, and packager `-version` inputs.

Each ZIP contains the platform library and the repository `LICENSE`. Each `.zip.sha256` line contains 64 lowercase hex characters, two spaces, and the archive basename. `checksums.txt` aggregates all seven lines.
