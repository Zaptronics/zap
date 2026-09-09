# README and SECURITY claim validation

This document separates **claims Zap makes today** from **security properties Zap wants
in the future**. It is an acceptance audit, not a security certification.

The core rule is simple:

- `TestCurrentClaim*` describes behaviour that README/SECURITY may state in the present
  tense and must pass for a release.
- `TestRoadmapClaim*` describes stronger architecture that is explicitly aspirational.
  Those probes remain genuine failures until the feature exists.
- `TestOWASP*` contains adversarial regression tests for concrete hardening defects found
  during review.

## How to run the release gate

Windows PowerShell:

```powershell
.\scripts\test-claims.ps1
```

Linux/macOS:

```sh
go test ./... -count=1 -timeout=5m
go test -tags claims ./internal/zap -run '^TestCurrentClaim' -count=1 -timeout=5m
go test -tags 'claims,owasp' ./internal/zap -run '^TestOWASP' -count=1 -timeout=5m
```

The gate runs:

```text
go test ./...
go test -tags claims ./internal/zap -run '^TestCurrentClaim'
go test -tags 'claims owasp' ./internal/zap -run '^TestOWASP'
```

CMake and Git are required by several integration fixtures. No public repository is
needed by the claim fixtures; they use disposable local repositories. The tests do not
program hardware.

## Roadmap probes

Run separately:

```powershell
go test -tags claims ./internal/zap -run '^TestRoadmapClaim' -count=1 -timeout=5m
```

or:

```sh
go test -tags claims ./internal/zap -run '^TestRoadmapClaim' -count=1 -timeout=5m
```

These execute `TestRoadmapClaim*`. A non-zero exit is expected while the stronger
security architecture is absent. That failure is evidence of an unmet roadmap property,
not a regression to paper over.

## Current claim matrix

| Current published behaviour | Acceptance test | Meaning |
| --- | --- | --- |
| URL packages reject non-HTTPS declarations | `TestCurrentClaimURLRequiresHTTPS` | Declaration policy, not a redirect/egress sandbox |
| URL packages require `SHA256=<64 hex>` | `TestCurrentClaimURLRequiresHash` | Validates the declaration; CMake performs archive hashing |
| Local/file Git is rejected without the development escape hatch | `TestCurrentClaimLocalGitRejectedByDefault` | Input policy only |
| Missing managed Git source is rejected online | `TestCurrentClaimMissingGitSourceRejectedOnline` | Verifier fails before normal consumption |
| Offline URL verification does not claim success without content | `TestCurrentClaimURLVerifyRequiresMaterialisedContent` | Current behaviour is explicit rejection |
| A new compatible tag does not silently replace a compatible lock | `TestCurrentClaimSyncPreservesCompatibleLock` | Lock bytes and materialised version remain unchanged |
| Remote manifests cannot inject project upload policy | `TestCurrentClaimRemoteManifestRejectsExecutablePolicy` | Remote package schema is constrained |
| Generated CMake rejects missing managed Git source | `TestCurrentClaimGeneratedCMakeRejectsMissingGit` | Both verification-on and explicit offline-bypass paths fail closed |

## Roadmap matrix

| Aspirational property | Roadmap probe | Current status |
| --- | --- | --- |
| Lock schema includes independent source identities | `TestRoadmapClaimLockSchema2` | Not implemented; current generated lock schema is 1 |
| Materialised source is independently verified against trusted identity | `TestRoadmapClaimIndependentSourceTampering` | Not implemented; `assume-unchanged` and ignored additions can evade Git status |
| Offline sync restores a locked dependency from a verified warm cache | `TestRoadmapClaimOfflineSyncFromWarmCache` | Not implemented; `zap sync --offline` and reusable cache do not exist |

## OWASP hardening probes

The adversarial suite covers concrete defects rather than future cache architecture. It
includes tests for:

- cleaning a project ancestor;
- duplicate mapping keys;
- dependency identities that CMake would case-fold/normalise together;
- truncated external-source audits;
- process-injected Git `insteadOf` configuration;
- checkout-controlled fsmonitor execution during verification;
- embedded URL credentials and diagnostic redaction;
- same-file firmware copy truncation;
- semver integer/range overflow;
- PowerShell release-tag expression injection; and
- predictable lockfile temporary-file clobbering.

## Latest local evidence in this source archive

On Linux on 2026-09-08, after the hardening changes in this archive (local toolchain: Go 1.23.2, Git 2.47.3, CMake 3.31.6; `go.mod` remains compatible with Go 1.22):

- ordinary regression suite: **40 passed, 0 failed, 0 skipped**;
- current claims: **8 passed, 0 failed, 0 skipped**;
- OWASP hardening probes: **12 passed, 0 failed, 1 skipped**;
- roadmap claims: **0 passed, 3 failed, 0 skipped**;
- `go vet ./...`: passed.

The single OWASP skip is the Windows-only dynamic PowerShell reproduction. The release
workflow source has been changed to pass the tag through an environment variable and to
validate `vMAJOR.MINOR.PATCH`, but this Linux run is not evidence that the Windows runner
executed that probe. CI is configured to run the OWASP suite on Windows, Linux and macOS.

Raw current evidence is under `test-results/`:

```text
regression.jsonl
current-claims.jsonl
owasp.jsonl
roadmap-claims.jsonl
```

Skipped tests are not evidence of a verified platform-specific property.

## What these tests do not prove

The current suites do not establish:

- independent source-byte identity for Git checkouts;
- safe archive extraction or final extracted-tree identity;
- cache corruption/injection resistance (there is no reusable cache yet);
- bit-for-bit reproducible firmware binaries;
- OS-level network isolation in offline mode;
- safety of malicious dependency CMake/source;
- live GitHub permissions, signing policy or artifact provenance;
- availability of package-manager channels or release assets; or
- exhaustive handling of symlinks, Windows reparse points, concurrent writes, disk-full
  failures, subprocess hangs or resource exhaustion.

When a current claim fails, either fix the implementation or narrow the documentation.
When a roadmap claim fails, keep the feature described as planned until the stronger
architecture and its acceptance tests are both real.
