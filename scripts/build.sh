#!/usr/bin/env bash
# Builds School Nanny for this machine.
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

CGO_ENABLED=0 go build -trimpath -o school-nanny .
echo "Built ./school-nanny"
