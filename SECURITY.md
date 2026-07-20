# Security policy

Report vulnerabilities through GitHub's private
[security advisory form](https://github.com/pawnkit/pawnserver/security/advisories/new).
Please do not open a public issue before a fix is available.

Bundles are untrusted. Extraction rejects links, devices, duplicate files,
path traversal, more than 100,000 entries, and more than 2 GiB of file data.
Files are opened through a filesystem root so existing links cannot redirect
writes outside the destination.

Installation verifies checksums before replacement and again after persistent
files are copied. Installations cannot target a filesystem root. Server
binaries run directly without a shell, but they and any bundled native
extensions remain trusted executable code once the operator starts them.
