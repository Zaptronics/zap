# `zap-package.yml`

A Git dependency can opt into component-aware sparse checkout by publishing a
`zap-package.yml` file at the repository root.

```yaml
schema: 1
package:
  name: DemoLibrary
  description: Example component library
  cmake_project: DemoLibrary
  paths:
    - cmake
    - include

components:
  demo::transport:
    kind: transport
    paths:
      - platform

  demo::chip:
    kind: driver
    depends:
      - demo::transport
    paths:
      - drivers/chip

  demo::board:
    kind: module
    description: Example board module
    pico: demo::pico_board
    zephyr: demo::zephyr_board
    depends:
      - demo::chip
    paths:
      - modules/board
```

`package.paths` are always present in a sparse checkout. Component `paths` are
added transitively from the targets selected in the consuming project's
`zap.yml`.

## Security rules

`zap-package.yml` is untrusted remote input. Zap therefore rejects the package
before checkout/generation if:

- a component, `pico`, or `zephyr` target is not a strict CMake target name;
- a dependency target name is malformed;
- a sparse path is absolute, contains traversal, `.git`, backslashes, or
  non-normalized path segments; or
- the component graph contains a dependency cycle or references an undeclared
  component.

Remote strings are never copied verbatim into executable CMake positions.

The package remains responsible for its own CMake target graph. `zap` uses the
manifest only to decide which source directories must exist in the worktree and
which validated target aliases should be selected for an environment.
