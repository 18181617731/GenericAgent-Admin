[CmdletBinding()]
param(
    [ValidateRange(1, 65535)]
    [int]$BackendPort = 8787,
    [ValidateRange(1, 65535)]
    [int]$HttpsPort = 443
)

$ErrorActionPreference = 'Stop'

function Find-Tailscale {
    $command = Get-Command tailscale.exe -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }

    foreach ($candidate in @(
        (Join-Path $env:ProgramFiles 'Tailscale\tailscale.exe'),
        (Join-Path ${env:ProgramFiles(x86)} 'Tailscale\tailscale.exe')
    )) {
        if ($candidate -and (Test-Path -LiteralPath $candidate)) {
            return $candidate
        }
    }

    return $null
}

function Invoke-Tailscale {
    param(
        [Parameter(Mandatory)]
        [string]$Executable,
        [Parameter(Mandatory)]
        [string[]]$Arguments
    )

    try {
        $output = & $Executable @Arguments 2>$null | Out-String
        $exitCode = $LASTEXITCODE
        return [pscustomobject]@{
            ExitCode = $exitCode
            Output = $output.Trim()
            Error = ''
        }
    } catch {
        return [pscustomobject]@{
            ExitCode = 1
            Output = ''
            Error = $_.Exception.Message
        }
    }
}

function Show-WarningAndContinue {
    param(
        [string]$Message,
        [string]$Detail = ''
    )

    Write-Warning $Message
    if ($Detail) {
        Write-Host "[HTTPS] $Detail"
    }
    Write-Host "[HTTPS] Local access remains available at http://127.0.0.1:$BackendPort"
}

$tailscale = Find-Tailscale
if (-not $tailscale) {
    Show-WarningAndContinue 'Tailscale was not found; HTTPS tailnet access was not configured.' 'Install and sign in to Tailscale, then run run.bat again.'
    exit 0
}

$status = Invoke-Tailscale -Executable $tailscale -Arguments @('status', '--json')
if ($status.ExitCode -ne 0) {
    Show-WarningAndContinue 'Unable to read Tailscale status; HTTPS tailnet access was not configured.' (($status.Error, $status.Output | Where-Object { $_ }) -join ' ')
    exit 0
}

try {
    $statusData = $status.Output | ConvertFrom-Json
    $dnsName = [string]$statusData.Self.DNSName
    $dnsName = $dnsName.TrimEnd('.')
} catch {
    Show-WarningAndContinue 'Tailscale returned an invalid status response; HTTPS tailnet access was not configured.' $_.Exception.Message
    exit 0
}

if (-not $dnsName) {
    Show-WarningAndContinue 'Tailscale has no MagicDNS name for this device; HTTPS tailnet access was not configured.' 'Enable MagicDNS and run run.bat again.'
    exit 0
}

$portSuffix = if ($HttpsPort -eq 443) { '' } else { ":$HttpsPort" }
$certificateDirectory = Join-Path ([IO.Path]::GetTempPath()) ("ga-admin-tailscale-cert-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $certificateDirectory -Force | Out-Null
try {
    $certificate = Invoke-Tailscale -Executable $tailscale -Arguments @(
        'cert',
        "--cert-file=$(Join-Path $certificateDirectory 'device.crt')",
        "--key-file=$(Join-Path $certificateDirectory 'device.key')",
        $dnsName
    )
} finally {
    Remove-Item -LiteralPath $certificateDirectory -Recurse -Force -ErrorAction SilentlyContinue
}
if ($certificate.ExitCode -ne 0) {
    Show-WarningAndContinue 'Tailscale HTTPS certificates are not enabled for this tailnet.' 'Open the Tailscale admin console, enable HTTPS certificates in DNS settings, then run run.bat again.'
    Write-Host '[HTTPS] Admin console: https://login.tailscale.com/admin/dns'
    Write-Host "[HTTPS] Expected URL after enabling: https://$dnsName$portSuffix/"
    Write-Host "[HTTPS] The direct Tailscale address remains HTTP; browser notifications require the HTTPS URL."
    exit 0
}

$serve = Invoke-Tailscale -Executable $tailscale -Arguments @(
    'serve',
    '--bg',
    '--yes',
    "--https=$HttpsPort",
    "http://127.0.0.1:$BackendPort"
)
if ($serve.ExitCode -ne 0) {
    $detail = (($serve.Error, $serve.Output | Where-Object { $_ }) -join ' ')
    Show-WarningAndContinue 'Tailscale HTTPS proxy configuration failed; the application is still running.' $detail
    exit 0
}

Write-Host "[HTTPS] Tailscale Serve is ready: https://$dnsName$portSuffix/"
Write-Host "[HTTPS] Backend proxy: http://127.0.0.1:$BackendPort"
exit 0
