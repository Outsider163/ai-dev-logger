[CmdletBinding()]
param(
    [string]$BinaryPath,

    [string]$InstallDir,

    [switch]$Force,

    [switch]$AddToPath,

    [switch]$AddCompletionToProfile,

    [string]$ProfilePath
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Assert-LastExitCode {
    param([string]$Step)

    if ($LASTEXITCODE -ne 0) {
        throw "$Step failed with exit code $LASTEXITCODE"
    }
}

function Get-NormalizedPathEntry {
    param([string]$PathEntry)

    if ([string]::IsNullOrWhiteSpace($PathEntry)) {
        return ''
    }

    $trimmed = $PathEntry.Trim().TrimEnd('\', '/')
    $expanded = [Environment]::ExpandEnvironmentVariables($trimmed)
    try {
        return [System.IO.Path]::GetFullPath($expanded).TrimEnd('\', '/')
    }
    catch {
        return $trimmed
    }
}

function Add-DirectoryToUserPath {
    param([string]$Directory)

    $normalizedDirectory = Get-NormalizedPathEntry $Directory
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = @($userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    $alreadyPresent = $false

    foreach ($entry in $entries) {
        $normalizedEntry = Get-NormalizedPathEntry $entry
        if ([string]::Equals($normalizedEntry, $normalizedDirectory, [StringComparison]::OrdinalIgnoreCase)) {
            $alreadyPresent = $true
            break
        }
    }

    if (-not $alreadyPresent) {
        $newUserPath = if ([string]::IsNullOrWhiteSpace($userPath)) {
            $Directory
        }
        else {
            $userPath.TrimEnd(';') + ';' + $Directory
        }
        [Environment]::SetEnvironmentVariable('Path', $newUserPath, 'User')
    }

    $processEntries = @($env:Path -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    $processAlreadyContainsDirectory = $false
    foreach ($entry in $processEntries) {
        $normalizedEntry = Get-NormalizedPathEntry $entry
        if ([string]::Equals($normalizedEntry, $normalizedDirectory, [StringComparison]::OrdinalIgnoreCase)) {
            $processAlreadyContainsDirectory = $true
            break
        }
    }
    if (-not $processAlreadyContainsDirectory) {
        $env:Path = $env:Path.TrimEnd(';') + ';' + $Directory
    }

    return -not $alreadyPresent
}

function Read-TextFileWithEncoding {
    param([string]$Path)

    $bytes = [System.IO.File]::ReadAllBytes($Path)
    $offset = 0

    if ($bytes.Length -ge 4 -and $bytes[0] -eq 0x00 -and $bytes[1] -eq 0x00 -and $bytes[2] -eq 0xFE -and $bytes[3] -eq 0xFF) {
        $encoding = New-Object System.Text.UTF32Encoding($true, $true)
        $offset = 4
    }
    elseif ($bytes.Length -ge 4 -and $bytes[0] -eq 0xFF -and $bytes[1] -eq 0xFE -and $bytes[2] -eq 0x00 -and $bytes[3] -eq 0x00) {
        $encoding = New-Object System.Text.UTF32Encoding($false, $true)
        $offset = 4
    }
    elseif ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) {
        $encoding = New-Object System.Text.UTF8Encoding($true)
        $offset = 3
    }
    elseif ($bytes.Length -ge 2 -and $bytes[0] -eq 0xFF -and $bytes[1] -eq 0xFE) {
        $encoding = [System.Text.Encoding]::Unicode
        $offset = 2
    }
    elseif ($bytes.Length -ge 2 -and $bytes[0] -eq 0xFE -and $bytes[1] -eq 0xFF) {
        $encoding = [System.Text.Encoding]::BigEndianUnicode
        $offset = 2
    }
    elseif ($PSVersionTable.PSVersion.Major -lt 6) {
        $encoding = [System.Text.Encoding]::Default
    }
    else {
        $encoding = New-Object System.Text.UTF8Encoding($false)
    }

    return [PSCustomObject]@{
        Content  = $encoding.GetString($bytes, $offset, $bytes.Length - $offset)
        Encoding = $encoding
    }
}

function Set-CompletionProfileBlock {
    param(
        [string]$TargetProfilePath,
        [string]$CompletionScriptPath
    )

    $profileDirectory = Split-Path -Parent $TargetProfilePath
    if (-not [string]::IsNullOrWhiteSpace($profileDirectory)) {
        New-Item -ItemType Directory -Force -Path $profileDirectory | Out-Null
    }

    $startMarker = '# >>> ai-dev-logger completion >>>'
    $endMarker = '# <<< ai-dev-logger completion <<<'
    $escapedCompletionPath = $CompletionScriptPath.Replace("'", "''")
    $managedBlock = @(
        $startMarker
        ". '$escapedCompletionPath'"
        $endMarker
    ) -join [Environment]::NewLine

    if (Test-Path -LiteralPath $TargetProfilePath -PathType Leaf) {
        $profileFile = Read-TextFileWithEncoding $TargetProfilePath
        $existingContent = $profileFile.Content
        $profileEncoding = $profileFile.Encoding
    }
    else {
        $existingContent = ''
        $profileEncoding = New-Object System.Text.UTF8Encoding($true)
    }

    $pattern = '(?ms)^' + [regex]::Escape($startMarker) + '\r?\n.*?^' + [regex]::Escape($endMarker) + '\r?\n?'
    $existingBlock = [regex]::Match($existingContent, $pattern)
    if ($existingBlock.Success) {
        $updatedContent = $existingContent.Substring(0, $existingBlock.Index) +
            $managedBlock + [Environment]::NewLine +
            $existingContent.Substring($existingBlock.Index + $existingBlock.Length)
    }
    else {
        $separator = if ([string]::IsNullOrEmpty($existingContent) -or $existingContent.EndsWith("`n")) {
            ''
        }
        else {
            [Environment]::NewLine
        }
        $updatedContent = $existingContent + $separator + $managedBlock + [Environment]::NewLine
    }

    [System.IO.File]::WriteAllText($TargetProfilePath, $updatedContent, $profileEncoding)
}

if ([string]::IsNullOrWhiteSpace($BinaryPath)) {
    $BinaryPath = Join-Path $PSScriptRoot 'ai-dev-logger.exe'
}
if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $localAppData = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
    $InstallDir = Join-Path $localAppData 'Programs\ai-dev-logger'
}

if (-not (Test-Path -LiteralPath $BinaryPath -PathType Leaf)) {
    throw "Binary not found: $BinaryPath"
}

$sourcePath = (Resolve-Path -LiteralPath $BinaryPath).Path
$resolvedInstallDir = [System.IO.Path]::GetFullPath($InstallDir)
$targetPath = Join-Path $resolvedInstallDir 'ai-dev-logger.exe'
$aliasPath = Join-Path $resolvedInstallDir 'adl.exe'
$completionPath = Join-Path $resolvedInstallDir 'ai-dev-logger-completion.ps1'
$sameBinaryPath = [string]::Equals($sourcePath, $targetPath, [StringComparison]::OrdinalIgnoreCase)

if ((Test-Path -LiteralPath $targetPath) -and -not (Test-Path -LiteralPath $targetPath -PathType Leaf)) {
    throw "Install target is not a file: $targetPath"
}
if ((Test-Path -LiteralPath $targetPath -PathType Leaf) -and -not $Force -and -not $sameBinaryPath) {
    throw "ai-dev-logger is already installed at $targetPath; rerun with -Force to upgrade it"
}
if ((Test-Path -LiteralPath $aliasPath) -and -not (Test-Path -LiteralPath $aliasPath -PathType Leaf)) {
    throw "Short command target is not a file: $aliasPath"
}

New-Item -ItemType Directory -Force -Path $resolvedInstallDir | Out-Null

if (-not $sameBinaryPath) {
    $stagedPath = "$targetPath.installing"
    try {
        Copy-Item -LiteralPath $sourcePath -Destination $stagedPath -Force
        Move-Item -LiteralPath $stagedPath -Destination $targetPath -Force
    }
    finally {
        if (Test-Path -LiteralPath $stagedPath) {
            Remove-Item -LiteralPath $stagedPath -Force
        }
    }
}

$stagedAliasPath = "$aliasPath.installing"
try {
    Copy-Item -LiteralPath $targetPath -Destination $stagedAliasPath -Force
    Move-Item -LiteralPath $stagedAliasPath -Destination $aliasPath -Force
}
finally {
    if (Test-Path -LiteralPath $stagedAliasPath) {
        Remove-Item -LiteralPath $stagedAliasPath -Force
    }
}

$versionOutput = @(& $targetPath --version)
Assert-LastExitCode 'Installed binary verification'
if ($versionOutput.Count -eq 0) {
    throw 'Installed binary returned no version information'
}

$aliasVersionOutput = @(& $aliasPath --version)
Assert-LastExitCode 'Short command verification'
if ($aliasVersionOutput.Count -eq 0) {
    throw 'Short command returned no version information'
}

$completionLines = @(& $targetPath completion powershell)
Assert-LastExitCode 'PowerShell completion generation'
if ($completionLines.Count -eq 0) {
    throw 'PowerShell completion generation returned no content'
}
$completionText = ($completionLines -join [Environment]::NewLine) + [Environment]::NewLine
$completionText += "Register-ArgumentCompleter -CommandName 'adl' -ScriptBlock `${__ai_dev_loggerCompleterBlock}" + [Environment]::NewLine
$utf8WithoutBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($completionPath, $completionText, $utf8WithoutBom)

Write-Host "Installed binary: $targetPath"
Write-Host "Short command: $aliasPath"
Write-Host "Completion script: $completionPath"
Write-Host "Verified version: $($versionOutput[0])"

if ($AddToPath) {
    $pathChanged = Add-DirectoryToUserPath $resolvedInstallDir
    if ($pathChanged) {
        Write-Host 'Added the install directory to the current user PATH.'
    }
    else {
        Write-Host 'The install directory is already present in the current user PATH.'
    }
}
else {
    Write-Host 'PATH was not changed. Use -AddToPath when you want to update the current user PATH.'
}

if ($AddCompletionToProfile) {
    if ([string]::IsNullOrWhiteSpace($ProfilePath)) {
        $ProfilePath = $PROFILE.CurrentUserAllHosts
    }
    $resolvedProfilePath = [System.IO.Path]::GetFullPath($ProfilePath)
    Set-CompletionProfileBlock -TargetProfilePath $resolvedProfilePath -CompletionScriptPath $completionPath
    Write-Host "PowerShell completion was enabled in: $resolvedProfilePath"
}
else {
    Write-Host 'PowerShell profile was not changed. Use -AddCompletionToProfile to enable completion in new sessions.'
}
