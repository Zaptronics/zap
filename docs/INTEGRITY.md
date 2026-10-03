# Shared source integrity (0.10.0)

Zap stores authorised public signing identities in **.zap/signers.json**. Each entry
has a public key, its SHA-256 fingerprint, and optional user/device labels. Private
keys remain hardware-protected; their local references are never published.

**The checked-out signer list is the authority.** Every valid entry is authorised.
Git review and human approval govern additions, edits and removals. Zap does not
ask a teammate to separately trust each fingerprint, require existing signers to
endorse a new signer, or authenticate the user/device labels. Someone who can change
the list can authorise their own key. Protect the repository and review those changes.

## First setup and another developer

Run:

~~~text
zap audit
~~~

When no baseline exists, audit offers to create your hardware key, adds its public
entry, and asks whether to sign the source snapshot. Both prompts default to No.
Your OS requests authentication for key creation/signing. There is no software fallback.

Another developer can pull the signer list and signed baseline, fetch the declared
dependencies, and run:

~~~text
zap audit --non-interactive
~~~

Verification uses the listed public key and needs neither your private key nor a
local signing enrollment. Missing/changed sources, invalid signatures, missing
signer lists and unlisted baseline signers fail verification.

To add their own signing identity, that developer runs:

~~~text
zap audit --enroll
~~~

If signers.json is missing, enrollment creates it when publishing your hardware
identity. If hash.json already exists without its signer list (for example in a
partial project copy), ordinary audit explains how to restore the list from Git.
With --enroll, a default-No prompt instead offers fresh setup. The old baseline is
archived locally with an unverified- prefix; its contents are not treated as trusted.
Only accepting and successfully signing the new snapshot replaces hash.json. A
malformed or unreadable signer list is not silently replaced.

This creates a key only if this user/checkout has no local reference. An existing
reference is reused. It appends the public entry without replacing teammates' entries,
then offers to sign a new baseline. A successful unchanged ordinary audit does not
create a key. If changes need approval and no local key exists, interactive audit
also offers enrollment. Non-interactive audit never enrolls or signs.

A signer-list edit is itself a source change. Enrollment updates that file before the
new baseline is captured. Declining the final signature leaves the new public entry
and local key reference available for a later audit. Submit the list and baseline
through your normal Git review process; Zap does not commit or push them.

Each baseline has one signer fingerprint. Any currently listed signer can sign a
replacement. Multiple signatures/quorum approval are not implemented. Before removing
the signer of the current baseline, have another listed signer create a replacement
(for example with --enroll, which reuses their existing key); then remove the retired
entry and sign again. Removing a key does not delete its private hardware key.

## Shared files and local files

Track these selected files:

~~~text
.zap/.gitignore
.zap/signers.json
.zap/hash.json
~~~

The generated .zap/.gitignore excludes everything except itself, signers.json and
hash.json. An old generated file containing only * is upgraded automatically. Custom
ignore rules are not overwritten: retain the private-file exclusions and adopt this
allowlist:

~~~gitignore
*
!.gitignore
!signers.json
!hash.json
~~~

An enclosing project/global ignore rule may still ignore .zap entirely. Adjust that
rule, or explicitly stage only the intended files:

~~~text
git add -f .zap/.gitignore .zap/signers.json .zap/hash.json
~~~

Review the staged files before committing. Do not force-add the entire .zap folder.

- signers.json uses schema 1 and a signers array. Each entry contains fingerprint,
  public_key, user, host and created. User/host are descriptive labels, not credentials.
  Use audit --enroll to create a correctly encoded public-key entry.
  New enrollment defaults to hashed account/host identifiers rather than readable
  user/host labels. See the identity privacy choice below. Existing entries are not
  automatically rewritten; guided init can update your own entry after approval. Editing labels changes the signed inventory and requires
  a new baseline; removing names later does not remove them from Git history.
- hash.json holds a signed manifest and the signing fingerprint. It contains file
  paths/hashes and coverage, not source contents or a private key.
- .zap/integrity/baselines retains earlier signed envelopes locally.
- .zap/integrity/objects retains exact, unredacted source contents locally.
- .zap/history retains unsigned diagnostic records, audit reports and tool output.

Local signing references remain in the OS user configuration directory at
zap/integrity-trust/<checkout-path-hash>.json. This is a per-user, per-checkout record;
a new folder does not automatically reuse another checkout's key. Windows normally
uses %APPDATA%, Linux uses XDG_CONFIG_HOME or ~/.config, and macOS uses
~/Library/Application Support. Linux's protected TPM blobs live under zap/hardware-keys.

The local latest_baseline_sha256 field records the last local signature. **It is no
longer an independent anti-replay trust gate.** A pulled baseline may be signed by
a teammate, and a previously valid baseline can be restored through Git. Git/human
review is responsible for whether that rollback is acceptable.

## Migrate an existing single-signer project

On the original developer's machine, with the existing key reference available:

~~~text
zap audit --migrate-signers
~~~

After confirmation, migration verifies and archives an existing legacy baseline,
adds the legacy public identity to signers.json, and removes only the top-level
integrity_public_key line from zap.yml. It does not create a replacement private
key or discard prior snapshots. The old envelope is retained with its original
signature; audit then offers to sign the updated source scope with the existing key.

Complete that signature before sharing: the new manifest format is portable and
covers signers.json. Commit the zap.yml edit together with the shared .zap files.
If signing is declined, the file migration remains and ordinary zap audit can resume.

The old zap.yml field remains parseable solely for migration. Normal audit asks for
--migrate-signers rather than silently switching authority. Read-only verification on
a teammate's machine does not require copying another person's local key reference.
Legacy 0.8.0 SSH keys/signatures are not converted into hardware identities.

This release does not add private-key deletion, automatic key reuse across copied
folders, a baseline-reset command, or key rotation/recovery automation. Deleting .zap
does not delete hardware keys. Do not clear the whole TPM to remove a Zap key.

## Coverage and portability

The signed schema-2 manifest uses logical source-root IDs and relative paths/exclusions,
not the developer's absolute checkout location. Physical source locations are resolved
locally from the project's dependency configuration and lock. The project, locked
local/Git/URL dependency directories and available cached CMake source overrides are
covered. Missing roots, symlinks, redirected roots and special files fail the scan.

Exact file bytes, SHA-256 hashes and executable bits are checked. Git-ignore and
assume-unchanged flags do not hide source changes. The project's .zap/signers.json
is explicitly included; other .zap content, .git entries and the configured build
directory are excluded. Dependencies nested in build directories are scanned separately.
Moves appear as deletion plus addition.

Different checkout folders can verify the same baseline when their logical coverage,
file bytes and executable bits match. CRLF/LF conversion, platform-specific executable
bits, generated files and differing dependency content can legitimately produce changes.
This is not a compiler-derived input inventory: external SDKs, toolchains, system
headers, later source overrides and downloads outside declared roots are not covered.

Shared hashes do not provide old file contents. In a fresh clone, a requested diff
reports unavailable old snapshots and their hashes instead of inventing old contents.
Use Git to retrieve historical code; local-only dependency changes may not exist in Git.

## Identity privacy choice during init

Interactive zap init asks what future signer entries should publish:

- **hashed** (default): omit readable user/host; include user_sha256, host_sha256 and
  identity_hash set to sha256-zap-identity-v1.
- **public**: include readable user and host labels.

For scripted setup, use zap init --signer-identity hashed or
zap init --signer-identity public alongside your other init options. Init records the
choice; it does not create a hardware key. The preference is stored outside the repo
beside the local checkout key record as <checkout-path-hash>.json.preferences.json.
It applies only to new entries enrolled by that local user in that checkout. If no
preference exists (including a fresh clone), enrollment defaults to hashed.
Rerun zap init to change this preference on an existing project. An optional public
signer display label can be set there or with --signer-label. The wizard offers to
update only your existing enrolled signer's labels, with separate default-No approval
and a final save summary. Such registry changes require a subsequent audit/signature.
See SETUP.md for details; --force is no longer required.

Identifiers are computed from the OS account's Username and the hostname at enrollment:
user_sha256 is SHA-256 of UTF-8 "zap-signer-user-v1", a NUL byte, and the exact username;
host_sha256 uses "zap-signer-host-v1", a NUL byte, and the exact hostname. The digest is
lowercase hexadecimal. No case folding is applied. Separate prefixes distinguish
account and host identifiers. Equal input labels produce equal identifiers, including
across projects, so they can be correlated.

These are **pseudonyms, not anonymous identities**. Common usernames/hostnames can be
guessed and hashed; no encryption or secret salt is used. Names and device names can
also be shared, renamed or impersonated. Enrollment records labels on that occasion;
it does not establish who authored every source file or attest the current OS host.

The signed baseline covers signers.json, binding the recorded identifiers to that
signed snapshot. Cryptographic verification establishes use of an authorised private
key, not proof of a person's real-world identity or a hostname's authenticity. The
reviewed key fingerprint remains the primary signing identity. Git/human review still
governs additions or edits to the list.

This option affects the shared signer registry only. Private operation history keeps
its existing username/hostname recording behaviour and remains ignored by Git.

## Platform signing requirements

| Platform | Key storage | Approval and prerequisites |
| --- | --- | --- |
| Windows | Microsoft Platform Crypto Provider / TPM | Native CNG dialogs; non-exportable key and high-protection UI policies are required. |
| macOS | Secure Enclave | Private-key usage and user-presence controls; supported hardware and Apple Swift command-line tools are required. |
| Linux | TPM 2.0 device | /dev/tpmrm0, tpm2-tools and systemd-ask-password; a prompted TPM-key passphrase and permitted owner-hierarchy provisioning are required. |

Unsupported hardware, policies or cancelled authentication fail without software
fallback. Windows dialog appearance is provider-dependent, not a guarantee of Windows
Hello PIN integration. Linux TPM blobs are wrapped private material tied to the TPM,
not plaintext keys; the passphrase is not stored or passed as a command-line argument.
Public signatures do not prove where a key was generated; accepting a manually edited
public-key entry does not attest hardware origin.

## Behaviour, reports and limits

Normal sync, make/build, upload and other mutating commands still warn and continue
when integrity is unverified. Existing dependency verification failures still fail.
A warning can also mean no baseline/list exists. Project-scoped zap e checks before
and after the child without changing the child's exit code.

Audit rescans after review and signing. An exclusive .zap/integrity/audit.lock prevents
concurrent Zap audits in the same checkout; source scans detect changes during the
review/signing interval, and a changed shared baseline aborts publication. Git/editor
writes are not locked. Check for a running audit before removing a stale lock left
by an interruption. No source directory is made read-only.

Before/after checks cannot detect every transient edit restored between scans.
This is not an atomic snapshot, build isolation, malware detection, or proof that
approved code is safe. A compromised verifier or signer list remains outside the
guarantee. Snapshots may contain secrets; they are not automatically uploaded, pruned,
or rolled back.

Each audit stores its full observed inventory, changes and external-source references
in .zap/history/operations/<id>/audit-report.json. Reports are observations, not signed
approvals, and remain after declined changes. Console summaries show file counts and
up to eight changed paths per root. Requested diffs are paginated at five files per
page when more than five files changed. Each page shows the remaining count; Enter
or Yes advances, and No stops viewing. The final signing prompt defaults to No,
including when viewing stopped early. Signing approves the entire snapshot, not
just displayed pages. No diffs are removed from the underlying changes/report.
Contextual diffs show red removals, green additions,
CR/tab markers, trailing-space dots and missing final newlines. Large rewrites may
produce larger replacement hunks without dropping changed content.

Terminal-only lightning animation accompanies scanning/signing. Logs retain phase
messages without animation frames. NO_COLOR, ZAP_COLOR=never and redirected output
use static progress. Report links use escaped file URLs so spaces do not split links;
opening a link depends on terminal support.

## Implementation status

The 0.9.0 changes were made as source/documentation edits only at the user's request.
No builds, tests, native signing runs or exhaustive review were performed for this
change. Existing tests that assert the former single-signer/local-checkpoint policy
still need updating for this deliberately changed trust model.
