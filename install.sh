#!/bin/sh
# KodaCode installer — downloads the latest release binary for your platform.
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

# Install binary.
if [ -w "$INSTALL_DIR" ]; then
    mv "${TMP}/kodacode" "${INSTALL_DIR}/kodacode"
else
    echo "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "${TMP}/kodacode" "${INSTALL_DIR}/kodacode"
fi
chmod +x "${INSTALL_DIR}/kodacode"

echo "Installed kodacode v${TAG} to ${INSTALL_DIR}/kodacode"
echo ""
echo "Run 'kodacode' to get started."
