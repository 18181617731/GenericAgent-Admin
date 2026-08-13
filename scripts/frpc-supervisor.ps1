param(
    [string]$FrpcPath = 'C:\ProgramData\GenericAgent\frp\frpc.exe',
    [string]$ConfigPath = 'C:\ProgramData\GenericAgent\frp\frpc.toml',
    [int]$RetrySeconds = 15
)

$ErrorActionPreference = 'Continue'

# The tunnel server is directly reachable from this machine. Do not inherit
# the user's browser proxy: it may start later than the logon task and would
# make the tunnel fail during boot.
foreach ($name in @('HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy')) {
    [Environment]::SetEnvironmentVariable($name, $null, 'Process')
}

$workingDirectory = Split-Path -Parent $FrpcPath

while ($true) {
    if ((Test-Path -LiteralPath $FrpcPath) -and (Test-Path -LiteralPath $ConfigPath)) {
        $child = Start-Process -FilePath $FrpcPath `
            -ArgumentList @('-c', $ConfigPath) `
            -WorkingDirectory $workingDirectory `
            -WindowStyle Hidden `
            -PassThru `
            -Wait
        $null = $child
    }

    Start-Sleep -Seconds ([Math]::Max(5, $RetrySeconds))
}
