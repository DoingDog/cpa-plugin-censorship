# Gemini tool descriptions and Claude MCP text validity

## Purpose and evidence

The plugin must filter documented natural-language request text without changing machine data or creating provider-invalid requests. This is a plugin-only patch against CLIProxyAPI/v7 v7.2.152, schema 5 and native ABI v1.

- Gemini `generateContent` accepts `tools[].functionDeclarations[]`, whose `description` and schema descriptions inform the model. `collectGemini` currently reads only system and conversation parts; `selectorHasEnabledRole("gemini", ...)` also rejects a `developer`-only scope. A configured `block` term in a function description therefore reaches the model without inspection. Source: [Google function calling](https://ai.google.dev/gemini-api/docs/function-calling), [GenerateContent API](https://ai.google.dev/api/generate-content).
- Claude beta `mcp_tool_result.content[]` has typed `text` blocks. The existing collector selects their text without the nonempty constraint already applied to ordinary `tool_result` typed text. A full `strip` can produce an empty typed text block and an invalid request. Source: [Claude beta Messages create reference](https://platform.claude.com/docs/en/api/beta/messages/create); this consequence must be reproduced by a focused failing plugin test before changing code.

## Required behavior

1. Gemini definitions use canonical `developer`. With that role enabled, inspect only `tools[*].functionDeclarations[*].description` and `description` leaves within the explicitly named `parameters`, `parametersJsonSchema`, `response`, and `responseJsonSchema` schema objects. Reuse the existing schema-description selector; no recursive generic string scan. Support `developer`-only scopes without requiring system, user, or assistant roles. Existing system/conversation selection remains unchanged.
2. For Gemini, `block` on selected description text terminates the request with the existing local block response. `strip`/`obfs` rewrite only the selected string tokens; unrelated metadata and machine fields such as names, types, enum values, call arguments and signatures remain byte-for-byte unchanged. Disabled `developer` leaves definitions untouched.
3. For Claude `messages[*].content[*]` with `type: "mcp_tool_result"`, only nested blocks with exact `type: "text"` and a string `text` are affected. An enabled `tool` role and a final `strip` result of the empty string must terminate locally with the existing `censorship_invalid_request` response for invalid text fields. Partial strip, nonempty obfuscation and `block` retain their established behavior. Scalar MCP result content and unrelated inner blocks keep their existing selection rules.

## Implementation boundaries

Keep the existing selector/transform architecture and the fixed `block` -> `strip` -> `obfs` rule order. Add the Gemini branch to its existing collector and role gate; use the existing `appendJSONSchemaDescriptions` helper on the four explicit schema roots. Use `appendNonEmptyStringSpan` for Claude MCP typed text. Do not modify CLIProxyAPI core, ABI ownership, CGO input copies, configuration shape, other providers, or generated `.integration/` and `dist/` artifacts. Gemini single-object `contents`/`parts` has conflicting documentation about accepted wire shape and is excluded without upstream acceptance evidence.

## Verification and release

First add focused failing tests for Gemini `developer`-only blocking and description-only rewrites with machine-field exclusions; add a focused failing test for Claude MCP typed text full-strip rejection alongside partial-strip and disabled-role controls. Then implement the minimum selector changes and rerun the focused tests. Update the Gemini selected-text paragraph in README and document this patch in release notes. Run `make test`, `make vet`, `make race`, `make build`, `make integration`, `make package` under PowerShell, plus independent packaging-script tests and a release-package checksum check. Verify native plugin registration through integration. Review the diff before merging to local `main`; release the next patch version only after all checks pass, push the commit and tag, and check the GitHub build/release result. If a prerequisite cannot be verified, report it rather than claiming release success.
