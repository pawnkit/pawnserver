# Compatibility

`pawnserver` follows semantic versioning. Before `v1.0.0`, minor releases may
change the CLI or Go API. Bundle schema version 1 remains governed by PawnKit
RFC 0006; incompatible format changes require a new schema version.

Unknown manifest fields and unsupported schema versions are rejected. Server
binaries and extensions are selected with specification platform names such as
`linux-x86_64`. An artifact marked `any` is accepted on every platform.

CI tests Linux, macOS, and Windows. Release archives target amd64 and arm64.

Updates keep one rollback copy. Persistent paths may live inside the
installation, but they cannot overlap bundled binaries, entry points,
extensions, or configuration files.
