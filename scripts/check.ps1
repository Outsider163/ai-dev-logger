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
$onlineInstallerPath = Join-Path $repoRoot 'scripts\install-online.ps1'

Push-Location $repoRoot
try {
    Write-Host '[1/8] Checking Go formatting...'
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

    Write-Host '[2/8] Downloading dependencies...'
    go mod download
    Assert-LastExitCode 'Dependency download'

    Write-Host '[3/8] Verifying go.mod and go.sum...'
    $goModBefore = Get-Content -Raw -LiteralPath (Join-Path $repoRoot 'go.mod')
    $goSumBefore = Get-Content -Raw -LiteralPath (Join-Path $repoRoot 'go.sum')
    go mod tidy
    Assert-LastExitCode 'Module tidy'
    $goModAfter = Get-Content -Raw -LiteralPath (Join-Path $repoRoot 'go.mod')
    $goSumAfter = Get-Content -Raw -LiteralPath (Join-Path $repoRoot 'go.sum')
    if ($goModBefore -ne $goModAfter -or $goSumBefore -ne $goSumAfter) {
        throw 'go mod tidy changed go.mod or go.sum; review the changes and run the check again'
    }

    Write-Host '[4/8] Running tests...'
    go test ./... -count=1
    Assert-LastExitCode 'Tests'

    Write-Host '[5/8] Running static analysis...'
    go vet ./...
    Assert-LastExitCode 'Static analysis'

    Write-Host '[6/8] Building CLI...'
    New-Item -ItemType Directory -Force -Path $buildDir | Out-Null
    go build -trimpath -o $binaryPath .
    Assert-LastExitCode 'Build'

    Write-Host '[7/8] Checking the online installer...'
    $parserTokens = $null
    $parserErrors = $null
    [System.Management.Automation.Language.Parser]::ParseFile(
        $onlineInstallerPath,
        [ref]$parserTokens,
        [ref]$parserErrors
    ) | Out-Null
    if (@($parserErrors).Count -gt 0) {
        $messages = @($parserErrors | ForEach-Object { $_.Message }) -join '; '
        throw "Online installer has PowerShell syntax errors: $messages"
    }

    $onlineFixtureDir = Join-Path $buildDir 'online-installer-package'
    $onlineFixtureArchiveName = 'ai-dev-logger_v9.8.7_windows_amd64.zip'
    $onlineFixtureArchive = Join-Path $buildDir $onlineFixtureArchiveName
    $onlineFixtureChecksums = Join-Path $buildDir 'online-installer-checksums.txt'
    $onlineInstallSmokeDir = Join-Path $buildDir 'online-install-smoke'
    New-Item -ItemType Directory -Force -Path $onlineFixtureDir | Out-Null
    Copy-Item -LiteralPath $binaryPath -Destination (Join-Path $onlineFixtureDir 'ai-dev-logger.exe') -Force
    Copy-Item -LiteralPath (Join-Path $repoRoot 'README.md') -Destination (Join-Path $onlineFixtureDir 'README.md') -Force
    Copy-Item -LiteralPath (Join-Path $repoRoot 'scripts\install.ps1') -Destination (Join-Path $onlineFixtureDir 'install.ps1') -Force
    $onlineFixtureFiles = @(
        (Join-Path $onlineFixtureDir 'ai-dev-logger.exe')
        (Join-Path $onlineFixtureDir 'README.md')
        (Join-Path $onlineFixtureDir 'install.ps1')
    )
    Compress-Archive -LiteralPath $onlineFixtureFiles -DestinationPath $onlineFixtureArchive -Force

    $fixtureHash = (Get-FileHash -LiteralPath $onlineFixtureArchive -Algorithm SHA256).Hash.ToLowerInvariant()
    [System.IO.File]::WriteAllText(
        $onlineFixtureChecksums,
        "$fixtureHash  $onlineFixtureArchiveName" + [Environment]::NewLine,
        [System.Text.Encoding]::ASCII
    )

    & {
        . $onlineInstallerPath

        $parsedHash = Get-AIDevLoggerExpectedChecksum `
            -ChecksumPath $onlineFixtureChecksums `
            -AssetName $onlineFixtureArchiveName
        if ($parsedHash -ne $fixtureHash) {
            throw "Online installer parsed the wrong checksum: $parsedHash"
        }

        $releaseInfo = Get-AIDevLoggerReleaseInfo -ChecksumPath $onlineFixtureChecksums
        if ($releaseInfo.Version -ne 'v9.8.7' -or
            $releaseInfo.ArchiveName -ne $onlineFixtureArchiveName -or
            $releaseInfo.Hash -ne $fixtureHash) {
            throw "Online installer parsed the wrong release information: $($releaseInfo | Out-String)"
        }

        Assert-AIDevLoggerArchive `
            -ArchivePath $onlineFixtureArchive `
            -ExtractionPath (Join-Path $buildDir 'online-installer-extraction')

        $unsafeUrlWasRejected = $false
        try {
            Assert-AIDevLoggerGitHubUri `
                -Value 'https://example.com/fake.zip' `
                -Description 'Test URL' | Out-Null
        }
        catch {
            $unsafeUrlWasRejected = $true
        }
        if (-not $unsafeUrlWasRejected) {
            throw 'Online installer accepted a download URL outside GitHub'
        }

        function Invoke-RestMethod {
            throw 'Online installer must not call the rate-limited GitHub API'
        }

        function Invoke-WebRequest {
            param(
                [string]$Uri,
                [hashtable]$Headers,
                [string]$OutFile,
                [switch]$UseBasicParsing
            )

            if ($Uri.EndsWith("/$onlineFixtureArchiveName", [StringComparison]::Ordinal)) {
                Copy-Item -LiteralPath $onlineFixtureArchive -Destination $OutFile
                return
            }
            if ($Uri.EndsWith('/checksums.txt', [StringComparison]::Ordinal)) {
                Copy-Item -LiteralPath $onlineFixtureChecksums -Destination $OutFile
                return
            }
            throw "Unexpected mocked download URL: $Uri"
        }

        $pathBeforeOnlineInstall = $env:Path
        Invoke-AIDevLoggerOnlineInstall `
            -TargetInstallDir $onlineInstallSmokeDir `
            -SkipPath

        if ($env:Path -ne $pathBeforeOnlineInstall) {
            throw 'Online installer changed PATH even though -SkipPath was used'
        }
        $onlineInstalledAlias = Join-Path $onlineInstallSmokeDir 'adl.exe'
        if (-not (Test-Path -LiteralPath $onlineInstalledAlias -PathType Leaf)) {
            throw "Online installer did not create the short command: $onlineInstalledAlias"
        }
        & $onlineInstalledAlias --version | Out-Null
        Assert-LastExitCode 'Online installer short command smoke test'
    }

    Write-Host '[8/8] Running installer smoke test...'
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
    $installedAliasPath = Join-Path $resolvedInstallSmokeDir 'adl.exe'
    $completionPath = Join-Path $resolvedInstallSmokeDir 'ai-dev-logger-completion.ps1'
    if (-not (Test-Path -LiteralPath $installedBinaryPath -PathType Leaf)) {
        throw "Installer did not create the binary: $installedBinaryPath"
    }
    if (-not (Test-Path -LiteralPath $installedAliasPath -PathType Leaf)) {
        throw "Installer did not create the short command: $installedAliasPath"
    }
    if (-not (Test-Path -LiteralPath $completionPath -PathType Leaf)) {
        throw "Installer did not create the completion script: $completionPath"
    }
    $completionContent = Get-Content -Raw -LiteralPath $completionPath
    if (-not $completionContent.Contains('Register-ArgumentCompleter')) {
        throw 'Generated PowerShell completion script is invalid'
    }
    if (-not $completionContent.Contains("-CommandName 'adl'")) {
        throw 'Generated PowerShell completion script does not register the adl command'
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
    & $installedAliasPath --version | Out-Null
    Assert-LastExitCode 'Short command smoke test'

    $helpSmokeDir = Join-Path $buildDir ('help-smoke-' + [Guid]::NewGuid().ToString('N'))
    $helpRequests = @(
        @{ Arguments = @(); Marker = 'adl list' }
        @{ Arguments = @('add'); Marker = 'adl add --title' }
        @{ Arguments = @('config', 'set'); Marker = 'adl config set --model' }
        @{ Arguments = @('doctor'); Marker = 'adl doctor --online' }
        @{ Arguments = @('restore'); Marker = 'adl restore --input' }
    )
    foreach ($request in $helpRequests) {
        $helpArgs = @(
            '--db', (Join-Path $helpSmokeDir 'notes.db'),
            '--config', (Join-Path $helpSmokeDir 'config.json')
        ) + $request.Arguments + @('--help')
        $helpOutput = @(& $installedAliasPath @helpArgs) -join "`n"
        Assert-LastExitCode 'CLI help smoke test'
        if (-not $helpOutput.Contains($request.Marker)) {
            throw "CLI help did not show the expected example: $($request.Marker)"
        }
    }
    if (Test-Path -LiteralPath $helpSmokeDir) {
        throw 'Reading CLI help must not create a database or configuration directory'
    }

    $configSmokeDir = Join-Path $buildDir ('config-smoke-' + [Guid]::NewGuid().ToString('N'))
    $configSmokePath = Join-Path $configSmokeDir 'config.json'
    $configAPIKeyBefore = $env:AI_DEV_LOGGER_API_KEY
    $configFallbackKeyBefore = $env:OPENAI_API_KEY
    try {
        $env:AI_DEV_LOGGER_API_KEY = 'environment-test-key-5678'
        $env:OPENAI_API_KEY = 'fallback-test-key-9012'
        & $installedAliasPath --config $configSmokePath config show | Out-Null
        Assert-LastExitCode 'Missing config display smoke test'
        if (Test-Path -LiteralPath $configSmokeDir) {
            throw 'Displaying missing config must not create its directory'
        }

        & $installedAliasPath --config $configSmokePath config set `
            --api-key 'stored-test-key-1234' `
            --base-url 'https://example.test/v1/' `
            --model 'test-chat' `
            --embedding-model 'test-embedding' | Out-Null
        Assert-LastExitCode 'Initial configuration smoke test'
        & $installedAliasPath --config $configSmokePath config set --model 'updated-chat' | Out-Null
        Assert-LastExitCode 'Partial configuration update smoke test'
        $storedConfig = Get-Content -Raw -LiteralPath $configSmokePath | ConvertFrom-Json
        if ($storedConfig.llm.api_key -ne 'stored-test-key-1234' -or
            $storedConfig.llm.base_url -ne 'https://example.test/v1' -or
            $storedConfig.llm.model -ne 'updated-chat' -or
            $storedConfig.llm.embedding_model -ne 'test-embedding') {
            throw 'Partial configuration update changed an unspecified field or persisted an environment key'
        }

        $configOutput = @(& $installedAliasPath --config $configSmokePath config show) -join "`n"
        Assert-LastExitCode 'Masked configuration display smoke test'
        if (-not $configOutput.Contains('llm.api_key: envi...5678') -or
            -not $configOutput.Contains('llm.api_key_source: AI_DEV_LOGGER_API_KEY') -or
            $configOutput.Contains('environment-test-key-5678') -or
            $configOutput.Contains('stored-test-key-1234') -or
            $configOutput.Contains('fallback-test-key-9012')) {
            throw 'Configuration display did not mask the effective environment key correctly'
        }

        & $installedAliasPath --config $configSmokePath config set --embedding-model= | Out-Null
        Assert-LastExitCode 'Clear optional configuration smoke test'
        $clearedConfig = Get-Content -Raw -LiteralPath $configSmokePath | ConvertFrom-Json
        if ($clearedConfig.llm.embedding_model -ne '' -or
            $clearedConfig.llm.model -ne 'updated-chat' -or
            $clearedConfig.llm.api_key -ne 'stored-test-key-1234') {
            throw 'Clearing the embedding model changed other configuration fields'
        }
    }
    finally {
        $env:AI_DEV_LOGGER_API_KEY = $configAPIKeyBefore
        $env:OPENAI_API_KEY = $configFallbackKeyBefore
    }

    $quickAddDBPath = Join-Path $resolvedInstallSmokeDir 'quick-add-smoke.db'
    $quickAddOutput = @(& $installedAliasPath `
        --db $quickAddDBPath `
        'installer quick add #smoke')
    Assert-LastExitCode 'Short command quick-add smoke test'
    if ($quickAddOutput -notcontains 'saved note #1: installer quick add') {
        throw "Short command did not save the expected note: $($quickAddOutput -join ' | ')"
    }

    $quickAddShowOutput = @(& $installedAliasPath --db $quickAddDBPath show 1)
    Assert-LastExitCode 'Short command quick-add readback test'
    if ($quickAddShowOutput -notcontains 'tags: smoke') {
        throw "Short command did not preserve the inline tag: $($quickAddShowOutput -join ' | ')"
    }

    $doctorAPIKeyBefore = $env:AI_DEV_LOGGER_API_KEY
    $doctorFallbackKeyBefore = $env:OPENAI_API_KEY
    try {
        $env:AI_DEV_LOGGER_API_KEY = $null
        $env:OPENAI_API_KEY = $null
        $doctorOutput = @(& $installedAliasPath `
            --db $quickAddDBPath `
            --config (Join-Path $resolvedInstallSmokeDir 'missing-doctor-config.json') `
            doctor)
        Assert-LastExitCode 'Local-only doctor smoke test'
        $doctorText = $doctorOutput -join "`n"
        if ($doctorText -notmatch '\[PASS\] local notes\s+required; ready' -or
            $doctorText -notmatch '\[WARN\] AI enhancement\s+optional; not configured' -or
            $doctorText -notmatch '\[WARN\] semantic search\s+optional; not configured') {
            throw "Doctor did not distinguish required and optional capabilities: $doctorText"
        }
    }
    finally {
        $env:AI_DEV_LOGGER_API_KEY = $doctorAPIKeyBefore
        $env:OPENAI_API_KEY = $doctorFallbackKeyBefore
    }

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
