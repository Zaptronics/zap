# Project history and command recording (0.7.0)

## Quick start

From your project directory:

```powershell
zap e git status
zap e cmake --build build
zap log
zap log -m "USB enumeration tested on the board"
zap log show <operation-id> --output
zap log diff <operation-id>
zap log diff <older-id> <newer-id>
zap log mark-good <successful-build-id> -m "Tested radio and USB on hardware"
```

Use the complete operation ID printed by `zap log`. `zap exec` is an alias for
`zap e`. An optional `--` can precede the executable. All following arguments
belong to the executable, not Zap. The working directory is preserved.

Shell builtins and pipelines need an explicit shell:

```powershell
zap e pwsh -NoProfile -Command 'Remove-Item old.bin'
zap e pwsh -NoProfile -Command 'Get-ChildItem | Sort-Object Name'
```

```sh
zap e bash -c 'mkdir build && chmod 755 build'
```

A pipeline outside the quoted command is handled by your calling shell and is not
fully recorded. Stdin is inherited but is never recorded. Child stdout and stderr
stream live and are saved separately. Ordinary child exit codes are preserved;
failure to launch or termination without a normal exit code reports failure.
The wrapper adds no success text to command stdout.

## What is recorded

Normal Zap operations (including sync, make, update and upload) record:

- UTC start/end timestamps, operation ID, local account name, hostname, working
  directory, Zap version, command arguments, outcome and duration;
- exact before/after copies of zap.yml and zap.lock, including when the operation
  fails, with SHA-256 digests in the event record;
- per-dependency before/after lock entries, including versions and commit IDs;
- internal tool commands and results; captured tool output is stored in events,
  while streamed output is in stdout.log and stderr.log;
- changes since the previous completed record, labelled as external or overlapping
  changes with unknown attribution.

Make additionally records manifest/lock snapshots immediately before configuration,
build options, available Git/CMake version output, and project Git HEAD/status when
available. Configure-only is not a successful build. A successful build is not
automatically known good: mark it explicitly after testing. The annotation references
the build without rewriting its record.

Help/version queries and history viewing are not themselves recorded. Notes and
known-good annotations are recorded. Commands run outside Zap cannot be observed
directly. Nested Zap invocations create their own operation records.

## Storage and retention

History uses JSON Lines, with one journal per operation to avoid interleaving
concurrent writers:

```text
.zap/
  .gitignore
  history/operations/<operation-id>/
    events.jsonl
    stdout.log
    stderr.log
    before/zap.yml
    before/zap.lock
    after/zap.yml
    after/zap.lock
    build-input/...
```

Absent manifests are recorded as absent; they are not invented. History prefers an
ancestor containing zap.yml, otherwise a Git repository root, otherwise the current
directory. Snapshot paths are relative to that selected root.

The generated .zap/.gitignore excludes private history and snapshots, while allowing .gitignore, signers.json and hash.json to be shared through Git. See INTEGRITY.md for the signer-list trust model and migration.
It does not untrack files already committed or stop an explicit git add -f.
Review git status before publishing. Existing incompatible history ignore files
cause recording to refuse startup rather than overwriting user content.

There is no automatic truncation, pruning, uploading, or size cap. Per-operation
files naturally separate recordings; disk use grows until you manage it.
A git push or release tag does not archive this ignored history.

If recording cannot initialise, the command is not launched. A write failure during
execution emits a warning, allows the running command to finish, and reports the
record incomplete where storage still permits it. A successful command with failed
recording produces a nonzero Zap exit status. Abrupt termination can leave a
started record without completion; zap log labels it RUNNING/INTERRUPTED.
Redaction buffers an unfinished line until a line boundary or command completion,
so an abrupt crash may lose the trailing partial line. Extremely long lines also
consume memory. This is not a crash-proof recorder.

## Privacy and limits

This is editable local diagnostic history, not authenticated security evidence.
Usernames and timestamps can be influenced by the local environment.

Known URL userinfo and common password/token/secret argument forms are redacted
from recorded text. Redaction is best effort: arbitrary programs can print credentials
in forms Zap does not recognise. Live terminal output is not redacted. Exact manifest
snapshots are deliberately unredacted so they remain useful for future restoration;
they can contain secrets. Files are created with private Unix modes; Windows access
depends on the account's filesystem ACLs. Review any archive before sharing it.

Output recording uses pipes, not a pseudo-terminal. Line-based prompts can read
stdin, but full-screen editors, pagers, terminal detection and some interactive
programs may behave differently. Tool output ordering across stdout and stderr is
not a single exact byte-order transcript. GUI side effects are not captured.

zap e inherits the user environment. In particular, zap e git is an ordinary Git
passthrough; it is not Zap's internal dependency-management Git wrapper, which keeps
its repository-selection protections.

Recording a delete does not back up the deleted files. Manifest snapshots are not
source-tree snapshots, artifact backups, or proof of reproducibility. Compiler/SDK
state is not comprehensively captured or restored.

## Not implemented in this increment

Automatic rollback, last-known-good restoration, Markdown exports, remote archives,
local pruning and PTY support are not implemented. zap audit keeps its existing
external-source scanning meaning. The stored snapshots and known-good annotations
are groundwork for a separately tested rollback feature.

## Validation

```powershell
go test ./internal/zap -run '^TestHistory' -count=1 -timeout=3m
```

Tests use disposable projects and cover child exit codes, separate streams, output
over 64 KiB, split-write redaction, failed-operation snapshots, notes, invalid IDs,
recording startup/write failures, and successful-build annotations.
