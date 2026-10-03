# Changelog

## Unreleased

### 0.11.1 - Windows upload artifact paths

- Expand upload `{artifact}` paths with forward slashes so OpenOCD Tcl commands preserve Windows paths, including paths with spaces.
- Document explicit OpenOCD selection, USB BOOTSEL requirements, and the effect of artifact overrides during auto fallback.

Validation: source and patch review only; no builds, tests, or hardware uploads run.

### 0.11.0 - managed Pico SDK and recursive submodules

- Initialize recursive Git submodules at the commits recorded by the locked superproject; use full checkouts for repositories declaring submodules.
- Verify recursive submodule revisions and working-tree modifications.
- Prefer the declared pico-sdk dependency over installed SDKs and stale CMake cache values; generate cmake/zap_sdk.cmake for inclusion before SDK import and project().
- Exclude the Pico SDK from ordinary FetchContent integration in Pico projects.

Validation not run: building and testing are left to the user.

### 0.10.0 - repeatable guided setup

- let zap init revisit existing project settings with current values as defaults, a proposed-change summary and default-No saving;
- update selected manifest fields while preserving unrelated settings and source text, without resolving dependencies or regenerating build files;
- add a public signer alias and optionally update only the local user's existing signer labels after review;
- add -init/--init aliases and explicit --yes for non-interactive existing-project updates; and
- add a welcoming, staged introduction and explanations for new and existing projects.

Source/documentation changes only; no builds, tests or exhaustive review performed.

### 0.9.3 - signer identity privacy choice

- ask during zap init whether new signer entries should publish hashed or readable account/host labels, defaulting to hashed;
- support init --signer-identity hashed|public and store the per-user preference outside the project;
- bind recorded identifiers through the signed signer-list inventory without claiming user/host attestation; and
- leave existing signer entries unchanged.

Source edits only; not built or tested.

### 0.9.2 - paginated source diffs

- show requested diffs in pages of five files, including page/remaining counts;
- default Enter to the next page, while keeping final signing approval default No;
- clarify that stopping the diff view does not exclude unseen files from approval; and
- document privacy considerations for optional public signer user/host labels.

Source/documentation edits only; no builds or tests run.

### 0.9.1 - missing signer-list recovery

- let interactive audit --enroll recover from an existing baseline with a missing signer list, using explicit default-No fresh setup and preserving the old baseline as unverified evidence;
- explain how ordinary audit can recover the original signer list from Git; and
- document the portable Go build command without an explicit output filename.

Source and documentation edits only; no builds or tests run.

### 0.9.0 - shared hardware signers

- move public identities into .zap/signers.json, with user/device labels and one entry per key;
- treat the checked-out signer list as authorised by Git/human review, without separate local trust approval;
- add zap audit --enroll and explicit --migrate-signers migration, reusing existing local private-key references;
- verify shared baselines without private-key enrollment, and sign portable logical-root manifests with a signer fingerprint;
- include signers.json in source integrity and allowlist the registry/baseline for Git while retaining private history exclusions; and
- retire the local baseline digest as an anti-replay trust gate; shared state and rollback review belong to Git.

Source-only change: not built, tested, or exhaustively reviewed. Existing single-signer test expectations have not been migrated.

### 0.8.5 - prominent integrity warnings

- highlight integrity warnings on stderr with a bold yellow heading and a separate audit action;
- preserve clear warning blocks when colour is disabled; and
- label routine integrity scans accurately.

### 0.8.4 - audit report links

- make audit report labels explicit terminal hyperlinks with escaped file URLs, including paths containing spaces;
- print an escaped file URL when output is redirected or terminal links are unavailable; and
- retain readable report labels and paths in history without hyperlink control sequences.

### 0.8.3 - project display names

- support optional project.name in zap.yml, independent of the CMake target;
- suggest a name from local Git origin configuration or the folder during zap init, with a --name override; and
- use the configured display name in project banners and audit summaries, retaining folder fallback for older manifests.

### 0.8.2 - audit presentation

- show terminal-only animated lightning progress during scanning, enrollment and signing;
- retain complete file inventories and changes in each audit operation’s audit-report.json, with concise scope trees on screen;
- explain initial key setup and highlight review prompts, preserving default-No approval;
- show contextual coloured diffs with visible line endings and final-newline changes; and
- preserve terminal colour detection through history capture.

### 0.8.1 - hardware key enrollment correction

- replace SSH signing with Windows TPM/CNG, macOS Secure Enclave and Linux TPM-device adapters; no software fallback;
- let zap audit generate the initial protected key after explicit approval, save a resumable local reference, and publish only the public key in zap.yml;
- refuse silent signer replacement and legacy SSH enrollment; preserve public identity through normal manifest updates; and
- add enrollment/trust tests and document platform prerequisites and pending native hardware validation.

### 0.8.0 - optional signed source baselines

- extend zap audit with source comparisons, explicit default-No approval and OpenSSH signing;
- retain exact source snapshots and prior signed baselines; pin the public key and latest checkpoint outside the project;
- warn during normal operations without changing their exit status; existing dependency checks remain enforced;
- detect local dependency edits independently of Git status and include downloaded dependency source directories; and
- document the limits: no native TPM enrollment, build isolation, rollback, source-history signatures or automatic key migration. See docs/INTEGRITY.md.

### 0.7.0 — local operation history

- add zap e / zap exec command recording with child exit codes and separate output streams;
- record normal Zap operations, before/after dependency snapshots, explicit dependency changes, and build inputs;
- add zap log viewing, notes, snapshot comparisons, and explicit known-good build annotations; and
- retain local history without automatic truncation or publishing. Rollback, archives, exports, and full terminal emulation remain future work; see docs/HISTORY.md.

### 0.6.4 — filesystem-identity cleanup checks

- compare cleanup targets against the actual filesystem identities of the project root and every resolved ancestor, addressing the macOS case-alias deletion reported by CI;
- retain path and symlink/junction checks, and fail closed if an ancestor cannot be inspected; and
- extend case-alias regression coverage to parent/grandparent directories and preserve distinct case-sensitive sibling directories as valid cleanup targets. Native macOS confirmation requires the next CI run; concurrent filesystem replacement remains outside this protection.

### 0.6.3 — GitHub Actions runtime and cache configuration

- update CI and release workflows to actions/checkout v7 and actions/setup-go v7, which declare the Node 24 action runtime; and
- explicitly key the Go cache from go.mod, avoiding the missing-go.sum warning in this standard-library-only module.

These changes address workflow warnings. The separate macOS cleanup failure was subsequently diagnosed and addressed in 0.6.4.

### 0.6.2 — visible Git protection

- warn on stderr when inherited Git repository selectors are removed, naming each variable once per invocation without exposing its value; no override is provided;
- resolve the cleanup test's temporary-directory path before checking fixture containment, including macOS temporary-directory aliases; and
- expose regression test results in the existing Windows/Linux/macOS CI matrix. Local Windows results do not establish Linux/macOS runtime success.

### 0.6.1 — repository and cleanup boundaries

- discard inherited Git repository, worktree, common-directory, index, and object-directory selectors in Zap's Git subprocesses;
- resolve symlink/junction aliases before cleanup and reject resolved project roots, ancestors, and filesystem roots;
- fail cleanup when existing paths cannot be inspected or resolved; and
- add disposable-repository and symlink/junction regression tests. Concurrent filesystem replacement, Git hooks/filters, and subprocess timeouts remain follow-up work.

### 0.6.0 — security review follow-up

Continues from the supplied 0.5.1 source baseline.

- reject differently cased Windows aliases of the project root during build cleanup;
- reject duplicate dependency fields even when a scalar value ends in a colon, while preserving namespaced component names;
- replace references to missing validation scripts with runnable Go commands; and
- add regression coverage for these cleanup and metadata-validation cases.

### Other unreleased work

- add schema-5 intent-only dependency manifests with deterministic `zap.lock` resolution and automatic migration of legacy in-manifest Git commit locks;
- add semantic-version constraints (`^`, `~`, explicit ranges), explicit Git tag/ref/commit constraints, and local path dependencies;
- add `zap-package.yml` schema 2 transitive package dependencies with one-version graph resolution, conflict diagnostics, cycle detection and manifest integrity hashes;
- make `zap add`/`zap remove` manage direct dependencies, move component selection to `zap component add/remove`, and retain deprecated compatibility aliases;
- make `zap update` deliberately advance lockfile resolutions within declared constraints, and add `zap outdated` and `zap tree --why`; and
- make generation, status and verification consume the complete locked dependency graph rather than only direct `zap.yml` entries.
- add an interactive ANSI terminal presentation with boxed command/result summaries, coloured phase markers, warnings, hints, and status output;
- preserve raw CMake/Ninja/west/Git/compiler streams so build diagnostics remain unchanged;
- automatically disable styling when output is redirected, honour `NO_COLOR`, and support `ZAP_COLOR=always|auto|never`; and
- enable Windows virtual-terminal processing directly when Zap is attached to a compatible console;
- build signed-ready Windows, universal macOS, DEB, RPM, and portable release artifacts from native GitHub runners; and
- add optional WinGet and Homebrew publishing plus release/code-signing documentation.

## 0.4.0

- add `zap upload` (aliases `zap program` and `zap flash`) with all programmer/tool policy declared in `zap.yml`;
- add schema-4 `upload:` configuration with named methods, `auto` fallback ordering and per-method artifacts;
- add generic `volume-copy` programming for labelled UF2-style mounted volumes on Windows/macOS/Linux;
- add generic `command` programming with PATH/recursive executable discovery, placeholders, environment-backed variables and auxiliary file discovery;
- add in-place optional argument groups for programmer settings such as CMSIS-DAP serial numbers;
- add `zap upload --build` / `--clean` integration with the native `zap make` pipeline;
- scaffold editable BOOTSEL + picotool upload methods for new Pico SDK projects; and
- document that upload commands are trusted project configuration and cannot be supplied by remote package manifests.

## 0.3.0

- add `zap make` (alias `zap build`) to synchronise, verify, configure, and compile projects without project-local PowerShell/shell build scripts;
- add schema-3 `build:` configuration with build type, CMake generator, and safe CMake cache settings;
- support `env:NAME` CMake cache mappings so local credentials/settings stay outside source control;
- add `--clean`, `--configuration`, `--board`, `--build-dir`, `--generator`, `--define`, `--configure-only`, `--offline`, and `--no-sync` build overrides;
- resolve Pico SDK/toolchain/picotool versions from VS Code-generated CMake metadata when available;
- report Pico UF2/ELF outputs after successful builds; and
- refuse dangerous clean targets such as the project root or filesystem root.


## 0.2.1

- fix `zap sync` so active local source overrides still resolve and record immutable remote Git commit locks;
- keep remote ref/commit integrity verification enabled while a local development override is active; and
- clarify that local override contents are developer-controlled while the declared upstream lock remains verified.

## 0.2.0

Security and project-safety release.

- add immutable Git commit locks to `zap.yml` schema 2;
- verify remote refs, local origin, locked HEAD and clean dependency worktrees;
- add configure-time `zap verify` with `ZAP_OFFLINE` support;
- require SHA-256 hashes for URL dependencies;
- validate all CMake target/dependency names derived from local or remote manifests;
- validate sparse checkout paths from remote package manifests;
- use CMake bracket arguments for URI/path scalar data;
- audit existing external dependency sources during `zap init` and require trust confirmation;
- restrict CMake edits to explicit Zap-managed BEGIN/END blocks and refuse malformed markers;
- add `zap audit` and `zap verify` commands;
- preserve schema-1 project compatibility and upgrade locks during synchronisation; and
- fix annotated-tag locking to record the underlying commit rather than the tag object.

## 0.1.0

Initial native Go implementation with project initialization, YAML manifests,
sparse Git dependencies, CMake/Zephyr generation, Pico discovery and adapter
scaffolding.
