# Code Signing

Zap's release workflow can sign Windows and macOS binaries before they are
packaged. Signing is disabled until the relevant GitHub repository variables
are explicitly enabled.

## Windows: Azure Artifact Signing

The prepared workflow uses Microsoft's Azure Artifact Signing service (formerly
Trusted Signing) with Public Trust and GitHub OpenID Connect (OIDC).

This is preferable to putting a private certificate key into GitHub secrets:
the signing key remains in Microsoft's signing service and the GitHub runner
receives only permission to request a signature.

### 1. Create the Azure signing resources

In Azure:

1. Register the `Microsoft.CodeSigning` **Artifact Signing** resource provider.
2. Create an Artifact Signing account.
3. Complete **Organization / Public** identity validation for Zaptronics Ltd. Public Trust currently supports New Zealand organizations; allow time for identity validation, which Microsoft says can take 1-20 business days and sometimes longer if more documentation is needed.
4. Create a **Public Trust** certificate profile.
5. Note:
   - signing endpoint, for example `https://...codesigning.azure.net/`
   - signing account name
   - certificate profile name

Use the legal organization name exactly as validated by Microsoft.

### 2. Create the `release-signing` GitHub environment

The prepared Windows and macOS jobs already reference a GitHub environment named:

```text
release-signing
```

In the Zap repository, open **Settings -> Environments -> New environment** and create `release-signing`. You can optionally add required reviewers later. This environment also gives the Azure OIDC identity a stable subject across every `v*` release tag.

### 3. Create a GitHub OIDC identity in Azure

Create an Entra application/service principal for the Zap GitHub Actions workflow. Under **Certificates & secrets -> Federated credentials**, add a credential using **GitHub actions deploying Azure resources** with:

- Organization: `Zaptronics`
- Repository: `zap`
- Entity type: **Environment**
- Environment: `release-signing`

Use the portal-generated subject rather than hand-typing one. If Azure ever reports that no federated identity matches, compare the credential with the exact `subject claim` shown by the `azure/login` step; GitHub's newer repositories may use immutable owner/repository IDs in their OIDC subject.

Grant that identity the **Artifact Signing Certificate Profile Signer** role on the certificate profile/account it needs to use.

Record:

- Azure client ID
- Azure tenant ID
- Azure subscription ID

No client secret is required when GitHub OIDC is configured.

### 4. Add GitHub settings

`release-signing` environment secrets:

```text
AZURE_CLIENT_ID
AZURE_TENANT_ID
AZURE_SUBSCRIPTION_ID
```

Repository variables:

```text
AZURE_ARTIFACT_SIGNING_ENDPOINT
AZURE_ARTIFACT_SIGNING_ACCOUNT
AZURE_ARTIFACT_SIGNING_PROFILE
ZAP_SIGN_WINDOWS=true
```

On the next release, GitHub Actions will:

1. build `zap.exe`;
2. authenticate to Azure using OIDC;
3. Authenticode-sign `zap.exe` using SHA-256;
4. RFC-3161 timestamp the signature;
5. verify that Windows reports the signature as valid; and
6. put the signed executable into the WinGet/GitHub ZIP.

### Local verification

On Windows:

```powershell
Get-AuthenticodeSignature .\zap.exe | Format-List
```

or with the Windows SDK:

```powershell
signtool verify /pa /v .\zap.exe
```

The signature should be valid and show the Zaptronics identity used by the
certificate profile.

## Alternative Windows signing providers

If Artifact Signing identity validation is unsuitable, use a public code-signing
service/certificate from a commercial CA. Modern publicly trusted code-signing
keys are commonly held in hardware or a managed cloud signing service rather
than exported as a PFX file, so the CI integration depends on the provider.

The release pipeline's signing boundary is deliberately simple: sign
`stage\\zap.exe` before the `Create Windows archive` step. A DigiCert,
Sectigo, SSL.com, or other provider-specific action can replace the Azure
Artifact Signing step without changing the packaging or WinGet logic.

For local/test certificates only, SignTool's basic pattern is:

```powershell
signtool sign /f test.pfx /p PASSWORD /fd SHA256 `
  /tr https://TIMESTAMP-SERVER /td SHA256 zap.exe
```

Do not put a production private signing key or PFX password directly in the
repository.

## macOS: Developer ID + notarization

For distribution outside the Mac App Store, use a **Developer ID Application**
certificate and Apple notarization.

### 1. Join the Apple Developer Program

The signing identity must belong to the Apple Developer team that will publish
Zap.

### 2. Create a Developer ID Application certificate

On a trusted Mac:

1. Create a certificate-signing request in Keychain Access if required.
2. In the Apple Developer portal, create a **Developer ID Application**
   certificate.
3. Download/import the certificate into Keychain Access.
4. Confirm that **My Certificates** shows the certificate together with its
   private key.
5. Export that identity as a password-protected `.p12` file.

Find the exact signing identity with:

```bash
security find-identity -v -p codesigning
```

It will look similar to:

```text
Developer ID Application: Zaptronics Ltd (TEAMID)
```

Use the exact string returned on your Mac.

### 3. Encode the P12 for GitHub

On macOS:

```bash
base64 -i Zaptronics-Developer-ID.p12 | pbcopy
```

Create `release-signing` environment secrets:

```text
APPLE_DEVELOPER_ID_P12_BASE64
APPLE_DEVELOPER_ID_P12_PASSWORD
```

Create the repository variable:

```text
APPLE_SIGNING_IDENTITY=Developer ID Application: ...
```

### 4. Configure notarization credentials

Create an Apple ID app-specific password for the account used to notarize.
Store:

```text
APPLE_ID
APPLE_APP_SPECIFIC_PASSWORD
APPLE_TEAM_ID
```

as `release-signing` environment secrets.

Finally set:

```text
ZAP_SIGN_MACOS=true
```

### What the workflow does

For each tagged release it:

1. builds Intel and Apple Silicon binaries;
2. combines them into one universal Mach-O binary;
3. signs the binary with Developer ID and hardened runtime options;
4. verifies the local code signature;
5. places the binary into `zap_darwin_universal.zip`; and
6. submits that archive to Apple with `notarytool --wait`.

### Local verification

After extracting a release:

```bash
codesign --verify --strict --verbose=2 ./zap
codesign -dv --verbose=4 ./zap
spctl --assess --type execute --verbose=4 ./zap
```

## Linux packages

The release workflow creates `.deb` and `.rpm` packages with nFPM. They install
Zap to `/usr/bin/zap`.

At this stage, GitHub Releases also publishes `SHA256SUMS.txt`. That is enough
for direct-download integrity checking, but it is not the same as operating a
signed APT or RPM repository.

If Zaptronics later hosts an APT/DNF repository, create a dedicated offline or
HSM-backed repository-signing key and sign the repository metadata. That is the
right point to add Linux repository/package signing rather than putting a
long-lived GPG private key into the initial GitHub release job.

## Release checklist once signing is enabled

Before announcing a release:

- Windows `Get-AuthenticodeSignature` reports `Valid`.
- macOS `codesign` verification succeeds.
- Apple notarization succeeds in the Release workflow.
- `SHA256SUMS.txt` contains every release asset.
- Homebrew points to the same notarized macOS archive.
- WinGet points to the same signed Windows ZIPs.
