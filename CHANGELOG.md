# Changelog

## Unreleased

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
