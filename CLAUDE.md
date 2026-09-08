# cpa-plugin-censorship

## Scope

This repository develops only the `censorship` CPA plugin. Do not modify CLIProxyAPI (CPA) core code here.

## Compatibility and boundary

- Target Go 1.26.0 and the pinned `github.com/router-for-me/CLIProxyAPI/v7 v7.2.152` dependency.
- Build a native `c-shared` plugin with CGO and preserve native ABI v1. Keep host interaction within the pinned CLIProxyAPI boundary.
- Preserve ABI ownership: do not free or retain memory owned by the other side of the C boundary.
- Keep `C.GoBytes` for before-auth input. Replace it only if a benchmark demonstrates that the candidate removes the input-sized before-auth allocation. After-auth does not read input; do not describe the retained copy as a no-copy optimization.

## Configuration and matching

- `words` is an Object with optional `block`, `strip`, and `obfs` term arrays. Do not add a global Object-mode selector.
- Global `mode` with a list-valued `words` is supported only for legacy handwritten YAML. The standard configuration panel sends Object `words` only and never sends global `mode`.
- Preserve the processing order: `block` -> `strip` -> `obfs`.
- Enumerate only documented natural-language text paths for each provider. Do not add generic recursive string walking or fallback recursion.

## Development commands

```bash
make build
make test
make race
make vet
make integration
make package VERSION=vX.Y.Z
```

`make build` and platform packaging require a working native or cross CGO compiler for the selected target.

## Generated artifacts and releases

- `.integration/` and `dist/` are generated integration and packaging artifacts; do not hand-edit them as source.
- Release archives use `censorship_<version>_<goos>_<goarch>.zip`, each with a matching `.zip.sha256`. Publish `checksums.txt` containing the seven per-platform checksum lines.

## Change discipline

- Make surgical changes only. Do not refactor, reformat, stage, or modify unrelated work.
- Use TDD for behavior changes: add the smallest failing test first, implement the minimum fix, then run the relevant checks.
