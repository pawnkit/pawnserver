# Bundle format

A `.pawnbundle` is a deterministic gzip-compressed tar archive. Its root
contains `pawn-bundle.json` and every file referenced by that manifest.

Version 1 follows
[PawnKit RFC 0006](https://github.com/pawnkit/pawnkit-spec/blob/main/rfcs/0006-server-bundle.md).
The manifest records:

- bundle name, version, and runtime profile;
- server binaries by platform;
- gamemode and filterscript AMX files;
- plugins, components, and their SHA-256 checksums;
- runtime configuration;
- external services and migration records;
- persistent paths and an optional health check.

The manifest checksum is calculated from compact JSON with its `checksum`
field empty. File checksums use `sha256:<lowercase hex>`.

Paths use forward slashes and stay relative to the bundle root. Links, device
files, control characters, and traversal segments are rejected. Platform names
follow the specification, such as `linux-x86_64`, `windows-x86_64`, and
`darwin-arm64`.

For open.mp bundles, the referenced `config.json` must use the PawnKit open.mp
schema. Its main and side script lists must match the manifest entry points.
