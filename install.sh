#!/bin/sh
# install.sh — install mework (CLI, server, worker, MCP) from GitHub Releases.
# Usage: curl -fsSL https://mework.sh/install | sh
#        curl -fsSL https://mework.sh/install | sh -s -- v0.1.0  # specific version

set -eu

REPO="${REPO:-minhlucncc/mework}"
VERSION="${1:-latest}"
TMPDIR=$(mktemp -d) && trap 'rm -rf "$TMPDIR"' EXIT

# --- detect platform ---
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
  darwin)  GOOS="darwin"  ;;
  linux)   GOOS="linux"   ;;
  mingw*|msys*|cygwin*) GOOS="windows"  ;;
  *)       echo "unsupported OS: $OS"; exit 1 ;;
esac

case "$ARCH" in
  x86_64|amd64) GOARCH="amd64" ;;
  arm64|aarch64) GOARCH="arm64" ;;
  *)            echo "unsupported arch: $ARCH"; exit 1 ;;
esac

# --- resolve version ---
if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -sfL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name":' | sed 's/.*"tag_name": "\(.*\)",/\1/')
  if [ -z "$VERSION" ]; then
    echo "failed to fetch latest release"
    exit 1
  fi
fi

echo "installing mework $VERSION ($GOOS/$GOARCH)..."

# --- download ---
ARCHIVE="mework-${VERSION}-${GOOS}-${GOARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/$REPO/releases/download/$VERSION/$ARCHIVE"

curl -sfL "$DOWNLOAD_URL" -o "$TMPDIR/$ARCHIVE"

# --- extract ---
tar -xzf "$TMPDIR/$ARCHIVE" -C "$TMPDIR"

# --- install ---
INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  # Fall back to ~/.local/bin
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

for bin in mework mework-server mework-mezon-worker mework-mcp; do
  if [ -f "$TMPDIR/$bin" ]; then
    chmod +x "$TMPDIR/$bin"
    cp "$TMPDIR/$bin" "$INSTALL_DIR/$bin"
    echo "  installed $INSTALL_DIR/$bin"
  fi
done

echo ""
echo "mework $VERSION installed to $INSTALL_DIR"
echo "Make sure $INSTALL_DIR is in your PATH."
echo ""
echo "Quick start:"
echo "  mework init --agent orchestrator"
echo "  mework mezon-worker start"
