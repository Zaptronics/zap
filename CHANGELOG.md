# Changelog

## Unreleased

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
