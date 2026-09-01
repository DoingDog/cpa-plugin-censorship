# Censorship initial release

Censorship is a pure CLIProxyAPI request-interceptor plugin. It filters selected text in request bodies before authentication and never inspects or modifies model responses, SSE chunks, or server WebSocket events. It has no panel, custom menu, Management API, built-in terms, fallback terms, or online word-list download.

## Configuration

```yaml
plugins:
  enabled: true
  configs:
    censorship:
      enabled: true
      mode: block
      ignore_case: false

      words: []

      scope:
        formats:
          - openai
          - openai-response
          - claude
          - gemini
          - interactions
        roles:
          - system
          - developer
          - user

      obfs:
        char: "​"
```

The plugin configuration is a YAML mapping. Types are strict; values are not coerced.

- `mode` is a string: `block`, `strip`, or `obfs`. Default: `block`.
- `ignore_case` is a YAML boolean. Default: `false`.
- `words` is a sequence of non-empty strings and is the only word-list source. Default: `[]`.
- `scope.formats` is a sequence containing only `openai`, `openai-response`, `claude`, `gemini`, and `interactions`. Default: all five. An explicit `[]` disables every format.
- `scope.roles` is a sequence containing only `system`, `developer`, `user`, `assistant`, and `tool`. Default: `system`, `developer`, and `user`. An explicit `[]` disables every role.
- `obfs` is a mapping. `obfs.char` is a string containing exactly one U+200B ZERO WIDTH SPACE or U+2060 WORD JOINER. Default: U+200B.
- `enabled`, `priority`, and `store` are host-owned keys accepted in the plugin mapping. Other unknown keys are rejected.

Words preserve YAML order, duplicates, case, whitespace, and newlines. They are not sorted, deduplicated, trimmed, normalized, or interpreted as regular expressions. In `obfs` mode each word must contain at least two Unicode scalars and must not contain the configured `obfs.char`.

With `ignore_case: false`, matching uses case-sensitive literal substrings. With `ignore_case: true`, matching uses Go `unicode.SimpleFold` equivalence at original Unicode scalar boundaries. `Alpha` matches `aLPHA`; Greek `Σ`, `σ`, and `ς` match one another; Kelvin sign `K` matches `K`; `straße` does not match `STRASSE`. No full case folding or Unicode normalization is performed.

## Modes and ordering

`words` always run in YAML order. Eligible text nodes are ordered by their original JSON string-token byte offsets.

- `block` checks words first, then nodes. The first matching YAML word wins; the earliest matching node for that word supplies the canonical role. It returns HTTP 400 with `censorship_blocked`, the original YAML `term`, and `role`, and does not send the request upstream.
- `strip` processes every word across every eligible node, removing all left-to-right non-overlapping occurrences before continuing to the next word. It removes the actual source match, including its original case.
- `obfs` uses the same rule-major, all-node order. It inserts exactly one `obfs.char` after the first Unicode scalar of every actual match and preserves the source match's original case.

Rules run once, not to a fixed point. A later word sees text produced by earlier words. For an identical request body, immutable configuration snapshot, and plugin version, output bytes are deterministic; idempotence is not promised.

## Request selectors and scope

The plugin supports the five `SourceFormat` values shown in the YAML. It selects only explicitly documented text leaves from system/developer/user messages, OpenAI Responses instructions/input messages, Claude text blocks, Gemini text parts, and Interactions text input. Legacy `/v1/completions` prompts are handled as canonical `user` text after CLIProxyAPI conversion.

`assistant` is opt-in. When explicitly listed, it covers assistant history carried in a later request; it never enables live model-output filtering.

`tool` is opt-in and intentionally narrow:

- OpenAI: only `messages[*]` with `role: "tool"` when `content` is a JSON string.
- Claude: only `tool_result.content[]` blocks whose `type` is `text` and whose `text` value is a JSON string.

Changing a valid non-Home CPA YAML configuration triggers `plugin.reconfigure`; the complete immutable snapshot changes atomically without restarting CPA. An invalid reconfiguration is logged and the last-known-good snapshot remains active. Direct edits to Home-mode local YAML do not trigger reconfiguration.

## Hard exclusions

The following are always excluded, regardless of scope:

- JSON keys, protocol discriminators, model names, IDs, metadata, and control fields.
- Tool calls, schemas, names, IDs, arguments, Responses `function_call_output` and `custom_tool_call_output`, Claude string/object tool results, Gemini `functionResponse`, Interactions `function_result`, and other tool data.
- Reasoning, thinking, thought, redacted thinking, and thought signatures.
- Images, audio, video, files, URLs, base64, uploaded binary data, multipart headers, filenames, and boundaries.
- Unknown roles, item types, content blocks, and event types.
- Every response body, SSE chunk, and server WebSocket event.

No fallback recursive string walker is used.

## Accepted pure-plugin limits

1. The hook is not raw ingress; document order is defined by spans in the current execution body.
2. `ModelRouter` and some handler preprocessing can see unfiltered content before the request hook.
3. Responses WebSocket coverage is limited to turns that enter model execution.
4. Local prewarm with `generate=false` does not enter the plugin.
5. `/v1/realtime`, Live, sideband, and DataChannel paths do not enter the plugin.
6. Alpha Search does not enter the plugin.
7. A blocked Responses WebSocket turn can only produce a status 400 error event and close; it cannot retain custom `term` and `role` fields.
8. Current RequestInterceptor errors, panics, and fuse events are fail-open in CLIProxyAPI.
9. Each handler execution calls BeforeAuth once. AfterAuth can run zero, one, or multiple times per credential/model attempt; every call carries the complete body, so the fixed AfterAuth no-op does not remove that body-encoding cost.
10. Direct edits to Home-mode local YAML do not trigger `plugin.reconfigure`.
11. Unknown `SourceFormat` values and future content types are not filtered by default; each CLIProxyAPI baseline upgrade requires schema-drift review.

## Artifacts

Seven archives are produced for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`, `windows/arm64`, and `freebsd/amd64`:

```plaintext
censorship_<version>_<goos>_<goarch>.zip
censorship_<version>_<goos>_<goarch>.zip.sha256
```

A lowercase ASCII `v` prefix is removed once from tag, `VERSION`, and `-version` inputs. Each ZIP contains the platform library and an optional repository `LICENSE` if one exists. This release does not add a license file. Each checksum line is lowercase SHA-256, two ASCII spaces, and the archive basename; `checksums.txt` aggregates all platforms.
