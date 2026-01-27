#!/bin/bash
# Ensure ocw binary is installed

set -e

OCW_BIN="${HOME}/.local/bin/ocw"
PLUGIN_ROOT="$(dirname "$(dirname "$0")")"

# Check if ocw exists and is executable
if command -v ocw &>/dev/null; then
    exit 0
fi

if [[ -x "$OCW_BIN" ]]; then
    exit 0
fi

# Create bin directory
mkdir -p "${HOME}/.local/bin"

# Try to build from source if Go is available
if command -v go &>/dev/null; then
    echo "Building ocw from source..."
    cd "$PLUGIN_ROOT/ocw"
    go build -o "$OCW_BIN" .
    echo "ocw installed to $OCW_BIN"
    exit 0
fi

# Try to download pre-built binary
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
esac

RELEASE_URL="https://github.com/metrovoc/cc-skill-opencode/releases/latest/download/ocw-${OS}-${ARCH}"

echo "Downloading ocw binary..."
if curl -fsSL "$RELEASE_URL" -o "$OCW_BIN" 2>/dev/null; then
    chmod +x "$OCW_BIN"
    echo "ocw installed to $OCW_BIN"
    exit 0
fi

echo "ERROR: Could not install ocw. Please install Go and run:"
echo "  cd $PLUGIN_ROOT/ocw && go build -o ~/.local/bin/ocw ."
exit 1
