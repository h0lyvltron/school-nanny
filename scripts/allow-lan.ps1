# Opens Windows Firewall so a tablet or phone on the home Wi-Fi can reach
# School Nanny when it is started with -lan.
#
# Double-click "Allow tablet access.bat" from the Windows package, or run:
#   .\allow-lan.ps1
#   .\allow-lan.ps1 -Port 8080
#   .\allow-lan.ps1 -Remove
#
# The rule is added for Private and Public profiles on purpose: many home
# Wi-Fi connections stay marked Public, and a Private-only rule will not fire.
param(
    [int]$Port = 8080,
    [switch]$Remove
)

$ErrorActionPreference = 'Stop'
$RuleName = 'School Nanny (LAN)'

function Wait-Enter {
    Write-Host ''
    Read-Host 'Press Enter to close'
}

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    $argList = @(
        '-NoProfile'
        '-ExecutionPolicy', 'Bypass'
        '-File', $PSCommandPath
    )
    if ($Port -ne 8080) { $argList += @('-Port', "$Port") }
    if ($Remove) { $argList += '-Remove' }
    try {
        $proc = Start-Process -FilePath powershell.exe -Verb RunAs -ArgumentList $argList -Wait -PassThru
    } catch {
        Write-Error 'Administrator approval was cancelled or blocked.'
    }
    exit $proc.ExitCode
}

if ($Port -lt 1 -or $Port -gt 65535) {
    Write-Error "Port must be between 1 and 65535 (got $Port)."
}

$existing = Get-NetFirewallRule -DisplayName $RuleName -ErrorAction SilentlyContinue

if ($Remove) {
    if (-not $existing) {
        Write-Host "No firewall rule named `"$RuleName`" was found. Nothing to remove."
        Wait-Enter
        exit 0
    }
    $existing | Remove-NetFirewallRule
    Write-Host "Removed `"$RuleName`"."
    Wait-Enter
    exit 0
}

if ($existing) {
    $existing | Remove-NetFirewallRule
}

New-NetFirewallRule `
    -DisplayName $RuleName `
    -Description "Lets tablets and phones on the home network open School Nanny (TCP $Port) when it is started with -lan." `
    -Direction Inbound `
    -Action Allow `
    -Protocol TCP `
    -LocalPort $Port `
    -Profile Private,Public `
    -Program Any `
    -Enabled True | Out-Null

Write-Host "Allowed inbound TCP $Port for Private and Public networks (`"$RuleName`")."
Write-Host "Start School Nanny with -lan (or `"Start on Home Network.bat`"), then open the printed address on the tablet."
Write-Host "Set a family password in Settings before leaving it reachable on Wi-Fi."
Write-Host ''
Write-Host "To undo later:  .\allow-lan.ps1 -Remove"
Wait-Enter
