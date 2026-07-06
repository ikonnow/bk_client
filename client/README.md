# Blendkit-Client

This is a Client for Blendkit (previously daemon).
It's a local server that listens for requests from Blendkit add-ons and processes them.
Written in Go.

## How is it run
The Client is built for Windows, MacOS and Linux for both x86_64 and arm64.
Blendkit-Client binaries are shipped in the blenderkit.zip file, in /client directory where normally in the repo Client's source code is placed.
On add-on start, the Client binary is copied into global_dir/client/bin/vX.Y.Z directory, and started.

### Client start
Client can be started from the add-on automatically, but it can be also started manually.
When Client is started from the add-on, the add-on automatically fills some flags:
- `--version` informing about the version of the add-on which starts Client
- `--software` in which software starting add-on runs - for now it is just Blender
- `--pid` the process number of the software whose add-on starts the Client

For manual start these flags are empty (if the user does not specify those from CLI).
In the future Client could behave differently when started manually - e.g. does not shutdown automatically after a while.

## API documentation

The Client exposes a local HTTP API. It is documented from a single source of truth — the
route registry in [`internal/apispec`](internal/apispec/apispec.go) — and rendered into two files:

- [`docs/openapi.json`](docs/openapi.json) — OpenAPI 3.1 spec. Import it into Postman/Insomnia,
  render it with Swagger UI / Redoc, or generate client SDKs in any language.
- [`docs/API.md`](docs/API.md) — human-readable reference for quick viewing on Git.

### Regenerating the docs

Run from the `client/` directory:

```sh
go generate ./...
```

This runs `cmd/apidocgen`, which reads the registry and the `VERSION` file and rewrites both
files in `docs/`.

### How it stays in sync

The drift test `TestAPISpecMatchesRoutes` (in `apidoc_test.go`) parses the `mux.HandleFunc`
registrations in `main.go` and fails if any route is missing from the registry, has a mismatched
handler, or a mismatched versioned alias — and vice versa. So `go test ./...` guarantees the
published docs always match the real routes.

When you add, remove or rename an endpoint:

1. Update the `mux.HandleFunc(...)` registration in `main.go` as usual.
2. Add/update the matching `Route` entry in `internal/apispec/apispec.go`.
3. Run `go generate ./...` and commit the updated `docs/`.

CI ([`.github/workflows/api-docs.yml`](../.github/workflows/api-docs.yml)) regenerates the docs and
fails the build if the committed files are stale, then runs `go vet` and the test suite.

> Note: request bodies are documented by their Go struct name (e.g. `GetReportData`). Full JSON
> schemas are not yet generated because those structs live in `package main`; a future,
> non-frozen change can move the shared request structs into a package so schemas can be emitted
> automatically.
