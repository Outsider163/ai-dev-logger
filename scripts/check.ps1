[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Assert-LastExitCode {
    param([string]$Step)

    if ($LASTEXITCODE -ne 0) {
        throw "$Step failed with exit code $LASTEXITCODE"
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$buildDir = Join-Path $repoRoot '.tmp\ci'
$binaryPath = Join-Path $buildDir 'ai-dev-logger.exe'

Push-Location $repoRoot
try {
    Write-Host '[1/6] Checking Go formatting...'
    $goFiles = @(git ls-files -- '*.go')
    Assert-LastExitCode 'List Go files'

    $unformatted = @()
    if ($goFiles.Count -gt 0) {
        $unformatted = @(gofmt -l @goFiles)
        Assert-LastExitCode 'Go formatting check'
    }
    if ($unformatted.Count -gt 0) {
        Write-Host 'The following Go files are not formatted:' -ForegroundColor Red
        $unformatted | ForEach-Object { Write-Host "  $_" -ForegroundColor Red }
        throw 'Run gofmt on the listed files before committing'
    }

    Write-Host '[2/6] Downloading dependencies...'
    go mod download
    Assert-LastExitCode 'Dependency download'

    Write-Host '[3/6] Verifying go.mod and go.sum...'
    $goModBefore = Get-Content -Raw -LiteralPath (Join-Path $repoRoot 'go.mod')
    $goSumBefore = Get-Content -Raw -LiteralPath (Join-Path $repoRoot 'go.sum')
    go mod tidy
    Assert-LastExitCode 'Module tidy'
    $goModAfter = Get-Content -Raw -LiteralPath (Join-Path $repoRoot 'go.mod')
    $goSumAfter = Get-Content -Raw -LiteralPath (Join-Path $repoRoot 'go.sum')
    if ($goModBefore -ne $goModAfter -or $goSumBefore -ne $goSumAfter) {
        throw 'go mod tidy changed go.mod or go.sum; review the changes and run the check again'
    }

    Write-Host '[4/6] Running tests...'
    go test ./... -count=1
    Assert-LastExitCode 'Tests'

    Write-Host '[5/6] Running static analysis...'
    go vet ./...
    Assert-LastExitCode 'Static analysis'

    Write-Host '[6/6] Building CLI...'
    New-Item -ItemType Directory -Force -Path $buildDir | Out-Null
    go build -trimpath -o $binaryPath .
    Assert-LastExitCode 'Build'

    Write-Host "All checks passed. Test binary: $binaryPath" -ForegroundColor Green
}
finally {
    Pop-Location
}
