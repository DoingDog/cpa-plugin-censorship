# OpenAI Responses `instructions` role coverage

## Intent and scope

Fix one confirmed plugin bug without changing CLIProxyAPI or native ABI v1. A configured `scope.roles: [developer]` must inspect the documented OpenAI Responses top-level `instructions` string; today the selector inspects it only when `system` is enabled. Retain existing behavior for configurations that include `system`. Do not expand the selected paths, modify machine fields, change configuration syntax, or address unrelated test infrastructure.

## Evidence

With `words: {block: [SECRET]}`, `scope.roles: [developer]`, source format `openai-response`, and request body `{"instructions":"SECRET","input":"safe"}`, a temporary test of the current plugin returned a zero-value `RequestInterceptResponse` instead of terminating. `collectOpenAIResponses` gates `instructions` solely on `roles.has("system")` (`selectors_openai.go:370-373`); the request processing path then receives no matching span. [OpenAI Responses API reference](https://developers.openai.com/api/reference/resources/responses/methods/create) describes `instructions` as a system **or developer** message inserted into the context. [OpenAI prompt engineering guide](https://developers.openai.com/api/docs/guides/prompt-engineering) demonstrates the field as roughly equivalent to a `developer` input message. The field does not carry a separate role discriminator in the request.

## Behavior

For source format `openai-response`, inspect a nonempty string `instructions` once. If `system` is enabled, assign its existing canonical `system` role; otherwise, if `developer` is enabled, assign `developer`. When neither role is enabled, leave it untouched. This precedence preserves existing `system`-only and default configurations and prevents duplicate/overlapping spans when both are enabled. Treat non-string `instructions` as unselected, as before. Keep `block -> strip -> obfs` processing and every other selector unchanged.

## Acceptance and exclusions

- A `developer`-only block terminates a matching Responses `instructions` request locally with `censorship_blocked`, `term: SECRET`, and `role: developer`, before contacting upstream.
- A `developer`-only strip changes only the selected `instructions` string token; unrelated `input` and metadata remain unchanged.
- `system`-only and `[system, developer]` configurations still select the field once as `system`; `user`-only leaves it unselected. Existing canonical role tests remain valid.
- The native plugin loads and registers with the pinned CLIProxyAPI; focused unit, full unit/race/vet, platform build/package, and HTTP/WebSocket integration pass. CI `Build` workflow succeeds for the patch tag and produces verified release artifacts and checksums.
- Do not change CLIProxyAPI, broaden path selection, or modify the separate second-turn WebSocket test deadline issue; that is test waiting behavior, not this plugin runtime defect.

## Release

Publish the next patch tag after `v0.3.2`, update release notes with the verified fix, merge changes to local `main`, push the branch and tag to trigger `.github/workflows/build.yml`, and verify CI jobs, the published GitHub release, and its assets. Stop and report any environmental or CI blocker rather than claiming release success.
