# Security policy

## Supported versions

Until the first tagged release, security fixes are applied to the default development branch. A version support table will be added when stable releases exist.

## Reporting a vulnerability

Do not open a public issue for vulnerabilities that could select the wrong drive bay, execute an unintended controller command, expose local inventory, or bypass the managed LED ownership boundary. Use GitHub's private security advisory reporting for this repository. If that channel is unavailable, contact the repository owner privately before disclosure.

Include the affected version or commit, platform, backend, reproduction steps using anonymized data, and the expected safety boundary. Never include production credentials, real serial-number inventories, or customer logs.

## Scope

Security-sensitive areas include command construction, ZED environment parsing, GUID-to-bay caching, state ownership, file permissions, package maintainer scripts, symlink/path handling, and ambiguous device matching.
