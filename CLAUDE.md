# CPA Censorship Plugin

## Scope

- Develop only this CPA censorship plugin. Do not modify CLIProxyAPI core.
- Keep the Go/CGO boundary compatible with the pinned `github.com/router-for-me/CLIProxyAPI/v7` dependency and native ABI v1. Treat ABI ownership and pointer lifetimes as compatibility contracts.

## Commands

Run the smallest relevant command before finishing:

```bash
make build
make test
make race
make vet
make integration
make package
```

`make build` and `make package` require a working CGO compiler for the selected target.

## Configuration and request handling

- `words` is a strict Object with optional ordered `block`, `strip`, and `obfs` arrays. Preserve terms exactly as supplied.
- A legacy `words` sequence with global `mode` is accepted only in handwritten YAML. The panel renders and emits only Object `words`; it must not emit a global `mode`.
- Process matching in this fixed order: `block` -> `strip` -> `obfs`.
- Select only documented, explicit natural-language text paths for each provider. Do not add a generic recursive string walker or inspect machine fields.

## ABI and generated artifacts

- Keep `C.GoBytes` in `cliproxyPluginCall` for request input. Remove it only after a benchmark proves the candidate removes the input-sized pre-auth allocation.
- Do not hand-edit generated `.integration/` or `dist/` artifacts. Release packages need their matching lowercase SHA-256 files and aggregate `checksums.txt` entries.

## Change discipline

- Make surgical changes only. Do not refactor or reformat unrelated code.
- Use TDD for behavior changes: add or update a focused failing test first, then make it pass.
