# Zap Architecture Discussion Summary
## From Conan build dependencies to “Zap — The Graph Commander”

This document summarises the design discussion beginning with the question:

> “Conan recipes like CMake for the build system. But would other dependencies depend on CMake to build?”

The discussion developed into a broader architecture for Zap as a dependency, tooling, build-orchestration, provenance, and audit system.

---

# 1. Build tools are dependencies too

A core design decision is that tools such as CMake, Ninja, Meson, Conan, Python, GCC, vendor SDKs, and similar software should be represented in the dependency graph rather than hard-coded into Zap.

For example:

```text
application
└─ package-a
   └─ build tool → cmake
```

Another package may use a completely different build system:

```text
application
├─ package-a
│  └─ build tool → cmake
└─ package-b
   └─ build tool → meson
      └─ tool → ninja
```

Zap therefore should not assume one build system for an entire project.

A package declares what it needs, and Zap resolves and provisions those requirements.

This means a package can depend on tools in the same general way it depends on libraries or source packages.

---

# 2. Multiple build systems must coexist

Zap should support a graph where different packages use completely different build mechanisms.

Example:

```text
application
├─ lib-a → CMake
├─ lib-b → Meson + Ninja
├─ lib-c → Conan
├─ lib-d → prebuilt binary
└─ lib-e → vendor-specific build tool
```

The important architectural principle is:

> Zap should orchestrate build systems, not replace them.

Zap should not evolve into a build system that every package must conform to.

---

# 3. Conan versus PlatformIO

The discussion compared Conan and PlatformIO because they take very different approaches.

Conan allows packages to declare their own build requirements and build mechanisms.

Conceptually:

```text
package-a
└─ build requirement → cmake

package-b
└─ build requirement → meson
   └─ ninja
```

PlatformIO is much more opinionated. PlatformIO itself acts as the central build environment and adapts libraries into its own build model.

Zap should follow the Conan direction:

> Packages own how they are built; Zap owns dependency resolution and orchestration.

This avoids turning Zap into another PlatformIO-style monolithic build environment.

---

# 4. Existing package ecosystems are valuable bootstrap sources

Conan Center, WinGet, vcpkg, Homebrew, PlatformIO, Nixpkgs, and similar ecosystems already contain large amounts of useful package information.

Examples include:

- package identity
- publisher
- versions
- official repositories
- download URLs
- hashes
- build recipes
- platform support
- dependencies
- tool dependencies

This could give Zap an enormous head start.

However, Zap should not blindly depend on those ecosystems internally.

Instead, they should be treated as external package information providers.

---

# 5. Avoid making Zap depend on Python

Conan recipes are Python programs.

A naïve implementation could have resulted in:

```text
Zap
└─ Conan
   └─ Python
```

as a permanent runtime requirement for Zap itself.

That was rejected.

The better rule is:

> Zap itself must remain independent of Python.

If a package uses Conan, then Conan becomes a dependency of that package.

If Conan requires Python, Python becomes a dependency of Conan.

For example:

```text
some-library
└─ build tool → conan
   └─ runtime → python
```

Zap itself remains a standalone Go executable.

This distinction also means that a future Zap package can legitimately target Python without Zap itself becoming Python-based.

---

# 6. Conan becomes just another tool in the graph

Rather than translating Conan recipes into Zap-native recipes, Conan can simply be treated as an ordinary tool node.

Example:

```text
foo-library
└─ build → conan
   └─ requires → python
```

Zap does not need to understand arbitrary `conanfile.py` logic.

Conan remains responsible for interpreting its own recipes.

This avoids several problems:

- Zap does not need to track Conan recipe-language compatibility.
- Zap does not need a Python parser/runtime.
- Zap avoids trying to flatten arbitrary conditional Conan logic.
- Conan’s own ecosystem remains usable as designed.

This is a much cleaner boundary.

---

# 7. Avoid combinatorial expansion of recipe conditions

A concern was raised that if Zap tried to preprocess Conan recipes, a package with many conditional options could produce an exponential number of combinations.

For `n` binary conditions, the theoretical number of combinations is:

```text
2^n
```

For example:

```text
shared: true/false
ssl: true/false
simd: true/false
tests: true/false
...
```

Trying to precompute all combinations would become impractical.

Treating Conan as a tool avoids this entirely.

Conan evaluates the relevant path for the actual build configuration.

---

# 8. “Zap — The Graph Commander”

The discussion produced a useful description of Zap’s role:

> **Zap — The Graph Commander**

Zap is not simply a package manager and not simply a build system.

It manages a graph containing things such as:

```text
source packages
libraries
build tools
compilers
interpreters
vendor SDKs
binary artifacts
package managers
build systems
generated artifacts
```

Zap’s responsibilities are:

```text
resolve
fetch
verify
materialise
order
execute
audit
lock
```

The actual compilation/build logic can remain owned by the native build tools.

---

# 9. Package resolvers

A new first-class concept emerged: **resolvers**.

Resolvers convert a short logical package name into a concrete package identity, source, artifact, or recipe.

Potential resolvers include:

```text
Zap Registry
WinGet
vcpkg
Conan Center
Homebrew
Git
vendor package indexes
```

For example:

```yaml
dependencies:
  - conan: fmt@11
  - vcpkg: zlib
  - winget: Kitware.CMake
```

or in a generic form:

```yaml
dependencies:
  - resolver: conan
    package: fmt
    version: "^11"
```

Raw URLs remain an escape hatch:

```yaml
dependencies:
  - git: https://github.com/example/foo.git
    ref: v1.2.0
```

The goal is for users to prefer short, meaningful identities rather than manually entering repository URLs.

---

# 10. Resolver federation rather than simple priority

Zap should not simply use a fixed resolver priority list.

When a user runs:

```text
zap add cmake
```

Zap should query all relevant available resolvers for the current platform.

Example:

```text
cmake
├─ WinGet
├─ Homebrew
├─ vcpkg
├─ Conan
└─ Zap Registry
```

Zap can then compare:

- package identity
- upstream repository
- publisher
- available versions
- release URLs
- hashes
- licence
- supported operating systems
- architectures

If several independent resolvers agree that `cmake` means Kitware CMake, that is useful provenance evidence.

Example output concept:

```text
Found CMake in 4 resolvers:

WinGet    Kitware.CMake     4.x
Homebrew  cmake             4.x
vcpkg     cmake             4.x
Zap       kitware/cmake     4.x

Identity agreement: HIGH
Publisher: Kitware
Upstream: Kitware/CMake
```

If resolvers disagree, Zap should surface that instead of silently choosing.

---

# 11. Short aliases versus canonical identities

Short package names are convenient for discovery, but they are potentially ambiguous.

For example:

```text
cmake
protobuf
python
```

After discovery, Zap should store a canonical identity.

Example:

```text
cmake
    ↓
kitware/cmake
```

or:

```text
protobuf
    ↓
protocolbuffers/protobuf
```

The short name is therefore a user-friendly alias.

The canonical package identity is what belongs in durable project state.

This also helps prevent package-name squatting.

---

# 12. A GitHub-hosted Zap registry

Zap should have its own registry, probably hosted publicly in GitHub.

The workflow could resemble WinGet:

```text
zap-registry/
├─ packages/
├─ aliases/
└─ publishers/
```

Community contributors submit pull requests.

CI can validate:

- schema
- URLs
- hashes
- platform compatibility
- dependency graph validity
- installability
- basic smoke tests

The registry does not need to host all upstream source or binaries.

Instead it can contain metadata and recipes that point to authoritative upstream locations.

---

# 13. Upstream project versus Zap package metadata

Many projects will not contain a native `zap.package`.

Zap therefore needs a mechanism similar to Homebrew/WinGet/vcpkg recipes.

Example:

```text
Kitware/CMake
└─ no zap.package
```

Zap Registry can contain:

```text
zap-registry/packages/kitware/cmake/zap.package
```

That package definition maps the logical Zap identity to the real upstream project.

If, later, Kitware chooses to maintain its own `zap.package`, the registry can recognise and prefer that publisher-controlled definition.

---

# 14. Publisher verification and signed package metadata

A future upstream publisher could maintain its own `zap.package` and sign it.

The important trust relationship is not simply:

> “This Git commit was signed.”

It is:

> “This signing key is authorised to publish this canonical package identity.”

Example:

```text
kitware/cmake
└─ authorised publisher key(s)
```

Zap should support:

- publisher identities
- approved signing keys
- key rotation
- key revocation

Possible package provenance classes:

```text
Native / publisher verified
Registry maintained
Imported / external resolver
```

These should be exposed as factual provenance information rather than compressed into a simplistic “safe” rating.

---

# 15. Binary versus source acquisition

Zap should let the user choose whether to use prebuilt binaries or build from source.

Possible policies include:

```text
prefer-binary
binary-only
prefer-source
source-only
```

Example:

```yaml
policy:
  acquisition: prefer-binary
```

Per-package overrides should be allowed:

```yaml
dependencies:
  cmake:
    acquisition: binary

  random-dev-tool:
    acquisition: source
```

The default can reasonably prefer mature official binaries for convenience.

---

# 16. Why source mode matters for Zap audit

The reason for source mode is not that source is automatically safe.

The benefit is inspectability.

With a source dependency update, Zap can show meaningful changes:

```text
foo 1.2.0 → 1.3.0

Source changes:
  + 5 files
  ~ 18 files
  - 1 file

Dependency changes:
  + zlib

Build recipe:
  CMake >= 3.20 → >= 3.24
```

The user can inspect changes before accepting them.

For a binary-only update, Zap can report:

```text
version changed
publisher signature
hash changed
artifact size
resolver agreement
provenance
```

but Zap cannot realistically show the source-level behavioural difference between the two binaries.

---

# 17. Reproducible binaries are not automatically safer

A potential trap was identified around reproducible binaries.

A reproducible binary proves:

> This binary corresponds to this source and build process.

It does **not** prove:

> This source is safe.

Malicious source can reproducibly produce a malicious binary.

Also, if the user has already built the source locally purely to compare it with the vendor binary, there may be little reason to then use the vendor binary.

Therefore reproducibility should be treated as a factual property, not a security score.

---

# 18. Separate security properties

Zap should expose several independent properties instead of one “security score”.

Useful categories include:

## Provenance

Where did this package come from?

Who published it?

## Integrity

Does the current artifact match the expected hash/signature?

## Transparency

Can the source and changes be inspected?

## Correspondence

Does a binary demonstrably correspond to a particular source/build?

## Resolver agreement

Do independent package indexes agree on package identity and publisher?

Zap should report facts such as:

```text
Publisher signature: verified
Source available: yes
Source diff available: yes
Binary available: yes
Artifact hash verified: yes
Resolver agreement: high
```

but should avoid asserting:

```text
SAFE
```

---

# 19. Source builds may have larger dependency graphs

Building from source can increase the graph significantly.

A binary install might be:

```text
cmake binary
```

A source build could require:

```text
cmake source
├─ compiler
├─ make/ninja
├─ compression libraries
├─ bootstrap utilities
└─ other build-time dependencies
```

Therefore:

> Source is more inspectable, but not necessarily simpler or lower risk.

Zap should expose these trade-offs and let the user choose.

---

# 20. Build drivers / adapters

Zap currently touches CMake files, but the preferred direction is to remove build-system-specific behaviour from Zap core.

A new abstraction was proposed:

> **Build drivers**

Resolvers answer:

> Where/how do I obtain this package?

Build drivers answer:

> How do I build or integrate this package?

Examples:

```text
Resolvers            Build drivers
---------            -------------
WinGet               CMake
Conan Center         Meson
vcpkg                Make
Homebrew             Ninja
Git                   Conan
Zap Registry          Vendor SDK
```

Zap core ideally only needs generic operations:

```text
resolve
fetch
verify
materialise
set environment
run command
collect artifact
```

Build-specific behaviour lives outside the core.

---

# 21. Zap should orchestrate builds, not own them

A crucial anti-lock-in principle emerged:

> After Zap has resolved and materialised the environment, the project should ideally remain buildable with its native build system.

For example:

```text
zap sync
```

could prepare:

```text
PATH
CMAKE_PREFIX_PATH
toolchain paths
generated presets
package directories
```

Then the developer could run:

```text
cmake --preset debug
cmake --build --preset debug
```

without requiring `zap make`.

`zap make` should therefore be a convenience/orchestration command, not a proprietary build format that permanently locks the project to Zap.

A useful command could eventually be:

```text
zap make --explain
```

showing the environment and native commands Zap is executing.

---

# 22. Avoid rewriting user CMake files when possible

Zap should avoid continually modifying `CMakeLists.txt`.

Prefer native CMake integration mechanisms such as:

```text
CMAKE_PREFIX_PATH
CMake package configuration
toolchain files
CMake presets
generated supplementary files
```

If some explicit hook is unavoidable, it is better to have a tiny stable project-side hook such as:

```cmake
include(.zap/cmake/dependencies.cmake OPTIONAL)
```

and let Zap own the generated file.

This is preferable to Zap rewriting arbitrary user CMake logic.

---

# 23. Zephyr modules

Zephyr modules are similar to a subset of Zap packages.

A Zephyr module is typically a repository that knows how to integrate into Zephyr’s build/configuration environment.

Example:

```text
foo/
├─ zephyr/
│  └─ module.yml
├─ CMakeLists.txt
├─ Kconfig
└─ src/
```

A Zap package is broader.

A Zap package might represent:

```text
library
compiler
tool
runtime
binary
vendor SDK
Zephyr module
Conan-backed package
```

So:

> A Zephyr module is one possible integration type for a Zap package.

---

# 24. West owns Zephyr’s internal workspace graph

West is Zephyr’s multi-repository/workspace manager.

It manages repositories specified by Zephyr’s manifest.

Conceptually:

```text
Zap
└─ Zephyr
   └─ West
      ├─ CMSIS
      ├─ HALs
      ├─ mbedTLS
      ├─ MCUboot
      └─ other Zephyr modules
```

Zap should not necessarily flatten all of West’s internal repositories into the top-level Zap graph.

A useful general rule emerged:

> A dependency manager nested inside a Zap node owns its internal graph unless the user explicitly promotes something into the Zap graph.

Examples:

```text
Zap → Zephyr → West modules
Zap → Conan → Conan dependencies
Zap → Python → Python packages
```

This prevents Zap from reimplementing every dependency ecosystem.

---

# 25. Python virtual environments

Python virtual environments provide isolated package environments.

Example:

```text
project/
└─ .venv/
   ├─ python
   ├─ pip
   ├─ west
   ├─ pyelftools
   └─ ...
```

This allows different projects or toolchains to use different Python package versions without polluting the system Python environment.

In Zap terms:

```text
Zephyr
└─ West
   └─ Python environment
      ├─ west
      └─ Zephyr Python dependencies
```

The Python interpreter itself may be shared while the environment around it is project-specific.

---

# 26. Tool installation scope

Zap needs to distinguish where tools come from.

Three important scopes were identified:

```text
system
shared Zap-managed
project-local
```

## System

Example:

```text
C:\Program Files\CMake\bin\cmake.exe
```

Installed externally to Zap.

## Shared Zap-managed

Example:

```text
~/.zap/tools/cmake/3.30.5/
```

Installed once by Zap and reusable across many projects.

## Project-local

Example:

```text
project/.zap/tools/cmake/3.30.5/
```

Fully isolated to one project.

---

# 27. Project requirements must be machine independent

A critical distinction is:

> The project locks what tool/version it needs, not where one developer happens to have it installed.

For example:

```yaml
# zap.yml
tools:
  cmake: "^3.30"
```

and:

```yaml
# zap.lock
cmake:
  version: 3.30.5
  identity: kitware/cmake
```

Developer A may satisfy that with:

```text
system CMake 3.30.5
```

Developer B may have:

```text
system CMake 3.31.2
```

which does not match the locked version.

Zap can then fall back to:

```text
~/.zap/tools/cmake/3.30.5/
```

The physical installation path is local machine state and should not pollute the portable project lockfile.

---

# 28. Tool selection policy

Zap should support local policies such as:

```text
prefer-system
prefer-zap
project-only
system-only
```

Example:

```yaml
tools:
  cmake:
    version: 3.30.5
    use: prefer-system
```

Meaning:

1. Try a compatible system CMake.
2. Otherwise use an existing Zap-managed copy.
3. Otherwise provision the required version.

For strict CI or reproducibility:

```text
project-only
```

could ignore system tools entirely.

---

# 29. Do not sign arbitrary system files

Zap should not sign external system binaries such as:

```text
C:\Program Files\CMake\bin\cmake.exe
```

because Zap did not publish or own those files.

Instead Zap should:

- discover them
- identify their version
- check upstream publisher signatures if available
- hash/fingerprint them
- record local trust state

For Zap-managed installations, Zap can integrity-protect its own manifest describing what it downloaded and installed.

Example:

```yaml
package: kitware/cmake
version: 3.30.5
artifact_sha256: ...
publisher_signature_verified: true
```

Zap is then asserting:

> These are the same files I previously verified and installed.

It is not asserting authorship of CMake.

---

# 30. `zap e` as an execution trust boundary

A particularly useful idea emerged around `zap e`.

Rule:

> Anything Zap directly executes through `zap e` is fingerprinted and entered into the local tool inventory before execution.

Example:

```text
zap e cmake --build .
```

Zap resolves:

```text
C:\Program Files\CMake\bin\cmake.exe
```

and records:

```text
name: cmake
version: 3.30.5
path: ...
SHA256: ABC...
publisher: Kitware
```

On every later `zap e cmake ...`, Zap rechecks the fingerprint.

If the same path/version now has a different hash:

```text
Recorded: ABC...
Current:  XYZ...
```

Zap should stop and require explicit user acceptance before execution.

---

# 31. Observed tools versus declared tools

Executing a tool through `zap e` should not automatically make it a project dependency.

For example:

```text
zap e git status
```

should not suddenly modify `zap.yml`.

Zap needs two concepts:

## Observed tool

Zap has seen, identified, and fingerprinted it.

## Declared tool

The project explicitly requires it.

Example:

```text
Observed:
git 2.x
cmake 3.30.5

Declared:
cmake 3.30.5
```

A declared dependency can be satisfied by a matching observed system tool.

---

# 32. Scripts must also be fingerprinted

For interpreter-driven commands such as:

```text
zap e python build.py
```

Zap should fingerprint both:

```text
python.exe
build.py
```

Likewise:

```text
zap e powershell build.ps1
zap e bash configure.sh
```

This avoids trusting only the interpreter while ignoring the actual executable script content.

Symlinks should be resolved to the true executable target before hashing.

---

# 33. Future process-tree tracing

A possible future audit feature is process-tree tracing.

For example:

```text
zap e --trace-tools cmake --build .
```

could discover:

```text
cmake
└─ ninja
   ├─ arm-none-eabi-gcc
   ├─ assembler
   └─ objcopy
```

Zap could then fingerprint the actual toolchain used during the build.

This would provide stronger evidence than simply trusting declared build-tool metadata.

It is likely OS-specific and therefore should not be required for the first implementation.

---

# 34. Zap needs user-wide state

Project-local fingerprints are not sufficient.

Otherwise every new project would establish a fresh baseline and could fail to notice that a system tool changed between projects.

Zap therefore needs persistent user-level state.

Conceptually:

```text
~/.zap/
├─ config.yml
├─ state/
│  ├─ tools.db
│  └─ trust.db
├─ tools/
└─ cache/
```

`config.yml` contains preferences.

Example:

```yaml
tools:
  prefer_system: true
  on_fingerprint_change: prompt

acquisition:
  prefer: binary
```

`tools.db` stores observed machine state:

```text
cmake 3.30.5
path: ...
fingerprint: ...
publisher: Kitware
first_seen: ...
last_verified: ...
```

---

# 35. Three distinct scopes of information

The final state model should separate:

## Project state

```text
zap.yml
zap.lock
.zap/
```

Contains portable project requirements and locked identities/versions.

## User Zap state

```text
~/.zap/
```

Contains:

- user policies
- tool fingerprints
- trust decisions
- shared Zap-managed tools
- caches
- resolver state

## Machine state

Represents what actually exists on the computer:

- installed executables
- system packages
- globally shared tools

Potentially there may later be machine-wide Zap state under paths such as:

```text
C:\ProgramData\Zap\
/var/lib/zap/
/etc/zap/
```

but user trust decisions should not automatically become machine-global.

---

# 36. Core design principles established

The conversation converged on several strong architectural principles.

## Zap does not own the build

Zap prepares and commands the build graph.

Native build systems remain native.

## Build systems are tools

CMake, Conan, Meson, Ninja, vendor build systems, etc. are graph nodes.

## Package managers can be nested

Conan, West, pip, and similar tools can own their own internal graphs.

Zap does not need to flatten everything.

## Zap itself stays lightweight

Zap should not acquire Python/CMake/Conan dependencies merely because some packages use them.

## Resolver identity and build mechanism are separate

A package may be found through one resolver and built by a completely different tool.

## The project locks logical requirements

Machine-specific paths and installations remain local state.

## Execution should be verifiable

`zap e` becomes a trust boundary through fingerprinting.

## Audit should report facts

Avoid simplistic “safe/unsafe” scoring.

Expose provenance, integrity, transparency, signatures, resolver agreement, and acquisition choice.

## Users control the trust/convenience trade-off

Mature official binaries may be preferred for convenience.

Source builds remain available when inspectability is important.

---

# 37. Candidate conceptual model

A project could eventually look something like:

```yaml
project:
  name: example

dependencies:
  - package: fmt
    version: "^11"

tools:
  cmake:
    version: "3.30.5"

  conan:
    version: "^2"

policy:
  acquisition: prefer-binary

resolvers:
  - zap
  - conan
  - vcpkg
  - winget
  - homebrew
```

The resulting graph might become:

```text
application
├─ fmt
│  └─ build → cmake
│
├─ another-library
│  └─ build → conan
│     └─ runtime → python
│
└─ zephyr
   └─ west
      └─ python environment
```

Zap resolves the graph, chooses acquisition paths, verifies tools, provisions missing pieces, and optionally orchestrates the native build.

---

# 38. Suggested implementation TODO

The following are the main implementation areas that fall out of the design.

## Graph model

Define first-class graph node/edge types for:

- packages
- tools
- runtimes
- build tools
- artifacts
- nested package managers

## Resolver interface

Define a generic resolver API supporting:

- search
- canonical identity
- versions
- platform compatibility
- source location
- binary location
- hashes/signatures
- publisher metadata

## Resolver federation

Implement:

```text
zap add <name>
```

as a federated lookup rather than simple resolver priority.

Compare resolver agreement and allow the user to select ambiguous matches.

## Canonical identity

Separate:

```text
friendly alias
canonical package identity
```

and persist canonical identity after selection.

## Zap Registry

Create a GitHub-hosted registry with:

- package metadata
- aliases
- publisher records
- PR validation
- signing support

## Acquisition policy

Support:

```text
prefer-binary
binary-only
prefer-source
source-only
```

globally and per package.

## Build drivers

Move CMake-specific behaviour out of Zap core.

Define a build-driver abstraction for:

- CMake
- Meson
- Make
- Ninja
- Conan
- vendor systems

## Native build interoperability

Prefer native integration through:

- environment variables
- package paths
- toolchain files
- presets
- generated adapter files

Avoid rewriting project build files where possible.

## Local tool inventory

Create user-wide state for:

```text
path
version
fingerprint
publisher
first seen
last verified
```

## `zap e`

Fingerprint and verify every directly executed tool.

Require explicit acceptance when the fingerprint changes.

## Script verification

Fingerprint interpreter-driven scripts as well as interpreters.

## Tool scope

Support:

```text
system
Zap shared
project local
```

while keeping the project lock machine-independent.

## Tool fallback

If the exact locked tool version is not available on the system:

```text
system
↓
Zap shared
↓
project-local/provision
```

## Audit

Expose factual audit properties:

- provenance
- integrity
- source availability
- source diff
- signature state
- resolver agreement
- acquisition path
- dependency changes
- tool changes

## Future process tracing

Investigate optional build process-tree tracing for discovering the actual compiler/toolchain invoked beneath the top-level build command.

---

# 39. Working product identity

The discussion produced a concise architectural identity that still fits all of these decisions:

> # Zap — The Graph Commander
>
> Zap resolves, verifies, provisions, and orchestrates dependency and build graphs without replacing the tools that already know how to do their jobs.

That captures the direction particularly well:

```text
Zap commands the graph.
CMake remains CMake.
Conan remains Conan.
West remains West.
Python remains Python.
The project remains buildable without being trapped inside Zap.
```
