[CmdletBinding()]
param(
    [string]$InstallDir,

    [switch]$NoPath,

    [switch]$AddCompletionToProfile
)

function Assert-AIDevLoggerGitHubUri {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Value,

        [Parameter(Mandatory = $true)]
        [string]$Description
    )

    try {
        $uri = [Uri]$Value
    }
    catch {
        throw "$Description is not a valid URL"
    }

    $allowedHosts = @(
        'github.com'
        'objects.githubusercontent.com'
        'release-assets.githubusercontent.com'
    )
    if ($uri.Scheme -ne 'https' -or $allowedHosts -notcontains $uri.Host.ToLowerInvariant()) {
        throw "$Description must use HTTPS and point to GitHub"
    }

    return $uri.AbsoluteUri
}

function Get-AIDevLoggerExpectedChecksum {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ChecksumPath,

        [Parameter(Mandatory = $true)]
        [string]$AssetName
    )

    $foundHashes = @()
    foreach ($line in Get-Content -LiteralPath $ChecksumPath) {
        if ($line -notmatch '^(?<hash>[0-9A-Fa-f]{64})\s+\*?(?<name>.+?)\s*$') {
            continue
        }
        if ([string]::Equals($Matches['name'], $AssetName, [StringComparison]::Ordinal)) {
            $foundHashes += $Matches['hash'].ToLowerInvariant()
        }
    }

    if ($foundHashes.Count -ne 1) {
        throw "Expected exactly one SHA-256 entry for $AssetName, found $($foundHashes.Count)"
    }

    return $foundHashes[0]
}

function Get-AIDevLoggerReleaseInfo {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ChecksumPath
    )

    $releasePattern = '^ai-dev-logger_(?<version>v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?(\+[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?)_windows_amd64\.zip$'
    $candidates = @()

    foreach ($line in Get-Content -LiteralPath $ChecksumPath) {
        $checksumMatch = [regex]::Match($line, '^(?<hash>[0-9A-Fa-f]{64})\s+\*?(?<name>.+?)\s*$')
        if (-not $checksumMatch.Success) {
            continue
        }

        $assetName = $checksumMatch.Groups['name'].Value
        $releaseMatch = [regex]::Match($assetName, $releasePattern)
        if (-not $releaseMatch.Success) {
            continue
        }

        $candidates += [PSCustomObject]@{
            Version     = $releaseMatch.Groups['version'].Value
            ArchiveName = $assetName
            Hash        = $checksumMatch.Groups['hash'].Value.ToLowerInvariant()
        }
    }

    if ($candidates.Count -ne 1) {
        throw "Expected exactly one Windows amd64 release archive in checksums.txt, found $($candidates.Count)"
    }

    return $candidates[0]
}

function Assert-AIDevLoggerArchive {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ArchivePath,

        [Parameter(Mandatory = $true)]
        [string]$ExtractionPath
    )

    Add-Type -AssemblyName System.IO.Compression.FileSystem

    $resolvedRoot = [System.IO.Path]::GetFullPath($ExtractionPath).TrimEnd('\', '/')
    $requiredPrefix = $resolvedRoot + [System.IO.Path]::DirectorySeparatorChar
    $entryNames = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
    $requiredEntries = @('ai-dev-logger.exe', 'README.md', 'install.ps1')
    $archive = [System.IO.Compression.ZipFile]::OpenRead($ArchivePath)

    try {
        foreach ($entry in $archive.Entries) {
            $normalizedName = $entry.FullName.Replace('\', '/')
            if ([string]::IsNullOrWhiteSpace($normalizedName)) {
                throw 'Release archive contains an empty entry name'
            }
            if ($normalizedName.StartsWith('/') -or $normalizedName -match '^[A-Za-z]:') {
                throw "Release archive contains an absolute path: $normalizedName"
            }
            if (-not $entryNames.Add($normalizedName)) {
                throw "Release archive contains a duplicate path: $normalizedName"
            }

            $destination = [System.IO.Path]::GetFullPath(
                (Join-Path $resolvedRoot $normalizedName.Replace('/', [System.IO.Path]::DirectorySeparatorChar))
            )
            if (-not $destination.StartsWith($requiredPrefix, [StringComparison]::OrdinalIgnoreCase)) {
                throw "Release archive entry escapes the extraction directory: $normalizedName"
            }

            $unixFileType = ($entry.ExternalAttributes -shr 16) -band 0xF000
            if ($unixFileType -eq 0xA000) {
                throw "Release archive contains a symbolic link: $normalizedName"
            }
        }

        foreach ($requiredEntry in $requiredEntries) {
            if (-not $entryNames.Contains($requiredEntry)) {
                throw "Release archive is missing required file: $requiredEntry"
            }
        }
    }
    finally {
        $archive.Dispose()
    }
}

function New-AIDevLoggerTempDirectory {
    $tempRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()).TrimEnd('\', '/')
    $candidate = Join-Path $tempRoot ('ai-dev-logger-' + [Guid]::NewGuid().ToString('N'))
    $resolvedCandidate = [System.IO.Path]::GetFullPath($candidate).TrimEnd('\', '/')
    $requiredPrefix = $tempRoot + [System.IO.Path]::DirectorySeparatorChar

    if (-not $resolvedCandidate.StartsWith($requiredPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Temporary install directory escaped the system temporary directory'
    }

    New-Item -ItemType Directory -Path $resolvedCandidate | Out-Null
    return $resolvedCandidate
}

function Remove-AIDevLoggerTempDirectory {
    param([string]$Path)

    if ([string]::IsNullOrWhiteSpace($Path)) {
        return
    }

    $tempRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()).TrimEnd('\', '/')
    $resolvedPath = [System.IO.Path]::GetFullPath($Path).TrimEnd('\', '/')
    $requiredPrefix = $tempRoot + [System.IO.Path]::DirectorySeparatorChar
    $leafName = Split-Path -Leaf $resolvedPath

    if (-not $resolvedPath.StartsWith($requiredPrefix, [StringComparison]::OrdinalIgnoreCase) -or
        -not $leafName.StartsWith('ai-dev-logger-', [StringComparison]::Ordinal)) {
        throw "Refusing to remove an unexpected directory: $resolvedPath"
    }

    if (Test-Path -LiteralPath $resolvedPath) {
        Remove-Item -LiteralPath $resolvedPath -Recurse -Force
    }
}

function Invoke-AIDevLoggerOnlineInstall {
    [CmdletBinding()]
    param(
        [string]$TargetInstallDir,

        [switch]$SkipPath,

        [switch]$EnableCompletion
    )

    $ErrorActionPreference = 'Stop'
    Set-StrictMode -Version Latest

    if ($PSVersionTable.PSVersion -lt [Version]'5.1') {
        throw 'ai-dev-logger requires Windows PowerShell 5.1 or newer'
    }
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
        throw 'The current ai-dev-logger release supports Windows only'
    }

    $latestDownloadBase = 'https://github.com/Outsider163/ai-dev-logger/releases/latest/download'
    $headers = @{
        'User-Agent' = 'ai-dev-logger-online-installer'
    }
    $previousProgressPreference = $ProgressPreference
    $previousSecurityProtocol = [Net.ServicePointManager]::SecurityProtocol
    $tempDirectory = $null

    try {
        $ProgressPreference = 'SilentlyContinue'
        [Net.ServicePointManager]::SecurityProtocol = $previousSecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

        $tempDirectory = New-AIDevLoggerTempDirectory
        $checksumPath = Join-Path $tempDirectory 'checksums.txt'
        $checksumUrl = Assert-AIDevLoggerGitHubUri `
            -Value "$latestDownloadBase/checksums.txt" `
            -Description 'Checksum URL'

        Write-Host 'Finding the latest ai-dev-logger release...'
        Invoke-WebRequest -Uri $checksumUrl -Headers $headers -OutFile $checksumPath -UseBasicParsing
        $releaseInfo = Get-AIDevLoggerReleaseInfo -ChecksumPath $checksumPath
        $version = $releaseInfo.Version
        $archiveName = $releaseInfo.ArchiveName
        $archiveUrl = Assert-AIDevLoggerGitHubUri `
            -Value "$latestDownloadBase/$archiveName" `
            -Description 'Release archive URL'
        $archivePath = Join-Path $tempDirectory $archiveName
        $extractionPath = Join-Path $tempDirectory 'package'

        Write-Host "Downloading ai-dev-logger $version..."
        Invoke-WebRequest -Uri $archiveUrl -Headers $headers -OutFile $archivePath -UseBasicParsing

        $expectedHash = $releaseInfo.Hash
        $actualHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
        if (-not [string]::Equals($actualHash, $expectedHash, [StringComparison]::Ordinal)) {
            throw "SHA-256 verification failed for $archiveName"
        }
        Write-Host "Verified SHA-256: $actualHash"

        New-Item -ItemType Directory -Path $extractionPath | Out-Null
        Assert-AIDevLoggerArchive -ArchivePath $archivePath -ExtractionPath $extractionPath
        Expand-Archive -LiteralPath $archivePath -DestinationPath $extractionPath

        $binaryPath = Join-Path $extractionPath 'ai-dev-logger.exe'
        $installerPath = Join-Path $extractionPath 'install.ps1'
        if (-not (Test-Path -LiteralPath $binaryPath -PathType Leaf) -or
            -not (Test-Path -LiteralPath $installerPath -PathType Leaf)) {
            throw 'Verified release archive did not extract the required installer files'
        }

        Get-ChildItem -LiteralPath $extractionPath -File | Unblock-File -ErrorAction SilentlyContinue

        $installerParameters = @{
            BinaryPath = $binaryPath
            Force      = $true
        }
        if (-not [string]::IsNullOrWhiteSpace($TargetInstallDir)) {
            $installerParameters.InstallDir = $TargetInstallDir
        }
        if (-not $SkipPath) {
            $installerParameters.AddToPath = $true
        }
        if ($EnableCompletion) {
            $installerParameters.AddCompletionToProfile = $true
        }

        $installerSource = [System.IO.File]::ReadAllText($installerPath)
        $installerBlock = [ScriptBlock]::Create($installerSource)
        & $installerBlock @installerParameters

        Write-Host "ai-dev-logger $version is ready." -ForegroundColor Green
        if ($SkipPath) {
            Write-Host 'Run it from the install directory, or add that directory to PATH later.'
        }
        else {
            Write-Host 'Run now: adl'
        }
    }
    finally {
        try {
            Remove-AIDevLoggerTempDirectory -Path $tempDirectory
        }
        finally {
            $ProgressPreference = $previousProgressPreference
            [Net.ServicePointManager]::SecurityProtocol = $previousSecurityProtocol
        }
    }
}

if ($MyInvocation.InvocationName -ne '.') {
    Invoke-AIDevLoggerOnlineInstall `
        -TargetInstallDir $InstallDir `
        -SkipPath:$NoPath `
        -EnableCompletion:$AddCompletionToProfile
}
