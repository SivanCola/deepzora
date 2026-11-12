$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'certum-login-ui.ps1')
function Assert-Equal($actual, $expected, $message) {
    if ($actual -ne $expected) { throw $message }
}
function Assert-Rejected([scriptblock]$action) {
    $rejected = $false
    try { & $action | Out-Null } catch { $rejected = $true }
    if (-not $rejected) { throw 'Expected fail-closed rejection.' }
}
$splash = [pscustomobject]@{ Handle = 1; EditCount = 0; Buttons = @() }
$login = [pscustomobject]@{ Handle = 2; EditCount = 2; Buttons = @('OK', 'Cancel') }
$update = [pscustomobject]@{ Handle = 3; EditCount = 0; Buttons = @('No') }
Assert-Equal (Select-CertumLoginWindow @($splash)) ([IntPtr]::Zero) 'A splash screen was mistaken for a login form.'
Assert-Equal (Select-CertumLoginWindow @($splash, $update, $login)) ([IntPtr]2) 'Control roles did not select the login form.'
Assert-Rejected { Select-CertumLoginWindow @($login, $login) }

# Reproduce the recorded ordering: a splash exists first, it disappears, and
# only then does the actual credential form become visible.
$script:poll = 0
$handle = Wait-CertumLoginWindow -EnumerateWindows {
    $script:poll++
    if ($script:poll -eq 1) { 1 }
    if ($script:poll -eq 3) { 1; 2; 3 }
} -ObserveWindow {
    param($value)
    if ($value -eq 1) { $splash }
    if ($value -eq 2) { $login }
    if ($value -eq 3) { $update }
} -StillRunning { $true } -MaxPolls 3 -PollMilliseconds 0
Assert-Equal $handle ([IntPtr]2) 'Readiness returned before credential controls existed.'
Assert-Equal $script:poll 3 'The transient splash window ended the wait early.'
Assert-Rejected { Wait-CertumLoginWindow -EnumerateWindows { 1 } -ObserveWindow { $splash } -StillRunning { $true } -MaxPolls 2 -PollMilliseconds 0 }
Assert-Rejected { Wait-CertumLoginWindow -StillRunning { $false } -PollMilliseconds 0 }

function Get-SimplySignWindows { 1; 2; 3; 4 }
function Get-CertumWindowObservation($Handle) {
    if ($Handle -eq 1) { return $splash }
    if ($Handle -eq 2) { return $login }
    if ($Handle -eq 3) { return $update }
    return [pscustomobject]@{ Handle = 4; EditCount = 2; Buttons = @('OK') }
}
$popups = @(Get-CertumPopupWindows -LoginHandle 2)
Assert-Equal $popups.Count 1 'Splash or credential windows were treated as popups.'
Assert-Equal $popups[0] ([IntPtr]3) 'The acknowledgement dialog was not selected.'
Write-Host 'Certum credential form readiness and popup isolation: PASS'
