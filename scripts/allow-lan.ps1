# Opens Windows Firewall so a tablet or phone on the home Wi-Fi can reach
# School Nanny when it is started with -lan.
#
# Double-click "Allow tablet access.bat" from the Windows package, or run:
#   .\allow-lan.ps1
#   .\allow-lan.ps1 -Port 8080
#   .\allow-lan.ps1 -ExePath "D:\path\school-nanny.exe"
#   .\allow-lan.ps1 -Remove
#   .\allow-lan.ps1 -Diagnose
#
# Home Wi-Fi is often still marked Public. A port allow-rule alone is not
# enough if that profile is in "block all incoming connections" mode, which
# ignores every allow rule. Separately, a Block rule for school-nanny.exe
# (from Cancel on the Windows firewall prompt) overrides a port Allow —
# that matches "works with the firewall off, still blocked with it on"
# even when the School Nanny (LAN) allow rule looks perfect.
param(
    [int]$Port = 8080,
    [string]$ExePath = '',
    [switch]$Remove,
    [switch]$Diagnose
)

$ErrorActionPreference = 'Stop'
$RuleName = 'School Nanny (LAN)'
$RuleIdPort = 'SchoolNannyLAN'
$RuleIdApp = 'SchoolNannyLANApp'
$RuleNameApp = 'School Nanny (LAN app)'

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

function Resolve-SchoolNannyExe {
    param([string]$Hint)

    $candidates = @()
    if ($Hint) { $candidates += $Hint }
    if ($PSScriptRoot) {
        $candidates += (Join-Path $PSScriptRoot 'school-nanny.exe')
    }
    try {
        $conn = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue |
            Select-Object -First 1
        if ($conn -and $conn.OwningProcess) {
            $proc = Get-Process -Id $conn.OwningProcess -ErrorAction SilentlyContinue
            if ($proc -and $proc.Path) { $candidates += $proc.Path }
        }
    } catch {}

    foreach ($c in $candidates) {
        if ($c -and (Test-Path -LiteralPath $c)) {
            return (Resolve-Path -LiteralPath $c).Path
        }
    }
    return ''
}

function Get-RulesByIdsOrNames {
    $rules = @()
    foreach ($id in @($RuleIdPort, $RuleIdApp)) {
        $rules += @(Get-NetFirewallRule -Name $id -ErrorAction SilentlyContinue)
    }
    foreach ($name in @($RuleName, $RuleNameApp)) {
        $rules += @(Get-NetFirewallRule -DisplayName $name -ErrorAction SilentlyContinue)
    }
    return @($rules | Sort-Object -Property Name -Unique)
}

function Get-ConflictingBlockRules {
    param([string]$Exe)

    $blocks = @()
    $inboundBlocks = @(Get-NetFirewallRule -Direction Inbound -Action Block -Enabled True -ErrorAction SilentlyContinue)
    foreach ($rule in $inboundBlocks) {
        $port = $rule | Get-NetFirewallPortFilter -ErrorAction SilentlyContinue
        $app = $rule | Get-NetFirewallApplicationFilter -ErrorAction SilentlyContinue
        $hitPort = $false
        if ($port -and "$($port.Protocol)" -match 'TCP|Any') {
            $ports = @($port.LocalPort)
            if ($ports -contains 'Any' -or $ports -contains "$Port") {
                # "Any" port on a block for a specific app is still a conflict for that app.
                if ($ports -contains "$Port") { $hitPort = $true }
            }
        }
        $hitApp = $false
        if ($Exe -and $app -and $app.Program -and $app.Program -ne 'Any') {
            try {
                $hitApp = ([string]$app.Program).Equals($Exe, [StringComparison]::OrdinalIgnoreCase)
            } catch {
                $hitApp = ("$($app.Program)" -ieq $Exe)
            }
        }
        $nameHit = "$($rule.DisplayName) $($rule.Name)" -match 'school.?nanny'
        if ($hitPort -or $hitApp -or $nameHit) {
            $blocks += [pscustomobject]@{
                Rule = $rule
                Why  = @(
                    $(if ($hitPort) { "TCP $Port" }),
                    $(if ($hitApp) { "exe $($app.Program)" }),
                    $(if ($nameHit) { 'name mentions School Nanny' })
                ) -join ', '
            }
        }
    }
    return $blocks
}

function Disable-ConflictingBlockRules {
    param([string]$Exe)

    $conflicts = Get-ConflictingBlockRules -Exe $Exe
    foreach ($c in $conflicts) {
        # Never touch our own allow rules; only disable blockers.
        if ($c.Rule.Name -in @($RuleIdPort, $RuleIdApp)) { continue }
        Disable-NetFirewallRule -InputObject $c.Rule
        Write-Host ("Disabled blocking rule `"$($c.Rule.DisplayName)`" ({0})" -f $c.Why)
    }
    return $conflicts.Count
}

function Repair-InboundAllowRules {
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

function Show-RuleDetails {
    param($Rule, [string]$StoreLabel)

    if (-not $Rule) {
        Write-Host "  ($StoreLabel) not present"
        return
    }
    $port = $Rule | Get-NetFirewallPortFilter -ErrorAction SilentlyContinue
    $addr = $Rule | Get-NetFirewallAddressFilter -ErrorAction SilentlyContinue
    $app = $Rule | Get-NetFirewallApplicationFilter -ErrorAction SilentlyContinue
    Write-Host ("  ($StoreLabel) Enabled={0} Action={1} Direction={2} Profile={3}" -f `
        $Rule.Enabled, $Rule.Action, $Rule.Direction, $Rule.Profile)
    if ($port) {
        Write-Host ("             Protocol={0} LocalPort={1}" -f $port.Protocol, ($port.LocalPort -join ','))
    }
    if ($addr) {
        Write-Host ("             RemoteAddress={0}" -f ($addr.RemoteAddress -join ','))
    }
    if ($app -and $app.Program) {
        Write-Host ("             Program={0}" -f $app.Program)
    }
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

    $exe = Resolve-SchoolNannyExe -Hint $ExePath
    Write-Host ''
    if ($exe) {
        Write-Host "school-nanny.exe: $exe"
    } else {
        Write-Host 'school-nanny.exe: not found beside this script and nothing is listening on the port'
    }

    Write-Host ''
    Write-Host 'Listeners on the School Nanny port:'
    try {
        $listeners = @(Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue)
        if ($listeners.Count -eq 0) {
            Write-Host "  nothing listening on TCP $Port (start Start on Home Network.bat first)"
        } else {
            foreach ($l in $listeners) {
                $proc = Get-Process -Id $l.OwningProcess -ErrorAction SilentlyContinue
                Write-Host ("  {0}:{1}  pid {2}  {3}" -f $l.LocalAddress, $l.LocalPort, $l.OwningProcess, $(if ($proc) { $proc.ProcessName } else { '?' }))
            }
        }
    } catch {
        Write-Host "  (could not query listeners: $_)"
    }

    Write-Host ''
    Write-Host "Rule `"$RuleName`" (port):"
    Show-RuleDetails (Get-NetFirewallRule -Name $RuleIdPort -ErrorAction SilentlyContinue | Select-Object -First 1) 'PersistentStore'
    Show-RuleDetails (Get-NetFirewallRule -Name $RuleIdPort -PolicyStore ActiveStore -ErrorAction SilentlyContinue | Select-Object -First 1) 'ActiveStore'

    Write-Host ''
    Write-Host "Rule `"$RuleNameApp`" (app):"
    Show-RuleDetails (Get-NetFirewallRule -Name $RuleIdApp -ErrorAction SilentlyContinue | Select-Object -First 1) 'PersistentStore'
    Show-RuleDetails (Get-NetFirewallRule -Name $RuleIdApp -PolicyStore ActiveStore -ErrorAction SilentlyContinue | Select-Object -First 1) 'ActiveStore'

    Write-Host ''
    $conflicts = Get-ConflictingBlockRules -Exe $exe
    if ($conflicts.Count -eq 0) {
        Write-Host 'No enabled inbound Block rules look like they target School Nanny or this port.'
    } else {
        Write-Host 'Enabled inbound Block rules that may override the allow rule:'
        foreach ($c in $conflicts) {
            Write-Host ("  `"$($c.Rule.DisplayName)`" [{0}] — {1}" -f $c.Rule.Name, $c.Why)
        }
        Write-Host 'Re-run without -Diagnose to disable these blockers and refresh the allow rules.'
    }
}

if (-not (Test-IsAdmin)) {
    $argList = @(
        '-NoProfile'
        '-ExecutionPolicy', 'Bypass'
        '-File', $PSCommandPath
    )
    if ($Port -ne 8080) { $argList += @('-Port', "$Port") }
    if ($ExePath) { $argList += @('-ExePath', $ExePath) }
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

$existing = Get-RulesByIdsOrNames
$exe = Resolve-SchoolNannyExe -Hint $ExePath

if ($Remove) {
    if ($existing.Count -eq 0) {
        Write-Host "No School Nanny firewall rules were found. Nothing to remove."
        Wait-Enter
        exit 0
    }
    $existing | Remove-NetFirewallRule
    Write-Host 'Removed School Nanny firewall allow rules.'
    Wait-Enter
    exit 0
}

Repair-InboundAllowRules
$null = Disable-ConflictingBlockRules -Exe $exe

if ($existing.Count -gt 0) {
    $existing | Remove-NetFirewallRule
}

New-NetFirewallRule `
    -Name $RuleIdPort `
    -DisplayName $RuleName `
    -Description "Lets tablets and phones on the home network open School Nanny (TCP $Port) when it is started with -lan." `
    -Direction Inbound `
    -Action Allow `
    -Protocol TCP `
    -LocalPort "$Port" `
    -Profile Any `
    -Program Any `
    -Enabled True | Out-Null

if ($exe) {
    New-NetFirewallRule `
        -Name $RuleIdApp `
        -DisplayName $RuleNameApp `
        -Description "Lets School Nanny accept home-network connections (program allow; overrides a prior Cancel on the Windows firewall prompt)." `
        -Direction Inbound `
        -Action Allow `
        -Program $exe `
        -Profile Any `
        -Enabled True | Out-Null
    Write-Host "Also allowed program: $exe"
} else {
    Write-Host "Could not find school-nanny.exe beside this script; port rule only."
    Write-Host "Put allow-lan.ps1 next to school-nanny.exe, or pass -ExePath, then run again."
}

Write-Host ''
Show-FirewallState
Write-Host ''
Write-Host "Allowed inbound TCP $Port (and the app, when found)."
Write-Host "Start School Nanny with -lan (or `"Start on Home Network.bat`"), then open the printed address on the tablet."
Write-Host "If Windows pops up a firewall prompt for school-nanny.exe, choose Allow on private AND public networks."
Write-Host ''
Write-Host "To check later:  .\allow-lan.ps1 -Diagnose"
Write-Host "To undo later:   .\allow-lan.ps1 -Remove"
Wait-Enter
