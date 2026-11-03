$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

# Dot-sourcing the shared installer owner binds its parameters in the caller's
# scope. The Team wrapper must retain the requested upload directory rather
# than replacing it with the shared script's temporary default.
$fixture = Join-Path ([IO.Path]::GetTempPath()) ('deepzora-team-installer-path-' + [guid]::NewGuid().ToString('N'))
$requestedEvidence = Join-Path $fixture 'uploaded-evidence'
$requestedInstaller = Join-Path $fixture 'fixture.exe'
. (Join-Path $PSScriptRoot 'team-windows-installer-acceptance.ps1') `
  -InstallerPath $requestedInstaller -ExpectedVersion 'v0.0.0' -EvidenceDirectory $requestedEvidence
if ($EvidenceDirectory -cne $requestedEvidence) {
  throw 'Team installer evidence escaped the requested upload directory.'
}
if ($InstallerPath -cne $requestedInstaller -or $ExpectedVersion -cne 'v0.0.0') {
  throw 'Shared installer binding changed the candidate identity.'
}
if (Test-Path -LiteralPath $fixture) {
  throw 'Loading Team installer helpers must not install or create user data.'
}
Write-Host 'PASS Team installer helpers retain requested evidence and candidate identity without side effects'
