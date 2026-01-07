#!/bin/sh
# xplat installer - auto-detects OS and architecture
# Usage: curl -fsSL https://raw.githubusercontent.com/joeblew999/xplat/main/install.sh | sh

set -e

REPO="joeblew999/xplat"
BINARY="xplat"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    darwin) OS="darwin" ;;
    linux) OS="linux" ;;
    mingw*|msys*|cygwin*) OS="windows" ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Windows needs .exe extension
EXT=""
if [ "$OS" = "windows" ]; then
    EXT=".exe"
fi

# Determine install directory
if [ "$OS" = "darwin" ] || [ "$OS" = "linux" ]; then
    if [ -w /usr/local/bin ]; then
        INSTALL_DIR="/usr/local/bin"
    elif [ -d "$HOME/.local/bin" ]; then
        INSTALL_DIR="$HOME/.local/bin"
    else
        mkdir -p "$HOME/.local/bin"
        INSTALL_DIR="$HOME/.local/bin"
    fi
elif [ "$OS" = "windows" ]; then
    INSTALL_DIR="$LOCALAPPDATA/xplat"
    mkdir -p "$INSTALL_DIR"
fi

# Get latest release URL
ASSET="${BINARY}-${OS}-${ARCH}${EXT}"
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo "Installing xplat..."
echo "  OS: $OS"
echo "  Arch: $ARCH"
echo "  URL: $URL"
echo "  Destination: $INSTALL_DIR/$BINARY$EXT"
echo

# Download
if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$URL" -o "$INSTALL_DIR/$BINARY$EXT"
elif command -v wget >/dev/null 2>&1; then
    wget -q "$URL" -O "$INSTALL_DIR/$BINARY$EXT"
else
    echo "Error: curl or wget required"
    exit 1
fi

# Make executable (not needed on Windows)
if [ "$OS" != "windows" ]; then
    chmod +x "$INSTALL_DIR/$BINARY$EXT"
fi

# Verify installation
if [ -x "$INSTALL_DIR/$BINARY$EXT" ]; then
    echo "Successfully installed xplat to $INSTALL_DIR/$BINARY$EXT"

    # Check if in PATH
    if ! command -v xplat >/dev/null 2>&1; then
        echo
        echo "Note: $INSTALL_DIR may not be in your PATH."
        echo "Add it with:"
        if [ "$OS" = "darwin" ] || [ "$OS" = "linux" ]; then
            echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
        fi
    else
        echo
        "$INSTALL_DIR/$BINARY$EXT" version 2>/dev/null || echo "Run 'xplat version' to verify"
    fi
else
    echo "Error: Installation failed"
    exit 1
fi
