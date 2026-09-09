# Current and planned security model

The authoritative description is [SECURITY.md](../SECURITY.md).

Current Zap combines exact dependency resolution with targeted input/local-operation
hardening. It validates source declarations, exact Git commits, selected manifest digests,
CMake-safe dependency identity, managed checkout presence/origin/HEAD and Git-reported
worktree state. It also carries specific guards for destructive clean/copy/write operations
and some Git subprocess configuration hazards.

Those checks **do not establish an independent cryptographic identity of every materialised
source byte**. Git-hidden edits and ignored files remain a demonstrated gap.

The planned stronger model depends on independent source identities established from
isolated Git object reads, direct materialised-tree verification, safe URL extraction,
verified temporary snapshots and atomic installation into an untrusted per-user cache.
That architecture is not present in the current source.

[Claim validation](CLAIM_VALIDATION.md) separates green present-tense acceptance tests from
intentionally failing roadmap probes. [Cache and offline status](CACHE_AND_OFFLINE.md)
describes the narrower offline behaviour available now.
