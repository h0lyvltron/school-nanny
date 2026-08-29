# Gets a Windows machine ready to build School Nanny.
#
# Only needed if you want to build on Windows itself. The usual path is to run
# scripts/build-windows.sh on the Linux machine and copy dist\windows across.
#
# Run in PowerShell:  .\scripts\setup.ps1
$ErrorActionPreference = 'Stop'

Set-Location (Join-Path $PSScriptRoot '..')

$goMinMinor = 25

function Get-GoMinor {
    $go = Get-Command go -ErrorAction SilentlyContinue
    if (-not $go) { return $null }
    $version = (& go env GOVERSION) 2>$null
    if ($version -match '^go1\.(\d+)') { return [int]$Matches[1] }
    return $null
}

$minor = Get-GoMinor
if ($null -ne $minor -and $minor -ge $goMinMinor) {
    Write-Host "Using $(& go version)"
} else {
    Write-Host "Go 1.$goMinMinor or newer was not found. Installing it with winget ..."
    if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
        Write-Error "winget is not available. Install Go by hand from https://go.dev/dl/ and run this again."
    }
    winget install --id GoLang.Go --accept-source-agreements --accept-package-agreements

    # winget does not refresh PATH for the running shell.
    $env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' +
                [Environment]::GetEnvironmentVariable('Path', 'User')

    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Error "Go was installed but is not on PATH yet. Close PowerShell, open it again, and re-run this script."
    }
    Write-Host "Installed $(& go version)"
}

Write-Host 'Downloading Go dependencies ...'
& go mod download
if ($LASTEXITCODE -ne 0) { Write-Error 'go mod download failed.' }

# HTMX and the stylesheet ship with the repo; re-fetch only if missing.
function Get-Asset($path, $url) {
    if ((Test-Path $path) -and ((Get-Item $path).Length -gt 0)) { return }
    Write-Host "Fetching $(Split-Path $path -Leaf) ..."
    Invoke-WebRequest -Uri $url -OutFile $path
}

New-Item -ItemType Directory -Force -Path 'static' | Out-Null
Get-Asset 'static\htmx.min.js'  'https://unpkg.com/htmx.org@2.0.7/dist/htmx.min.js'
Get-Asset 'static\pico.min.css' 'https://cdn.jsdelivr.net/npm/@picocss/pico@2.1.1/css/pico.min.css'

Write-Host ''
Write-Host 'Setup complete.'
Write-Host '  Run it:    .\scripts\run.ps1'
Write-Host '  Build it:  .\scripts\build.ps1'
