# Upload and programming

`zap upload` is a generic, optional replacement for project-local programming
scripts. Zap does not contain a hard-coded programmer choice. The project owns
its upload policy in `zap.yml`.

Zap currently provides two generic method types:

- `volume-copy`: copy a configured artifact to a mounted volume with one of the
  configured labels (useful for UF2-style bootloaders);
- `command`: discover and execute a configured native programmer/debug tool with
  configured arguments.

The project may declare any number of named methods and an `auto` fallback
order. Selecting a method explicitly never falls through to another method.

```text
zap upload
zap upload --method openocd
zap upload --build
zap upload --build --clean
```

`--build` invokes the same native `zap make` pipeline before programming. The
build directory and target placeholders therefore point at the resulting
artifacts.

## Example

```yaml
upload:
  default: auto
  order:
    - bootsel
    - picotool
    - openocd
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
      variables:
        serial: env:ZAP_PICO_SERIAL
      args:
        - load
        - -f
        - -v
        - -x
        - "{artifact}"
      optional_args:
        serial:
          - --ser
          - "{serial}"

    openocd:
      type: command
      artifact: "{build_dir}/{target}.elf"
      executable: openocd
      search:
        - PATH
        - "{pico_home}/openocd"
      variables:
        probe_serial: env:ZAP_PROBE_SERIAL
        adapter_speed_khz: 5000
      find:
        scripts:
          root: "{pico_home}/openocd"
          file: rp2350.cfg
          parent: 2
      args:
        - -s
        - "{scripts}"
        - -f
        - interface/cmsis-dap.cfg
        - "{?probe_serial}"
        - -f
        - target/rp2350.cfg
        - -c
        - "adapter speed {adapter_speed_khz}"
        - -c
        - "program \"{artifact}\" verify reset exit"
      optional_args:
        probe_serial:
          - -c
          - "adapter serial {probe_serial}"
```

## Placeholders

Command, artifact, working-directory, search and find templates may use:

- `{project_root}`
- `{build_dir}`
- `{target}`
- `{board}`
- `{pico_home}`
- `{artifact}`
- `{tool}` and `{tool_dir}` after executable discovery
- names declared under `variables:` or `find:`

A method variable may be literal or `env:NAME`. An unset environment-backed
variable resolves to an empty value. An optional argument group is emitted only
when its same-named variable is non-empty. Put `{?name}` in `args:` to insert the
group at that exact point; if no insertion marker is present the group is
appended for compatibility with simple tools.

## Executable discovery

`command` methods declare an executable and search locations. `PATH` means the
normal process search path; other entries are directories searched recursively.
This is useful for SDK-managed tool installations without hard-coding a version
into Zap itself.

`find:` resolves auxiliary files/directories. It recursively finds the named
file below `root:` and walks up `parent:` path levels. This lets a project find,
for example, a programmer's script directory while keeping all knowledge of the
programmer layout in `zap.yml`.

## Trust model

Upload commands are intentionally executable project configuration. Treat a
project's `zap.yml` with the same trust as its CMake/build scripts. Remote
`zap-package.yml` files cannot define upload methods or upload commands.
