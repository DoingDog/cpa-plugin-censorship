# Censorship v0.2.0 Architectural Design

**Date:** 2026-09-07
**Status:** Approved for implementation
**Evidence basis:** The supplied partition reports only. This design does not depend on a new repository scan.

## 1. Decision summary

v0.2.0 extends the existing configuration compiler and provider-specific selector modules. It does not add a package, public interface, parser dependency, host patch, or ABI version.

The release has four product changes:

1. Canonical configuration uses `words: {block: [...], strip: [...], obfs: [...]}`. The handwritten YAML parser alone retains the legacy sequence plus global `mode` compatibility form.
2. A compiled snapshot retains one contiguous ordered rule slice with block and strip cut points. Requests always execute `block -> strip -> obfs`.
3. Existing provider selectors cover the additional API-valid prose fields proven by the scan reports while continuing to exclude protocol and machine fields.
4. Plugin metadata exposes only the canonical `words` object editor and the repository icon.

Before those changes, upgrade `github.com/router-for-me/CLIProxyAPI/v7` and the integration host pin atomically to v7.2.152 at commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. This advances the RPC schema to 5 while native ABI v1 and the `RequestInterceptor` contract remain compatible. The release also repairs the integration and release tooling defects needed to verify the product changes.

## 2. Goals

- Preserve every valid legacy configuration and its behavior.
- Make the canonical configuration a strict `words` mapping with optional `block`, `strip`, and `obfs` term sequences; any zero, one, two, or three buckets may appear.
- Preserve the legacy sequence plus global `mode` only in the handwritten YAML parser, compatibility documentation, and compatibility tests.
- Allow all three actions in one immutable snapshot while retaining one contiguous ordered rule slice.
- Make cross-action behavior deterministic and independent of YAML mapping order.
- Preserve atomic, last-known-good configuration publication.
- Select all API-valid natural-language request fields identified by the provider scan reports.
- Leave IDs, arguments, commands, URLs, binary data, signatures, reasoning data, statuses, and other machine fields byte-identical.
- Advertise the canonical mapping through existing plugin metadata and set the icon URL.
- Make the tagged integration package buildable, bounded in time, attributable to the pinned CPA revision, and able to prove exact configuration replacement.
- Remove the proven avoidable work in integration checkout reuse, archive hashing, artifact upload, and Windows teardown.
- Require the ABI input-copy proof-and-measurement gate and remove the copy when that gate proves a call-scoped foreign-memory view is safe.
- Reject ambiguous release versions and colliding package paths before artifacts are written.
- Release and document the result as v0.2.0.

## 3. Non-goals

- No response-output censorship.
- No change to existing term matching, Unicode folding, replacement text, role defaults, block response, or JSON span-rewrite semantics except where this document explicitly changes ordering or field coverage.
- No term trimming, normalization, sorting, or deduplication.
- No new configurable role for legacy OpenAI `function` messages. They map to `tool`.
- No generic recursive string traversal in provider payloads.
- No mutation of tool calls, tool definitions, function arguments, reasoning/thinking content, citations, encoded documents, media, URLs, IDs, names, commands, status fields, or signatures.
- No attempt to express a YAML union in `pluginapi.ConfigField`; that interface supports one type only.
- No host-side validation or management UI change.
- No native C ABI bump and no custom RPC schema.
- No CLIProxyAPI SDK or host change beyond the required coordinated v7.2.152/schema-5 upgrade.
- No ABI input-copy removal before the safety and benchmark gates defined in Section 13.7 pass.
- No speculative Gemini `toolCall` or `toolResponse` handling for malformed mixed text/tool parts.

## 4. Observed architecture

### 4.1 Registration and lifecycle

`main.go` builds plugin registration metadata and editable configuration fields. Both initial registration and reconfiguration parse a fresh candidate. Initial registration publishes only a valid candidate. Invalid reconfiguration is logged, returns the existing successful registration envelope, and leaves the last-known-good snapshot installed.

`config.go` owns the active `atomic.Pointer[configSnapshot]`. A request loads that pointer once and passes the same snapshot through the entire interception. The current production path does not mutate a snapshot after publication.

### 4.2 Configuration compiler

The current strict YAML parser builds defaults, validates the root mapping, compiles derived rule data, and returns a candidate only after all work succeeds. The current snapshot exposes one global `Mode` and one ordered `Rules` slice, so it cannot represent mixed actions.

### 4.3 Request pipeline

The shared selector layer requires a JSON object, enforces maximum nesting depth 1024, rejects duplicate JSON member names, applies canonical role gates, sorts selected string spans by raw byte offset, and rejects invalid or overlapping spans. Provider selector modules identify only prose-bearing request fields. The transformer then applies compiled rules to those spans.

This design keeps that seam: provider modules return role-tagged spans; the shared transformer applies one loaded snapshot. Provider modules do not learn about YAML or rule compilation.

### 4.4 Native ABI and integration

`abi_cgo.go` exposes native ABI v1 initialization, call, free, and shutdown functions. A synchronous before-auth call currently copies the host request into Go memory with `C.GoBytes`. Returned buffers are plugin-owned `malloc` allocations released through `cliproxyPluginFree`.

The tagged integration harness launches an external CPA process per scenario, points it at an `httptest` upstream, and tests HTTP, WebSocket, reload, and native ABI behavior. Its ordinary package currently cannot compile because an ABI benchmark imports CPA `internal` packages from the plugin module.

### 4.5 Build and release

The integration runner maintains a pinned CPA checkout and generates plugin-specific tests inside it. The release packager creates already-deflated ZIP archives, per-platform checksum files, and aggregate `checksums.txt`. GitHub Actions builds three artifact groups and uses `go-cross/cgo-actions` for two cross-build jobs.

## 5. Dependency and compatibility target

### 5.1 Selected target

The first implementation change is one coordinated SDK and integration-host upgrade:

| Contract | Selected value |
|---|---|
| Go module | `github.com/router-for-me/CLIProxyAPI/v7` |
| Resolved module version | `v7.2.152` |
| Module checksum | `h1:FkvGzpOCvuDGswaOyoVfbY5Ua7OlP/wMXw3agiNMUQI=` |
| go.mod checksum | `h1:lTHwMAGajc1wKGQiRtDvYbwV0FWsM7sy+N0ZU5/gxJQ=` |
| Host commit | `c76dfd4e0edabab9000628b1560ab8ab379eadb8` |
| Native ABI | `pluginabi.ABIVersion == 1` |
| RPC schema advertised by the selected SDK | `pluginabi.SchemaVersion == 5` |
| WebSocket test dependency | `github.com/gorilla/websocket v1.5.3` |

Update `go.mod` and `go.sum`, `.github/scripts/integration-runner.go`, its tests, and the README as one change. No intermediate supported state may combine the schema-5 SDK with a different host pin. The integration harness must verify that `CPA_INTEGRATION_BIN` was built from the exact selected host commit before starting it.

Native ABI v1 is unchanged. The v7.2.152 `RequestInterceptor` request/response interface used by this plugin is compatible; no host production patch or plugin ABI bump is required.

Generate and commit the selected module sums and the missing `github.com/gorilla/websocket v1.5.3` sum with narrow module commands. Assert the exact v7.2.152 module and go.mod checksums above in dependency/provenance tests or the existing dependency lock fixture, then run the integration suite with `-mod=readonly`.

### 5.2 Upgrade ordering and schema compatibility

Perform and verify the coordinated dependency change before configuration or selector implementation. A schema-5 plugin registration must run only against the selected schema-5 host. Tests must reject a binary built from any other revision before process startup and must decode a registration envelope whose schema version is 5.

The SDK upgrade is intentionally limited to v7.2.152. It does not authorize changes to native ABI v1, `RequestInterceptor`, or CPA production source.

### 5.3 Build action pin

Replace both `go-cross/cgo-actions@v1` references with the exact revision:

```yaml
uses: go-cross/cgo-actions@d0b8f2f2d67923ce9a42d92a7ef0ed1ebd905f0a # v1
```

The branch currently resolves to that revision, so this changes provenance, not build behavior.

## 6. YAML configuration contract

### 6.1 Legacy sequence

The existing form remains valid:

```yaml
mode: strip
words:
  - alpha
  - beta
```

`mode` defaults to `block` when absent and must be exactly `block`, `strip`, or `obfs`. Every sequence term is assigned to that one action.

### 6.2 Canonical mapping

The canonical v0.2.0 form is:

```yaml
words:
  block:
    - credential
  strip:
    - internal-only
  obfs:
    - secret-code
obfs:
  char: "​"
```

`words` may contain any subset of `block`, `strip`, and `obfs`. Omitted buckets and empty arrays mean no rules for that action. An empty mapping is valid.

A mapping-form `words` ignores global `mode` behaviorally. Supplying `mode` still requires a valid value. A misspelled mode is never accepted merely because the mapping does not use it.

### 6.3 Grammar

In YAML-node terms:

```plaintext
words := absent | legacy-sequence | action-mapping
legacy-sequence := sequence<string-term>
action-mapping := mapping<action-key, sequence<string-term>>
action-key := "block" | "strip" | "obfs"
string-term := a YAML string scalar whose decoded value has length greater than zero
```

The following are valid:

- absent `words`;
- `words: []`;
- `words: {}`;
- omitted mapping keys;
- empty mapping buckets;
- whitespace-only terms;
- duplicate terms within one bucket;
- duplicate terms across buckets;
- any ordering of root keys or action mapping keys.

The following are invalid:

- `words: null`;
- scalar `words`;
- a non-string action key;
- an unknown action key;
- a duplicate action key;
- a null, scalar, or mapping bucket value;
- a non-string item in any sequence;
- an empty string item;
- an invalid supplied global `mode`.

Existing strict validation for all other root keys remains unchanged.

### 6.4 Obfuscation-specific validation

Resolve the complete root before applying these checks, so an `obfs.char` appearing after `words` is honored.

For terms that actually enter the obfuscation bucket:

- each term must contain at least two Unicode scalar values;
- a term must not contain the configured `obfs.char` marker.

For a legacy sequence, these checks apply only when global `mode` is `obfs`. For a mapping, they apply only to `words.obfs`. One-scalar and marker-containing terms remain valid in `block` and `strip`.

No term is trimmed before validation. An empty string is invalid; whitespace is not empty.

## 7. Compiled snapshot and data flow

### 7.1 Parser staging

Parsing has two phases behind the existing configuration parser interface:

1. Parse and validate the strict root shape into defaults, scalar settings, final `mode`, final `obfs.char`, role settings, and a temporary `words` representation.
2. After the entire root is known, resolve the temporary representation into ordered action slices, validate action-specific constraints, and compile all derived data.

Do not assign a legacy sequence to a bucket when the `words` key is encountered. YAML permits `mode` and `obfs.char` to appear later.

The temporary representation needs only:

- whether `words` was a sequence or mapping;
- the ordered legacy sequence, if present;
- the ordered block, strip, and obfuscation sequences, if present.

Do not use a Go map for behavioral rule buckets. A small parser-local struct or local variables are sufficient.

### 7.2 Snapshot representation

Keep one contiguous `Rules []rule` and add two cut points:

```go
Rules     []rule // block rules, then strip rules, then obfs rules
BlockEnd  int
StripEnd  int
```

The effective ranges are `Rules[:BlockEnd]`, `Rules[BlockEnd:StripEnd]`, and `Rules[StripEnd:]`. The parser flattens mapping buckets into that fixed order. A legacy sequence occupies exactly one range according to its global `mode`; global mode then remains parser-local and is not published in `configSnapshot`.

This representation reuses the existing rule type, direct matcher indexes, replacement data, and ordered rewrite functions without adding mode tags or index maps. Derived data is phase-specific:

- exact and folded block matchers use only `Rules[:BlockEnd]`;
- one folded total-miss rewrite preflight matcher uses only `Rules[BlockEnd:]` and never decides blocking;
- exact replacement data is compiled only for `Rules[StripEnd:]` using final `obfs.char`;
- adaptive matcher thresholds receive the relevant block or rewrite rule count, not `len(Rules)`;
- existing matcher nodes, folded KMP implementation, and their thresholds remain unchanged;
- configured order and duplicates remain intact inside every range.

No compiled slice, map, matcher, or replacement table may be reused from `loadedSnapshot` or appended into its backing storage. Every candidate owns all of its data.

### 7.3 Publication

The lifecycle contract remains:

```plaintext
YAML candidate
  -> parse full root
  -> resolve words union
  -> validate all buckets
  -> compile every derived structure
  -> one atomic pointer store
```

Initial registration with an invalid candidate returns the existing registration failure and publishes nothing. Reconfiguration with an invalid candidate logs the rejection, returns the existing successful registration envelope, and retains the exact prior pointer.

### 7.4 Request execution

Each before-auth request performs one snapshot load. The loaded snapshot is used for role selection, block detection, and all rewrites.

The action pipeline is fixed:

1. Select and validate provider text spans, preserving their raw-byte order.
2. Evaluate every block rule against the original selected text in configured rule order. If any block rule matches, return the existing block result without running rewrites.
3. Apply all strip rules in configured order.
4. Apply all obfuscation rules in configured order to the post-strip text.
5. Rebuild the request with the existing span-rewrite mechanism.

All block evaluation happens before any selected text is mutated. This prevents a strip or obfuscation rule from hiding a term that must block the request. `block -> strip -> obfs` also makes cross-bucket duplicates and overlaps deterministic and retains the existing stronger-action-first behavior. YAML mapping insertion order never affects behavior.

Within each bucket, existing rule-order and matching semantics remain unchanged. Duplicate rules deliberately execute as configured.

## 8. Registration metadata

### 8.1 Plugin icon

Set the existing metadata field directly:

```go
Logo: "https://raw.githubusercontent.com/DoingDog/cpa-plugin-censorship/main/logo.png",
```

The field is `pluginapi.Metadata.Logo`; the host exposes it as management JSON key `logo`. The selected v7.2.152 SDK supports it. `logo.png` is already the published 1024x1024 RGBA asset, so no image file change is required.

### 8.2 Editable configuration fields

`pluginapi.ConfigField` has one `Type` and no union or nested-schema facility. The visual editor therefore exposes only the canonical model:

- register exactly one `words` field with `Type: pluginapi.ConfigFieldTypeObject`;
- describe only the optional `{block, strip, obfs}` array buckets and empty-object behavior;
- remove `mode` from `ConfigFields` entirely;
- do not register an array-form field or mention the legacy array in panel-facing metadata.

This affects editor presentation only. The handwritten YAML parser continues to accept legacy arrays and top-level `mode`; those compatibility details belong only in the separate legacy YAML documentation and parser tests.

Keep the source default `pluginVersion = "0.0.0-dev"`. Existing release ldflags inject `0.2.0` for tagged artifacts. Metadata tests that need the release value temporarily set and restore the variable; production source does not hardcode the release version.

## 9. Selector design

### 9.1 Shared rules

Provider selectors remain explicit allowlists. A selected node must be a documented natural-language string and must be assigned to one existing canonical role: `system`, `developer`, `user`, `assistant`, or `tool`.

Apply these shared boundary fixes before provider traversal:

- run the existing iterative 1024-level nesting scan before recursive GJSON validation or parsing, so excessive depth is rejected before recursive stack use or a body copy;
- reject a body that is not valid UTF-8 before parsing JSON, preventing parser differentials at the plugin boundary;
- make `appendStringSpan` return immediately when the decoded string is empty, avoiding useless span growth and sorting.

Continue to:

- reject invalid root JSON, excessive depth, duplicate members, invalid spans, and overlaps;
- preserve raw JSON outside selected string tokens;
- skip traversal as early as possible when no relevant canonical role is enabled;
- avoid generic recursion into unknown objects or arrays.

A null-valued ProtoJSON discriminator is semantically absent. In the shared `scanTextPart` helper, a machine or signature field excludes text only when its value is non-null. `thought: true` continues to exclude the part; absent, null, or false `thought` does not.

### 9.2 OpenAI Chat Completions

Retain existing selection for canonical system, developer, user, assistant, and tool message text. Add only these forms:

| Request form | Canonical role | Selected text | Preserved fields |
|---|---|---|---|
| `role: "tool"`, string `content` | `tool` | `content` | `tool_call_id`, name, metadata |
| `role: "tool"`, array `content` | `tool` | `content[].text` where `type == "text"` | non-text parts and all other fields |
| `role: "assistant"`, top-level refusal | `assistant` | string `refusal` | tool calls and other fields |
| `role: "assistant"`, refusal part | `assistant` | `content[].refusal` where `type == "refusal"` | other content parts |
| legacy `role: "function"` | `tool` | non-null string `content` | `name` and every non-content field |

Delete the tool-only early return that prevents the existing text-part traversal. Do not add `function` to user configuration; canonicalize it to `tool` before the role gate.

### 9.3 OpenAI Responses

Allow `tool` as a canonical role for `openai-response`. Existing `instructions`, string `input`, and message text behavior remains unchanged.

For non-message input items, switch on explicit current result-item discriminators and select only the documented text payload:

| Exact `type` discriminator | Selected text under `tool` |
|---|---|
| `function_call_output` | string `output`, or `output[].text` only when the nested type is `input_text` |
| `custom_tool_call_output` | string `output`, or `output[].text` only when the nested type is `input_text` |
| `local_shell_call_output` | string `output` |
| `shell_call_output` | string `stdout` and `stderr` fields in `output[]` entries |
| `apply_patch_call_output` | string `output` |
| `mcp_call` | string `output` and string `error` |
| `program_result` | string `result` |

Do not recognize aliases or select a field merely because it is named `output`, `error`, `stdout`, `stderr`, or `result`.

Do not select call IDs, arguments, commands, environment values, URLs, files, screenshots, image/file input content, statuses, or any non-text nested output part. With `tool` disabled, these items produce no spans and the request remains byte-identical.

### 9.4 Claude Messages

Retain existing system, user, assistant, and typed text selection. Extend explicit traversal as follows:

| Enclosing form | Canonical role | Selected text |
|---|---|---|
| user-message `tool_result` with string `content` | `tool` | that string |
| user-message `tool_result` with array `content` | `tool` | existing text blocks plus the explicit nested forms below |
| top-level user `search_result` | `user` | `content[].text` strings |
| `search_result` nested in `tool_result` | `tool` | `content[].text` strings |
| top-level user `document` whose `source.type == "text"` | `user` | string `source.data` |
| such a `document` nested in `tool_result` | `tool` | string `source.data` |

Keep search source, title, citation metadata, document media type, URL/file source, base64 source, `tool_use_id`, `is_error`, tool inputs, thinking, redacted thinking, and all non-text blocks unchanged.

### 9.5 Gemini GenerateContent

Treat `Content.role` as omitted when it is missing, explicit JSON null, or the empty string. Route all three forms through the existing user/model alternation. A nonempty role other than `user` or `model` retains the existing behavior: advance alternation but skip its parts.

After processing system instruction, return before reading `contents` when neither `user` nor `assistant` is enabled. This removes traversal that cannot produce a span.

For text parts, treat null-valued machine/signature discriminators as absent for both camelCase and ProtoJSON snake_case spellings. In particular, these remain selectable text-only parts:

```json
{"text":"SECRET","functionCall":null}
{"functionCall":null,"text":"SECRET"}
{"text":"SECRET","function_call":null}
```

Any non-null function, media, code, executable-code, function-response, thought-signature, or other existing machine discriminator still excludes the part. `thought: true` still excludes it. Opaque thought signatures remain byte-identical.

### 9.6 Gemini Interactions

At top-level `input` dispatch only:

- a string remains `user` text;
- an object with exact discriminator `type: "text"` is scanned with the existing `scanTextPart` safety checks and mapped to `user`;
- in an array, each exact `type: "text"` item is handled the same way;
- every other object continues through `collectInteractionItem`.

This adds the documented direct `TextContent` and `Content[]` forms without recursively exposing tool, thought, or control steps. Existing `user_input`, `model_output`, system-instruction compatibility forms, and nested-step behavior remain unchanged. `model_output` remains gated by `assistant`.

## 10. Accepted finding scope

The following table is exhaustive for implementation findings accepted from the reports. Evidence gates must pass before their production optimization is applied; a failed gate records its exact blocker rather than claiming the optimization. No unlisted cleanup or refactor belongs in v0.2.0.

| ID | Area | Proven problem | Required v0.2.0 action |
|---|---|---|---|
| CFG-1 | Config | Sequence-only `words` cannot represent mixed actions. | Implement the strict union and flatten block, strip, and obfs into one rule slice with two cut points. |
| CFG-2 | Metadata | `words` is advertised only as an array and `mode` is panel-visible. | Advertise one canonical object field and remove `mode` from `ConfigFields`; keep both only in handwritten YAML compatibility. |
| MAT-1 | Transform | Global mode dispatch cannot execute mixed buckets. | Execute the three contiguous ranges in fixed block -> strip -> obfs order. |
| MAT-2 | Matchers | Unbounded block matchers would treat rewrite terms as blockers, while a block-only folded matcher cannot preflight rewrites. | Restrict block matchers to the prefix and add a separate folded rewrite total-miss matcher over the suffix. |
| MAT-3 | Matching performance | Mixed unrelated rules can distort adaptive thresholds. | Pass phase-specific block and rewrite counts; preserve matcher-node and folded KMP algorithms. |
| CORE-1 | JSON boundary | The depth limit currently runs after recursive GJSON validation. | Run the iterative nesting guard first. |
| CORE-2 | JSON boundary | Invalid UTF-8 bodies pass GJSON byte-syntax validation. | Reject invalid UTF-8 before JSON parsing. |
| CORE-3 | Selector performance | Decoded empty strings append and sort useless spans. | Skip empty decoded strings in `appendStringSpan`. |
| OA-1 | OpenAI Chat | Tool text arrays bypass `tool`. | Traverse documented tool text parts. |
| OA-2 | OpenAI Chat | Assistant refusal text bypasses `assistant`. | Select top-level and typed refusal strings. |
| OA-3 | OpenAI Chat | Legacy function-message content is discarded. | Map its string content to `tool`. |
| OA-4 | OpenAI Responses | Textual tool-result input items bypass `tool`. | Enable `tool` and add the explicit result allowlist in Section 9.3. |
| CL-1 | Claude | String `tool_result.content` bypasses `tool`. | Select the string before the array guard. |
| CL-2 | Claude | Search-result prose and plain-text document data bypass user/tool scopes. | Add only the explicit paths in Section 9.4. |
| GM-1 | Gemini | Empty or null role bypasses omitted-role handling. | Treat missing, null, and empty as omitted. |
| GM-2 | Gemini | `functionCall: null` creates a text bypass. | Treat null machine/signature fields as absent in the shared helper. |
| GM-3 | Gemini | System-only/tool-only scopes traverse every content turn. | Return after system selection when user and assistant are both disabled. |
| GI-1 | Interactions | Direct `TextContent` object/array input bypasses `user`. | Handle exact top-level `type: "text"` items. |
| INT-1 | Integration | Ordinary tagged tests cannot compile due to foreign `internal` imports. | Move the ABI benchmark source into the pinned CPA module at run time. |
| INT-2 | Integration | Gorilla WebSocket checksum is missing. | Commit the v1.5.3 `go.sum` entry and verify readonly setup. |
| INT-3 | Integration | Relative binary and plugin paths resolve under the temporary run directory. | Resolve both against the caller directory before creating the run directory. |
| INT-4 | Integration | A WebSocket read timeout is accepted as peer closure. | Bound the first read and reject timeout on the post-400 close read. |
| INT-5 | Integration | HTTP and raw TCP helpers can hang until package timeout. | Use a five-second HTTP client timeout and connection deadline. |
| INT-6 | Integration | Declared CPA SHA is not enforced. | Verify executable VCS revision with `debug/buildinfo.ReadFile`. |
| INT-7 | Integration | Readiness depends on a private log sentence. | Use authenticated `/v1/models` health only. |
| INT-8 | Integration | Reload test does not distinguish B from stale A or A union B. | Prove A first, then pair B-blocked with A-allowed. |
| INT-9 | Integration | HTTP `/v1/responses` has no coverage. | Add block and transform cases for string and structured input. |
| INT-10 | Integration | Windows teardown waits for an unsupported interrupt. | Invoke existing `terminateCPA` immediately on Windows. |
| INT-11 | Native ABI | `C.GoBytes` adds an input-sized allocation and copy to every before-auth call. | Repair the benchmark, prove synchronous read-only non-retention, then use a bounded call-scoped `unsafe.Slice` when the evidence gate passes. |
| REL-1 | Integration runner | Runner-owned generated tests invalidate checkout reuse. | Remove only that contained generated directory before checkout verification. |
| REL-2 | Packager | Aggregate mode hashes every ZIP twice. | Retain first-pass digest lines and write the aggregate from them. |
| REL-3 | README | It falsely says no license file exists. | Delete that sentence; retain conditional packaging wording. |
| REL-4 | Workflow | Artifact upload recompresses already-deflated ZIPs. | Set `compression-level: 0` on all three upload steps. |
| REL-5 | Workflow | Missing artifact paths only warn. | Set `if-no-files-found: error` on all three upload steps. |
| REL-6 | Workflow | Cross builds execute a moving branch. | Pin both action uses to `d0b8f2f2d67923ce9a42d92a7ef0ed1ebd905f0a`. |
| REL-7 | Versioning | Bare `v` normalizes to an empty artifact version. | Reject an empty normalized version in Make and direct packaging paths before output creation. |
| REL-8 | Packager safety | Colliding library, archive, and checksum paths can overwrite an input or artifact. | Reject every path collision before opening or writing files. |
| DOC-1 | Linux installation | Generic Linux wording hides the built artifact's glibc 2.34 minimum. | State glibc 2.34+ for published Linux artifacts. |

## 11. Integration harness design

### 11.1 ABI benchmark location

Move `integration/abi_benchmark_test.go` to an inert runner fixture such as `.github/scripts/testdata/abi_benchmark_test.go`. The integration runner copies that source into its existing generated `integration/censorshipplugin` directory inside the pinned CPA checkout, then runs the host-level benchmark there.

This keeps CPA `internal/config` and `internal/pluginhost` imports inside the CPA module, where Go permits them. The ordinary plugin command `go test -tags=integration ./integration` no longer compiles the benchmark or imports host internals. Do not implement a new cross-platform C ABI loader when the existing pinned host benchmark already provides the needed seam.

### 11.2 Path resolution and provenance

Before creating the temporary run directory:

1. Resolve `CPA_INTEGRATION_BIN` through `exec.LookPath`, then `filepath.Abs`.
2. Resolve `CENSORSHIP_PLUGIN_DIR` with `filepath.Abs`.
3. Read executable build information with `debug/buildinfo.ReadFile`.
4. Find `vcs.revision`; fail if it is absent or differs from `c76dfd4e0edabab9000628b1560ab8ab379eadb8`.
5. Include expected and actual revisions in a mismatch error.

Only the resolved absolute paths may enter generated YAML or `exec.Cmd`.

### 11.3 Readiness

Readiness is the first authenticated `200` response from `/v1/models`. Do not inspect informational log text. Individual tests prove plugin behavior through their own request canaries.

### 11.4 I/O bounds

Use one harness-owned `http.Client` with a five-second timeout. Immediately after every raw TCP or WebSocket connection is established, set a deadline no later than five seconds in the future. These operation bounds are shorter than the existing 20-second readiness/reload loops.

For the terminal WebSocket block scenario:

- set a deadline before the first `ReadMessage`;
- require the expected terminal 400 event;
- perform the next read under a fresh deadline;
- fail if the error implements `net.Error` with `Timeout() == true`;
- accept only a Gorilla peer-close error or EOF, without inventing a close-code requirement that CPA does not specify.

### 11.5 Reload linearization

The reload test performs this sequence:

1. Start with snapshot A containing only lowercase `alpha-only`.
2. Observe `alpha-only` blocked before writing B.
3. Write snapshot B containing only lowercase `beta-only`.
4. Poll until `beta-only` is blocked.
5. After that observation, send `alpha-only` and require it to reach upstream successfully.
6. For any repeated post-linearization checks, keep the `beta-only` blocked and `alpha-only` allowed assertions paired.

This rejects stale A, A union B, and an empty snapshot.

### 11.6 HTTP Responses coverage

Add HTTP `/v1/responses` cases independent of the WebSocket `response.create` path:

- a prohibited term in string `input` returns the existing block response and never reaches upstream;
- a transform-mode term in string `input` reaches upstream with the exact transformed input;
- a transform-mode term in structured `input_text` reaches upstream with only that text field changed.

### 11.7 Windows teardown

On Windows, call the existing `terminateCPA` path immediately. On non-Windows platforms, retain `os.Interrupt` and the existing grace period before forced termination.

## 12. Release tooling design

### 12.1 Cached checkout

Before `verifyCheckout`, call the existing containment-checked `removeContained` helper for only:

```plaintext
.integration/cpa/integration/censorshipplugin
```

Then run the existing strict checkout verification. Every other tracked modification or untracked path must still invalidate reuse. The runner regenerates its directory after verification.

### 12.2 One checksum pass

In aggregate packaging, keep the digest line produced while writing each per-platform checksum. Append those lines in deterministic existing artifact order and write `checksums.txt` from the retained lines. Each ZIP is read by `sha256File` exactly once. Reuse the current digest formatting and file writer; do not add a hashing interface.

### 12.3 Artifact upload contract

All three `actions/upload-artifact@v4` steps receive:

```yaml
compression-level: 0
if-no-files-found: error
```

The inner release ZIP remains Deflate-compressed. The upload action stores that already-compressed file without another zlib pass.

### 12.4 Version and path safety

Normalize a leading lowercase `v` once, then reject an empty result before build or packaging starts. The Make entry points and standalone packager must enforce the same rule, so a bare `v` cannot create `censorship__...` artifacts.

Before opening an archive or checksum for writing, canonicalize and compare the library, archive, and checksum paths. Reject every equality or alias detected by the platform path rules; never overwrite the input library or one output with another. Reuse the existing version validator and standard path utilities rather than adding a validation framework.

### 12.5 Published Linux compatibility

The current release build produces glibc-linked Linux libraries requiring glibc 2.34+. Installation documentation must state that minimum and must not imply compatibility with musl-only hosts such as Alpine.

## 13. TDD acceptance cases

Implementation follows red, green, then only local cleanup. Existing helpers and table-test style are reused.

### 13.0 Recorded baseline

Before implementation, `make integration` and `go vet ./...` passed. `make test` and `make race` were blocked only by `TestDocumentationListsConfigAndLimits`: `RELEASE_NOTES.md` did not contain the exact expected configuration block. The v0.2.0 documentation and its contract fixture must remove that failure before any green completion claim.

### 13.1 Configuration parser and compiler

Add failing tests before changing production code:

1. Legacy sequence with absent mode routes all terms to block.
2. Legacy sequence routes to each valid explicit mode.
3. Legacy `words` appearing before `mode` still uses the later mode.
4. Mapping with all three buckets preserves each array order and duplicate.
5. Every permutation of mapping key order compiles equivalent ordered buckets.
6. A valid global mode does not affect mapping behavior.
7. Invalid global mode rejects mapping form.
8. Absent words, empty sequence, empty mapping, omitted buckets, and empty buckets succeed.
9. Whitespace-only and cross-bucket duplicate terms succeed unchanged.
10. Null/scalar words, non-string/unknown/duplicate action keys, non-sequence bucket values, non-string items, and empty strings fail.
11. One-scalar or marker-containing block/strip terms succeed.
12. The same terms in effective obfuscation rules fail.
13. `obfs.char` placed after `words` controls obfuscation validation and compilation.
14. The flattened `Rules` slice is ordered block, strip, obfs with correct `BlockEnd` and `StripEnd`; block matcher indexes stay within the prefix, rewrite preflight sees only the suffix, and obfuscation replacement data covers only the final range.

### 13.2 Cross-mode execution

Use overlapping and duplicate terms to prove this exact behavior:

- a block term present in original selected text blocks even if a strip rule would remove it;
- without a block match, strip runs before obfuscation;
- mapping key order does not change output;
- order within each action array remains observable where overlapping rules make it relevant;
- duplicate terms are not removed from the compiled snapshot;
- exact and folded strip/obfs-only terms never block, while block terms retain rule-major and earliest-role behavior;
- strip-to-obfs and same-bucket cascades retain ordered per-rule semantics, including overlap, invalid UTF-8 bytes inside matcher inputs, Sigma, and Kelvin cases;
- strategy selection uses `BlockEnd` and the suffix length, so irrelevant buckets cannot trigger the wrong adaptive path;
- the independent fuzz oracle models the two cut points, every bucket subset, block-first safety, and cross-phase cascades;
- legacy sequence outputs remain byte-for-byte equal to pre-v0.2.0 expectations.

### 13.3 Lifecycle and concurrency

- Invalid mapping on initial registration installs no candidate.
- Valid mixed mapping installs one complete snapshot.
- Invalid mapping on reconfiguration retains the exact prior mixed snapshot.
- Alternate two distinct mixed snapshots through 1,000 reconfigurations while 32 readers execute requests. Every result must match complete snapshot A or complete snapshot B, never a hybrid.
- A request that begins with one pointer uses it throughout selection and all three actions.

### 13.4 Metadata

Decode the actual registration envelope and assert:

- after temporarily setting and restoring `pluginVersion`, release metadata reports `0.2.0`, while the source default remains `0.0.0-dev`;
- `metadata.logo` equals `https://raw.githubusercontent.com/DoingDog/cpa-plugin-censorship/main/logo.png`;
- exactly one `words` field exists and its type is `object`;
- its description names only the optional `{block, strip, obfs}` buckets and empty-object behavior;
- no `mode` field or array-form `words` field is registered.

### 13.5 Provider selectors

Before provider cases, add shared RED tests:

- a subprocess supplies JSON nested far beyond 1024 levels and must return `errInvalidRequest` quickly without recursive stack growth or a body-sized parse copy;
- invalid UTF-8 in both JSON keys and values returns `errInvalidRequest`;
- arbitrarily many enabled empty strings produce zero spans, while escaped non-empty strings remain selectable.

For every provider case below, first test with only the named canonical role enabled, then with that role disabled and require byte-for-byte input preservation.

**OpenAI Chat**

- Tool string and tool text-array forms select text; non-text parts and `tool_call_id` do not change.
- Assistant top-level refusal and refusal content part select only refusal text.
- Legacy function-message content maps to `tool`; its `name` does not change.

**OpenAI Responses**

- One table row for every result family in Section 9.3.
- Cover string output and structured `input_text` output where both are valid.
- Preserve nested image/file content, IDs, arguments, commands, URLs, screenshots, statuses, and other machine data.
- Retain existing system/user/assistant role-gate behavior.

**Claude**

- Plain-string `tool_result.content` changes under `tool`.
- Direct-user and nested-tool `search_result.content[].text` change under their enclosing role.
- Direct-user and nested-tool text-document `source.data` change under their enclosing role.
- Search source/title/citations, document media type, URL/file/base64 sources, `tool_use_id`, `is_error`, thinking, and non-text blocks remain byte-identical.

**Gemini GenerateContent**

- A first or only content with `role: ""` or `role: null` is selected as `user`.
- The same representations after an explicit user turn follow existing alternation.
- Both key orders of `functionCall: null` and snake_case `function_call: null` remain selectable.
- Non-null function call/response, code, media, thought, and signature fields remain excluded.
- Empty, system-only, and tool-only role sets produce the same spans as before; a large system-only request benchmark confirms `contents` is not traversed.

**Gemini Interactions**

- Direct object `TextContent` and array `Content[]` select only text under `user`.
- A mixed content array preserves image/media data.
- A mixed step array preserves tool/thought/control fields.
- Existing `model_output` continues to require `assistant`.

### 13.6 Integration and release tooling

- `go test -mod=readonly -tags=integration ./integration` reaches test execution without an internal-package or missing-checksum setup error.
- The v7.2.152 dependency advertises schema 5, and the integration runner builds and accepts only host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`.
- Absolute and equivalent caller-relative environment paths start the same binary and load the same plugin.
- A binary built from another revision and a binary without VCS revision are rejected before startup; the selected pinned build proceeds.
- Readiness succeeds when the health endpoint is ready even if the former watcher log sentence is absent.
- Fixtures that accept but never respond make HTTP, raw TCP, and first WebSocket reads fail within five seconds.
- A fixture that sends the terminal 400 event but keeps the socket open fails the close assertion with a timeout.
- The strengthened A-to-B reload test fails against a union-on-reconfigure test double and passes against replacement behavior.
- HTTP Responses tests fail under a selector defect limited to that route.
- Windows process teardown reaches `taskkill` without the initial two-second interrupt wait.
- A second integration-runner invocation reuses the checkout and performs no fetch; an unrelated untracked file still causes rejection.
- Per-platform checksum lines exactly equal their lines in `checksums.txt`, and archive-read instrumentation observes one full hash pass per ZIP.
- Workflow contract tests assert the exact cross-action SHA, `compression-level: 0`, and `if-no-files-found: error` on all three uploads.
- Bare `v`, empty normalized versions, unsafe filename components, and direct Make/package entry points all fail before creating an artifact.
- Equal or aliasing library, archive, and checksum paths fail before any input or output is opened for writing.
- README installation rows and release notes state that published Linux binaries require glibc 2.34+.

### 13.7 ABI no-copy evidence gate

The input-sized allocation and copy performed by `C.GoBytes` is proven. v0.2.0 first relocates and repairs the host benchmark, then runs this gate before changing production code:

1. Trace every `handleMethod` branch and prove that request bytes are read synchronously, never mutated, and no subslice, aliased string, closure, or pointer survives the call.
2. Add a lifetime test that poisons or releases the foreign buffer immediately after return and detects post-return dependency.
3. Require identical matching and nonmatching outputs at 1 KiB, 1 MiB, and 20 MiB.
4. Require allocation measurements and repeated benchmark medians showing the input-sized allocation disappears and latency improves proportionally enough to justify the unsafe view.

When all four checks pass, replace `C.GoBytes` with a call-scoped `unsafe.Slice`. Check `size_t` against the architecture's maximum `int` before conversion, treat zero length safely, and keep all output in plugin-owned copied or allocated memory before returning across the ABI. Document the foreign buffer lifetime next to the unsafe view.

If any check fails, retain `C.GoBytes`, record the exact failing evidence in the TDD record and release notes, and do not claim the optimization. Completion still requires a definitive gate result rather than an unmeasured deferral.

## 14. Documentation and release changes

### 14.1 README

Update only the affected material:

- make the panel-facing and primary YAML example the canonical Object without `mode`;
- place the array plus global `mode` in a separate handwritten-YAML compatibility subsection;
- state that mapping form ignores a supplied global `mode` behaviorally but still syntax-validates it;
- document fixed `block -> strip -> obfs` ordering and preservation of duplicates/order;
- document scoped obfuscation validation;
- describe the expanded provider text coverage and the machine fields that remain excluded;
- document CLIProxyAPI v7.2.152 and exact host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`;
- state that the published Linux artifacts require glibc 2.34+;
- delete the false sentence `This repository does not add a license file`;
- retain the accurate statement that packaging includes `LICENSE` when present.

### 14.2 Release notes

Add a v0.2.0 section to `RELEASE_NOTES.md` covering:

- canonical Object panel configuration and handwritten-YAML-only legacy compatibility;
- deterministic action precedence;
- OpenAI, Claude, Gemini, and Interactions selector corrections;
- shared JSON-boundary and empty-span fixes;
- plugin icon metadata;
- CLIProxyAPI v7.2.152/schema 5, selected host commit, and unchanged native ABI v1;
- integration reliability, version/path validation, Linux glibc requirement, and release reproducibility changes;
- the measured ABI gate outcome, claiming no-copy only if all safety and benchmark checks passed.

### 14.3 Release artifacts

The release remains the existing platform library plus optional `LICENSE` in each ZIP, with per-platform checksum and aggregate `checksums.txt`. `logo.png` is referenced remotely and is not added to platform archives unless the existing packaging contract already includes it.

## 15. Risks and pure-plugin boundaries

### 15.1 Pure-plugin boundaries

- All production behavior remains inside this repository's configuration compiler, selector modules, request transformer, registration metadata, and native plugin entry points.
- CPA is an external host. The implementation may generate tests inside a pinned checkout but must not patch host production source.
- Provider APIs are not called during selection. Selectors operate on the request bytes CPA supplies.
- Native ABI v1 ownership remains: host owns input for the synchronous call; plugin owns returned `malloc` buffers until host calls `cliproxyPluginFree`.
- RPC schema is supplied by the pinned SDK and is not redefined locally.

### 15.2 Risks and mitigations

| Risk | Mitigation |
|---|---|
| Metadata clients cannot render both YAML shapes. | Expose only the canonical object; keep the legacy sequence solely in handwritten YAML parsing and documentation. |
| Mapping order could leak into behavior. | Flatten buckets in fixed order with two cut points and execute fixed action precedence. |
| A later YAML key could change validation. | Resolve terms only after parsing the full root. |
| Candidate data could alias the active snapshot. | Allocate and compile a complete owned candidate before one atomic store. |
| New selector coverage could corrupt protocol fields. | Use explicit discriminator/path allowlists and byte-preservation tests. |
| Provider schemas may drift. | Pin behavioral fixtures to documented current forms and fail closed for unknown typed items. |
| Null ProtoJSON placeholders could bypass selection. | Treat null discriminators as absent in the shared text-part helper. |
| Integration failures could be setup artifacts. | Verify path, provenance, readiness, module sums, and bounded I/O before behavioral assertions. |
| Unsafe ABI views could outlive host memory. | Apply the view only after synchronous non-retention and lifetime tests pass; otherwise record the blocker and retain the copy. |
| SDK and host schema could diverge. | Upgrade both to the coordinated v7.2.152/schema-5 target and enforce the exact host revision. |

## 16. Rejected alternatives

- **Keep one global mode and concatenate mapping terms.** Rejected because it cannot preserve per-action behavior.
- **Publish a map or three independent slices of action rules.** Rejected because mapping order is not behavior and one flattened existing slice plus two cut points reuses matcher indexes with less state.
- **Use YAML mapping insertion order as precedence.** Rejected because management JSON conversion can reorder keys and source order is not stable.
- **Apply strip or obfuscation before block.** Rejected because a weaker rewrite could erase a blocking term.
- **Deduplicate or trim terms.** Rejected because legacy order, duplicates, and whitespace are deliberate contract behavior.
- **Add two `words` metadata fields or invent a union type.** Rejected because upstream `ConfigField` exposes one type and no union schema.
- **Publish global mode in mixed snapshots.** Rejected because it permits accidental fallback to the wrong action.
- **Recursively scan every provider string.** Rejected because it would mutate IDs, arguments, reasoning, signatures, media, citations, and other machine data.
- **Add a configurable OpenAI `function` role.** Rejected because existing `tool` is the canonical scope for tool results.
- **Change Responses `instructions` from system to developer.** Rejected because upstream permits both meanings and existing local tests define system.
- **Remove permissive Responses user-message output paths.** Rejected because tested compatibility exists and no measured benefit justifies narrowing it.
- **Remove Interactions `model_output`.** Rejected because assistant-scope selection is an explicit existing contract.
- **Add Gemini `toolCall`/`toolResponse` exclusions solely because the fields exist.** Rejected because valid union instances do not also expose top-level text.
- **Build a new local C ABI loader.** Rejected because relocating the existing benchmark into the pinned host module reuses the real host path with less code.
- **Replace `C.GoBytes` without the evidence gate.** Rejected because lifetime, non-retention, output, allocation, and benchmark proof must precede the unsafe view.
- **Bump native ABI because RPC schema advanced upstream.** Rejected because the contracts are independent and native ABI remains version 1.
- **Upgrade only the SDK or only the integration host.** Rejected because schema 5 requires the coordinated v7.2.152 and exact-host update.
- **Optimize singleton span sorting or duplicate maps without measurements.** Rejected because no permitted benchmark showed material cost.

## 17. Excluded findings and missing evidence

These items are not implementation work for v0.2.0. The listed evidence is required before reconsideration.

| Excluded item | Missing evidence required |
|---|---|
| Reclassify OpenAI Responses `instructions` as developer. | A product decision changing the local role contract or an upstream rule that makes system mapping invalid. |
| Remove permissive `output_text`/`refusal` traversal under a user-role Responses message. | A compatibility decision plus evidence that supported callers do not rely on it. |
| Add Gemini `toolCall`/`toolResponse` mixed-part logic. | A valid supported wire payload containing both machine data and top-level text, or an upstream union-contract change. |
| Remove Gemini Interactions `model_output`. | A product decision that assistant history must always be immutable. |
| Add plugin-side shutdown locking. | Evidence that the supported host can call the plugin concurrently after shutdown begins; current host guards and drains calls. |
| Add selector micro-optimizations beyond the empty-span and Gemini role guards. | A benchmark showing material hot-path cost without changing global validation behavior. |
| Wholesale-upgrade official GitHub actions. | A reproduced plugin-relevant failure or a separate dependency-maintenance decision. |


## 18. File-level change map

| Path | Change |
|---|---|
| `config.go` | Parse the `words` union in two phases; flatten buckets into one rule slice with `BlockEnd` and `StripEnd`; compile phase-specific derived data. |
| `config_test.go` | Add union grammar, cut-point ordering, validation, derived-data, lifecycle snapshot, and concurrency cases. |
| `transform.go` | Replace global mode dispatch with block, strip, and obfs ranges; use phase-specific matcher inputs and thresholds. |
| `transform_test.go` | Add block-first, bucket-boundary, exact/folded, cascade, overlap, and legacy-equivalence cases. |
| `matcher.go` | No algorithm redesign; preserve existing byte matcher and folded KMP behavior. |
| `matcher_test.go` | Retain matcher oracles while proving local indexes remain correct at block/rewrite boundaries. |
| `fuzz_test.go` | Model `BlockEnd` and `StripEnd`, every bucket subset, block-first safety, and cross-phase cascades. |
| `main.go` | Keep the dev version default; set logo and canonical panel fields, excluding `mode`. |
| `main_test.go` | Update registration-envelope expectations and add mixed-mode interception/lifecycle cases with temporary release-version injection. |
| `selectors.go` | Run depth before GJSON, reject invalid UTF-8, skip empty decoded spans, enable `tool` for OpenAI Responses, and make null text-part handling ProtoJSON-correct. |
| `selectors_openai.go` | Add explicit Chat tool/refusal/function and Responses tool-result paths. |
| `selectors_openai_test.go` | Replace stale exclusion assertions and add current request-form cases. |
| `selectors_role_gate_test.go` | Add deep-input subprocess, invalid UTF-8, empty-span, and direct canonical-role enable/disable coverage. |
| `selectors_claude.go` | Add string tool results, search results, and text-document paths. |
| `selectors_claude_test.go` | Replace the string-result bypass expectation and add byte-preservation cases. |
| `selectors_gemini.go` | Treat null/empty roles as omitted and add the no-user/no-assistant traversal guard. |
| `selectors_gemini_test.go` | Reverse null-discriminator bypass expectations and add role/guard cases and benchmark. |
| `selectors_interactions.go` | Handle exact direct top-level `TextContent` items. |
| `selectors_interactions_test.go` | Add direct object/array and mixed-content preservation cases. |
| `abi_cgo.go` | After the evidence gate passes, replace `C.GoBytes` with a size-bounded call-scoped `unsafe.Slice`; retain plugin-owned output memory. |
| `abi_cgo_test.go` | Add size conversion, zero length, lifetime/non-retention, output-equivalence, and allocation checks for the ABI view. |
| `integration/abi_benchmark_test.go` | Move out of the plugin module so ordinary tagged tests no longer import CPA internals. |
| `.github/scripts/testdata/abi_benchmark_test.go` | Hold the relocated host-module benchmark source for runner copying. |
| `integration/harness_test.go` | Add absolute path resolution, build provenance, health-only readiness, HTTP/TCP deadlines, and immediate Windows termination. |
| `integration/websocket_test.go` | Bound reads and distinguish peer closure from timeout. |
| `integration/http_test.go` | Prove exact A-to-B reload and add HTTP Responses cases. |
| `.github/scripts/integration-runner.go` | Clean only runner-owned generated tests before verification and copy/run the host benchmark in the pinned checkout. |
| `.github/scripts/integration-runner_test.go` | Prove cache reuse, unrelated-dirt rejection, and benchmark placement. |
| `.github/scripts/package-release.go` | Reuse first-pass digest lines, reject empty normalized versions, and reject input/output path collisions before writes. |
| `.github/scripts/package-release_test.go` | Prove exact digest equality, one archive hash pass, version rejection, and collision safety; assert action pins and upload inputs. |
| `.github/workflows/build.yml` | Pin cross action SHA and set both upload-artifact controls on all three steps. |
| `Makefile` | Reject bare `v` after normalization before invoking build or packaging outputs. |
| `README.md` | Document canonical panel Object, separate handwritten legacy YAML, ordering, provider coverage, v7.2.152 host pin, glibc 2.34+, and accurate license packaging. |
| `RELEASE_NOTES.md` | Replace stale content with the v0.2.0 contract and only verified performance claims. |
| `logo.png` | No content change; use its published raw URL in metadata. |
| `go.mod` | Upgrade CLIProxyAPI to v7.2.152. |
| `go.sum` | Record v7.2.152 and Gorilla WebSocket checksums. |

## 19. Scan partition coverage

| Partition | Proven actionable coverage included | Explicitly retained or excluded |
|---|---|---|
| Lifecycle/config parsing and matcher/transform | Strict words union, parser ordering, scoped obfuscation validation, one owned flattened slice with two cut points, phase-specific matchers, fixed execution, fuzz oracle, atomic publication, metadata editor contract, and mixed-snapshot tests. | Retain duplicates, whitespace, one-load requests, existing matcher nodes/KMP algorithms, and failed-reconfigure last-known-good behavior; no lifecycle redesign. |
| Selector core plus OpenAI Chat and Responses | Early depth and UTF-8 validation, empty-span removal, tool arrays, assistant refusal, legacy function mapping, Responses tool-result allowlist, and direct role-gate tests. | Retain system mapping for Responses instructions, permissive tested user-role output paths, explicit allowlists, and existing span reconstruction. |
| Claude Messages | String tool results, search-result prose, plain-text document data, enclosing user/tool roles. | Retain tool protocol, reasoning, citation metadata, URL/file/base64 data, and signatures unchanged. |
| Gemini GenerateContent | Null/empty omitted roles, null discriminator semantics, system-only traversal guard. | Retain alternation for unknown roles, `thought:false` text selection, non-null machine/signature exclusions; no speculative toolCall/toolResponse branch. |
| Gemini Interactions | Direct `TextContent` object and array selection with exact top-level dispatch. | Retain `model_output`, compatibility forms, and explicit tool/thought exclusions. |
| C ABI and CPA integration harness | Buildable benchmark placement, checksum, paths, provenance, readiness, deadlines, reload proof, HTTP Responses, Windows teardown, and the required ABI no-copy evidence gate. | Native ABI v1 and plugin-owned output allocation remain; no shutdown lock. |
| Build, release, dependency, docs, icon | Coordinated v7.2.152/schema-5 host update, checkout reuse, one-pass hashing, version/path validation, upload settings, action SHA pin, glibc documentation, release notes, and icon metadata. | Retain inner ZIP Deflate, current packaging contents, existing official action majors, dev version default, and unchanged logo bytes. |

## 20. Completion criteria

v0.2.0 is ready to release only when:

1. All acceptance cases in Section 13 pass.
2. Unit and race suites pass without changing unrelated expectations.
3. `go vet` passes for the plugin and build scripts.
4. `go test -mod=readonly -tags=integration ./integration` passes setup and behavior against a CPA binary whose build revision equals the selected commit.
5. The runner-hosted ABI benchmark and all four no-copy evidence checks execute. `unsafe.Slice` is present only if every check passes; otherwise the exact blocker is recorded and no performance claim is made.
6. Release-packager tests prove one hash pass, byte-identical checksums, empty-version rejection, and collision-safe writes.
7. Workflow contract tests prove pinned cross actions and strict, uncompressed artifact uploads.
8. README, registration metadata, and release notes agree on v0.2.0, panel-only Object configuration, separate handwritten legacy YAML, fixed action ordering, icon URL, CLIProxyAPI v7.2.152/schema 5, native ABI v1, glibc 2.34+, and the exact CPA host commit.
9. The final diff contains no provider-recursive selector, new runtime dependency, ABI bump, host production patch, term normalization, hardcoded release version, or unrelated refactor.
