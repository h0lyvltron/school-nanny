# Trust School Nanny mkcert root CA on Windows (Track 2 C4).
# Run in an elevated PowerShell (Run as administrator):
#   .\scripts\trust-mkcert-windows.ps1 -RootCAPath .\rootCA.pem
#
# Or double-click after placing rootCA.pem next to this script.

param(
    [Parameter(Mandatory = $false)]
    [string]$RootCAPath = ""
)

$ErrorActionPreference = "Stop"

if (-not $RootCAPath) {
    $RootCAPath = Join-Path $PSScriptRoot "rootCA.pem"
}
if (-not (Test-Path -LiteralPath $RootCAPath)) {
    $RootCAPath = Join-Path (Get-Location) "rootCA.pem"
}
if (-not (Test-Path -LiteralPath $RootCAPath)) {
    Write-Error "rootCA.pem not found. Copy it from the Linux host (~/.local/share/school-nanny/certs/rootCA.pem) and pass -RootCAPath."
}

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "Run this script as Administrator so the CA can go into Trusted Root Certification Authorities."
}

$full = (Resolve-Path -LiteralPath $RootCAPath).Path
Write-Host "Importing $full into LocalMachine\Root ..."
Import-Certificate -FilePath $full -CertStoreLocation Cert:\LocalMachine\Root | Out-Null
Write-Host "Done. Restart Edge/Chrome, then open https://school-nanny.home (after Coolify is up)."
Write-Host "If you still see a warning, confirm DNS resolves and the proxy is serving the matching leaf cert."
