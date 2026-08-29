#!/usr/bin/env bash
# Builds the portable Windows folder to copy to her computer.
#
# Produces dist/windows/ containing the program, a double-click starter, and a
# short note for whoever opens the folder. Nothing else needs to be installed
# on the Windows machine.
set -euo pipefail

cd "$(dirname "$0")/.."

if [ -x ".toolchain/go/bin/go" ] && ! command -v go >/dev/null 2>&1; then
    export GOROOT="$PWD/.toolchain/go"
    export PATH="$GOROOT/bin:$PATH"
fi

if ! command -v go >/dev/null 2>&1; then
    echo "Go was not found. Run ./scripts/setup.sh first."
    exit 1
fi

OUT=dist/windows
rm -rf "$OUT"
mkdir -p "$OUT"

echo "Building school-nanny.exe ..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$OUT/school-nanny.exe" .

# Windows batch files want CRLF line endings.
write_crlf() {
    sed 's/$/\r/' > "$1"
}

write_crlf "$OUT/Start School Nanny.bat" <<'BAT'
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
BAT

write_crlf "$OUT/README.txt" <<'TXT'
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

If Windows warns about an unrecognized app:
    Click "More info", then "Run anyway". The program was built at home and
    is not signed by a company, which is what triggers the warning.
TXT

echo
echo "Ready: $OUT"
ls -la "$OUT"
echo
echo "Copy that folder to her Windows PC and double-click \"Start School Nanny.bat\"."
