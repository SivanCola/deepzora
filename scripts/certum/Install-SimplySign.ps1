param([string]$InstallerPath = 'SimplySignDesktop.msi')
$ErrorActionPreference = 'Stop'
if (-not (Test-Path -LiteralPath $InstallerPath)) {
    Invoke-WebRequest 'https://files.certum.eu/software/SimplySignDesktop/Windows/9.4.3.90/SimplySignDesktop-9.4.3.90-64-bit-en.msi' -OutFile $InstallerPath -TimeoutSec 90
}
if ((Get-FileHash -Algorithm SHA256 -LiteralPath $InstallerPath).Hash -ne 'CE2B38EF0124A574BC08E5558726099CAAC79A7C58B8D3D712F629371FE1BCB7') {
    throw 'SimplySign installer does not match the pinned 9.4.3.90 build.'
}
$signature = Get-AuthenticodeSignature -LiteralPath $InstallerPath
if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch 'Asseco Data Systems') {
    throw 'SimplySign installer publisher signature is not trusted.'
}
$process = Start-Process msiexec.exe -ArgumentList @('/i', ('"' + (Resolve-Path $InstallerPath).Path + '"'), '/quiet', '/norestart', 'ALLUSERS=1', 'REBOOT=ReallySuppress') -PassThru
$null = $process.Handle
if (-not $process.WaitForExit(300000)) {
    $process | Stop-Process -Force
    throw 'SimplySign installation did not finish within five minutes.'
}
if ($process.ExitCode -ne 0) { throw "SimplySign installation failed: $($process.ExitCode)" }
$installPath = Join-Path $env:ProgramFiles 'Certum\SimplySign Desktop'
if (-not (Test-Path -LiteralPath (Join-Path $installPath 'SimplySignDesktop.exe'))) {
    throw 'Verified SimplySign executable is missing after installation.'
}
$registry = 'HKCU:\Software\Certum\SimplySign'
New-Item -Path $registry -Force | Out-Null
# Same settings as the pinned upstream action; no credentials are stored here.
$settings = @{ ShowLoginDialogOnStart = 1; ShowLoginDialogOnAppRequest = 1; RememberLastUserName = 1; Autostart = 0; UnregisterCertificatesOnDisconnect = 0; RememberPINinCSP = 1; ForgetPINinCSPonDisconnect = 1; LangID = 9 }
foreach ($name in $settings.Keys) {
    New-ItemProperty -Path $registry -Name $name -Value $settings[$name] -PropertyType DWord -Force | Out-Null
}
$env:SS_PATH = $installPath
if ($env:GITHUB_ENV) { "SS_PATH=$installPath" >> $env:GITHUB_ENV }
Write-Host 'Pinned SimplySign client installed and configured.'
