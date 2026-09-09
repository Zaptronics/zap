# Security policy and current limitations

Zap provides dependency locking, source-declaration policy and a set of targeted
hardening checks. **It does not yet provide independent verification of every source
byte on disk, an untrusted package-cache boundary, or cache-backed offline restoration.**
Those stronger properties are roadmap work, not claims about the current release.

The executable evidence for this distinction is in
[`docs/CLAIM_VALIDATION.md`](docs/CLAIM_VALIDATION.md).

## Reporting a vulnerability

Do not publish exploit details, credentials or sensitive information in a public issue.
Use this repository's private GitHub Security Advisory reporting control when available.
Otherwise open a minimal issue requesting a private contact channel, without exploit
details. Include the Zap version, OS, dependency type, command and expected/observed
behaviour in the private report.

## Implemented now and covered by tests

- `zap.yml` schema 5 records project intent. `zap.lock` schema 1 records exact Git
  commits, versions/refs, dependency relationships, selected components and a SHA-256
  digest of `zap-package.yml` when that manifest exists. It does **not** contain a
  complete-source or materialised-source identity.
- Managed Git source declarations accept HTTPS, SSH URLs and SCP-style
  `user@host:path`. HTTP, `git://`, `ext::`, local/file Git and similar sources are
  rejected by default. `ZAP_ALLOW_LOCAL_GIT=1` is a deliberate development/test escape
  hatch; ordinary local development should use `type: path`.
- HTTPS source URLs may not embed userinfo credentials. Error formatting redacts
  URL userinfo when it appears in captured command arguments. This is targeted URI
  protection, not a general secret-redaction system.
- URL package declarations require HTTPS and `SHA256=<64 hex digits>`. Zap emits
  CMake `URL_HASH`; **CMake**, not Zap, downloads the archive, verifies that archive hash
  and extracts it. Zap does not independently hash the downloaded or extracted source.
- The supported YAML subset rejects duplicate mapping keys instead of silently using a
  last value. Dependency identities that collapse under CMake `FetchContent`'s
  case-insensitive naming are rejected before generation/materialisation.
- Verification checks lock/config compatibility, managed checkout presence, origin,
  HEAD, Git-reported worktree changes and the package manifest read from the locked Git
  object. Online verification also checks stable release tags for movement.
- Git status verification disables `core.fsmonitor` so checkout-controlled fsmonitor
  hooks are not executed by that check. Zap also strips process-injected
  `GIT_CONFIG_COUNT`, `GIT_CONFIG_PARAMETERS` and their key/value entries from Git
  subprocesses.
- Offline verification keeps the local Git/path checks and skips remote lookups. It
  rejects URL dependencies because Zap cannot independently verify their local extracted
  content. `zap make --offline` requires the managed sources to already exist.
- Generated CMake invokes Zap verification by default. Missing managed Git source or a
  missing dependency `CMakeLists.txt` stops normal configure and asks the user to run
  `zap sync` rather than silently substituting another source.
- Remote package manifests cannot define project-level upload policy. Names, component
  references and sparse source paths are validated before generation.
- `zap make --clean` refuses to delete the project root, filesystem roots, and lexical
  ancestors that contain the project root. Lockfile replacement uses a unique temporary
  file in the destination directory, and volume-copy avoids truncating a file by copying
  it onto itself.
- Release workflow source passes the Git tag to PowerShell through an environment
  variable rather than splicing it into script text, and accepts only
  `vMAJOR.MINOR.PATCH` release tags. CI/release workflows are configured to run ordinary
  regression tests, current-claim tests and the OWASP hardening probes.

## Current limitations

### Git checkout verification is not independent content verification

Git status can be influenced by Git metadata. Acceptance tests demonstrate that a tracked
file marked `assume-unchanged`, or an added ignored file, can escape the current checks
while `zap.lock` remains unchanged. Therefore current verification does **not** prove that
attacker-modified materialised files equal the intended dependency source.

Disabling fsmonitor removes one concrete hook-execution path, but it does not solve the
content-identity problem.

### Git execution is only partially isolated

Zap removes process-injected Git config pairs used by `GIT_CONFIG_COUNT`/
`GIT_CONFIG_PARAMETERS`, but repository and global Git configuration, other Git-related
environment variables, the Git executable, credential helpers, transport configuration,
filters and checkout behaviour remain part of the trusted local environment. A complete
hardened Git object-reading sandbox is not implemented.

The declared URI policy therefore should not be read as a complete network-egress or
redirect sandbox.

### URL packages rely on CMake for content handling

Zap validates the HTTPS URL and SHA-256 declaration, then delegates download, archive
hash checking and extraction to CMake. Zap does not independently verify extracted file
paths/types/symlinks or hash the final materialised tree.

### No reusable verified machine cache exists

There is no reusable Zap dependency cache, secure snapshot extractor, cache ownership
check, verified temporary materialisation or cache corruption recovery. Git dependencies
are ordinary project-local checkouts. No cache-injection resistance claim is offered.

`ZAP_CACHE_DIR`, `zap cache ...` and `zap sync --offline` are roadmap interfaces and are
not supported commands in the current source.

### Offline mode is not network isolation

`zap verify --offline` and `zap make --offline` skip Zap's remote checks and use existing
Git/path materialisations. They cannot restore a missing dependency. Project CMake, SDKs,
compilers or other invoked tools may still access the network. `ZAP_OFFLINE=ON` is not an
OS-level network sandbox.

### Local filesystem hardening is targeted, not complete

The current clean/copy/lockfile fixes cover reproduced destructive primitives. They do
not establish a complete policy for every symlink, Windows junction/reparse point,
concurrent writer, disk-full condition or interrupted filesystem mutation.

### Release provenance remains a separate trust problem

The workflows now use a supported Go toolchain and run the current security gates, but
some third-party Actions are still referenced by version tags, the nFPM container is
referenced by a tag rather than an image digest, and WingetCreate is downloaded from a
`latest` URL. Local source tests do not prove GitHub permissions, signing policy, artifact
provenance, package-manager publication or the contents of a published binary.

## Trust boundaries and deliberate exceptions

Trust the Zap executable, `zap.yml`, `zap.lock`, local OS account, the remaining Git
configuration described above, toolchain and project build code. Review project YAML,
CMake and scripts as executable policy. Upload command methods intentionally run
project-selected tools; argument-vector execution reduces shell interpretation but does
not make a malicious executable safe.

Local paths and source overrides are intentionally mutable. `ZAP_ALLOW_LOCAL_GIT=1`
relaxes the source policy. `ZAP_VERIFY_DEPENDENCIES=OFF` bypasses configure-time Zap
verification and can enable CMake Git fallback when offline mode is off. These are
developer exceptions, not verified supply-chain workflows.

Content identity, even once stronger hashing is implemented, will not establish benignness.
Dependency C/C++, CMake, SDKs and programmer utilities execute with the user's privileges.
Zap does not sandbox them or promise bit-identical compiled firmware.

## Planned, not implemented

- A new lock schema containing independent complete-source and selected/materialised-source
  SHA-256 identities established from isolated Git object reads.
- Direct filesystem verification independent of mutable Git index/status metadata.
- Hardened Git identity creation that does not run checkout hooks/filters and rejects or
  explicitly handles replacement objects, submodules and unusual object types.
- Direct URL download verification plus safe extraction with path, type, symlink and
  platform-specific reparse checks.
- An untrusted per-user cache with verified temporary materialisation, ownership checks,
  atomic installation and corruption detection.
- Cache-backed offline restoration that fails closed when cached content is damaged.
- Broader subprocess/resource limits, cancellation and transactional sync behaviour.

These are roadmap requirements, not current guarantees or release commitments. The
`TestRoadmapClaim*` acceptance tests intentionally remain failing until the corresponding
architecture exists.

## Validation

For the release gate, run:

```powershell
.\scripts\test-claims.ps1
```

or on Linux/macOS:

```sh
go test ./... -count=1 -timeout=5m
go test -tags claims ./internal/zap -run '^TestCurrentClaim' -count=1 -timeout=5m
go test -tags 'claims,owasp' ./internal/zap -run '^TestOWASP' -count=1 -timeout=5m
```

That gate runs ordinary regression tests, `TestCurrentClaim*`, and `TestOWASP*` hardening
probes. It does **not** hide roadmap failures inside an expected-success test.

Run the stronger roadmap probes separately:

```powershell
go test -tags claims ./internal/zap -run '^TestRoadmapClaim' -count=1 -timeout=5m
```

```sh
go test -tags claims ./internal/zap -run '^TestRoadmapClaim' -count=1 -timeout=5m
```

A failing roadmap probe means the stronger guarantee is still absent; do not change the
test merely to make the suite green.

## Release artifacts

Check the checksums and signing information actually provided with each release. Local
source tests do not establish that a published binary, checksum file, signature or
package-manager channel exists or matches this source. Release provenance requires
separate validation.
