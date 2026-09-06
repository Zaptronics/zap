param(
    [Parameter(Mandatory = $true)]
    [string]$Version
)

$ErrorActionPreference = 'Stop'
$version = $Version.TrimStart('v')
$tag = "v$version"
$base = "https://github.com/Zaptronics/zap/releases/download/$tag"

if (-not (Get-Command wingetcreate -ErrorAction SilentlyContinue)) {
    throw "wingetcreate is not installed. Run: winget install wingetcreate"
}

Write-Host "Starting the one-time interactive WinGet manifest creation for Zap $version"
Write-Host "Use PackageIdentifier Zaptronics.Zap, portable command alias zap, and review all metadata before submitting."

wingetcreate new `
    "$base/zap_windows_amd64.zip" `
    "$base/zap_windows_arm64.zip"
