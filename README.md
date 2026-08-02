# pawnserver

[![Maturity: experimental](https://img.shields.io/badge/maturity-experimental-orange)](.pawnkit/support.json)

`pawnserver` packages and operates SA-MP and open.mp server installations. A
bundle records the server binary, AMX entry points, native extensions,
configuration, checksums, and files that must survive an update.

## Install

Download a release archive or install from source:

```sh
go install github.com/pawnkit/pawnserver/cmd/pawnserver@latest
pawnserver --version
```

Release archives are available for Linux, macOS, and Windows on amd64 and
arm64.

PawnKit's tested server set pins the supported archives and checksums:
`server-preview-2026-07-29` in `pawnkit-spec v0.1.100`. That set currently
keeps the v0.1.1 server archive for compatibility coverage; the release
archive above is the current CLI package.
RFC 0020 runtime indexes separately pin clean upstream server downloads.
Isolated open.mp sessions can stage verified plugins, components, and
filterscripts without changing the shared runtime cache.

## Build and inspect a bundle

Prepare a directory containing `pawn-bundle.json` and every declared file:

```sh
pawnserver verify ./release
pawnserver pack ./release game.pawnbundle
pawnserver inspect game.pawnbundle
```

Packing is deterministic and refuses to write the archive inside its source
directory. Verification accepts either a directory or a `.pawnbundle` archive.

## Install and update

Install and update are plan-only unless `--apply` is present:

```sh
pawnserver install game.pawnbundle ./server
pawnserver install --apply game.pawnbundle ./server
pawnserver update --apply next.pawnbundle ./server
pawnserver rollback ./server
```

An update keeps the previous installation at `./server.rollback`. Paths listed
under `persistence.paths` are copied into the staged update before it replaces
the current installation.

## Operate an installation

```sh
pawnserver doctor ./server
pawnserver configure ./server
pawnserver run ./server
pawnserver export-container ./server Dockerfile
```

`configure` prints the declared configuration path and schema. `run` executes
the verified server binary directly without a shell. Container export defaults
to `FROM scratch`; pass `--base debian:bookworm-slim` when the server needs a
runtime image.

See the [operator guide](docs/operator.md), [bundle format](docs/format.md), and
[compatibility policy](docs/compatibility.md).

## Contributing

Operator feedback and small archive fixtures are welcome. See
[CONTRIBUTING.md](CONTRIBUTING.md).
