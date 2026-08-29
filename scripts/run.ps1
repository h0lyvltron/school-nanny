# Runs School Nanny from source on Windows.
# Extra arguments pass straight through, for example:  .\scripts\run.ps1 -lan
$ErrorActionPreference = 'Stop'

Set-Location (Join-Path $PSScriptRoot '..')

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error 'Go was not found. Run .\scripts\setup.ps1 first.'
}

& go run . @args
