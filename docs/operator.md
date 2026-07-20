# Operator guide

Start by inspecting and verifying the archive:

```sh
pawnserver inspect game.pawnbundle
pawnserver verify game.pawnbundle
```

An install without `--apply` prints the proposed destination, bundle version,
and whether an installation will be replaced. Review that output before
applying it.

```sh
pawnserver install game.pawnbundle ./server
pawnserver install --apply game.pawnbundle ./server
```

Updates are staged beside the destination. Declared persistent files are copied
into the stage, then the current installation moves to `<destination>.rollback`.
If replacement fails, pawnserver restores the current installation.

Use `rollback` to swap the saved installation back into place. Only one
rollback copy is kept.

`doctor` verifies the installed manifest, selected server binary, checksums,
entry points, and configuration agreement. `configure` prints the configuration
file operators should edit. Keep secrets out of distributable bundles.

`run` starts the selected binary with the installation directory as its working
directory. It inherits stdin, stdout, stderr, and the current environment.

Container export writes a Dockerfile from the same verified manifest:

```sh
pawnserver export-container --base debian:bookworm-slim ./server Dockerfile
```

The default `scratch` base is suitable only when the bundle carries everything
the server needs at runtime.
