# Builds School Nanny on Windows and leaves a ready-to-copy folder in dist\windows.
$ErrorActionPreference = 'Stop'

Set-Location (Join-Path $PSScriptRoot '..')

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error 'Go was not found. Run .\scripts\setup.ps1 first.'
}

$out = 'dist\windows'
if (Test-Path $out) { Remove-Item $out -Recurse -Force }
New-Item -ItemType Directory -Force -Path $out | Out-Null

Write-Host 'Building school-nanny.exe ...'
$env:CGO_ENABLED = '0'
& go build -trimpath -o (Join-Path $out 'school-nanny.exe') .
if ($LASTEXITCODE -ne 0) { Write-Error 'Build failed.' }

$bat = @'
@echo off
cd /d "%~dp0"
title School Nanny
echo.
echo   Starting School Nanny...
echo   Your browser will open in a moment.
echo.
echo   Keep this window open while you use the app.
echo   Closing this window closes School Nanny.
echo.
school-nanny.exe -open
echo.
echo   School Nanny has stopped.
pause
'@

$readme = @'
School Nanny
============

To start it:
    Double-click "Start School Nanny.bat".
    A black window opens and your browser goes to the app.
    Leave the black window open while you are using it.

To stop it:
    Close the black window.

Where your information lives:
    The "data" folder next to this file. It holds the database and every
    file you have attached.

To back it up:
    Copy the "data" folder somewhere safe (a USB stick or cloud folder).
    That is the whole backup.

To install a newer version:
    Replace school-nanny.exe with the new one. Keep the "data" folder.
'@

Set-Content -Path (Join-Path $out 'Start School Nanny.bat') -Value $bat -Encoding ASCII
Set-Content -Path (Join-Path $out 'README.txt') -Value $readme -Encoding ASCII

Write-Host ''
Write-Host "Ready: $out"
Get-ChildItem $out
