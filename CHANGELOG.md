# Changelog

## 0.5.0 - 2026-07-31

- Apply accepted sampctl runtime settings to isolated open.mp sessions.

## 0.4.2 - 2026-07-31

- Load staged gamemodes from open.mp's gamemode search directory.

## 0.4.1 - 2026-07-31

- Verify cached runtime executables before reuse.

## 0.4.0 - 2026-07-31

- Prepare isolated open.mp sessions without modifying the runtime cache.
- Run verified runtimes with session-local configuration and output.

## 0.3.1 - 2026-07-30

- Accept standard trailing slashes on runtime archive directories.
- Reject backslash paths consistently on every host.

## 0.3.0 - 2026-07-30

- Define the shared cache layout for installed server runtimes.

## 0.2.1 - 2026-07-30

- Install verified ZIP and tar.gz runtimes with recoverable replacement.

## 0.2.0 - 2026-07-30

- Verify and select server downloads from RFC 0020 runtime indexes.

## 0.1.2 - 2026-07-30

- Verify released server archives through the shared tested release set.
- Install release-set dependencies inside the shared action.

## 0.1.1 - 2026-07-25

- Added the experimental support record with CI validation.

## 0.1.0 - 2026-07-20

- Added bundle verification and deterministic archives.
- Added plan-first install and update commands.
- Added rollback, doctor, run, and container export commands.
- Added staged replacement recovery.
- Preserved declared persistent files across updates.
- Added service and migration manifest fields from RFC 0006.
- Added archive inspection and open.mp configuration consistency checks.
- Added bounded extraction, persistent-copy limits, and rooted file writes.
