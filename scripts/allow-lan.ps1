# Opens Windows Firewall so a tablet or phone on the home Wi-Fi can reach
# School Nanny when it is started with -lan.
#
# Double-click "Allow tablet access.bat" from the Windows package, or run:
#   .\allow-lan.ps1
#   .\allow-lan.ps1 -Port 8080
#   .\allow-lan.ps1 -Remove
#   .\allow-lan.ps1 -Diagnose
#
# Home Wi-Fi is often still marked Public. A port allow-rule alone is not
# enough if that profile is in "block all incoming connections" mode, which
# ignores every allow rule — including this one. That matches "works with
# the firewall off, still blocked with it on."
param(
    [int]$Port = 8080,
    [switch]$Remove,
    [switch]$Diagnose
)

$ErrorActionPreference = 'Stop'
$RuleName = 'School Nanny (LAN)'
$RuleId = 'SchoolNannyLAN'

function Wait-Enter {
    Write-Host ''
    Read-Host 'Press Enter to close'
}

function Test-IsAdmin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Get-ActiveProfiles {
    return @(Get-NetFirewallProfile -PolicyStore ActiveStore)
}

function Show-FirewallState {
    Write-Host 'Active firewall profiles:'
    foreach ($p in Get-ActiveProfiles) {
        $rulesOk = $p.AllowInboundRules
        $note = ''
        if ("$rulesOk" -eq 'False') {
            $note = '  ← allow rules are IGNORED (this blocks tablets)'
        }
        Write-Host ("  {0,-8} Enabled={1,-5} DefaultInbound={2,-6} AllowInboundRules={3}{4}" -f `
            $p.Name, $p.Enabled, $p.DefaultInboundAction, $rulesOk, $note)
    }

    Write-Host ''
    Write-Host 'Networks Windows thinks you are on:'
    Get-NetConnectionProfile | ForEach-Object {
        Write-Host ("  {0}  →  {1}" -f $_.Name, $_.NetworkCategory)
    }

    $rule = Get-NetFirewallRule -Name $RuleId -ErrorAction SilentlyContinue
    if (-not $rule) {
        $rule = Get-NetFirewallRule -DisplayName $RuleName -ErrorAction SilentlyContinue |
            Select-Object -First 1
    }
    Write-Host ''
    if (-not $rule) {
        Write-Host "No `"$RuleName`" firewall rule is present."
        return
    }
    $port = $rule | Get-NetFirewallPortFilter
    $addr = $rule | Get-NetFirewallAddressFilter
    Write-Host "Rule `"$($rule.DisplayName)`":"
    Write-Host ("  Enabled={0} Action={1} Direction={2} Profile={3}" -f `
        $rule.Enabled, $rule.Action, $rule.Direction, $rule.Profile)
    Write-Host ("  Protocol={0} LocalPort={1} RemoteAddress={2}" -f `
        $port.Protocol, ($port.LocalPort -join ','), ($addr.RemoteAddress -join ','))
}

function Repair-InboundAllowRules {
    # "Block all incoming connections, including those in the list of allowed
    # apps" sets AllowInboundRules=False. Our port rule then does nothing.
    $fixed = @()
    foreach ($p in Get-ActiveProfiles) {
        if ("$($p.AllowInboundRules)" -ne 'False') {
            continue
        }
        Set-NetFirewallProfile -Profile $p.Name -AllowInboundRules True
        $fixed += $p.Name
    }
    if ($fixed.Count -gt 0) {
        Write-Host ("Turned off 'block all incoming' on: {0}" -f ($fixed -join ', '))
        Write-Host 'Allow rules (including School Nanny) can take effect again.'
    }
}

function Get-SchoolNannyRules {
    $byId = @(Get-NetFirewallRule -Name $RuleId -ErrorAction SilentlyContinue)
    if ($byId.Count -gt 0) {
        return $byId
    }
    return @(Get-NetFirewallRule -DisplayName $RuleName -ErrorAction SilentlyContinue)
}

if (-not (Test-IsAdmin)) {
    $argList = @(
        '-NoProfile'
        '-ExecutionPolicy', 'Bypass'
        '-File', $PSCommandPath
    )
    if ($Port -ne 8080) { $argList += @('-Port', "$Port") }
    if ($Remove) { $argList += '-Remove' }
    if ($Diagnose) { $argList += '-Diagnose' }
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

if ($Diagnose) {
    Show-FirewallState
    Wait-Enter
    exit 0
}

$existing = Get-SchoolNannyRules

if ($Remove) {
    if ($existing.Count -eq 0) {
        Write-Host "No firewall rule named `"$RuleName`" was found. Nothing to remove."
        Wait-Enter
        exit 0
    }
    $existing | Remove-NetFirewallRule
    Write-Host "Removed `"$RuleName`"."
    Wait-Enter
    exit 0
}

Repair-InboundAllowRules

if ($existing.Count -gt 0) {
    $existing | Remove-NetFirewallRule
}

New-NetFirewallRule `
    -Name $RuleId `
    -DisplayName $RuleName `
    -Description "Lets tablets and phones on the home network open School Nanny (TCP $Port) when it is started with -lan." `
    -Direction Inbound `
    -Action Allow `
    -Protocol TCP `
    -LocalPort $Port `
    -Profile Any `
    -Program Any `
    -Enabled True | Out-Null

Write-Host ''
Show-FirewallState
Write-Host ''
Write-Host "Allowed inbound TCP $Port on every network profile (`"$RuleName`")."
Write-Host "Start School Nanny with -lan (or `"Start on Home Network.bat`"), then open the printed address on the tablet."
Write-Host "Set a family password in Settings before leaving it reachable on Wi-Fi."
Write-Host ''
Write-Host "To check later:  .\allow-lan.ps1 -Diagnose"
Write-Host "To undo later:   .\allow-lan.ps1 -Remove"
Wait-Enter
