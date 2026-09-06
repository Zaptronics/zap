# Zap

**Friendly dependency and project tooling for embedded C/CMake.**

`zap` is a small native command-line tool from Zaptronics. It gives embedded
projects a human-readable `zap.yml` dependency manifest while leaving ordinary
CMake and Zephyr underneath.

## Install

Zap is a single native executable. No Python, Node.js, .NET, JVM, or other
runtime is required.

### Download the latest release

These links always follow the GitHub release currently marked **Latest**:

| Platform | Download |
| --- | --- |
| Windows x64 (Intel/AMD) | [zap_windows_amd64.zip](https://github.com/Zaptronics/zap/releases/latest/download/zap_windows_amd64.zip) |
| Windows ARM64 | [zap_windows_arm64.zip](https://github.com/Zaptronics/zap/releases/latest/download/zap_windows_arm64.zip) |
| macOS Universal (Intel + Apple Silicon) | [zap_darwin_universal.zip](https://github.com/Zaptronics/zap/releases/latest/download/zap_darwin_universal.zip) |
| Linux x64 portable | [zap_linux_amd64.tar.gz](https://github.com/Zaptronics/zap/releases/latest/download/zap_linux_amd64.tar.gz) |
| Linux ARM64 portable | [zap_linux_arm64.tar.gz](https://github.com/Zaptronics/zap/releases/latest/download/zap_linux_arm64.tar.gz) |
| Checksums | [SHA256SUMS.txt](https://github.com/Zaptronics/zap/releases/latest/download/SHA256SUMS.txt) |

Versioned `.deb` and `.rpm` packages are available from the
[Latest Release](https://github.com/Zaptronics/zap/releases/latest).

### Windows

#### WinGet

Once the Zap package has been accepted into the WinGet community repository,
the preferred install will be:

```powershell
winget install --id Zaptronics.Zap -e
```

Until then, download the x64 or ARM64 ZIP above and place `zap.exe` somewhere on
your `PATH`. For a per-user installation from PowerShell, after downloading the
ZIP to your Downloads folder:

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

For Windows on ARM, use `zap_windows_arm64.zip` instead.

### macOS

Once the Zaptronics Homebrew tap is enabled, the preferred install will be:

```bash
brew install --cask Zaptronics/tap/zap
```

For a manual install from the current GitHub release:

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

Use the portable archive and install the single binary into `/usr/local/bin`:

```bash
# Intel / AMD 64-bit
curl -fL https://github.com/Zaptronics/zap/releases/latest/download/zap_linux_amd64.tar.gz -o /tmp/zap.tar.gz

# For ARM64, use zap_linux_arm64.tar.gz instead.
tar -xzf /tmp/zap.tar.gz -C /tmp
sudo install -m 0755 /tmp/zap /usr/local/bin/zap
zap version-tool
```

After installation, `zap help` should work from a new shell. Release and
publisher setup for maintainers is documented in
`docs/RELEASE_DISTRIBUTION.md`.

## First project

From an existing CMake firmware project:

```text
zap init
```

Interactive setup detects Pico SDK, Zephyr, or generic CMake where possible and
asks for the application target, board, dependency directory, ZapEE source, and
modules. Before writing anything, Zap scans the existing project-controlled
CMake/West manifests for literal external dependency URLs, lists them together
with the proposed ZapEE source, and asks the user to confirm that they are
expected and trusted.

`zap init` also prints the files it intends to create or edit. It will:

- create `zap.yml`;
- create `cmake/zap_deps.cmake`;
- create `cmake/zap_zephyr.conf`; and
- edit only two clearly marked regions of the root `CMakeLists.txt`.

Zap does **not** edit `west.yml`, `prj.conf`, Pico SDK files, Zephyr source/build
files, or other vendor files. Existing `zap.yml` files are not overwritten
unless `zap init --force` is explicitly used.

The managed CMake regions look like:

```cmake
# ZAP MANAGED DEPENDENCIES BEGIN
# Managed by zap. Zap may replace only text between this BEGIN/END pair.
# User CMake outside this block is never rewritten by zap.
...
# ZAP MANAGED DEPENDENCIES END
```

and an equivalent `ZAP MANAGED TARGETS` block. Malformed/duplicated markers are
an error; Zap refuses to guess and will not rewrite the file.

Then:

```text
zap sync
```

Bare `zap` is shorthand for `zap sync`.

## Example `zap.yml`

New manifests use schema 4. They record both the human-friendly release/ref and
its immutable Git commit, plus optional project build settings for `zap make`:

```yaml
schema: 3

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
    version: v0.5.0
    commit: 0123456789abcdef0123456789abcdef01234567
    override_var: ZAPEE_SOURCE
    zephyr_module: true
    components:
      - zap::pico_ztm_pwm
```

`zap init` resolves the latest stable version when requested, then locks that
version to the exact commit. Existing older manifests are upgraded by `zap sync`; build settings default to `Release` when absent.

## Dependency integrity

### Git dependencies

`version:` remains readable (`v0.5.0`, a branch, etc.) but `commit:` is the
immutable source lock. `zap sync` and `zap verify` fail if the configured remote
ref later resolves to a different commit.

Managed local Git checkouts are also verified for:

- the configured origin URI;
- exact locked `HEAD` commit; and
- a clean working tree.

### URL/archive dependencies

A URL dependency must include a SHA-256 hash:

```yaml
archive:
  type: url
  uri: https://example.com/library-1.2.3.tar.gz
  hash: SHA256=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
  components:
```

Unhashed URL downloads are rejected.

## Configure-time verification

Generated `cmake/zap_deps.cmake` verifies dependencies before CMake consumes
them. By default it invokes:

```text
zap verify --cmake
```

For disconnected work use:

```text
-DZAP_OFFLINE=ON
```

Offline verification skips the remote `git ls-remote` check but **still** checks
the local origin, commit, and clean working tree.

Experienced/offline environments that intentionally do not install the Zap
binary can explicitly disable the configure-time verifier:

```text
-DZAP_VERIFY_DEPENDENCIES=OFF
```

This is an explicit security bypass; the immutable commit/hash remains in the
generated FetchContent configuration.

## Sparse package checkout

Git dependencies may publish a root `zap-package.yml` manifest. It describes
component source paths and transitive component dependencies. When present,
`zap sync` fetches the requested ref with depth 1, validates the remote package
manifest, resolves its component graph, and checks out only the required paths.

Remote component names and environment aliases are validated against a strict
CMake-target grammar before they can enter generated CMake. Sparse paths are
also restricted to normalized repository-relative paths.

A dependency without `zap-package.yml` still works: Zap uses a normal full
shallow checkout for that dependency.

## Plain CMake remains first-class

Generated CMake first uses a local `deps/<name>` checkout if present. Otherwise
it falls back to normal `FetchContent`. Git fallback uses the immutable `commit:`
lock rather than a mutable tag/branch name. Because CMake does not allow
`GIT_SHALLOW` with a commit hash, this security-first fallback may fetch more Git
history than native Zap's shallow sparse checkout.


## Build with `zap make`

`zap make` is the optional native replacement for project-local build scripts. By
default it synchronises/verifies dependencies, generates the managed CMake glue,
configures the project, and compiles it.

```text
zap make
zap make --clean
zap make --configuration Debug
zap make --configure-only
zap make --offline
zap make --define FEATURE_X=ON
```

Build defaults live in `zap.yml` so IDE users and command-line users share the
same settings. CMake cache values may be literal or sourced from the developer's
environment without storing secrets in source control:

```yaml
build:
  configuration: Release
  generator: Ninja
  cmake:
    WIFI_SSID: env:PROJECT_WIFI_SSID
    WIFI_PASSWORD: env:PROJECT_WIFI_PASSWORD
    SOME_LITERAL_OPTION: ON
```

An unset `env:NAME` value is omitted, matching an optional local build setting.
`literal:` may be used when a literal value itself begins with `env:`. Values are
passed directly to CMake as argument-vector entries; Zap does not invoke a shell.

For Pico SDK projects, Zap reads the VS Code-generated `sdkVersion`,
`toolchainVersion`, and `picotoolVersion` values when present and resolves the
matching installations under `~/.pico-sdk`, falling back to the newest installed
version only when an exact configured version is unavailable.

`--clean` removes only the configured build directory and explicitly refuses to
remove the project root or filesystem root.

## Program with `zap upload`

`zap upload` replaces project-local upload/programming scripts without choosing
a programmer on the user's behalf. Programming methods, executable discovery,
artifacts, arguments, fallback order and tool-specific variables live in the
project's `upload:` section of `zap.yml`.

Zap itself implements only generic `volume-copy` and `command` method types. A
Pico project can therefore describe BOOTSEL/UF2, picotool, OpenOCD, or a company
programmer without changing or recompiling Zap.

```text
zap upload
zap upload --method openocd
zap upload --build
zap upload --build --clean
```

See `docs/UPLOAD.md` for the complete schema, placeholders and optional argument
groups.

## Commands

```text
zap init
zap sync                 # bare `zap` means the same thing
zap make [--clean]        # sync + configure + compile
zap upload [--build]      # program using zap.yml upload methods
zap status
zap verify [--offline]
zap audit
zap list
zap update <dependency>
zap version <dependency> <tag-or-ref>
zap add <dependency> <cmake-target>
zap remove <dependency> <cmake-target>
zap adapter [name]
zap generate
zap check
```

`zap audit` lists literal external dependency/source URLs found in `zap.yml`,
root CMake, project `cmake/*.cmake`, and common West/Zephyr manifests.

## Documentation

- [`docs/FIRST_RUN.md`](docs/FIRST_RUN.md) — first-run project setup
- [`docs/PACKAGE_MANIFEST.md`](docs/PACKAGE_MANIFEST.md) — package/component manifest format
- [`docs/UPLOAD.md`](docs/UPLOAD.md) — upload/programming configuration
- [`docs/HOW_TO_GIT.md`](docs/HOW_TO_GIT.md) — everyday Git workflow and creating GitHub releases
- [`docs/RELEASE_DISTRIBUTION.md`](docs/RELEASE_DISTRIBUTION.md) — WinGet, Homebrew, DEB/RPM and release automation
- [`docs/CODE_SIGNING.md`](docs/CODE_SIGNING.md) — Windows and macOS production signing setup

## License

Apache-2.0.

## Terminal output

Zap uses ANSI colour and compact phase boxes when attached to an interactive terminal. Output automatically falls back to plain text when redirected, and honours the standard `NO_COLOR` environment variable.

Colour behaviour can be overridden with `ZAP_COLOR=always`, `ZAP_COLOR=auto`, or `ZAP_COLOR=never`. External build-tool output from CMake, Ninja, west, Git, and compilers is streamed unchanged so diagnostics remain machine- and copy/paste-friendly.
