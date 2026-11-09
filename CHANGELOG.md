# Changelog

Assets2Godot fork of `BlenderKit/bk_client`. Upstream releases use plain
`vX.Y.Z` tags; this fork continues the same `v1.13.x` series and records
its deltas here.

## 1.13.12 - 2026-11-09

First Assets2Godot fork release — unsigned binaries for Windows, macOS
and Linux (x86_64 + arm64).

### Added

- Assets2Godot provider/index endpoints on top of the upstream client
  (`providers/` scaffolding, SQLite index, cgo-free build).
- User-directed scanning via settings-store roots (`fab_vault`/`local_dir`
  providers adopt detected caches only after explicit opt-in).
- Unit tests for providers, the scan walker and the index; fixed an
  empty-root scan bug.
- `SIGNING_ENABLED` repo variable gates the code-signing jobs so forks
  without secrets build + release unsigned; `dev.py release --no-verify`.

### Changed

- Dropped the Windows 7/8/8.1 build target — the last compatible
  toolchain (Go 1.20) can't build current dependencies (`go-humanize`
  v1.1.0+ needs Go 1.21's `max()`). The plugin ships only the six modern
  binaries anyway.
- Release gate reads `github.event.inputs` so REST-API dispatches can
  publish (push triggers don't fire on this fork).
