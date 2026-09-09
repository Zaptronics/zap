# `zap-package.yml`

A Git or local path dependency can publish a `zap-package.yml` file at the
package root. The manifest describes package-level source paths, selectable
components, and (with schema 2) transitive package dependencies.

## Schema 2

```yaml
schema: 2
package:
  name: DemoLibrary
  description: Example component library
  cmake_project: DemoLibrary
  paths:
    - cmake
    - include

dependencies:
  transport:
    type: git
    uri: https://github.com/example/transport.git
    version: ^1.4.0
    components:
      - transport::spi

components:
  demo::chip:
    kind: driver
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
added transitively from the targets selected by the consuming package/project.
Component `depends` describes dependencies **inside the same package**.

The top-level `dependencies` section describes dependencies on **other
packages**. Each package dependency can declare:

- `type: git` with `uri:` and `version:`;
- `type: path` with `uri:` when the parent package is itself local;
- optional `components:` that the parent package requires; and
- optional `zephyr_module: true`.

Git version syntax is documented in `DEPENDENCIES.md`. Transitive URL/archive
dependencies are not supported in resolver v1.

## Schema 1 compatibility

Schema 1 remains accepted for existing packages and supports the same package
metadata and component graph, but it cannot declare top-level package
`dependencies:`. Change the manifest to `schema: 2` when transitive package
dependencies are needed.

## CMake responsibility

Zap resolves/materialises package dependencies in child-before-parent order so
required source trees and CMake targets can exist before the parent package is
made available. The package itself remains responsible for linking its CMake
targets to the targets it consumes.

## Security rules

`zap-package.yml` is untrusted remote input. Zap therefore rejects the package
before checkout/generation if:

- a component, `pico`, `zephyr`, or transitive component target is not a strict
  CMake target name;
- a package dependency name or Git version/ref is malformed;
- package requirements with the same dependency name disagree on source/type;
- a sparse path is absolute, contains traversal, `.git`, backslashes, or
  non-normalized path segments;
- the component graph contains a dependency cycle or references an undeclared
  component; or
- the package dependency graph contains an unsatisfiable version conflict or a
  cycle.

Remote strings are never copied verbatim into executable CMake positions.
Package-manifest content is SHA-256 recorded in `zap.lock`; `zap verify` checks
that materialised package metadata still matches the resolved graph.
