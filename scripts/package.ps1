[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?(\+[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$')]
    [string]$Version,

    [switch]$AllowDirty
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Assert-LastExitCode {
    param([string]$Step)

    if ($LASTEXITCODE -ne 0) {
        throw "$Step failed with exit code $LASTEXITCODE"
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$distDir = Join-Path $repoRoot 'dist'
$packageDir = Join-Path $distDir 'package'
$binaryPath = Join-Path $packageDir 'ai-dev-logger.exe'
$readmePath = Join-Path $packageDir 'README.md'
$installerPath = Join-Path $packageDir 'install.ps1'
$onlineInstallerPath = Join-Path $distDir 'install-online.ps1'
$archiveName = "ai-dev-logger_${Version}_windows_amd64.zip"
$archivePath = Join-Path $distDir $archiveName
$checksumPath = Join-Path $distDir 'checksums.txt'
$previousGoos = [Environment]::GetEnvironmentVariable('GOOS', 'Process')
$previousGoarch = [Environment]::GetEnvironmentVariable('GOARCH', 'Process')

Push-Location $repoRoot
try {
    $changes = @(git status --porcelain)
    Assert-LastExitCode 'Read Git status'
    $isDirty = $changes.Count -gt 0
    if ($isDirty -and -not $AllowDirty) {
        throw 'Working tree is not clean; commit or stash changes before packaging, or use -AllowDirty for a local test build'
    }

    $commit = (git rev-parse HEAD).Trim()
    Assert-LastExitCode 'Read Git commit'
    if ($isDirty) {
        $commit += '-dirty'
    }
    $builtAt = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
    $linkerFlags = "-s -w -X ai-dev-logger/internal/buildinfo.Version=$Version -X ai-dev-logger/internal/buildinfo.Commit=$commit -X ai-dev-logger/internal/buildinfo.BuiltAt=$builtAt"

    Write-Host "Building ai-dev-logger $Version..."
    New-Item -ItemType Directory -Force -Path $packageDir | Out-Null
    [Environment]::SetEnvironmentVariable('GOOS', 'windows', 'Process')
    [Environment]::SetEnvironmentVariable('GOARCH', 'amd64', 'Process')
    go build -trimpath -ldflags $linkerFlags -o $binaryPath .
    Assert-LastExitCode 'Release build'

    $versionOutput = @(& $binaryPath version)
    Assert-LastExitCode 'Version verification'
    if ($versionOutput -notcontains "version: $Version") {
        throw "Built binary does not report version $Version"
    }

    Copy-Item -LiteralPath (Join-Path $repoRoot 'README.md') -Destination $readmePath -Force
    Copy-Item -LiteralPath (Join-Path $repoRoot 'scripts\install.ps1') -Destination $installerPath -Force
    Copy-Item -LiteralPath (Join-Path $repoRoot 'scripts\install-online.ps1') -Destination $onlineInstallerPath -Force
    Compress-Archive -LiteralPath $binaryPath, $readmePath, $installerPath -DestinationPath $archivePath -Force

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [System.IO.Compression.ZipFile]::OpenRead($archivePath)
    try {
        $archiveEntries = @($archive.Entries | ForEach-Object { $_.FullName })
        foreach ($requiredEntry in @('ai-dev-logger.exe', 'README.md', 'install.ps1')) {
            if ($archiveEntries -notcontains $requiredEntry) {
                throw "Release archive is missing required file: $requiredEntry"
            }
        }
    }
    finally {
        $archive.Dispose()
    }

    $archiveHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    $onlineInstallerHash = (Get-FileHash -LiteralPath $onlineInstallerPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $checksumLines = @(
        "$archiveHash  $archiveName"
        "$onlineInstallerHash  install-online.ps1"
    )
    [System.IO.File]::WriteAllText(
        $checksumPath,
        ($checksumLines -join [Environment]::NewLine) + [Environment]::NewLine,
        [System.Text.Encoding]::ASCII
    )

    Write-Host "Created: $archivePath" -ForegroundColor Green
    Write-Host "Created: $onlineInstallerPath" -ForegroundColor Green
    $checksumLines | ForEach-Object { Write-Host "Checksum: $_" -ForegroundColor Green }
    Write-Host 'Embedded version information:'
    $versionOutput | ForEach-Object { Write-Host "  $_" }
}
finally {
    [Environment]::SetEnvironmentVariable('GOOS', $previousGoos, 'Process')
    [Environment]::SetEnvironmentVariable('GOARCH', $previousGoarch, 'Process')
    Pop-Location
}
