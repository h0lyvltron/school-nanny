# Builds School Nanny on Windows and leaves a ready-to-copy folder in dist\windows.
$ErrorActionPreference = 'Stop'

Set-Location (Join-Path $PSScriptRoot '..')

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error 'Go was not found. Run .\scripts\setup.ps1 first.'
}

$out = 'dist\windows'
New-Item -ItemType Directory -Force -Path $out | Out-Null

# The folder is never cleared, because the launcher runs the app from here and
# so the family's "data" folder lives here too: emptying the folder would
# delete their records.
Write-Host 'Building school-nanny.exe ...'
$env:CGO_ENABLED = '0'
& go build -trimpath -o (Join-Path $out 'school-nanny.exe') .
if ($LASTEXITCODE -ne 0) {
    Write-Error 'Build failed. If School Nanny is open, close its window and run this again.'
}

$tools = Join-Path $out 'tools'
$toolsReadme = @'
TOC to curriculum YAML
======================

Copy the book's table of contents, paste it into a plain text file, then run:

    toc2yaml -name "Grade 1 Math" -subject math -in toc.txt -out grade1-math.yaml

Import that YAML on the Curriculum page in School Nanny. Minutes can be filled
in per lesson after import. You do not need Odin installed to run this program.
'@

if (Get-Command odin -ErrorAction SilentlyContinue) {
    New-Item -ItemType Directory -Force -Path $tools | Out-Null
    Write-Host 'Building toc2yaml.exe ...'
    & odin build tools/toc2yaml -target:windows_amd64 -out:(Join-Path $tools 'toc2yaml.exe')
    if ($LASTEXITCODE -ne 0) {
        Write-Error 'Building toc2yaml.exe failed.'
    }
    Set-Content -Path (Join-Path $tools 'README.txt') -Value $toolsReadme -Encoding ASCII
} else {
    Write-Warning 'odin was not found; skipping dist\windows\tools\toc2yaml.exe'
}

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
    Not in this folder. It is kept safely out of the way, so that replacing
    the program can never disturb it. The Settings page inside the app shows
    you the exact folder, and so does the black window when it starts.

To back it up:
    The app saves a backup of your records every day by itself, and the
    Settings page can put things back to any of them.

    For a copy that would survive this computer dying, open Settings and
    copy the folder it names onto a USB stick or into a cloud folder. That
    holds everything, including the files you have attached.

To install a newer version:
    Replace school-nanny.exe with the new one. Nothing else to do, and
    nothing here to preserve.
'@

Set-Content -Path (Join-Path $out 'Start School Nanny.bat') -Value $bat -Encoding ASCII
Set-Content -Path (Join-Path $out 'README.txt') -Value $readme -Encoding ASCII

Write-Host ''
Write-Host "Ready: $out"
Get-ChildItem $out

$data = Join-Path $out 'data'
if (Test-Path $data) {
    Write-Host ''
    Write-Host "Kept the existing records in $data"
}
