[CmdletBinding()]
param(
  [Parameter(Mandatory=$true)][string]$InstallerPath,
  [Parameter(Mandatory=$true)][string]$ExpectedVersion,
  [Parameter(Mandatory=$true)][string]$EvidenceDirectory
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$script:TeamAcceptanceRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'test-windows-installer-startup.ps1') -InstallerPath $InstallerPath -ExpectedVersion $ExpectedVersion

# Reuse the installation owner's clean-account checks, native install,
# identity/registration verification, uninstall and retained-data assertions.
function Invoke-InstalledRuntimeAcceptance {
  param([string]$InstallRoot, [string]$ExpectedVersion, [string]$ArtifactPath, [string]$EvidenceDirectory)
  & (Join-Path $PSScriptRoot 'test-windows-startup-recovery.ps1') @PSBoundParameters | Out-Host
  $current = Get-Content -LiteralPath (Join-Path $InstallRoot 'current.json') -Raw | ConvertFrom-Json
  $release = Join-Path $InstallRoot ($current.activeDir -replace '/', [IO.Path]::DirectorySeparatorChar)
  $executable = Join-Path $release 'app\Deepzora.exe'
  & node (Join-Path $script:TeamAcceptanceRoot 'desktop\packaging\team-native-smoke.mjs') $executable (Join-Path $EvidenceDirectory 'team')
  if ($LASTEXITCODE -ne 0) { throw "Installed Team acceptance failed with exit code $LASTEXITCODE" }
}
Invoke-WindowsInstallerAcceptance -InstallerPath $InstallerPath -ExpectedVersion $ExpectedVersion `
  -FixtureBuilderPath '' -EvidenceDirectory $EvidenceDirectory
