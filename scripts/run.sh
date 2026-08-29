#!/usr/bin/env bash
# Runs School Nanny from source. Extra arguments are passed straight through,
# for example: ./scripts/run.sh -lan
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

exec go run . "$@"
