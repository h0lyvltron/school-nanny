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
mkdir -p "$OUT"

# Only the three files written below belong to the build. The folder is never
# cleared, because the launcher runs the app from here and so a "data" folder
# can live here too: emptying the folder would delete the family's records.
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

If Windows warns about an unrecognized app:
    Click "More info", then "Run anyway". The program was built at home and
    is not signed by a company, which is what triggers the warning.
TXT

echo
echo "Ready: $OUT"
ls -la "$OUT"
if [ -d "$OUT/data" ]; then
    echo
    echo "Kept the existing records in $OUT/data"
fi
echo
echo "Copy that folder to her Windows PC and double-click \"Start School Nanny.bat\"."
