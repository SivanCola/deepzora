# Read only control roles; never read field values or capture authentication UI.
function Get-CertumWindowObservation {
    param([IntPtr]$Handle)
    if (-not ('CertumControlRoles' -as [type])) {
        Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;
public static class CertumControlRoles {
    public sealed class Control { public IntPtr Handle; public string ClassName; public string ButtonName; }
    private delegate bool EnumProc(IntPtr handle, IntPtr parameter);
    [DllImport("user32.dll")] private static extern bool EnumChildWindows(IntPtr parent, EnumProc callback, IntPtr parameter);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)] private static extern int GetClassName(IntPtr handle, StringBuilder text, int count);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)] private static extern int GetWindowText(IntPtr handle, StringBuilder text, int count);
    public static Control[] Read(IntPtr parent) {
        var controls = new List<Control>();
        EnumChildWindows(parent, (handle, parameter) => {
            var name = new StringBuilder(256);
            GetClassName(handle, name, name.Capacity);
            var control = new Control { Handle = handle, ClassName = name.ToString() };
            // Never read edit text. Only acknowledgement button labels matter.
            if (control.ClassName == "Button") {
                var label = new StringBuilder(128);
                GetWindowText(handle, label, label.Capacity);
                control.ButtonName = label.ToString();
            }
            controls.Add(control);
            return true;
        }, IntPtr.Zero);
        return controls.ToArray();
    }
}
'@
    }
    try {
        $controls = @([CertumControlRoles]::Read($Handle))
        $edits = @($controls | Where-Object { $_.ClassName -eq 'Edit' })
        $names = @($controls | Where-Object { $_.ClassName -eq 'Button' } | ForEach-Object { ($_.ButtonName -replace '&', '').Trim() } |
            Where-Object { $_ -in @('OK', 'No', 'Cancel') })
        return [pscustomobject]@{ Handle = $Handle; EditCount = $edits.Count; Buttons = $names }
    } catch {
        # A window may disappear while Qt constructs or replaces its controls.
        return $null
    }
}

function Select-CertumLoginWindow {
    param([object[]]$Observations)
    $forms = @($Observations | Where-Object { $null -ne $_ -and $_.EditCount -eq 2 -and $_.Buttons -contains 'OK' })
    if ($forms.Count -gt 1) { throw 'Multiple SimplySign credential forms; refusing ambiguous input.' }
    if ($forms.Count -eq 1) { return [IntPtr]$forms[0].Handle }
    return [IntPtr]::Zero
}

function Wait-CertumLoginWindow {
    param(
        [scriptblock]$EnumerateWindows = { Get-SimplySignWindows },
        [scriptblock]$ObserveWindow = { param($handle) Get-CertumWindowObservation -Handle $handle },
        [scriptblock]$StillRunning = { $proc.Refresh(); -not $proc.HasExited },
        [int]$MaxPolls = 120,
        [int]$PollMilliseconds = 250
    )
    $lastShape = $null
    for ($poll = 0; $poll -lt $MaxPolls; $poll++) {
        if (-not (& $StillRunning)) { throw 'SimplySign exited before its credential form was ready.' }
        $observations = @(& $EnumerateWindows | ForEach-Object { & $ObserveWindow $_ })
        $shape = (@($observations | Where-Object { $null -ne $_ } | ForEach-Object {
            "handle=$($_.Handle) edits=$($_.EditCount) acknowledgementButtons=$($_.Buttons.Count)"
        }) -join '; ')
        if ($shape -ne $lastShape) {
            Write-Host "SimplySign control roles: $shape"
            $lastShape = $shape
        }
        $handle = Select-CertumLoginWindow -Observations $observations
        if ($handle -ne [IntPtr]::Zero) {
            Write-Host 'SimplySign credential form is ready (two edit controls and OK); splash windows were ignored.'
            return $handle
        }
        if ($PollMilliseconds -gt 0) { Start-Sleep -Milliseconds $PollMilliseconds }
    }
    throw 'SimplySign did not expose an unambiguous credential form; no credentials were submitted.'
}

function Get-CertumPopupWindows {
    param([IntPtr]$LoginHandle)
    foreach ($handle in @(Get-SimplySignWindows)) {
        if ([IntPtr]$handle -eq $LoginHandle) { continue }
        $observation = Get-CertumWindowObservation -Handle $handle
        # Background/splash windows are not acknowledgement dialogs. In
        # particular, never press Enter into a second credential form.
        if ($null -ne $observation -and $observation.EditCount -eq 0 -and $observation.Buttons.Count -gt 0) {
            $handle
        }
    }
}
