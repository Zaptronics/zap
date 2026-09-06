# Release and Distribution

Zap releases are driven by Git tags. A tag such as `v0.6.0` runs the release
workflow and creates native downloads for Windows, macOS, Debian/Ubuntu, and
RPM-based Linux distributions.

## What a release produces

For a tag `vX.Y.Z`, GitHub Releases receives:

- `zap_windows_amd64.zip`
- `zap_windows_arm64.zip`
- `zap_darwin_universal.zip`
- `zap_linux_amd64.tar.gz`
- `zap_linux_arm64.tar.gz`
- `zap_X.Y.Z_linux_amd64.deb`
- `zap_X.Y.Z_linux_arm64.deb`
- `zap_X.Y.Z_linux_amd64.rpm`
- `zap_X.Y.Z_linux_arm64.rpm`
- `SHA256SUMS.txt`

The DEB and RPM packages install `zap` into `/usr/bin`, so no manual PATH
editing is required. The workflow runs the pinned nFPM `v2.47.0` container for
packaging, keeping nFPM's own Go/toolchain requirements separate from Zap's Go
1.22 build toolchain.

The macOS archive contains one universal binary supporting Intel and Apple
Silicon.

## User-facing install commands

### Windows

Once the first WinGet package has been accepted:

```powershell
winget install --id Zaptronics.Zap -e
```

WinGet treats the release ZIP as a portable package and creates the `zap`
command alias for the user.

### macOS

Once the Zaptronics tap exists and Homebrew publishing is enabled:

```bash
brew install --cask Zaptronics/tap/zap
```

The cask links `zap` into Homebrew's binary directory.

### Debian / Ubuntu

Download the matching `.deb` from GitHub Releases, then:

```bash
sudo apt install ./zap_X.Y.Z_linux_amd64.deb
```

Use the `arm64` package on ARM64 machines.

### Fedora / RHEL / Rocky / AlmaLinux

Download the matching `.rpm`, then:

```bash
sudo dnf install ./zap_X.Y.Z_linux_amd64.rpm
```

### Portable fallback

The ZIP/tar.gz assets remain the universal fallback for users who do not want
the package manager integration.

## Repository variables

The release pipeline intentionally leaves signing and package-manager
publication disabled until the corresponding accounts are configured.

Set these in **GitHub -> repository -> Settings -> Secrets and variables ->
Actions -> Variables**.

| Variable | Value | Purpose |
| --- | --- | --- |
| `ZAP_SIGN_WINDOWS` | `true` | Enable Windows Artifact Signing |
| `AZURE_ARTIFACT_SIGNING_ENDPOINT` | Azure endpoint URL | Artifact Signing region endpoint |
| `AZURE_ARTIFACT_SIGNING_ACCOUNT` | account name | Artifact Signing account |
| `AZURE_ARTIFACT_SIGNING_PROFILE` | profile name | Public Trust certificate profile |
| `ZAP_SIGN_MACOS` | `true` | Enable Developer ID signing + notarization |
| `APPLE_SIGNING_IDENTITY` | full Developer ID identity | `codesign` identity |
| `ZAP_PUBLISH_HOMEBREW` | `true` | Update `Zaptronics/homebrew-tap` after a release |
| `ZAP_PUBLISH_WINGET` | `true` | Submit WinGet updates after a release |
| `ZAP_WINGET_INITIALIZED` | `true` | Confirms the first WinGet package is already accepted |

Keep a variable absent or set to `false` until that part of the release system
is ready.

## Signing environment and secrets

The Windows and macOS release jobs reference the GitHub environment:

```text
release-signing
```

Create/configure it under **Settings -> Environments -> release-signing** before enabling signing. GitHub will also create the environment automatically if an unsigned release references it before you configure it. Keeping signing secrets in this environment is preferable to repository-wide signing secrets, especially if you later add required reviewers.

### Windows / Azure Artifact Signing

- `AZURE_CLIENT_ID`
- `AZURE_TENANT_ID`
- `AZURE_SUBSCRIPTION_ID`

The workflow is designed for GitHub OIDC, so there is deliberately no Azure
client secret.

### macOS

- `APPLE_DEVELOPER_ID_P12_BASE64`
- `APPLE_DEVELOPER_ID_P12_PASSWORD`
- `APPLE_ID`
- `APPLE_APP_SPECIFIC_PASSWORD`
- `APPLE_TEAM_ID`

### Package-publisher repository secrets

The Homebrew and WinGet publisher jobs do not use the signing environment. Store these under **Settings -> Secrets and variables -> Actions -> Secrets**.

#### Homebrew

- `HOMEBREW_TAP_TOKEN`

Use a fine-grained token with Contents read/write permission only on the
`Zaptronics/homebrew-tap` repository.

#### WinGet

- `WINGET_CREATE_GITHUB_TOKEN`

WingetCreate uses this token to fork/update `microsoft/winget-pkgs` and open the
package pull request. Keep the token in the environment rather than passing it
on the command line.

## One-time Homebrew setup

Create a public repository named:

```text
Zaptronics/homebrew-tap
```

It may initially be empty. The Zap release workflow creates/updates
`Casks/zap.rb` when `ZAP_PUBLISH_HOMEBREW=true` and macOS signing is enabled.

After that, the user-facing command is:

```bash
brew install --cask Zaptronics/tap/zap
```

## One-time WinGet setup

The first WinGet submission is intentionally manual because WingetCreate's
`new` flow asks the publisher to review package metadata before opening the
initial PR.

1. Create a signed Zap release first.
2. On Windows, install WingetCreate:

   ```powershell
   winget install wingetcreate
   ```

3. For release `vX.Y.Z`, run:

   ```powershell
   wingetcreate new `
     "https://github.com/Zaptronics/zap/releases/download/vX.Y.Z/zap_windows_amd64.zip" `
     "https://github.com/Zaptronics/zap/releases/download/vX.Y.Z/zap_windows_arm64.zip"
   ```

4. Use these values when prompted:

   - Package identifier: `Zaptronics.Zap`
   - Publisher: `Zaptronics Ltd`
   - Package name: `Zap`
   - Moniker: `zap`
   - License: `Apache-2.0`
   - Homepage: `https://github.com/Zaptronics/zap`
   - Description: `Embedded project and dependency tooling for C/CMake`
   - Installer type: ZIP containing a portable executable
   - Nested executable: `zap.exe`
   - Portable command alias: `zap`

5. Review the generated manifest carefully and submit its PR.
6. Once that PR is accepted into `microsoft/winget-pkgs`, set:

   ```text
   ZAP_WINGET_INITIALIZED=true
   ZAP_PUBLISH_WINGET=true
   ```

Future tagged releases are then submitted automatically with `wingetcreate
update`.

## Creating a release

Before tagging:

```powershell
git pull --rebase
go test ./...
git status
```

Then:

```powershell
git tag vX.Y.Z
git push origin vX.Y.Z
```

Watch **GitHub -> Actions -> Release**. The GitHub Release appears only after
all platform builds succeed.

If Homebrew/WinGet publication is enabled, those jobs run after the GitHub
Release exists.
