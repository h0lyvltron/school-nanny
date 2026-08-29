#!/usr/bin/env bash
# Gets this machine ready to build School Nanny: a Go toolchain, the Go
# dependencies, and the vendored browser assets.
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT=$(pwd)

GO_MIN_MINOR=25
GO_VERSION=1.27.0
TOOLCHAIN="$ROOT/.toolchain"

have_new_enough_go() {
    command -v go >/dev/null 2>&1 || return 1
    local minor
    minor=$(go env GOVERSION 2>/dev/null | sed -n 's/^go1\.\([0-9]*\).*/\1/p')
    [ -n "$minor" ] && [ "$minor" -ge "$GO_MIN_MINOR" ]
}

if have_new_enough_go; then
    echo "Using $(go version)"
elif [ -x "$TOOLCHAIN/go/bin/go" ]; then
    export GOROOT="$TOOLCHAIN/go"
    export PATH="$GOROOT/bin:$PATH"
    echo "Using $(go version) from .toolchain"
else
    echo "Go 1.$GO_MIN_MINOR or newer was not found. Downloading Go $GO_VERSION into .toolchain ..."
    case "$(uname -m)" in
        x86_64)          arch=amd64 ;;
        aarch64|arm64)   arch=arm64 ;;
        *) echo "Unsupported architecture $(uname -m). Install Go manually from https://go.dev/dl/"; exit 1 ;;
    esac
    case "$(uname -s)" in
        Linux)  os=linux ;;
        Darwin) os=darwin ;;
        *) echo "Unsupported system $(uname -s). Install Go manually from https://go.dev/dl/"; exit 1 ;;
    esac

    mkdir -p "$TOOLCHAIN"
    tarball="$TOOLCHAIN/go.tar.gz"
    curl -fsSL -o "$tarball" "https://go.dev/dl/go${GO_VERSION}.${os}-${arch}.tar.gz"
    rm -rf "$TOOLCHAIN/go"
    tar -xzf "$tarball" -C "$TOOLCHAIN"
    rm -f "$tarball"

    export GOROOT="$TOOLCHAIN/go"
    export PATH="$GOROOT/bin:$PATH"
    echo "Installed $(go version)"
fi

echo "Downloading Go dependencies ..."
go mod download

# HTMX and the stylesheet ship with the repo so the app never needs the
# internet at runtime. Re-fetch only if someone deleted them.
fetch_asset() {
    local path=$1 url=$2
    if [ -s "$path" ]; then
        return
    fi
    echo "Fetching $(basename "$path") ..."
    curl -fsSL -o "$path" "$url"
}

mkdir -p "$ROOT/static"
fetch_asset "$ROOT/static/htmx.min.js" "https://unpkg.com/htmx.org@2.0.7/dist/htmx.min.js"
fetch_asset "$ROOT/static/pico.min.css" "https://cdn.jsdelivr.net/npm/@picocss/pico@2.1.1/css/pico.min.css"

echo
echo "Setup complete."
echo "  Run it here:            ./scripts/run.sh"
echo "  Build a Windows copy:   ./scripts/build-windows.sh"
