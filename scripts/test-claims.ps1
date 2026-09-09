# Release gate for ordinary regression, documented current claims, and hardened probes.
# Roadmap guarantees are intentionally excluded; run the TestRoadmapClaim tests separately.
[CmdletBinding()]
param([string]$OutputDirectory = 'test-results')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repoRoot
try {
    Get-Command go, git, cmake -ErrorAction Stop | Out-Null
    New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
    $failed = $false
    $suites = @(
        @{ Name = 'regression'; Args = @('test', './...', '-count=1', '-timeout=5m', '-json') },
        @{ Name = 'current-claims'; Args = @('test', '-tags', 'claims', './internal/zap', '-run', '^TestCurrentClaim', '-count=1', '-timeout=5m', '-json') },
        @{ Name = 'owasp'; Args = @('test', '-tags', 'claims,owasp', './internal/zap', '-run', '^TestOWASP', '-count=1', '-timeout=5m', '-json') }
    )
    foreach ($suite in $suites) {
        $resultPath = Join-Path $OutputDirectory "$($suite.Name).jsonl"
        $errorPath = Join-Path $OutputDirectory "$($suite.Name).stderr.txt"
        & go @($suite.Args) 2> $errorPath | Out-File -Encoding utf8 $resultPath
        $code = $LASTEXITCODE
        if ($code -ne 0) { $failed = $true }
        Write-Host "$($suite.Name) exit code: $code; evidence: $resultPath"
        Get-Content $resultPath | ForEach-Object {
            $event = $_ | ConvertFrom-Json
            if ($event.Test -and $event.Action -in @('pass', 'fail', 'skip')) {
                Write-Host ('{0,-5} {1}' -f $event.Action.ToUpper(), $event.Test)
            }
        }
    }
    Write-Host "Roadmap probes are separate: go test -tags claims ./internal/zap -run '^TestRoadmapClaim' -count=1 -timeout=5m"
    if ($failed) { exit 1 }
} finally {
    Pop-Location
}
