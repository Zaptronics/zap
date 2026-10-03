# Guided setup (0.10.0)

Run zap init from a new or existing project. zap -init and zap --init are aliases.
The wizard introduces the settings in four stages:

1. Project identity: the display name shown in banners.
2. Signing identity: hashed/readable account and host labels, plus an optional public alias.
3. Build and dependencies: environment, target, folders and dependency settings.
4. A change summary and a default-No save prompt.

## Existing projects

Existing zap.yml files enter update mode automatically. --force is no longer needed
and no longer selects destructive replacement. Current values are prompt defaults:
Enter retains them. A single - clears an optional text setting. An unset display
name stays unset unless entered, retaining the folder fallback.

Setup reads the current manifest and local signer preferences. It does not fetch
packages, query release tags, replace the lockfile, create hardware keys or regenerate
CMake integration for an existing project. Dependency entries are reviewed individually;
each defaults to keeping its settings. Dependency types are retained; setup does not
add a default ZapEE dependency to a project that has none.

Existing setup covers the project name, environment, target, board, build/dependency/
adapter folders, build configuration/generator, signer preferences, and selected
dependency source/version/hash/components. Other manifest settings, including custom
build definitions, upload methods and dependencies you did not edit, remain unchanged.

Proposed values are shown before saving. Only selected YAML fields/list blocks are
edited; unrelated lines and comments are retained. Comments inside a changed list
are retained after its replacement entries. Existing line endings are preserved.
Candidate manifests are parsed and validated before writing. Unsupported structures
produce an error rather than a whole-file rewrite.

No changes are saved if all values remain the same or final approval is declined.
Normal Zap operation history is still recorded under .zap/history. Source changes
approved here are not automatically approved for integrity: run zap audit afterward.
Run zap sync when dependency or build configuration changes need applying to the
lockfile and generated integration.

Setup checks that the files it read have not changed before saving, uses temporary
file replacement, and attempts rollback of earlier writes on a write failure. It is
not a cross-process transaction; a crash or filesystem failure can leave a partial
multi-file update. Errors identify the affected file.

## Signer privacy and your public alias

The hashed mode hides readable account/host names; public mode includes them.
An optional display label, such as Firmware maintainer, is public regardless of mode.
It is an alias, not an authenticated username. Hashes are also self-reported,
guessable identifiers; the key fingerprint remains the cryptographic signing identity.

The preference is local to the user and checkout. Existing projects start with the
stored preference; absent preferences default to hashed. Keys are not generated here.

If this checkout has a local key reference and its signer entry has different labels,
setup offers an additional default-No choice to update that entry. The final save
summary includes the registry change. Only that key's descriptive fields change;
private keys, fingerprints, signatures and teammates' entries are retained.

Accepting a registry-label update changes the signed inventory, so the next audit
requires review and a new signature. Declining that update can still save the local
preference for future enrollment. Non-interactive setup updates local preferences
only; editing an existing public signer entry remains an interactive choice.

## Non-interactive updates

Only explicitly supplied flags override existing values. Preview proposed changes:

~~~text
zap init --non-interactive --name "Hub"
~~~

Apply them:

~~~text
zap init --non-interactive --name "Hub" --yes
~~~

Other supported options include --environment, --target, --board, --build-dir,
--deps-dir, --adapters-dir, --signer-identity and --signer-label. The --zapee-uri,
--zapee-version and repeated --component flags update only an existing zapee entry.
They do not silently add a dependency. Preview-only changes return nonzero when
--yes is absent; an unchanged configuration completes without saving.

## New projects

New-project setup still expects CMakeLists.txt, infers build settings, suggests a
display name from local Git origin configuration (or the folder), inspects ZapEE
releases/components and confirms external sources. Interactive setup now shows a
final summary and asks before creating the manifest, lock and managed integration.
Release/package discovery can occur before this final prompt.

Non-interactive creation retains its existing explicit source-confirmation behaviour.
The --yes option is needed for unattended updates to an existing project, not as a
replacement for new-project source approval.

## Implementation status

Source and documentation changes only. No builds, tests or exhaustive review were
performed for this version. Existing init tests that assert refusal/overwrite behaviour
need adjustment to the new guided-update contract.
