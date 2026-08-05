<#
Build a self-contained portable bundle of GA Admin.

Layout produced (extract anywhere, no installer, no system Python):

  <name>\
    ga-admin.exe            built from this repo
    cmd\chat_worker.py      chat worker shipped next to the exe
    config.local.json       ga_root / python_path left empty, filled on 1st run
    start.bat               launcher: bootstrap.py then ga-admin.exe
    bootstrap.py            self-heals pyvenv.cfg + config for the real path
    python\                 uv standalone CPython (base interpreter, ~60MB)
    GenericAgent\           upstream source archive from codeload (no .git)
      .venv\                venv created from python\, deps installed

Why this shape:
  * main.go appRoot() defaults to the exe directory, so config.local.json in
    the bundle root is the config the exe reads.
  * api.go resolvePythonForRoot() probes <ga_root>\.venv\Scripts\python.exe,
    so putting the venv inside GenericAgent makes Python discovery automatic.
  * A relocated venv dies with rc=103 because pyvenv.cfg keeps absolute paths;
    bootstrap.py rewrites them on every start.

Requires on the build machine: uv, go, npm (npm/go only when building).
git is optional: used for the version string and the upstream commit sha,
both of which degrade to a placeholder when git is absent.
#>
[CmdletBinding()]
param(
    # Where the bundle folder and zip are written.
    [string]$OutDir = "",
    # CPython version fetched by uv for the bundled base interpreter.
    [string]$PythonVersion = "3.12",
    # Upstream branch (or tag) whose source archive is downloaded.
    [string]$Ref = "main",
    # codeload serves the tree as a zip over plain HTTPS: no git, no .git
    # history, ~24MB instead of ~52MB on the wire.
    [string]$ArchiveUrl = "https://codeload.github.com/lsdefine/GenericAgent/zip/refs/heads/{0}",
    # Download attempts before giving up on the archive.
    [int]$ZipRetries = 3,
    # Fall back to git clone when the archive download keeps failing.
    [switch]$PreferGitClone,
    [string]$RepoUrl = "https://github.com/lsdefine/GenericAgent",
    # Reuse dist\ga-admin.exe instead of running build.bat.
    [switch]$SkipBuild,
    # Leave the staged folder only, skip zipping.
    [switch]$NoZip
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Say([string]$msg) { Write-Host "[portable] $msg" }
function Die([string]$msg) { throw "[portable] $msg" }

function Need([string]$exe) {
    $found = Get-Command $exe -ErrorAction SilentlyContinue
    if (-not $found) { Die "required tool not found on PATH: $exe" }
    return $found.Source
}

# git is optional now: only used for the bundle version string and for the
# opt-in clone fallback. Returns $null instead of dying.
function Optional([string]$exe) {
    $found = Get-Command $exe -ErrorAction SilentlyContinue
    if (-not $found) { return $null }
    return $found.Source
}

function DirSizeMB([string]$path) {
    if (-not (Test-Path -LiteralPath $path)) { return 0 }
    $sum = (Get-ChildItem -LiteralPath $path -Recurse -File -Force -ErrorAction SilentlyContinue |
        Measure-Object -Sum Length).Sum
    if (-not $sum) { return 0 }
    return [math]::Round($sum / 1MB, 1)
}

# GA runtime imports these directly; everything else is installed on demand
# by the agent itself, which is why the bundle ships pip inside the venv.
$CoreDeps = @(
    "requests",
    "beautifulsoup4",
    "bottle",
    "simple-websocket-server",
    "aiohttp"
)

# Trimmed from the clone: dead weight for a runnable bundle.
# NOTE assets\tools_schema.json is a hard startup dependency - never drop
# the whole assets directory, only assets\demo.
$PruneRelative = @(
    ".git",
    ".github",
    "assets\demo"
)

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptDir "..\..")
if (-not (Test-Path -LiteralPath (Join-Path $repoRoot "main.go"))) {
    Die "cannot locate repo root from $scriptDir (no main.go at $repoRoot)"
}

if ([string]::IsNullOrWhiteSpace($OutDir)) {
    $OutDir = Join-Path $repoRoot "release"
}
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$OutDir = (Resolve-Path -LiteralPath $OutDir).Path

$git = Optional "git"
$uv = Need "uv"
if (-not $git) { Say "git not on PATH: archive download only, version string falls back" }

Say "repo root  $repoRoot"
Say "output dir $OutDir"

# ---------------------------------------------------------------- 1. exe
$distExe = Join-Path $repoRoot "dist\ga-admin.exe"
if ($SkipBuild) {
    if (-not (Test-Path -LiteralPath $distExe)) { Die "-SkipBuild given but $distExe is missing" }
    Say "reusing existing $distExe"
} else {
    Need "go" | Out-Null
    Need "npm.cmd" | Out-Null
    Say "building ga-admin.exe via build.bat (frontend + go) ..."
    Push-Location $repoRoot
    try {
        & cmd.exe /c "build.bat"
        if ($LASTEXITCODE -ne 0) { Die "build.bat failed with exit code $LASTEXITCODE" }
    } finally { Pop-Location }
    if (-not (Test-Path -LiteralPath $distExe)) { Die "build.bat finished but $distExe is missing" }
}

# Version string for the bundle name, taken from the repo we just built.
$version = "dev"
if ($git) {
    Push-Location $repoRoot
    try {
        $described = & $git describe --tags --always 2>$null
        if ($LASTEXITCODE -eq 0 -and $described) { $version = ($described | Select-Object -First 1).Trim() }
    } finally { Pop-Location }
}

$bundleName = "ga-admin-portable-$version"
$stage = Join-Path $OutDir $bundleName
if (Test-Path -LiteralPath $stage) {
    Say "clearing previous stage $stage"
    Remove-Item -LiteralPath $stage -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $stage | Out-Null

Copy-Item -LiteralPath $distExe -Destination (Join-Path $stage "ga-admin.exe") -Force
$workerSrc = Join-Path $repoRoot "cmd\chat_worker.py"
if (Test-Path -LiteralPath $workerSrc) {
    New-Item -ItemType Directory -Force -Path (Join-Path $stage "cmd") | Out-Null
    Copy-Item -LiteralPath $workerSrc -Destination (Join-Path $stage "cmd\chat_worker.py") -Force
} else {
    Say "WARNING cmd\chat_worker.py not found, chat worker will be missing"
}
Copy-Item -LiteralPath (Join-Path $scriptDir "start.bat") -Destination $stage -Force
Copy-Item -LiteralPath (Join-Path $scriptDir "bootstrap.py") -Destination $stage -Force

# ------------------------------------------------- 2. fetch GA source tree
$gaRoot = Join-Path $stage "GenericAgent"
$gaCommit = "unknown"
$gaSource = "unknown"

function Get-GaCommit([string]$gitExe, [string]$repo, [string]$reference) {
    # ls-remote is the only cheap way to record which commit the archive is,
    # since codeload zips carry no metadata. Optional: never fail the build.
    if (-not $gitExe) { return "unknown" }
    $out = & $gitExe ls-remote $repo ("refs/heads/" + $reference) 2>$null
    if ($LASTEXITCODE -ne 0 -or -not $out) { return "unknown" }
    $sha = ($out | Select-Object -First 1).ToString().Split("`t")[0].Trim()
    if ($sha.Length -ge 7) { return $sha.Substring(0, 7) }
    return "unknown"
}

function Fetch-GaArchive([string]$url, [string]$dest, [int]$tries) {
    # codeload streams the tree as a plain zip: no git needed on the machine,
    # ~24MB on the wire vs ~52MB for a depth-1 clone (half of which is .git
    # history that this script prunes anyway).
    # Stage next to $dest, not in $env:TEMP: Move-Item on a directory across
    # volumes is unreliable, and a same-volume move is a rename (instant).
    $work = Split-Path -Parent $dest
    $tag = [guid]::NewGuid().ToString("N").Substring(0, 8)
    $tmpZip = Join-Path $work ("ga-src-" + $tag + ".zip")
    $tmpDir = Join-Path $work ("ga-src-" + $tag)
    $ok = $false
    for ($attempt = 1; $attempt -le $tries; $attempt++) {
        try {
            $sw = [diagnostics.stopwatch]::StartNew()
            $old = $ProgressPreference
            $ProgressPreference = "SilentlyContinue"
            try {
                Invoke-WebRequest -Uri $url -OutFile $tmpZip -UseBasicParsing -TimeoutSec 300
            } finally { $ProgressPreference = $old }
            $sw.Stop()
            $mb = [math]::Round((Get-Item -LiteralPath $tmpZip).Length / 1MB, 1)
            Say ("downloaded {0} MB in {1:n1}s" -f $mb, $sw.Elapsed.TotalSeconds)
            $ok = $true
            break
        } catch {
            Say "download attempt $attempt failed: $($_.Exception.Message)"
            if (Test-Path -LiteralPath $tmpZip) { Remove-Item -LiteralPath $tmpZip -Force }
            Start-Sleep -Seconds 3
        }
    }
    if (-not $ok) { return $false }

    try {
        if (Test-Path -LiteralPath $tmpDir) { Remove-Item -LiteralPath $tmpDir -Recurse -Force }
        # ZipFile beats Expand-Archive by a wide margin on trees with many
        # small files, and it is present on every supported PowerShell host.
        Add-Type -AssemblyName System.IO.Compression.FileSystem -ErrorAction SilentlyContinue
        [System.IO.Compression.ZipFile]::ExtractToDirectory($tmpZip, $tmpDir)
        # The zip wraps everything in <repo>-<ref>\ ; lift that up to GenericAgent\.
        $inner = Get-ChildItem -LiteralPath $tmpDir -Directory | Select-Object -First 1
        if (-not $inner) { Say "archive has no top-level directory"; return $false }
        if (Test-Path -LiteralPath $dest) { Remove-Item -LiteralPath $dest -Recurse -Force }
        Move-Item -LiteralPath $inner.FullName -Destination $dest -Force
        return $true
    } catch {
        Say "extract failed: $($_.Exception.Message)"
        return $false
    } finally {
        if (Test-Path -LiteralPath $tmpZip) { Remove-Item -LiteralPath $tmpZip -Force -ErrorAction SilentlyContinue }
        if (Test-Path -LiteralPath $tmpDir) { Remove-Item -LiteralPath $tmpDir -Recurse -Force -ErrorAction SilentlyContinue }
    }
}

function Fetch-GaClone([string]$gitExe, [string]$repo, [string]$reference, [string]$dest) {
    if (-not $gitExe) { Say "git not available for clone fallback"; return $false }
    # A flaky link drops sideband packets ("early EOF" / curl 56). Retry, and
    # on later attempts pin HTTP/1.1, which survives broken HTTP/2 multiplexing.
    for ($attempt = 1; $attempt -le 3; $attempt++) {
        if (Test-Path -LiteralPath $dest) { Remove-Item -LiteralPath $dest -Recurse -Force }
        $cloneArgs = @()
        if ($attempt -gt 1) {
            $cloneArgs += @("-c", "http.version=HTTP/1.1", "-c", "http.postBuffer=524288000")
        }
        $cloneArgs += @("clone", "--depth", "1", "--branch", $reference, "--single-branch", $repo, $dest)
        & $gitExe @cloneArgs
        if ($LASTEXITCODE -eq 0) { return $true }
        Say "clone attempt $attempt failed (exit $LASTEXITCODE)"
        Start-Sleep -Seconds 3
    }
    return $false
}

$archive = [string]::Format($ArchiveUrl, $Ref)

if ($PreferGitClone) {
    Say "cloning $RepoUrl ($Ref, depth 1) ..."
    if (Fetch-GaClone $git $RepoUrl $Ref $gaRoot) {
        $gaSource = "git clone"
    } else {
        Say "clone failed, falling back to source archive"
        if (-not (Fetch-GaArchive $archive $gaRoot $ZipRetries)) { Die "both git clone and archive download failed" }
        $gaSource = "archive"
    }
} else {
    Say "fetching source archive $archive ..."
    if (Fetch-GaArchive $archive $gaRoot $ZipRetries) {
        $gaSource = "archive"
    } else {
        Say "archive download failed, falling back to git clone"
        if (-not (Fetch-GaClone $git $RepoUrl $Ref $gaRoot)) { Die "both archive download and git clone failed" }
        $gaSource = "git clone"
    }
}

if ($gaSource -eq "git clone") {
    Push-Location $gaRoot
    try {
        $sha = & $git rev-parse --short HEAD 2>$null
        if ($LASTEXITCODE -eq 0 -and $sha) { $gaCommit = ($sha | Select-Object -First 1).Trim() }
    } finally { Pop-Location }
} else {
    $gaCommit = Get-GaCommit $git $RepoUrl $Ref
}
Say "GenericAgent $Ref at commit $gaCommit (via $gaSource)"

# The archive path has no .git, so record provenance for whoever unpacks this.
$stampPath = Join-Path $gaRoot "PORTABLE_SOURCE.txt"
$stampLines = @(
    "repo   $RepoUrl",
    "ref    $Ref",
    "commit $gaCommit",
    "source $gaSource",
    ("fetched " + (Get-Date).ToString("yyyy-MM-ddTHH:mm:ssK"))
)
# WriteAllText, not Set-Content -Encoding utf8: the latter emits a BOM on
# PowerShell 5, which makes this stamp awkward to parse downstream.
[System.IO.File]::WriteAllText($stampPath, ($stampLines -join "`r`n") + "`r`n",
    (New-Object System.Text.UTF8Encoding($false)))

foreach ($rel in $PruneRelative) {
    $target = Join-Path $gaRoot $rel
    if (Test-Path -LiteralPath $target) {
        $mb = DirSizeMB $target
        Remove-Item -LiteralPath $target -Recurse -Force
        Say "pruned GenericAgent\$rel (-$mb MB)"
    }
}
if (-not (Test-Path -LiteralPath (Join-Path $gaRoot "assets\tools_schema.json"))) {
    Die "assets\tools_schema.json missing after prune - GA will not start"
}

# ------------------------------------------- 3. bundled base interpreter
$pyHome = Join-Path $stage "python"
$pyStage = Join-Path $OutDir ".uvpy-$version"
if (Test-Path -LiteralPath $pyStage) { Remove-Item -LiteralPath $pyStage -Recurse -Force }
New-Item -ItemType Directory -Force -Path $pyStage | Out-Null

Say "fetching standalone CPython $PythonVersion via uv ..."
& $uv python install --install-dir $pyStage $PythonVersion
if ($LASTEXITCODE -ne 0) { Die "uv python install failed with exit code $LASTEXITCODE" }

# uv unpacks into <install-dir>\cpython-<ver>-<platform>\; flatten it so the
# bundle always exposes python\python.exe.
$pyExe = Join-Path $pyStage "python.exe"
if (Test-Path -LiteralPath $pyExe) {
    Move-Item -LiteralPath $pyStage -Destination $pyHome
} else {
    $nested = Get-ChildItem -LiteralPath $pyStage -Directory |
        Where-Object { Test-Path -LiteralPath (Join-Path $_.FullName "python.exe") } |
        Select-Object -First 1
    if (-not $nested) { Die "no python.exe found under $pyStage after uv install" }
    Move-Item -LiteralPath $nested.FullName -Destination $pyHome
    Remove-Item -LiteralPath $pyStage -Recurse -Force -ErrorAction SilentlyContinue
}
$basePy = Join-Path $pyHome "python.exe"
if (-not (Test-Path -LiteralPath $basePy)) { Die "bundled interpreter missing: $basePy" }

$verOut = & $basePy -c "import sys;print(sys.version.split()[0])"
if ($LASTEXITCODE -ne 0) { Die "bundled interpreter does not run" }
Say "bundled python $($verOut.Trim()) ($(DirSizeMB $pyHome) MB)"

# --------------------------------------------------- 4. venv under GA root
# uv's standalone build is PEP 668 externally managed, so pip must run inside
# a venv, not against the base interpreter.
$venvDir = Join-Path $gaRoot ".venv"
Say "creating venv at GenericAgent\.venv ..."
& $basePy -m venv $venvDir
if ($LASTEXITCODE -ne 0) { Die "venv creation failed with exit code $LASTEXITCODE" }

$venvPy = Join-Path $venvDir "Scripts\python.exe"
if (-not (Test-Path -LiteralPath $venvPy)) { Die "venv interpreter missing: $venvPy" }

Say "installing core deps: $($CoreDeps -join ' ')"
& $venvPy -m pip install --upgrade pip --quiet --disable-pip-version-check
if ($LASTEXITCODE -ne 0) { Die "pip self-upgrade failed with exit code $LASTEXITCODE" }
& $venvPy -m pip install --quiet --disable-pip-version-check @CoreDeps
if ($LASTEXITCODE -ne 0) { Die "pip install of core deps failed with exit code $LASTEXITCODE" }

$probe = "import requests,bs4,bottle,aiohttp;print('deps ok')"
$probeOut = & $venvPy -c $probe
if ($LASTEXITCODE -ne 0) { Die "dependency import probe failed" }
Say "venv $($probeOut.Trim()) ($(DirSizeMB $venvDir) MB)"

# ------------------------------------------------------------- 5. config
# ga_root / python_path stay empty on purpose: bootstrap.py fills them with
# absolute paths for wherever the user extracted the bundle.
$config = [ordered]@{
    ga_root              = ""
    python_path          = ""
    host                 = "127.0.0.1"
    port                 = 8787
    vite_host            = "127.0.0.1"
    vite_port            = 5173
    vite_allowed_hosts   = @()
    backend_proxy_host   = "127.0.0.1"
    log_tail_lines       = 200
    buffer_lines         = 1000
    service_autostart    = @()
}
$configPath = Join-Path $stage "config.local.json"
# No BOM: Go's json.Unmarshal rejects a UTF-8 BOM, and Set-Content -Encoding
# UTF8 writes one on PowerShell 5.1.
$configJson = ($config | ConvertTo-Json -Depth 5) + "`r`n"
[System.IO.File]::WriteAllText($configPath, $configJson, (New-Object System.Text.UTF8Encoding($false)))
Say "wrote config.local.json (ga_root/python_path resolved at first start)"

# ------------------------------------------------- 6. bootstrap dry run
Say "running bootstrap.py against the staged bundle ..."
& $basePy (Join-Path $stage "bootstrap.py")
if ($LASTEXITCODE -ne 0) { Die "bootstrap.py failed with exit code $LASTEXITCODE" }

# ------------------------------------------------------------- 7. package
$stageMB = DirSizeMB $stage
Say "bundle staged: $stage ($stageMB MB)"

if ($NoZip) {
    Say "done (zip skipped)"
    return
}

$zipPath = Join-Path $OutDir "$bundleName.zip"
if (Test-Path -LiteralPath $zipPath) { Remove-Item -LiteralPath $zipPath -Force }
Say "compressing to $zipPath ..."
Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $zipPath -CompressionLevel Optimal
$zipMB = [math]::Round((Get-Item -LiteralPath $zipPath).Length / 1MB, 1)

Say "done"
Say "  folder $stage ($stageMB MB)"
Say "  zip    $zipPath ($zipMB MB)"
Say "  admin  $version / GenericAgent $gaCommit / python $($verOut.Trim())"
