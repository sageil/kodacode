#!/bin/sh
# KodaCode installer. Downloads the latest release bundle for your platform.
# Usage: curl -fsSL https://raw.githubusercontent.com/sageil/kodacode/main/install.sh | sh
set -e

REPO="sageil/kodacode"
INSTALL_DIR="/usr/local/bin"

# Detect OS and architecture.
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
    linux|darwin) ;;
    *) echo "Unsupported OS: $OS (Windows users: install via WSL)"; exit 1 ;;
esac

# Fetch latest release tag from GitHub API.
echo "Fetching latest release..."
TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
    echo "Error: could not determine latest release"
    exit 1
fi
echo "Latest version: v${TAG}"

# Download and extract.
ARCHIVE="kodacode_${TAG}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${TAG}/${ARCHIVE}"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading ${URL}..."
curl -fsSL "$URL" -o "${TMP}/${ARCHIVE}"

echo "Extracting..."
tar -xzf "${TMP}/${ARCHIVE}" -C "$TMP"

install_file() {
    SRC="$1"
    DST="$2"
    MODE="$3"
    DST_DIR="$(dirname "$DST")"
    if mkdir -p "$DST_DIR" 2>/dev/null && mv "$SRC" "$DST" 2>/dev/null; then
        chmod "$MODE" "$DST"
        return
    fi
    echo "Installing to ${DST} (requires sudo)..."
    sudo mkdir -p "$DST_DIR"
    sudo mv "$SRC" "$DST"
    sudo chmod "$MODE" "$DST"
}

install_file "${TMP}/kodacode" "${INSTALL_DIR}/kodacode" 755

echo "Installed kodacode v${TAG} to ${INSTALL_DIR}/kodacode"
echo ""
echo "Run 'kodacode' to get started."
