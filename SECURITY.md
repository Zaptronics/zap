# Security model

Zap treats dependency manifests and repositories as supply-chain inputs, not as
trusted code merely because they are referenced by a project.

Key controls:

- strict validation of dependency names and CMake target identifiers;
- validation of remote `zap-package.yml` aliases and sparse paths;
- immutable Git commit locks stored in `zap.yml`;
- remote ref-to-lock verification using Git metadata;
- local origin/HEAD/clean-worktree verification;
- mandatory SHA-256 hashes for URL dependencies;
- CMake bracket arguments for untrusted URI/path scalar data;
- configure-time `zap verify` by default;
- explicit `ZAP_OFFLINE` and `ZAP_VERIFY_DEPENDENCIES=OFF` controls;
- dirty managed checkouts are never silently revision-switched; and
- Zap edits user CMake only between recognized Zap-managed marker pairs.

`ZAPEE_SOURCE` and other explicit local override variables are developer-controlled
source overrides. When active, Zap still resolves and verifies the declared remote
ref against the immutable commit lock, but the local override contents themselves
are intentionally not required to match that commit.
for that dependency and Zap reports this state.

## Upload commands

The `upload:` section of the project-owned `zap.yml` may intentionally execute
configured native programs. It is therefore trusted project configuration, like
CMake or a build script. Remote dependency `zap-package.yml` manifests are not
allowed to define upload methods, executables, arguments, or environment
variables. Zap invokes configured tools directly with an argument vector and
does not pass upload commands through a shell.
