# Opens Windows Firewall so a tablet or phone on the home Wi-Fi can reach
# School Nanny when it is started with -lan.
#
# Requires Windows PowerShell 5.1+ (the built-in powershell.exe).
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
# (from Cancel on the Windows firewall prompt) overrides a port Allow.
param(
    [int]$Port = 8080,
    [string]$ExePath = '',
    [switch]$Remove,
    [switch]$Diagnose
)

# PS 5.1: keep this file ASCII-only (no BOM-less UTF-8 punctuation).
$ErrorActionPreference = 'Stop'
$RuleName = 'School Nanny (LAN)'
$RuleIdPort = 'SchoolNannyLAN'
$RuleIdApp = 'SchoolNannyLANApp'
$RuleNameApp = 'School Nanny (LAN app)'

function Wait-Enter {
    Write-Host ''
    [void](Read-Host 'Press Enter to close')
}

function Test-IsAdmin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Get-ActiveProfiles {
    @(Get-NetFirewallProfile -PolicyStore ActiveStore)
}

function Resolve-SchoolNannyExe {
    param([string]$Hint)

    $candidates = New-Object System.Collections.Generic.List[string]
    if (-not [string]::IsNullOrWhiteSpace($Hint)) {
        [void]$candidates.Add($Hint)
    }
    if ($PSScriptRoot) {
        [void]$candidates.Add((Join-Path $PSScriptRoot 'school-nanny.exe'))
    }
    try {
        $conn = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue |
            Select-Object -First 1
        if ($null -ne $conn -and $conn.OwningProcess) {
            $proc = Get-Process -Id $conn.OwningProcess -ErrorAction SilentlyContinue
            if ($null -ne $proc -and -not [string]::IsNullOrWhiteSpace($proc.Path)) {
                [void]$candidates.Add($proc.Path)
            }
        }
    } catch {
        # Get-NetTCPConnection is missing on some older builds; ignore.
    }

    foreach ($c in $candidates) {
        if (-not [string]::IsNullOrWhiteSpace($c) -and (Test-Path -LiteralPath $c)) {
            return (Resolve-Path -LiteralPath $c).Path
        }
    }
    return ''
}

function Get-RulesByIdsOrNames {
    $rules = @()
    foreach ($id in @($RuleIdPort, $RuleIdApp)) {
        $found = Get-NetFirewallRule -Name $id -ErrorAction SilentlyContinue
        if ($null -ne $found) {
            $rules += @($found)
        }
    }
    foreach ($name in @($RuleName, $RuleNameApp)) {
        $found = Get-NetFirewallRule -DisplayName $name -ErrorAction SilentlyContinue
        if ($null -ne $found) {
            $rules += @($found)
        }
    }
    if ($rules.Count -eq 0) {
        return @()
    }
    $rules | Sort-Object -Property Name -Unique
}

function Test-SamePath {
    param([string]$Left, [string]$Right)
    if ([string]::IsNullOrWhiteSpace($Left) -or [string]::IsNullOrWhiteSpace($Right)) {
        return $false
    }
    return ([string]::Equals($Left, $Right, [System.StringComparison]::OrdinalIgnoreCase))
}

function Get-ConflictingBlockRules {
    param([string]$Exe)

    $blocks = @()
    $inboundBlocks = @(Get-NetFirewallRule -Direction Inbound -Action Block -Enabled True -ErrorAction SilentlyContinue)
    foreach ($rule in $inboundBlocks) {
        if ($null -eq $rule) { continue }

        $portFilter = $rule | Get-NetFirewallPortFilter -ErrorAction SilentlyContinue
        $appFilter = $rule | Get-NetFirewallApplicationFilter -ErrorAction SilentlyContinue

        $hitPort = $false
        if ($null -ne $portFilter) {
            $proto = [string]$portFilter.Protocol
            if ($proto -eq 'TCP' -or $proto -eq 'Any') {
                foreach ($lp in @($portFilter.LocalPort)) {
                    if ([string]$lp -eq [string]$Port) {
                        $hitPort = $true
                        break
                    }
                }
            }
        }

        $hitApp = $false
        $appProgram = ''
        if ($null -ne $appFilter -and $null -ne $appFilter.Program) {
            $appProgram = [string]$appFilter.Program
            if ($appProgram -ne '' -and $appProgram -ne 'Any' -and (Test-SamePath $appProgram $Exe)) {
                $hitApp = $true
            }
        }

        $label = ([string]$rule.DisplayName) + ' ' + ([string]$rule.Name)
        $nameHit = $label -match 'school.?nanny'

        if ($hitPort -or $hitApp -or $nameHit) {
            $whyParts = New-Object System.Collections.Generic.List[string]
            if ($hitPort) { [void]$whyParts.Add("TCP $Port") }
            if ($hitApp) { [void]$whyParts.Add("exe $appProgram") }
            if ($nameHit) { [void]$whyParts.Add('name mentions School Nanny') }

            $blocks += New-Object psobject -Property @{
                Rule = $rule
                Why  = ($whyParts -join ', ')
            }
        }
    }
    return ,$blocks
}

function Disable-ConflictingBlockRules {
    param([string]$Exe)

    $conflicts = @(Get-ConflictingBlockRules -Exe $Exe)
    $count = 0
    foreach ($c in $conflicts) {
        if ($null -eq $c -or $null -eq $c.Rule) { continue }
        if ($c.Rule.Name -eq $RuleIdPort -or $c.Rule.Name -eq $RuleIdApp) { continue }
        Disable-NetFirewallRule -InputObject $c.Rule
        Write-Host ('Disabled blocking rule "{0}" ({1})' -f $c.Rule.DisplayName, $c.Why)
        $count++
    }
    return $count
}

function Repair-InboundAllowRules {
    $fixed = New-Object System.Collections.Generic.List[string]
    foreach ($p in Get-ActiveProfiles) {
        if ([string]$p.AllowInboundRules -ne 'False') {
            continue
        }
        Set-NetFirewallProfile -Profile $p.Name -AllowInboundRules True
        [void]$fixed.Add([string]$p.Name)
    }
    if ($fixed.Count -gt 0) {
        Write-Host ('Turned off "block all incoming" on: {0}' -f ($fixed -join ', '))
        Write-Host 'Allow rules (including School Nanny) can take effect again.'
    }
}

function Show-RuleDetails {
    param(
        $Rule,
        [string]$StoreLabel
    )

    if ($null -eq $Rule) {
        Write-Host ("  ({0}) not present" -f $StoreLabel)
        return
    }

    $portFilter = $Rule | Get-NetFirewallPortFilter -ErrorAction SilentlyContinue
    $addrFilter = $Rule | Get-NetFirewallAddressFilter -ErrorAction SilentlyContinue
    $appFilter = $Rule | Get-NetFirewallApplicationFilter -ErrorAction SilentlyContinue

    Write-Host ('  ({0}) Enabled={1} Action={2} Direction={3} Profile={4}' -f `
        $StoreLabel, $Rule.Enabled, $Rule.Action, $Rule.Direction, $Rule.Profile)

    if ($null -ne $portFilter) {
        Write-Host ('             Protocol={0} LocalPort={1}' -f `
            $portFilter.Protocol, (($portFilter.LocalPort | ForEach-Object { "$_" }) -join ','))
    }
    if ($null -ne $addrFilter) {
        Write-Host ('             RemoteAddress={0}' -f `
            (($addrFilter.RemoteAddress | ForEach-Object { "$_" }) -join ','))
    }
    if ($null -ne $appFilter -and $null -ne $appFilter.Program) {
        Write-Host ('             Program={0}' -f $appFilter.Program)
    }
}

function Show-FirewallState {
    Write-Host 'Active firewall profiles:'
    foreach ($p in Get-ActiveProfiles) {
        $rulesOk = $p.AllowInboundRules
        $note = ''
        if ([string]$rulesOk -eq 'False') {
            $note = '  << allow rules are IGNORED (this blocks tablets)'
        }
        Write-Host ('  {0,-8} Enabled={1,-5} DefaultInbound={2,-6} AllowInboundRules={3}{4}' -f `
            $p.Name, $p.Enabled, $p.DefaultInboundAction, $rulesOk, $note)
    }

    Write-Host ''
    Write-Host 'Networks Windows thinks you are on:'
    Get-NetConnectionProfile | ForEach-Object {
        Write-Host ('  {0}  ->  {1}' -f $_.Name, $_.NetworkCategory)
    }

    $exe = Resolve-SchoolNannyExe -Hint $ExePath
    Write-Host ''
    if (-not [string]::IsNullOrWhiteSpace($exe)) {
        Write-Host ("school-nanny.exe: $exe")
    } else {
        Write-Host 'school-nanny.exe: not found beside this script and nothing is listening on the port'
    }

    Write-Host ''
    Write-Host 'Listeners on the School Nanny port:'
    try {
        $listeners = @(Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue)
        if ($listeners.Count -eq 0) {
            Write-Host ("  nothing listening on TCP $Port (start Start on Home Network.bat first)")
        } else {
            foreach ($l in $listeners) {
                $proc = Get-Process -Id $l.OwningProcess -ErrorAction SilentlyContinue
                $procName = '?'
                if ($null -ne $proc) { $procName = $proc.ProcessName }
                Write-Host ('  {0}:{1}  pid {2}  {3}' -f $l.LocalAddress, $l.LocalPort, $l.OwningProcess, $procName)
            }
        }
    } catch {
        Write-Host ("  (could not query listeners: $($_.Exception.Message))")
    }

    Write-Host ''
    Write-Host ("Rule `"$RuleName`" (port):")
    $persistentPort = Get-NetFirewallRule -Name $RuleIdPort -ErrorAction SilentlyContinue | Select-Object -First 1
    $activePort = Get-NetFirewallRule -Name $RuleIdPort -PolicyStore ActiveStore -ErrorAction SilentlyContinue | Select-Object -First 1
    Show-RuleDetails $persistentPort 'PersistentStore'
    Show-RuleDetails $activePort 'ActiveStore'

    Write-Host ''
    Write-Host ("Rule `"$RuleNameApp`" (app):")
    $persistentApp = Get-NetFirewallRule -Name $RuleIdApp -ErrorAction SilentlyContinue | Select-Object -First 1
    $activeApp = Get-NetFirewallRule -Name $RuleIdApp -PolicyStore ActiveStore -ErrorAction SilentlyContinue | Select-Object -First 1
    Show-RuleDetails $persistentApp 'PersistentStore'
    Show-RuleDetails $activeApp 'ActiveStore'

    Write-Host ''
    $conflicts = @(Get-ConflictingBlockRules -Exe $exe)
    if ($conflicts.Count -eq 0) {
        Write-Host 'No enabled inbound Block rules look like they target School Nanny or this port.'
    } else {
        Write-Host 'Enabled inbound Block rules that may override the allow rule:'
        foreach ($c in $conflicts) {
            Write-Host ('  "{0}" [{1}] - {2}' -f $c.Rule.DisplayName, $c.Rule.Name, $c.Why)
        }
        Write-Host 'Re-run without -Diagnose to disable these blockers and refresh the allow rules.'
    }
}

function Start-ElevatedSelf {
    $argParts = New-Object System.Collections.Generic.List[string]
    [void]$argParts.Add('-NoProfile')
    [void]$argParts.Add('-ExecutionPolicy')
    [void]$argParts.Add('Bypass')
    [void]$argParts.Add('-File')
    [void]$argParts.Add(('"{0}"' -f $PSCommandPath))
    if ($Port -ne 8080) {
        [void]$argParts.Add('-Port')
        [void]$argParts.Add([string]$Port)
    }
    if (-not [string]::IsNullOrWhiteSpace($ExePath)) {
        [void]$argParts.Add('-ExePath')
        [void]$argParts.Add(('"{0}"' -f $ExePath))
    }
    if ($Remove) { [void]$argParts.Add('-Remove') }
    if ($Diagnose) { [void]$argParts.Add('-Diagnose') }

    # PS 5.1: pass ArgumentList as one string so paths with spaces survive.
    $argString = [string]::Join(' ', $argParts.ToArray())
    try {
        $proc = Start-Process -FilePath "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" `
            -Verb RunAs `
            -ArgumentList $argString `
            -Wait `
            -PassThru
    } catch {
        Write-Error 'Administrator approval was canceled or blocked.'
        exit 1
    }
    if ($null -eq $proc) {
        exit 1
    }
    exit $proc.ExitCode
}

if (-not (Test-IsAdmin)) {
    Start-ElevatedSelf
}

if ($Port -lt 1 -or $Port -gt 65535) {
    Write-Error "Port must be between 1 and 65535 (got $Port)."
}

if ($Diagnose) {
    Show-FirewallState
    Wait-Enter
    exit 0
}

$existing = @(Get-RulesByIdsOrNames)
$exe = Resolve-SchoolNannyExe -Hint $ExePath

if ($Remove) {
    if ($existing.Count -eq 0) {
        Write-Host 'No School Nanny firewall rules were found. Nothing to remove.'
        Wait-Enter
        exit 0
    }
    $existing | Remove-NetFirewallRule
    Write-Host 'Removed School Nanny firewall allow rules.'
    Wait-Enter
    exit 0
}

Repair-InboundAllowRules
[void](Disable-ConflictingBlockRules -Exe $exe)

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
    -LocalPort ([string]$Port) `
    -Profile Any `
    -Program Any `
    -Enabled True | Out-Null

if (-not [string]::IsNullOrWhiteSpace($exe)) {
    New-NetFirewallRule `
        -Name $RuleIdApp `
        -DisplayName $RuleNameApp `
        -Description 'Lets School Nanny accept home-network connections (program allow; overrides a prior Cancel on the Windows firewall prompt).' `
        -Direction Inbound `
        -Action Allow `
        -Program $exe `
        -Profile Any `
        -Enabled True | Out-Null
    Write-Host "Also allowed program: $exe"
} else {
    Write-Host 'Could not find school-nanny.exe beside this script; port rule only.'
    Write-Host 'Put allow-lan.ps1 next to school-nanny.exe, or pass -ExePath, then run again.'
}

Write-Host ''
Show-FirewallState
Write-Host ''
Write-Host ("Allowed inbound TCP $Port (and the app, when found).")
Write-Host 'Start School Nanny with -lan (or "Start on Home Network.bat"), then open the printed address on the tablet.'
Write-Host 'If Windows pops up a firewall prompt for school-nanny.exe, choose Allow on private AND public networks.'
Write-Host ''
Write-Host 'To check later:  .\allow-lan.ps1 -Diagnose'
Write-Host 'To undo later:   .\allow-lan.ps1 -Remove'
Wait-Enter
