# Zap OWASP security assessment

Review conducted 2026-09-08; report completed 2026-09-09 (Pacific/Auckland). Scope: the local source after the earlier claim fixes.

## Assessment

Zap is useful trusted-project automation, but it is not yet a strong security
boundary between untrusted dependency state and a workstation or CI runner.
Corrected documentation is necessary; it does not mitigate code defects.
Fix destructive cleanup, dependency identity collisions and release-script
injection before another release, then make verification independent of mutable
checkout metadata.

No unauthenticated remote takeover was demonstrated. Zap has no server or account
system. Several high-impact findings require control of local Git metadata,
project files or release tags. Those prerequisites matter to the severity.
This review added tests and recommendations, not fixes for the new findings.

## Method and evidence

This is an OWASP-informed source review and adversarial local test exercise,
not an OWASP certification or complete penetration test. It adapts
[OWASP Top 10:2025](https://owasp.org/Top10/2025/0x00_2025-Introduction/) to a native
package/build CLI, with the [OWASP CI/CD risks](https://owasp.org/www-project-top-10-ci-cd-security-risks/)
for release automation. Web-only controls such as CSRF are not invented requirements.

Reviewed: config/package/lock parsing; version and graph resolution; Git execution;
checkout verification/materialisation; CMake and Zephyr generation; cleanup;
upload arguments, tool discovery and volume copying; audit/terminal output;
CI, signing and release workflows.

Evidence retained locally:

- `internal/zap/owasp_test.go`: 12 new top-level adversarial acceptance tests.
  All 12 fail their security expectations. They are targeted gap probes, not a
  statistical measure of the entire application.
- `test-results/owasp.jsonl`: final Go test events, including subtests.
- `test-results/owasp-vuln.json`: govulncheck v1.7.0, symbol-level source scan,
  Go 1.27.1, database timestamp 2026-09-02T19:12:04Z. Exit 0; no finding events.
  Advisory records in that stream are not themselves findings against Zap.
- `test-results/owasp-source-manifest.json`: SHA-256 inventory of reviewed Go
  sources/tests, workflows, go.mod and principal docs. No .git metadata was
  available, so this report does not assert a commit SHA.
- `go vet ./...`: passed. The preceding regression run on this production source
  had 39 passes and one Unix-only skip after loading the Visual Studio compiler
  environment. Existing claim tests had 8 passes and 3 planned-feature failures.

Govulncheck did not scan Git, CMake, SDKs, programmer tools, signing services,
downloaded archives, or binaries built with the release workflow's old compiler.
A clean scan is not a general security clearance.
[Go vulnerability tooling](https://go.dev/doc/security/vuln/).

All probes used disposable local files/repos and synthetic credentials. The
cleanup target is explicitly checked to be inside its fixture. Hook/tag payloads
only create or print markers. No remote targets were probed, real credentials
read, devices programmed, tags pushed or releases published. Live GitHub/cloud
permissions, approvals, published signatures and Linux/macOS execution were not
verified. There was no exhaustive fuzz campaign or resource-exhaustion attack.

## Threat model

Assets: project source, lock integrity, selected dependencies, workstation/CI
files, firmware artifacts and release credentials.

| Controller | Intended authority | Boundary to preserve |
| --- | --- | --- |
| Project owner | Select dependencies and build/upload policy | Accidental cleanup must not destroy unrelated source |
| Remote package maintainer | Supply package metadata/source | Metadata must not silently replace another package identity or hide incomplete audits |
| Actor controlling a checkout and its .git metadata | Currently trusted local state in SECURITY | A future verifier must not let that state redefine checks or execute during verification |
| Local Git/tool configuration | Developer customization | Strict CI needs bounded explicit policy, not ambient inheritance |
| Release tag creator | Name/select an approved release ref | Tag text must remain data, not shell code |
| CI tool distributor | Supply a reviewed tool version | Mutable downloads must not silently acquire release authority |

Building malicious dependency CMake is already code execution by project choice.
That is a trust decision, not a novel Zap exploit. Executing code during verify,
or silently consuming the wrong dependency identity, is a distinct issue.

## Priorities

Severity assumes the stated prerequisites. P0: before the next release; P1: next
security milestone; P2: planned hardening. No Critical label is assigned without
a demonstrated attack path and appropriate attacker authority.

| ID | Severity / priority | Evidence | Issue |
| --- | --- | --- | --- |
| ZAP-01 | High / P0 | Reproduced | Cleanup deletes project through ancestor directory |
| ZAP-02 | High / P0 | Reproduced, verifier enabled | Case-variant identities collapse in CMake |
| ZAP-03 | High, conditional / P1 | Reproduced and prior tests | Mutable Git state bypasses verification and executes hooks |
| ZAP-04 | Medium / P1 | Rewrite reproduced; egress inspected | URI spelling does not constrain effective transport/destination |
| ZAP-05 | Medium / P1 | Inspection and prior evidence | URL verification does not establish source content identity |
| ZAP-06 | Medium / P1 | Reproduced | Duplicate metadata and overflowing versions accepted |
| ZAP-07 | Medium / P0 | Reproduced | Source audit silently succeeds after truncated input |
| ZAP-08 | High impact, restricted actor / P0 | Local reproduction | Release tag becomes PowerShell code |
| ZAP-09 | High-impact supply-chain risk / P0-P1 | Workflow inspection | Unsupported compiler, mutable tools and broad authority |
| ZAP-10 | Medium / P1 | Reproduced | Source credentials persist and appear in errors |
| ZAP-11 | Medium / P1 | Reproduced | Temporary writes and same-file copying clobber data |
| ZAP-12 | Medium / P1-P2 | Inspection | Incomplete resource limits and failure handling |

## ZAP-01: cleanup can delete an ancestor containing the project

**Location:** `internal/zap/make.go:76`, lexical comparisons and RemoveAll at 98.
OWASP A01/A06; CWE-22/CWE-73.

`safeRemoveBuildDir` rejects the project and filesystem roots, but permits a
parent. A build directory of `..`, or equivalent override, can make `--clean`
remove the project and sibling files. `TestOWASPCleanProtectsProjectAncestor`
demonstrated deletion of a disposable project's source marker. The primitive
was tested directly; the CLI performs its normal prerequisites before calling it.

A typo should not turn a build option into workspace deletion. Malicious project
configuration remains within the user's filesystem rights; this is not privilege
escalation. Windows case aliases and symlink/junction ancestry need further tests.

**Change:** refuse the project, all ancestors, filesystem roots and protected
project directories. Compare resolved filesystem identity, not only text. Require
a Zap-owned build marker tied to the project before recursive deletion, including
external build directories. Reject unsafe link/reparse components and show the
resolved cleanup target.

**Acceptance:** owned build cleanup works; project/parent/sibling, case-alias,
junction and unowned-directory fixtures preserve all protected sentinel files
on Windows, Linux and macOS.

## ZAP-02: package identity disagrees with CMake identity

**Location:** name checks in `internal/zap/security.go`; `lock.go:209`;
`cmake.go:17` and generated declarations. OWASP A08/A06; CWE-706.

Zap accepts `Demo` and `demo` as distinct dependencies. Generated FetchContent
collapses them. The test used two distinct source directories, built the actual
Zap verifier, configured with verification enabled, and observed only one
package marker. Configure succeeded while the second package was silently omitted.

The reproduction uses local path packages to isolate the namespace defect.
Remote packages can introduce transitive names into the same namespace; a full
remote-Git chain was not executed. Windows case folding adds a storage collision;
the CMake collision is not Windows-only. Integration identity must match its
consumer's rules. [FetchContent reference](https://cmake.org/cmake/help/latest/module/FetchContent.html).

**Change:** define one canonical identity across project, transitive graph, lock,
filesystem and CMake. Reject case-folded and normalized collisions, including
reserved device names. Use separate safe storage identifiers where necessary;
never merge different sources silently.

**Acceptance:** case and hyphen/underscore aliases fail before generation/fetch.
Two permitted distinct packages both reach real CMake with verification enabled
on all platforms.

## ZAP-03: verification is controlled by mutable Git metadata

**Location:** `internal/zap/verify.go:79`; `git.go:19`, `:36`, `:62`.
OWASP A08/A02; CWE-345/CWE-829.

Prior tests show assume-unchanged edits and ignored extra files passing against
an unchanged lock. The new fsmonitor test configured a harmless checkout-local
hook; `verify --offline` executed it. No build/upload was requested. This is
observed execution, not a speculative shell-injection claim.

The attacker must control checkout metadata or equivalent Git configuration.
SECURITY currently trusts that state: this is a conditional high-impact limitation,
not remote unauthenticated RCE. It blocks any future untrusted-cache guarantee.
[Git configuration reference](https://git-scm.com/docs/git-config).

**Change:** centralize hardened Git execution with explicit environment/config,
disabled fsmonitor and replacement-object behaviour, and no checkout hooks/filters
during identity establishment. Compute canonical hashes from isolated Git object
reads; verify actual files independently of index/status. Reject unsupported
submodules/object types until handled. Establish new lock hashes from the
configured source, not an existing mutable checkout.

**Acceptance:** hidden edits, ignored additions, forged index state, replacement
objects, hooks and filters cannot alter acceptance or execute during inspection.
Verify complete source and selected subsets separately. Add mutation/race tests;
hashes alone do not stop changes between verification and compilation. Strict
CI should consume an immutable or separately isolated snapshot.

## ZAP-04: declared URI policy is not effective transport/egress policy

**Location:** `internal/zap/security.go:15` and Git fetch/ls-remote calls.
OWASP A01/A02/A06; CWE-918/CWE-15 adapted to a network-capable CLI.

The recent declaration fix works for the input spelling. A new test nevertheless
used inherited `url.*.insteadOf` to turn an accepted HTTPS source into a local
repository while local-Git opt-in was disabled. The exact rewrite targeted a
local fixture; no network request was made. This requires local configuration
control, already identified as trusted in the docs.

Transitive manifests also select arbitrary HTTPS/SSH destinations without a
parent-specific host authorization or egress policy. Internal access, credential
forwarding and redirects were not actively tested; impacts are deployment-dependent.

**Change:** restrict effective Git protocols after rewrites, redirects and
external helpers. Add explicit approved-host/source policy in strict CI. Review
new transitive origins before network access. Permit enterprise/private servers
through deliberate policy rather than a universal public-only rule. Bound DNS,
redirect and private-address behaviour where egress guarantees are promised.
[OWASP SSRF guidance](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html).

**Acceptance:** controlled-server tests cover rewrites, HTTPS-to-HTTP redirects,
helpers, internal destinations and unauthorized child origins; approved private
servers still work.

## ZAP-05: online URL verification covers declarations, not source

**Location:** `internal/zap/verify.go:108`, `project.go:322` and CMake URL generation.
OWASP A08/A06; CWE-345.

Zap checks declaration syntax and delegates download hashes to CMake. It does
not hash extracted files. An existing local CMakeLists.txt can select SOURCE_DIR
instead of the archive-download/hash path. This is source inspection, not a
newly executed archive-tampering exploit. Offline rejection and direct online
warnings reduce false assurance; they do not implement content verification.

**Change:** either classify URL packages as declaration-only in structured
reports and reject them in strict mode, or implement bounded downloading,
digest validation, safe extraction and locked materialised identity. Preserve
legacy CMake handling only as a clearly weaker compatibility mode. Test the
actual extractor before claiming archive traversal defenses.
[OWASP file-handling guidance](https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html).

**Acceptance:** valid content passes; altered archives/extracted files, escaping
paths, links/devices, duplicate/case-colliding entries and expansion bombs fail
before build. Set explicit redirect, file-count and expansion limits.

## ZAP-06: ambiguous metadata and integer overflow

**Location:** `internal/zap/yaml.go:39`, `:311`, `:532`, `:609`; `lock.go:56`;
`semver.go:21`. OWASP A05/A08/A10; CWE-20/CWE-190.

Tests demonstrate duplicate project URI keys, duplicate remote dependency URI
keys and duplicate lock blocks being accepted. Maps replace earlier values while
order lists may retain duplicates. The effective source can differ from a reviewer's
reading of the first entry. A malicious maintainer could also openly declare an
unwanted dependency, so ambiguity alone is not a complete authentication bypass.

A separate probe accepts a 100-digit semver major despite Atoi overflow. Ignored
conversion errors and overflowing range arithmetic can distort selection; this
does not demonstrate memory corruption or arbitrary execution in Go.

**Change:** reject duplicate mappings/blocks, malformed booleans/scalars and
inconsistent graphs. Use a strict bounded documented subset or a maintained
parser configured to reject ambiguity. Check every conversion, bound version
components and check upper-bound arithmetic. Add bounded parser fuzzing.

**Acceptance:** duplicates at every supported level and overflows yield
line-specific errors. Valid manifests, older supported schemas, unique graph
round trips and normal version ranges still work.
[OWASP input validation](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html).

## ZAP-07: audit silently stops on read failure

**Location:** `internal/zap/audit.go:23`, `:57` and missing Scanner.Err check.
OWASP A09/A10; CWE-252/CWE-391.

A 70 KiB comment before a literal dependency URL caused scanning to stop, return
no error and report no sources. No large resource attack was required. Open/walk
errors are also ignored. A limited literal scanner must still distinguish a
complete scan from unreadable/truncated input.

**Change:** surface read/walk/scanner errors or a structured incomplete result
that can never produce the clean/no-sources message. Set explicit file/line
limits. Init source review should fail closed on incomplete coverage unless an
explicitly recorded policy permits it.

**Acceptance:** the long-line fixture either finds the later URL or reports
incomplete audit. Permission-denied and I/O faults must not yield success.
[OWASP exceptional conditions](https://owasp.org/Top10/2025/A10_2025-Mishandling_of_Exceptional_Conditions/).

## ZAP-08: release tag injection into PowerShell

**Location:** `.github/workflows/release.yml:40` and `:296`.
OWASP A05/A03; CWE-78; CI/CD pipeline execution risk.

`${{ github.ref_name }}` is inserted directly into a single-quoted PowerShell
script. A tag accepted by git check-ref-format, beginning with v, broke out of
that string and printed a marker locally. The v* trigger is not a version
validator. The affected job subsequently builds and may sign a binary.

An actor needs permission to supply a triggering tag and pass applicable approval
gates. Those controls were not inspected live. If the same actor already controls
arbitrary release code, this adds no authority; it matters where tag creation
is intended to be less powerful than editing scripts. No workflow was triggered.

**Change:** pass ref names in environment variables and read them as data.
Validate a strict release-version grammar before writing outputs or calling tools.
Never splice GitHub expressions into shell source. Review repository-variable
interpolation and generated Homebrew Ruby too. Protect release refs and restrict
them to approved commits/workflow versions.

**Acceptance:** the valid-tag marker probe cannot execute; non-version names and
shell metacharacters are rejected as release inputs. Validate locally, then in
a non-signing workflow environment.
[OWASP command-injection defense](https://cheatsheetseries.owasp.org/cheatsheets/OS_Command_Injection_Defense_Cheat_Sheet.html).

## ZAP-09: broad release supply-chain trust

**Location:** `.github/workflows/ci.yml`; `release.yml:8`, `:19`, `:35`, `:101`,
`:174`, `:201`, `:289`. OWASP A03/A02; CI/CD dependency, privilege and integrity risks.

Configuration selects Go 1.22.x, mutable action version tags and an nfpm image
tag rather than a digest. WingetCreate is downloaded from a latest URL and
executed without an explicit signature/digest check. Workflow-wide contents-write
and id-token-write permissions reach jobs that do not need both. Signing is
conditional; no attestation or SBOM generation is visible in the workflow.

Go only supports a major release until two newer majors exist. Go 1.22 is out
of support. The clean local scan used 1.27.1; it does not validate old release
artifacts. This is a maintenance gap, not proof of a particular exploitable CVE.
[Go release policy](https://go.dev/doc/devel/release).

**Change:** release with a supported patched Go version and automated updates.
Keep old Go only in an explicitly labeled compatibility job if required. Pin
actions to reviewed full SHAs and containers to digests; verify the exact
WingetCreate download. Default to contents-read and grant write/OIDC per job.
Disable checkout credential persistence where unnecessary. Inspect OIDC subject/
audience restrictions and signing-environment approvals. Require signing when
claiming signed releases; publish provenance/SBOMs and verify the artifact set.
Checksums alongside assets are not independent provenance.

**Acceptance:** missing required signer, approved ref, pinned tool or provenance
fails release. Test/build jobs cannot publish. CI runs implemented security
acceptance tests; current go test ./... excludes build-tagged OWASP/claim tests.
Track future failing criteria separately, without hiding current regressions.

## ZAP-10: credentials persist and appear in diagnostics

**Location:** `internal/zap/security.go:15`, `yaml.go:504`, `git.go:36`,
`configure.go:124`. OWASP A04/A09; CWE-532/CWE-312.

Synthetic tests show a password-bearing HTTPS URI being accepted and serialized
into configuration, and an execution error containing a complete URI/password
argument. Lock and generated declarations carry the same URI. No real credentials
were used.

Environment-derived CMake values avoid committing secrets to YAML, but they are
passed as arguments and may enter CMakeCache/build outputs. That tool-dependent
exposure was not dynamically tested here. Environment transport is not secret
storage; deliberately embedding credentials in firmware is a further application
risk outside this CLI review.

**Change:** reject source URI passwords/tokens; prefer scoped credential helpers
or SSH identities. Centralize URI/argument redaction before diagnostics. Preserve
useful host/path context. Identify sensitive build values, restrict output
permissions and document process/cache exposure.

**Acceptance:** synthetic userinfo/query/helper secrets never appear in logs or
committed source declarations. Valid credential-helper authentication still works.
[OWASP logging guidance](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html).

## ZAP-11: writes destroy data through aliases

**Location:** `internal/zap/lock.go:308`, `upload.go:439`; related direct writes
in `project.go:39`, `:56`. OWASP A01/A08/A10; CWE-377/CWE-59.

WriteLock uses predictable zap.lock.tmp. A test planted a hardlink there pointing
to another disposable file: WriteLock overwrote the unrelated file. This requires
local ability to plant the link, not a remote manifest alone. Final rename cannot
undo the earlier write through a hardlink.

A second test copied a firmware fixture onto itself and observed truncation.
An artifact already at its volume-copy destination, or an alias, can trigger
this primitive. No device was used. Generated-file symlink/reparse handling is
a related inspection gap, not an additional reproduced exploit.

**Change:** create unique exclusive temporary files in the destination directory,
validate ownership/link policy, sync and atomically replace. Serialize project
mutations. Check file identity before truncating a copy destination. Preserve
UF2 device semantics: rename is not necessarily equivalent to copying firmware.

**Acceptance:** planted temp files, hardlink/symlink aliases, same-file copies,
interruptions and concurrent operations preserve unrelated/original data while
normal writes still succeed.

## ZAP-12: operations and failures are not consistently bounded

**Location:** `internal/zap/git.go:19`, `:36`, `:41`; `resolver.go:78`, `:392`;
`project.go:183`, `:192`. OWASP A06/A10; CWE-400/CWE-252.

Subprocesses have no context deadlines and captured output grows in bytes.Buffer.
Tags, blobs and graph breadth have no overall budget. The useful 64-iteration
resolution limit does not bound network time, bytes or node count. Remote manifest
reads treat any git show failure as an absent optional manifest. Sync writes a
new lock before all dependencies materialise; failure can leave mixed state.
The lock replacement itself does use a final rename, not a simple in-place write.

These are inspection findings. No hung-server, OOM, manifest-read fault or
concurrent-sync race was injected in this review.

**Change:** introduce operation contexts, bounded output/manifests/objects/graphs,
noninteractive CI network behaviour and cancellation with child cleanup.
Distinguish confirmed absent metadata from corruption/process failure. Stage the
candidate lock and sources, then commit verified state or use an explicit
recoverable transaction journal.

**Acceptance:** test timeout, oversized response, fan-out, missing versus unreadable
manifest, disk-full/interrupted write and concurrent sync. Failures must not claim
complete verification or silently select a different graph.

## OWASP coverage map

The categories follow the 2025 taxonomy; applicability is this review's assessment,
not an assertion that OWASP has evaluated Zap.

| OWASP category | Application to Zap |
| --- | --- |
| A01 Broken Access Control | Filesystem containment and destination policy: ZAP-01, 04, 11. OS permissions remain the account boundary |
| A02 Security Misconfiguration | Ambient Git configuration and pipeline authority: ZAP-03, 04, 09 |
| A03 Software Supply Chain Failures | Release execution, tools, compiler support, provenance: ZAP-08, 09 |
| A04 Cryptographic Failures | Credential exposure: ZAP-10. Standard SHA-256 usage is not itself a finding |
| A05 Injection | PowerShell injection and metadata ambiguity: ZAP-08, 06. No SQL/web injection surface identified |
| A06 Insecure Design | Ownership, identity, verification and budgets: ZAP-01, 02, 03, 05, 12 |
| A07 Authentication Failures | No Zap account/session system. Git/OS/cloud authenticate; live credential and OIDC policy need separate review |
| A08 Software or Data Integrity Failures | Dependency identity, materialised bytes and writes: ZAP-02, 03, 05, 06, 11 |
| A09 Security Logging and Alerting Failures | Incomplete audit and secret-bearing errors: ZAP-07, 10 |
| A10 Mishandling of Exceptional Conditions | Scanner failures, overflow, partial state, subprocess limits: ZAP-06, 07, 11, 12 |

Additional hardening opportunities: sanitize control characters in Zap-owned
output derived from remote descriptions; expose structured complete/partial
verification results; make recursive programmer discovery deterministic and
explicitly approved; validate real mount/device identity rather than relying
only on labels or, on Unix, directory names. These surfaces were inspected,
not independently exploited. Keep raw tool output distinguishable from Zap's
own trusted status messages.

## Controls worth preserving

There is useful existing defensive work: safe target/ref checks, bracket-quoted
CMake values, argument-vector execution, immutable commit selection, moved-tag
detection, manifest digests, conflict/cycle checks, managed-marker validation and
explicit override documentation. Remote Git packages cannot introduce transitive
local path dependencies through the resolver. The small standard-library-only
Go dependency graph also reduces third-party Go exposure.

The recent declaration-policy, missing-source and offline-URL fixes have passing
evidence. None of these findings means all dependency management is broken.
They identify the boundaries that need strengthening before stronger claims.

## Recommended framework changes

Use shared security boundaries instead of scattered per-command checks:

| Component | Responsibility and invariant |
| --- | --- |
| SourcePolicy | Canonical identities, approved transports/hosts, explicit overrides, no embedded credentials; validate before remote access |
| GitRunner | Controlled environment, bounded output/time, typed failures, no unexpected execution during inspection |
| ManifestParser / GraphValidator | Unique keys, checked integers, bounded graphs and consistent identity across consumers |
| VerifiedSnapshot | Created only after content verification; carries complete-source and selected-source digests with schema/version |
| Materializer | Safe extraction and verified source placement; no trust in cache names or keys |
| ProjectTransaction | Owned paths, unique temporary files, locking, recoverable updates and guarded cleanup |
| VerificationReport | Explicit evidence levels: declaration-only, Git-state-checked, content-verified, override, missing, unsupported |
| ReleaseGate | Approved ref, supported compiler, pinned tools, scoped credentials, tests, signing policy and attestations |

Add a strict/CI policy: frozen lock, no automatic resolution/update, no silent
local override, no declaration-only URL acceptance, and no build until every
required source meets the required verification level. Retain a clearly labeled
developer mode for mutable paths and existing CMake compatibility.

Types help structure these invariants but cannot stop filesystem races or an
attacker controlling the OS account. Strict CI needs isolation/read-only source
consumption as well as content hashes.

### Small repair batches

1. **Safety and release blockers:** ZAP-01/02/07/08; supported release Go and
   scoped workflow permissions. Gate: protected cleanup fixtures survive,
   every dependency reaches CMake, incomplete audits are visible, and tag text
   cannot execute. Promote fixed tests into required default CI coverage.
2. **Input and local-operation safety:** strict duplicate/number handling,
   credential redaction, unique temporary files and same-file copy protection.
   Gate: metadata has one interpretation and operations preserve unrelated data.
3. **Controlled Git execution:** one hardened runner, explicit host/protocol
   policy, deadlines, resource limits and typed failures. Gate: inspection cannot
   execute hooks or inherit transport-policy bypasses in strict mode.
4. **Content integrity:** schema-2 migration, isolated object identities,
   VerifiedSnapshot and URL materialisation. Gate: hidden/ignored edits and archive
   attacks fail with an unchanged trusted lock; valid positive controls build.
5. **Only then add the reusable cache:** verify before and after extraction,
   support atomic installation and offline recovery. Gate: cache tampering cannot
   change accepted build source and corruption fails closed offline.
6. **Release assurance:** required cross-platform security tests, signed/attested
   artifacts, inventory and isolated build inputs. Inspect repository/cloud
   permissions live; source inspection cannot establish those settings.

Do not turn every unfinished roadmap test into an always-red release gate.
Require implemented guarantees in normal CI; track future criteria separately
with owners and explicit closure tests. Never change a security expectation into
a passing assertion that the unsafe behaviour is correct.

## Reproduction

From the workspace root with Go, Git and CMake on PATH:

```powershell
.\scripts\test-owasp.ps1
```

Equivalent command (the PowerShell tag test is Windows-specific):

```text
go test -tags claims,owasp ./internal/zap -run '^TestOWASP' -count=1 -timeout=5m -v
```

The claims tag supplies shared disposable fixtures. Nonzero exit currently means
unmet security expectations. These tests do not use production repositories,
credentials, programming volumes or release accounts.

The official scanner can be rerun without adding modules to Zap's go.mod:

```text
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 -json ./...
go vet ./...
```

Keep findings open until their acceptance tests pass on the relevant platforms.
Honest documentation and a clean known-vulnerability scan are useful evidence;
neither substitutes for fixing demonstrated unsafe behaviour.
