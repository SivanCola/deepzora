$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted') {
    throw 'The credential-free SimplySign UI probe requires a disposable GitHub-hosted runner.'
}
foreach ($name in @('CERTUM_USERNAME', 'CERTUM_OTP_URI', 'CERTUM_KEY_ID')) {
    [Environment]::SetEnvironmentVariable($name, $null, 'Process')
}
# This disposable probe has no credentials and never submits a login. Capture
# its empty UI on failure only; normal authentication keeps captures disabled.
$env:CAPTURE_DIAGNOSTICS = 'true'
try {
    & (Join-Path $PSScriptRoot 'certum/Install-SimplySign.ps1') -InstallerPath (Join-Path $env:RUNNER_TEMP 'SimplySign-probe.msi')
    & (Join-Path $PSScriptRoot 'certum/Connect-SimplySign.ps1') -ProbeOnly
} finally {
    Get-Process -Name '*SimplySign*' -ErrorAction SilentlyContinue | Stop-Process -Force
}
