[CmdletBinding()]
param([string]$OutputDirectory = 'test-results')
$ErrorActionPreference = 'Stop'
Push-Location (Split-Path -Parent $PSScriptRoot)
try {
    Get-Command go, git, cmake -ErrorAction Stop | Out-Null
    New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
    $evidence = Join-Path $OutputDirectory 'owasp.jsonl'
    & go test -tags 'claims,owasp' ./internal/zap -run '^TestOWASP' -count=1 -timeout=5m -json |
        Out-File -Encoding utf8 $evidence
    $result = $LASTEXITCODE
    Get-Content $evidence | ForEach-Object {
        $event = $_ | ConvertFrom-Json
        if ($event.Test -and $event.Action -in @('pass', 'fail', 'skip')) {
            Write-Host ('{0,-5} {1}' -f $event.Action.ToUpper(), $event.Test)
        }
    }
    Write-Host "Evidence: $evidence. Any failure is a security regression; skips are platform-specific and are not proof for that platform."
    exit $result
} finally {
    Pop-Location
}
