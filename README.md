# ⚡ Zap

**Friendly dependency and project tooling for embedded C/CMake.**

**Current security status:** Zap locks dependency versions and performs limited Git
checkout consistency checks. Independent source hashes, a verified machine cache and
cache-backed offline sync are planned, not implemented. Git-hidden edits can still evade
verification. Current guarantees and roadmap probes are tested separately; see
[SECURITY.md](SECURITY.md) and [claim evidence](docs/CLAIM_VALIDATION.md).

Zap is a small native command-line tool from Zaptronics. It gives embedded C/C++
projects package-manager-style dependency handling without replacing the build systems
you already use. Your project still builds with ordinary CMake, Pico SDK, or Zephyr;
Zap manages dependency intent, locked resolution, generated integration, builds,
and programming around them.

Zap is deliberately boring underneath: no Python, Node.js, .NET, JVM, daemon, or
proprietary build format is required.

## What Zap does

Zap currently provides:

- interactive setup for an existing **Pico SDK, Zephyr, or generic CMake** project;
- human-readable `zap.yml` project/dependency configuration;
- `zap.lock` resolution that records exact Git commits and package-manifest digests;
- semantic-version constraints, Git tags/refs/commits, local paths, and hashed HTTPS
  archives;
- transitive package dependencies and conflict detection;
- package/component selection with sparse source materialisation when supported;
- `add`, `remove`, `update`, `outdated`, `tree`, `list`, and `status` dependency
  lifecycle commands;
- offline verification/build options for already-present Git/path dependencies;
- limited Git checkout consistency checks and literal external-source auditing;
- generated CMake/Zephyr integration that remains ordinary, inspectable build input;
- `zap make` for configure/build workflows;
- configurable `zap upload` programming methods such as UF2 volume copy, picotool,
  OpenOCD, or other command-line tools;
- platform-adapter skeleton generation for portable ZapEE integration; and
- readable ANSI terminal output that falls back cleanly to plain text in logs/CI.

Zap does **not** install your compiler, MCU SDK, CMake, Ninja, `west`, Git, programmer,
or debug probe software. Those remain normal platform/toolchain prerequisites.

---

## Install

Zap is one native executable.

### Download the latest release

These links always follow the GitHub release currently marked **Latest**:

| Platform | Download |
| --- | --- |
| Windows x64 (Intel/AMD) | [zap_windows_amd64.zip](https://github.com/Zaptronics/zap/releases/latest/download/zap_windows_amd64.zip) |
| Windows ARM64 | [zap_windows_arm64.zip](https://github.com/Zaptronics/zap/releases/latest/download/zap_windows_arm64.zip) |
| macOS Universal (Intel + Apple Silicon) | [zap_darwin_universal.zip](https://github.com/Zaptronics/zap/releases/latest/download/zap_darwin_universal.zip) |
| Linux x64 portable | [zap_linux_amd64.tar.gz](https://github.com/Zaptronics/zap/releases/latest/download/zap_linux_amd64.tar.gz) |
| Linux ARM64 portable | [zap_linux_arm64.tar.gz](https://github.com/Zaptronics/zap/releases/latest/download/zap_linux_arm64.tar.gz) |
| SHA-256 checksums | [SHA256SUMS.txt](https://github.com/Zaptronics/zap/releases/latest/download/SHA256SUMS.txt) |

Versioned `.deb` and `.rpm` packages are also attached to the
[Latest Release](https://github.com/Zaptronics/zap/releases/latest).

### Windows

When `Zaptronics.Zap` is available in the WinGet community repository, install with:

```powershell
winget install --id Zaptronics.Zap -e
```

If WinGet reports that the package is not available yet, use the ZIP release. A simple
per-user installation is:

```powershell
$ZapDir = "$env:LOCALAPPDATA\Programs\Zap"
New-Item -ItemType Directory -Force -Path $ZapDir | Out-Null
Expand-Archive "$HOME\Downloads\zap_windows_amd64.zip" -DestinationPath $ZapDir -Force

$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($UserPath -split ';') -notcontains $ZapDir) {
    [Environment]::SetEnvironmentVariable("Path", ($UserPath.TrimEnd(';') + ";" + $ZapDir), "User")
}
$env:Path += ";$ZapDir"

zap version-tool
```

For Windows on ARM, download `zap_windows_arm64.zip` instead.

### macOS

When the Zaptronics Homebrew tap is enabled, install with:

```bash
brew install --cask Zaptronics/tap/zap
```

Manual installation from the latest release:

```bash
curl -fL https://github.com/Zaptronics/zap/releases/latest/download/zap_darwin_universal.zip -o /tmp/zap.zip
rm -rf /tmp/zap-install
mkdir -p /tmp/zap-install
unzip -q /tmp/zap.zip -d /tmp/zap-install
sudo install -m 0755 /tmp/zap-install/zap /usr/local/bin/zap
zap version-tool
```

### Ubuntu / Debian

Download the `.deb` for your architecture from the
[Latest Release](https://github.com/Zaptronics/zap/releases/latest), then:

```bash
# Intel / AMD 64-bit
sudo apt install ./zap_*_linux_amd64.deb

# ARM64
sudo apt install ./zap_*_linux_arm64.deb
```

### Fedora / RHEL

Download the `.rpm` for your architecture from the
[Latest Release](https://github.com/Zaptronics/zap/releases/latest), then:

```bash
# Intel / AMD 64-bit
sudo dnf install ./zap_*_linux_amd64.rpm

# ARM64
sudo dnf install ./zap_*_linux_arm64.rpm
```

### Other Linux distributions

Use the portable archive:

```bash
# Intel / AMD 64-bit
curl -fL https://github.com/Zaptronics/zap/releases/latest/download/zap_linux_amd64.tar.gz -o /tmp/zap.tar.gz

# For ARM64, use zap_linux_arm64.tar.gz instead.
tar -xzf /tmp/zap.tar.gz -C /tmp
sudo install -m 0755 /tmp/zap /usr/local/bin/zap
zap version-tool
```

After installation, open a new shell and check:

```text
zap help
```

---

## Five-minute start

Zap currently **initialises an existing CMake project**; it does not generate an MCU
vendor project from nothing. Create/open your normal Pico SDK, Zephyr, or CMake project
first, then run Zap from its project root.

### 1. Initialise

```text
zap init
```

Zap tries to detect the environment and target, asks a few questions, scans existing
project-controlled CMake/West files for literal external dependency URLs, shows exactly
what it plans to create/edit, and asks you to confirm the sources are expected.

`zap init` creates:

```text
zap.yml                    human project/dependency intent
zap.lock                   exact dependency resolution
cmake/zap_deps.cmake       generated CMake dependency integration
cmake/zap_zephyr.conf      generated Zephyr integration
```

It also adds two clearly marked managed regions to the root `CMakeLists.txt`. Zap will
only replace text inside those regions; malformed or duplicated markers are an error.

Zap does **not** rewrite `west.yml`, `prj.conf`, Pico SDK files, Zephyr source/build
files, or arbitrary vendor files. An existing `zap.yml` is not overwritten unless you
explicitly use `zap init --force`.

### 2. Synchronise dependencies

```text
zap sync
```

Bare `zap` means the same thing as `zap sync`.

A normal sync resolves/validates `zap.lock`, materialises dependencies, regenerates the
managed CMake/Zephyr files, and configures the project. To sync/generate without running
CMake configure:

```text
zap sync --no-configure
```

### 3. Build

```text
zap make
```

This synchronises/verifies dependencies, generates integration, configures the project,
and compiles it.

### 4. Program the target (if configured)

```text
zap upload --build
```

### 5. Commit the resolved dependency state

Commit both:

```text
zap.yml
zap.lock
```

Do not hand-edit `zap.lock`.

---

## The dependency model

The easiest way to understand Zap is:

| File/location | Meaning |
| --- | --- |
| `zap.yml` | What the project **wants**: direct sources, constraints, components, build/upload settings |
| `zap.lock` | What Zap **resolved**: complete graph, exact Git commits, declared URL hashes, manifest digests, selected source paths |
| `deps/` | Project-local dependency checkouts, checked using Git state |
| `zap-package.yml` | What a package exposes and which other packages/components it requires |
| Zap cache | Planned; no reusable machine cache is implemented |

New project manifests use **`zap.yml` schema 5**. Current lockfiles use
**`zap.lock` schema 1**.

### Example `zap.yml`

```yaml
schema: 5

project:
  environment: pico-sdk
  target: controller
  board: pico2_w
  build_dir: build
  deps_dir: deps
  adapters_dir: adapters

build:
  configuration: Release
  generator: Ninja
  cmake:
    DEVICE_NAME: env:MY_DEVICE_NAME

upload:
  default: auto
  order:
    - bootsel
    - picotool
  methods:
    bootsel:
      type: volume-copy
      artifact: "{build_dir}/{target}.uf2"
      volume_labels:
        - RPI-RP2
        - RP2350
    picotool:
      type: command
      artifact: "{build_dir}/{target}.uf2"
      executable: picotool
      search:
        - PATH
        - "{pico_home}/picotool"
      args:
        - load
        - -f
        - -v
        - -x
        - "{artifact}"

dependencies:
  zapee:
    type: git
    uri: https://github.com/Zaptronics/zap-ee.git
    version: ^0.5.0
    override_var: ZAPEE_SOURCE
    zephyr_module: true
    components:
      - zap::pico_ztm_pwm
```

Older Zap projects that stored `commit:` beside `version:` in `zap.yml` are migrated by
`zap sync`: the exact commit is preserved in `zap.lock` and human intent remains in
`zap.yml`.

---

## Add, remove, and select dependencies

Zap has no central package registry yet, so a new package needs an explicit Git source
or local path.

### Add a Git dependency

```text
zap add zapee --git https://github.com/Zaptronics/zap-ee.git --version ^0.5.0
```

If `--version` is omitted, Zap looks for the latest stable `x.y.z`/`vX.Y.Z` tag and
writes a compatible-major (`^`) constraint.

Select components while adding with repeatable `--component`:

```text
zap add zapee --git https://github.com/Zaptronics/zap-ee.git --version ^0.5.0 \
  --component zap::ztm_pwm --component zap::ztm_adc
```

### Add a local development dependency

```text
zap add mylib --path ../mylib
```

Local path dependencies are intentionally mutable. They do not provide the immutable
resolution semantics of an exact locked Git commit or the declared archive identity of a
URL dependency. Neither of those mechanisms is a bit-for-bit build reproducibility guarantee.

### Remove a dependency

```text
zap remove mylib
```

### Add/remove package components

```text
zap component add zapee zap::ztm_pwm
zap component remove zapee zap::ztm_pwm
```

The old `zap add <dependency> <component>` and
`zap remove <dependency> <component>` forms are still accepted temporarily as
deprecated compatibility aliases.

---

## Versions, updates, and transitive dependencies

Git dependencies support:

| Syntax | Meaning |
| --- | --- |
| `1.2.3` or `v1.2.3` | Exact semantic version |
| `^1.2.3` | Compatible major: `>=1.2.3 <2.0.0` |
| `^0.5.1` | Compatible pre-1.0 minor: `>=0.5.1 <0.6.0` |
| `~1.2.3` | Compatible minor: `>=1.2.3 <1.3.0` |
| `>=1.2.0 <2.0.0` | Explicit range |
| `tag:v1.2.3` | Explicit Git tag |
| `ref:develop` | Branch/ref |
| `commit:<40-char-sha>` | Exact immutable commit |

Change the constraint of a direct Git dependency with:

```text
zap version zapee ^0.6.0
```

`zap sync` preserves an existing compatible lock. A new matching tag appearing remotely
will **not** silently change tomorrow's build.

Inspect available updates without changing anything:

```text
zap outdated
```

Deliberately update locked versions within the constraints already declared in
`zap.yml`:

```text
zap update
zap update zapee
```

Packages can declare transitive package dependencies in `zap-package.yml` schema 2.
Zap resolves the complete graph and currently chooses one version of a package name for
the whole graph. If requirements cannot share one compatible version/source, Zap fails
with a conflict showing which parent required what.

Inspect the graph with:

```text
zap tree
zap tree --why transport
```

`--why` prints the path(s) that caused a transitive dependency to be present.

---

## Lockfile and verification

For Git dependencies, `zap.lock` records exact commits, resolved versions/refs,
requested constraints and parents, selected components, and a SHA-256 digest of
package-manifest text when present. It does **not** record independent hashes
of complete repository source or materialised files.

```text
zap verify
zap verify --offline
```

Verification checks lock compatibility, managed checkout presence, origin, HEAD,
Git-reported changes and the locked manifest. Online verification also checks
release tags for movement; offline verification omits remote lookups. Git-hidden
edits and ignored files can evade these checks. This is not proof of content
integrity against an attacker controlling checkout files or Git metadata.

URL/archive declarations require HTTPS and an explicit SHA-256:

```yaml
dependencies:
  archive:
    type: url
    uri: https://example.com/library-1.2.3.tar.gz
    hash: SHA256=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

Zap validates the declaration and passes `URL_HASH` to CMake, which handles the
download, archive hash check and extraction. Zap does not hash archive bytes or
extracted source itself. Online verification warns about this limited coverage;
offline verification rejects URL dependencies as unsupported.

See [SECURITY.md](SECURITY.md) for the trust boundaries and roadmap.

---

## Offline work and planned cache

There is no reusable machine dependency cache in this version. `ZAP_CACHE_DIR`,
`zap cache` commands and `zap sync --offline` are not implemented.

With an existing lock and local Git/path dependencies, use:

```text
zap verify --offline
zap make --offline
```

These skip Zap's remote checks and require the managed sources to exist locally.
They cannot restore missing dependencies and reject URL dependencies. Project
CMake, SDKs or other invoked tools can still access the network; the option is
not a network sandbox.

A verified per-user cache, safe snapshot extraction, atomic materialisation and
cache-backed offline restoration are **planned**, after independent source
verification is implemented. See [cache/offline status](docs/CACHE_AND_OFFLINE.md).

---

## Package manifests and sparse materialisation

A Git or local path package can publish a root `zap-package.yml`.

Schema 2 can describe:

- package metadata;
- selectable CMake components;
- source paths required by each component;
- dependencies between components in the same package;
- Pico/Zephyr component variants; and
- transitive dependencies on other packages.

When a package provides source-path metadata, Zap materialises only the package and
component paths needed by the resolved graph. Packages without `zap-package.yml` still
work and are materialised as complete dependencies.

Remote manifest input is validated before being used for CMake generation. See
[`docs/PACKAGE_MANIFEST.md`](docs/PACKAGE_MANIFEST.md).

---

## CMake and Zephyr integration

Zap generates:

```text
cmake/zap_deps.cmake
cmake/zap_zephyr.conf
```

These files are generated from `zap.yml` + `zap.lock` and should not be edited manually.
Use:

```text
zap generate
```

to regenerate them, and:

```text
zap check
```

to confirm generated files and managed `CMakeLists.txt` integration markers are in sync.

Generated CMake invokes Zap's limited checks before consuming dependencies. With
verification enabled, missing managed Git source is an error telling you to run
`zap sync`; CMake does not fetch a replacement. URL archives are handled by CMake
online and are unsupported in offline verification.

To request local verification in generated CMake (project build code can still use the network):

```text
-DZAP_OFFLINE=ON
```

Advanced users can explicitly disable configure-time Zap verification with:

```text
-DZAP_VERIFY_DEPENDENCIES=OFF
```

That is a **security/reproducibility bypass** and should not be the normal workflow.

---

## Build with `zap make`

`zap make` is an optional native replacement for project-local build scripts.

```text
zap make
zap make --clean
zap make --configuration Debug
zap make --configure-only
zap make --offline
zap make --no-sync
zap make --build-dir out
zap make --board pico2_w
zap make --generator Ninja
zap make --define FEATURE_X=ON --define LOG_LEVEL=3
```

Build defaults live under `build:` in `zap.yml`. CMake cache values can be literal or
come from environment variables so local credentials/settings do not need to be stored
in source control:

```yaml
build:
  configuration: Release
  generator: Ninja
  cmake:
    WIFI_SSID: env:PROJECT_WIFI_SSID
    WIFI_PASSWORD: env:PROJECT_WIFI_PASSWORD
    SOME_LITERAL_OPTION: ON
```

An unset `env:NAME` value is omitted. Use `literal:` when a literal value itself begins
with `env:`. Zap passes values directly to CMake as argument-vector entries rather than
through a shell.

For Pico SDK projects, Zap can use the SDK/toolchain/picotool versions selected by the
Pico VS Code tooling under `~/.pico-sdk`, falling back to the newest installed matching
tool when the exact configured version is unavailable.

`--clean` removes only the configured build directory. It refuses the project root, a
filesystem root, or any configured clean target that lexically contains the project root.
This is a guard against destructive configuration mistakes, not a complete symlink/junction
security boundary.

---

## Program with `zap upload`

Programming policy lives in the project's `upload:` section; Zap does not choose a
programmer on the user's behalf.

Zap currently provides two generic method types:

- `volume-copy` — copy an artifact to a matching mounted volume, useful for UF2/BOOTSEL;
- `command` — discover and run a configured native tool such as picotool or OpenOCD.

Useful commands:

```text
zap upload
zap upload --method openocd
zap upload --build
zap upload --build --clean
zap upload --build --offline
zap upload --artifact path/to/custom.uf2
```

An `auto` upload method can try configured methods in order. Explicitly selecting a
method never falls through to another method.

See [`docs/UPLOAD.md`](docs/UPLOAD.md) for placeholders, variables, optional arguments,
tool discovery, and examples.

---

## Inspect and maintain a project

These commands are intentionally read-only unless noted:

```text
zap list
```

Shows direct dependency intent, locked resolution, selected components, and whether
transitive packages are present.

```text
zap status
```

Shows lock state and whether materialised managed Git dependencies match `zap.lock`.

```text
zap tree [--why <dependency>]
```

Shows/explains the complete resolved dependency graph.

```text
zap outdated
```

Shows compatible updates and newer releases outside the current constraint.

```text
zap verify [--offline]
```

Checks lock compatibility and Git checkout state; URL content is not verified by Zap.

```text
zap audit
```

Lists **literal** external dependency/source URLs found in `zap.yml`, root CMake,
project `cmake/*.cmake`, and common West/Zephyr manifests. It is a useful review aid,
not a general-purpose static security scanner.

```text
zap generate
zap check
```

Regenerate/check Zap-managed CMake/Zephyr integration.

```text
zap adapter [name]
```

Creates a portable ZapEE adapter skeleton under the configured `adapters_dir` with TODO
I2C/SPI/time/locking callbacks for your MCU SDK or RTOS.

```text
zap version-tool
zap help
```

Show the installed Zap version or command help.

---

## Command reference

| Command | Purpose |
| --- | --- |
| `zap` | Same as `zap sync` |
| `zap init` | Initialise Zap in an existing CMake/Pico/Zephyr project |
| `zap sync [--no-configure]` | Resolve/materialise dependencies, generate integration, configure |
| `zap add <name> --git <uri> [--version ...]` | Add a Git dependency |
| `zap add <name> --path <path>` | Add a local path dependency |
| `zap remove <dependency>` | Remove a direct dependency |
| `zap component add/remove ...` | Change selected package components |
| `zap version <dep> <constraint>` | Change a direct Git dependency constraint |
| `zap update [dependency ...]` | Move lock resolutions within declared constraints |
| `zap outdated` | Report available dependency updates |
| `zap tree [--why <dependency>]` | Show/explain the resolved dependency graph |
| `zap list` | Show configured dependencies and locked resolutions |
| `zap status` | Show lock/materialisation state |
| `zap verify [--offline]` | Check lock and Git checkout consistency |
| `zap audit` | List literal external source URLs in project manifests |
| `zap generate` | Regenerate managed CMake/Zephyr files |
| `zap check` | Check generated files and integration markers |
| `zap make [...]` | Sync/configure/build |
| `zap upload [...]` | Program using `zap.yml` upload methods |
| `zap adapter [name]` | Generate a ZapEE platform-adapter skeleton |
| `zap version-tool` | Show installed Zap version |
| `zap help` | Show terminal help |

Aliases also exist for a few common verbs: `new` → `init`, `build` → `make`, and
`program`/`flash` → `upload`.

---

## Security summary

Zap's current security model is intentionally narrower than a cryptographically verified
package cache. Current checks establish lock/config consistency and limited Git checkout
consistency. Git-hidden source edits and ignored files can still pass verification, so the
managed checkout and its Git metadata are not an untrusted security boundary.

Implemented hardening includes HTTPS/SSH source policy, required SHA-256 declarations for
URL packages, rejection of credential-bearing HTTPS source URLs, duplicate-mapping rejection,
and rejection of dependency names that CMake `FetchContent` would merge after case-folded
normalisation. Git status checks explicitly disable `core.fsmonitor`, and Zap strips
process-injected `GIT_CONFIG_COUNT`/`GIT_CONFIG_PARAMETERS` key-value configuration before
running Git. Repository/global Git configuration, the Git executable, redirects, filters,
checkout behaviour, CMake and other build tools remain trusted.

Local destructive operations also have targeted guards: `--clean` rejects project ancestors,
lockfiles are staged through unique temporary files, and volume-copy refuses a source/destination
that resolves to the same file. These are useful safety controls, not filesystem sandboxing.

Independent complete-source hashes, isolated Git object reads, verified snapshots, safe archive
extraction and a tamper-resistant reusable cache are **roadmap items, not current guarantees**.
`TestCurrentClaim*` acceptance tests must pass for documented current behaviour;
`TestRoadmapClaim*` tests remain real failures until those stronger guarantees are implemented.
See [claim validation](docs/CLAIM_VALIDATION.md).

Local paths, source overrides and `ZAP_VERIFY_DEPENDENCIES=OFF` are intentional developer
exceptions. Zap cannot make malicious dependency CMake/source safe or promise bit-for-bit
reproducible firmware binaries.

---

## Terminal output

Zap uses ANSI colour and compact phase boxes on an interactive terminal. External output
from CMake, Ninja, west, Git, and compilers is streamed unchanged so diagnostics remain
copy/paste- and machine-friendly.

When output is redirected, Zap automatically falls back to plain text. It honours the
standard `NO_COLOR` environment variable. You can override colour behaviour with:

```text
ZAP_COLOR=always
ZAP_COLOR=auto
ZAP_COLOR=never
```

---

## Documentation

- [`docs/FIRST_RUN.md`](docs/FIRST_RUN.md) — first project setup
- [`docs/DEPENDENCIES.md`](docs/DEPENDENCIES.md) — constraints, lockfiles, transitive resolution, updates, graph inspection
- [`docs/CACHE_AND_OFFLINE.md`](docs/CACHE_AND_OFFLINE.md) — current offline limits and planned cache
- [`docs/SECURITY_MODEL.md`](docs/SECURITY_MODEL.md) — current checks and planned security architecture
- [`docs/PACKAGE_MANIFEST.md`](docs/PACKAGE_MANIFEST.md) — `zap-package.yml` schema and component/transitive package metadata
- [`docs/UPLOAD.md`](docs/UPLOAD.md) — upload/programming configuration
- [`docs/HOW_TO_GIT.md`](docs/HOW_TO_GIT.md) — everyday Git workflow and creating releases
- [`docs/RELEASE_DISTRIBUTION.md`](docs/RELEASE_DISTRIBUTION.md) — WinGet, Homebrew, DEB/RPM and release automation
- [`docs/CODE_SIGNING.md`](docs/CODE_SIGNING.md) — Windows and macOS production signing setup

## License

Apache-2.0.
