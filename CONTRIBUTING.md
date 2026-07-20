# Contributing

PawnKit is maintained by volunteers, so reviews may take a little time.

Bug reports, operator notes, and focused bundle fixes are welcome. Include a
small manifest or archive fixture when possible.

Run the local release checks before opening a pull request:

```sh
task check
```

Bundle input is untrusted. Changes to extraction, paths, checksums, installation,
or rollback need failure-path tests.

Public format changes start in `pawnkit-spec`. Do not change the bundle schema only in this repository.
