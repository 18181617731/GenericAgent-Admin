param(
    [Parameter(Mandatory = $true)]
    [string]$InstallDir,

    [string]$MinimumVersion = "1.23"
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

function Get-WindowsGoArchitecture {
    $architecture = if ($env:PROCESSOR_ARCHITEW6432) {
        $env:PROCESSOR_ARCHITEW6432
    } else {
        $env:PROCESSOR_ARCHITECTURE
    }

    switch ($architecture.ToUpperInvariant()) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { throw "Unsupported Windows architecture: $architecture" }
    }
}

function Test-GoVersion {
    param(
        [string]$GoExe,
        [version]$RequiredVersion
    )

    if (-not (Test-Path -LiteralPath $GoExe -PathType Leaf)) {
        return $false
    }

    try {
        $versionOutput = & $GoExe version 2>$null
        if ($LASTEXITCODE -ne 0 -or $versionOutput -notmatch '\bgo(\d+\.\d+(?:\.\d+)?)\b') {
            return $false
        }

        return ([version]$Matches[1] -ge $RequiredVersion)
    } catch {
        return $false
    }
}

$requiredVersion = [version]$MinimumVersion
$installRoot = [IO.Path]::GetFullPath($InstallDir)
$goExe = Join-Path $installRoot "bin\go.exe"

if (Test-GoVersion -GoExe $goExe -RequiredVersion $requiredVersion) {
    Write-Host "Using cached portable Go: $goExe"
    exit 0
}

if ([Net.ServicePointManager]::SecurityProtocol -band [Net.SecurityProtocolType]::Tls12 -eq 0) {
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
}

$architecture = Get-WindowsGoArchitecture
$downloadListUrl = "https://go.dev/dl/?mode=json"
Write-Host "Finding the latest stable Go release for windows-$architecture..."
$releases = Invoke-RestMethod -UseBasicParsing -Uri $downloadListUrl
$release = $releases |
    Where-Object { $_.stable -and ([version]($_.version -replace '^go', '') -ge $requiredVersion) } |
    Select-Object -First 1

if (-not $release) {
    throw "Go download metadata did not contain a stable release satisfying $MinimumVersion or newer."
}

$archive = $release.files |
    Where-Object { $_.os -eq "windows" -and $_.arch -eq $architecture -and $_.kind -eq "archive" } |
    Select-Object -First 1

if (-not $archive) {
    throw "Go $($release.version) does not provide a Windows $architecture archive."
}

$toolsRoot = Split-Path -Parent $installRoot
$downloadRoot = Join-Path $toolsRoot ".downloads"
$stagingRoot = Join-Path $toolsRoot (".go-extract-" + $PID)
$archivePath = Join-Path $downloadRoot $archive.filename
$downloadUrl = "https://go.dev/dl/$($archive.filename)"

New-Item -ItemType Directory -Force -Path $downloadRoot | Out-Null
if (Test-Path -LiteralPath $stagingRoot) {
    Remove-Item -LiteralPath $stagingRoot -Recurse -Force
}

try {
    Write-Host "Downloading $($archive.filename)..."
    Invoke-WebRequest -UseBasicParsing -Uri $downloadUrl -OutFile $archivePath

    $actualHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash
    if ($actualHash -ne $archive.sha256.ToUpperInvariant()) {
        throw "SHA-256 mismatch for $($archive.filename)."
    }
    Write-Host "SHA-256 verified."

    New-Item -ItemType Directory -Force -Path $stagingRoot | Out-Null
    Expand-Archive -LiteralPath $archivePath -DestinationPath $stagingRoot -Force
    $extractedRoot = Join-Path $stagingRoot "go"
    if (-not (Test-Path -LiteralPath (Join-Path $extractedRoot "bin\go.exe") -PathType Leaf)) {
        throw "The downloaded archive did not contain go\bin\go.exe."
    }

    if (Test-Path -LiteralPath $installRoot) {
        Remove-Item -LiteralPath $installRoot -Recurse -Force
    }
    Move-Item -LiteralPath $extractedRoot -Destination $installRoot
} finally {
    if (Test-Path -LiteralPath $stagingRoot) {
        Remove-Item -LiteralPath $stagingRoot -Recurse -Force
    }
}

if (-not (Test-GoVersion -GoExe $goExe -RequiredVersion $requiredVersion)) {
    throw "Portable Go was extracted, but it could not be executed from $goExe."
}

Write-Host "Portable Go is ready: $goExe"
