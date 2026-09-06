# First-run experience

1. Download the `zap` release archive for the host OS and put the executable on
   `PATH` (or place it temporarily in the project root).
2. Create/open the normal MCU/RTOS CMake project using the vendor's preferred
   IDE or project generator.
3. From the project root run `zap init`.
4. Review the external dependency/source URLs Zap discovered in existing CMake,
   West/Zephyr manifests and the proposed ZapEE dependency. Confirm that they
   are expected and trusted.
5. Review the file-change plan. Zap creates its manifest/generated files and
   edits only the clearly marked Zap-managed regions of root `CMakeLists.txt`.
6. Confirm the detected environment/target/board, repository and module choices.
7. Run `zap sync` (or just `zap`) to fetch/configure, or `zap make` to fetch, configure, and compile.
8. Open the project in the IDE. Sparse dependency sources are under `deps/` and
   CMake compile metadata exposes public includes to IntelliSense/clangd.

New Git dependencies are recorded with both a readable `version:` and immutable
`commit:` lock.

For CI/non-interactive initialization, external sources must be acknowledged
explicitly:

```text
zap init --non-interactive --accept-sources ...
```

An existing `zap.yml` is never overwritten by normal `zap init`; `--force` is
required for replacement.


## Optional native build workflow

`zap init` writes a `build:` section with a `Release` default. Pico SDK projects
default to the Ninja generator. Project-specific CMake cache values can be added
there, including `env:VARIABLE_NAME` references for local credentials/settings.
This allows `zap make` to replace a project-local shell/PowerShell build script
without putting machine-specific values into source control.
