[CmdletBinding()]
param(
  [Parameter(Mandatory=$true)][string]$InstallerPath,
  [Parameter(Mandatory=$true)][string]$ExpectedVersion,
  [Parameter(Mandatory=$true)][ValidateSet('amd64','arm64')][string]$Architecture,
  [Parameter(Mandatory=$true)][string]$EvidenceDirectory
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'test-windows-installer-startup.ps1') -InstallerPath $InstallerPath -ExpectedVersion $ExpectedVersion -EvidenceDirectory $EvidenceDirectory
Assert-InstallerTestAccount
$installer = (Resolve-Path -LiteralPath $InstallerPath).Path
if ((Get-AuthenticodeSignature -LiteralPath $installer).Status -ne 'Valid') { throw 'Candidate installer signature is not trusted' }
$targetHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $installer).Hash.ToLowerInvariant()
$evidence = [IO.Path]::GetFullPath($EvidenceDirectory)
if (Test-Path -LiteralPath $evidence) { throw 'Legacy update evidence directory must be new' }
New-Item -ItemType Directory -Path $evidence | Out-Null
# Exact v0.0.7 bytes from the sealed and published release. Never follow Latest
# for the old client, or silently test an unsigned/rebuilt approximation of it.
$published = @{
  amd64 = @{
    installed = 'b19176ec83b500975235e554ce4935207dbefb3c987ba58dcf735a4120c7ad25'
    portable = 'b828fcc61282966270a43bc9451a395745e309ffcf503291b0d8e145770195a3'
  }
  arm64 = @{
    installed = '6c253aee4082c157334511593350b0be84b33a5e111e8356fdfd651fd42b18b6'
    portable = 'dc9fa13c7c721a3a3c95c61d5dc83802cca0dcb1fc15fc1d1328f9356e393567'
  }
}
$cpu = if ($Architecture -eq 'amd64') { 'x64' } else { 'arm64' }
$summary = @{ sourceVersion = 'v0.0.7'; targetVersion = $ExpectedVersion; targetSHA256 = $targetHash; architecture = $Architecture }
foreach ($mode in @('installed','portable')) {
  $root = Join-Path $evidence $mode
  $install = Join-Path $root 'installed'
  $homePath = Join-Path $root 'home'
  New-Item -ItemType Directory -Path $homePath -Force | Out-Null
  $snapshot = Enter-WindowsAcceptanceEnvironment -DataHome $homePath
  try {
    $suffix = if ($mode -eq 'installed') { 'Setup.exe' } else { 'Portable.zip' }
    $name = "Deepzora-0.0.7-Windows-$cpu-$suffix"
    $oldPackage = Join-Path $root $name
    $downloaded = $false
    foreach ($url in @("https://downloads.deepzora.app/desktop/desktop-v0.0.7/$name", "https://github.com/SivanCola/deepzora/releases/download/desktop-v0.0.7/$name")) {
      try {
        Invoke-WebRequest $url -OutFile $oldPackage -TimeoutSec 180 -UserAgent "Deepzora-Updater/v0.0.7 (windows/$Architecture; acceptance)"
        $downloaded = $true
        break
      } catch { Write-Host 'Old package origin unavailable; trying the official fallback.' }
    }
    if (-not $downloaded) { throw 'Published old package is unavailable from both official origins' }
    if ((Get-FileHash -Algorithm SHA256 -LiteralPath $oldPackage).Hash.ToLowerInvariant() -ne $published[$Architecture][$mode]) {
      throw 'Published v0.0.7 package does not match its sealed digest'
    }
    if ($mode -eq 'installed') {
      Install-Deepzora -Installer $oldPackage -InstallRoot $install
      $null = Assert-InstalledIdentity -InstallRoot $install -ExpectedVersion 'v0.0.7'
    } else {
      Expand-Archive -LiteralPath $oldPackage -DestinationPath $install
    }
    [IO.File]::WriteAllText((Join-Path $homePath 'config.toml'), "[desktop]`nlanguage = `"en`"`ncheck_updates = false`ntelemetry = false`nmetrics = false`n")
    $marker = Join-Path $homePath 'legacy-update-retention.txt'
    [IO.File]::WriteAllText($marker, [guid]::NewGuid().ToString('N'))
    $markerHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $marker).Hash
    & (Join-Path $PSScriptRoot 'test-windows-startup-recovery.ps1') -InstallRoot $install -ExpectedVersion 'v0.0.7' `
      -DataHome $homePath -EvidenceDirectory (Join-Path $root 'before') -KeepRunning | Out-Host
    if (-not $?) { throw 'Published old client did not reach ready' }
    $before = Get-Content -LiteralPath (Join-Path $root 'before/result.json') -Raw | ConvertFrom-Json
    $oldShell = Get-Process -Id $before.shellPID
    $oldService = Get-Process -Id $before.servicePID
    $null = $oldShell.Handle
    $null = $oldService.Handle
    $oldHelper = Join-Path $root 'v0.0.7-update-helper.exe'
    Copy-Item -LiteralPath (Join-Path $install 'versions/v0.0.7/deepzora-update-helper.exe') -Destination $oldHelper
    if ((Get-AuthenticodeSignature -LiteralPath $oldHelper).Status -ne 'Valid') { throw 'Published old update helper is not trusted' }
    $helperHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $oldHelper).Hash.ToLowerInvariant()
    # Frozen v0.0.7 instance identity on the disposable runner's ordinary,
    # case-insensitive, newly created data directory. The old helper itself
    # revalidates this identity and the new endpoint before reporting success.
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $digest = $sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($homePath.ToLowerInvariant())) } finally { $sha.Dispose() }
    $env:DEEPZORA_UPDATE_INSTANCE_ID = 'com.deepzora.desktop.' + ([BitConverter]::ToString($digest).Replace('-','').ToLowerInvariant().Substring(0,16))
    $arguments = @('--parent-pid', $oldService.Id, '--installer', ('"' + $installer + '"'), '--installer-sha256', $targetHash,
      '--to-version', $ExpectedVersion, '--install-layout', 'versioned-v1', '--install-dir', ('"' + $install + '"'),
      '--relaunch', ('"' + (Join-Path $install 'Deepzora.exe') + '"'))
    $helper = Start-Process -FilePath $oldHelper -ArgumentList $arguments -PassThru `
      -RedirectStandardOutput (Join-Path $root 'helper.stdout.log') -RedirectStandardError (Join-Path $root 'helper.stderr.log')
    $null = $helper.Handle
    Start-Process -FilePath $oldShell.Path -ArgumentList '--deepzora-lifecycle-request=quit' | Out-Null
    if (-not $oldShell.WaitForExit(15000) -or -not $oldService.WaitForExit(1000)) { throw 'Old client did not exit normally for automatic update' }
    if (-not $helper.WaitForExit(240000)) { throw 'Published old helper did not finish the automatic update' }
    if ($helper.ExitCode -ne 0) { throw "Published old helper failed automatic update: $($helper.ExitCode)" }
    # The old helper verifies the ready shell but does not join the stable
    # launcher it started. Await that exact child's exit before testing another
    # launch; otherwise the immutable old launcher's teardown can overlap our
    # next process snapshot. Keep the helper handle alive so its PID cannot be
    # reused, and never skip a still-present child that cannot be inspected.
    $launchDeadline = [DateTime]::UtcNow.AddSeconds(40)
    while ($true) {
      $children = @(Get-CimInstance Win32_Process -Filter "ParentProcessId = $($helper.Id) AND Name = 'Deepzora.exe'")
      if ($children.Count -eq 0) { break }
      $remaining = [int]($launchDeadline - [DateTime]::UtcNow).TotalMilliseconds
      if ($remaining -le 0) { throw 'Old helper relauncher did not finish; child evidence remains unverified' }
      foreach ($child in $children) {
        $relaunchProcess = $null
        try {
          $relaunchProcess = Get-Process -Id $child.ProcessId -ErrorAction Stop
          $null = $relaunchProcess.Handle
        } catch {
          if ($null -ne $relaunchProcess) { $relaunchProcess.Dispose() }
          continue
        }
        try {
          if (-not $relaunchProcess.WaitForExit($remaining)) { throw 'Old helper relauncher remained alive' }
          if ($relaunchProcess.ExitCode -ne 0) { throw "Old helper relauncher failed: $($relaunchProcess.ExitCode)" }
        } finally { $relaunchProcess.Dispose() }
      }
      Start-Sleep -Milliseconds 100
    }
    & (Join-Path $PSScriptRoot 'test-windows-startup-recovery.ps1') -InstallRoot $install -ExpectedVersion $ExpectedVersion `
      -DataHome $homePath -EvidenceDirectory (Join-Path $root 'after') -ArtifactPath $installer | Out-Host
    if (-not $?) { throw 'Updated client failed startup, repeat launch or normal exit' }
    if ((Get-FileHash -Algorithm SHA256 -LiteralPath $marker).Hash -ne $markerHash) { throw 'Automatic update modified old user data' }
    if ((Get-FileHash -Algorithm SHA256 -LiteralPath $installer).Hash.ToLowerInvariant() -ne $targetHash) { throw 'Candidate bytes changed during upgrade' }
    if ($mode -eq 'installed') {
      $null = Assert-InstalledIdentity -InstallRoot $install -ExpectedVersion $ExpectedVersion
      $uninstaller = Join-Path $install 'uninstall.exe'
      $process = Start-Process -FilePath $uninstaller -ArgumentList '/S' -PassThru -Wait
      if ($process.ExitCode -ne 0) { throw 'Upgraded installation failed to uninstall' }
      Assert-UninstalledState -InstallRoot $install -DataSnapshot @{ $marker = $markerHash }
    }
    $summary[$mode] = 'passed'
    $summary[$mode + 'SourceSHA256'] = $published[$Architecture][$mode]
    $summary[$mode + 'HelperSHA256'] = $helperHash
  } finally {
    Restore-WindowsAcceptanceEnvironment -Snapshot $snapshot
  }
}
$summary | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $evidence 'legacy-update.json') -Encoding utf8
Write-Host 'Published v0.0.7 installed and portable clients upgraded using their own signed helper; target startup and retained data passed.'
