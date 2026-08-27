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
$installSmokeDir = Join-Path $buildDir 'install-smoke-$literal'

Push-Location $repoRoot
try {
    Write-Host '[1/7] Checking Go formatting...'
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

    Write-Host '[2/7] Downloading dependencies...'
    go mod download
    Assert-LastExitCode 'Dependency download'

    Write-Host '[3/7] Verifying go.mod and go.sum...'
    $goModBefore = Get-Content -Raw -LiteralPath (Join-Path $repoRoot 'go.mod')
    $goSumBefore = Get-Content -Raw -LiteralPath (Join-Path $repoRoot 'go.sum')
    go mod tidy
    Assert-LastExitCode 'Module tidy'
    $goModAfter = Get-Content -Raw -LiteralPath (Join-Path $repoRoot 'go.mod')
    $goSumAfter = Get-Content -Raw -LiteralPath (Join-Path $repoRoot 'go.sum')
    if ($goModBefore -ne $goModAfter -or $goSumBefore -ne $goSumAfter) {
        throw 'go mod tidy changed go.mod or go.sum; review the changes and run the check again'
    }

    Write-Host '[4/7] Running tests...'
    go test ./... -count=1
    Assert-LastExitCode 'Tests'

    Write-Host '[5/7] Running static analysis...'
    go vet ./...
    Assert-LastExitCode 'Static analysis'

    Write-Host '[6/7] Building CLI...'
    New-Item -ItemType Directory -Force -Path $buildDir | Out-Null
    go build -trimpath -o $binaryPath .
    Assert-LastExitCode 'Build'

    Write-Host '[7/7] Running installer smoke test...'
    $resolvedBuildDir = [System.IO.Path]::GetFullPath($buildDir).TrimEnd('\', '/')
    $resolvedInstallSmokeDir = [System.IO.Path]::GetFullPath($installSmokeDir).TrimEnd('\', '/')
    $requiredPrefix = $resolvedBuildDir + [System.IO.Path]::DirectorySeparatorChar
    if (-not $resolvedInstallSmokeDir.StartsWith($requiredPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Installer smoke directory must stay inside the build directory: $resolvedInstallSmokeDir"
    }
    if (Test-Path -LiteralPath $resolvedInstallSmokeDir) {
        Remove-Item -LiteralPath $resolvedInstallSmokeDir -Recurse -Force
    }

    & (Join-Path $repoRoot 'scripts\install.ps1') `
        -BinaryPath $binaryPath `
        -InstallDir $resolvedInstallSmokeDir `
        -Force

    $installedBinaryPath = Join-Path $resolvedInstallSmokeDir 'ai-dev-logger.exe'
    $completionPath = Join-Path $resolvedInstallSmokeDir 'ai-dev-logger-completion.ps1'
    if (-not (Test-Path -LiteralPath $installedBinaryPath -PathType Leaf)) {
        throw "Installer did not create the binary: $installedBinaryPath"
    }
    if (-not (Test-Path -LiteralPath $completionPath -PathType Leaf)) {
        throw "Installer did not create the completion script: $completionPath"
    }
    $completionContent = Get-Content -Raw -LiteralPath $completionPath
    if (-not $completionContent.Contains('Register-ArgumentCompleter')) {
        throw 'Generated PowerShell completion script is invalid'
    }

    $overwriteWasRefused = $false
    try {
        & (Join-Path $repoRoot 'scripts\install.ps1') `
            -BinaryPath $binaryPath `
            -InstallDir $resolvedInstallSmokeDir
    }
    catch {
        if ($_.Exception.Message -notlike '*already installed*') {
            throw
        }
        $overwriteWasRefused = $true
    }
    if (-not $overwriteWasRefused) {
        throw 'Installer overwrote an existing installation without -Force'
    }

    & $installedBinaryPath --version | Out-Null
    Assert-LastExitCode 'Installed binary smoke test'

    $smokeProfilePath = Join-Path $resolvedInstallSmokeDir 'profile\Microsoft.PowerShell_profile.ps1'
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $smokeProfilePath) | Out-Null
    $profileEncoding = New-Object System.Text.UTF8Encoding($true)
    [System.IO.File]::WriteAllText(
        $smokeProfilePath,
        '# existing profile content' + [Environment]::NewLine,
        $profileEncoding
    )

    1..2 | ForEach-Object {
        & (Join-Path $repoRoot 'scripts\install.ps1') `
            -BinaryPath $binaryPath `
            -InstallDir $resolvedInstallSmokeDir `
            -Force `
            -AddCompletionToProfile `
            -ProfilePath $smokeProfilePath
    }

    $profileContent = Get-Content -Raw -LiteralPath $smokeProfilePath
    if (-not $profileContent.Contains('# existing profile content')) {
        throw 'Installer overwrote existing PowerShell profile content'
    }
    if (-not $profileContent.Contains($completionPath)) {
        throw 'Installer did not preserve the literal completion path in the PowerShell profile'
    }
    $profileBytes = [System.IO.File]::ReadAllBytes($smokeProfilePath)
    if ($profileBytes.Length -lt 3 -or $profileBytes[0] -ne 0xEF -or $profileBytes[1] -ne 0xBB -or $profileBytes[2] -ne 0xBF) {
        throw 'Installer did not preserve the existing PowerShell profile encoding'
    }
    $managedBlockCount = [regex]::Matches(
        $profileContent,
        [regex]::Escape('# >>> ai-dev-logger completion >>>')
    ).Count
    if ($managedBlockCount -ne 1) {
        throw "Expected one managed completion block, found $managedBlockCount"
    }

    Write-Host "All checks passed. Test binary: $binaryPath" -ForegroundColor Green
}
finally {
    Pop-Location
}
