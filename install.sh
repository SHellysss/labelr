#!/bin/sh
set -e

REPO="Pankaj3112/labelr"
INSTALL_DIR="/usr/local/bin"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  darwin|linux) ;;
  *)            echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Get latest release tag
LATEST=$(curl -sI "https://github.com/$REPO/releases/latest" | grep -i "^location:" | sed 's/.*tag\///' | tr -d '\r')

if [ -z "$LATEST" ]; then
  echo "Failed to find latest release"
  exit 1
fi

echo "Installing labelr $LATEST ($OS/$ARCH)..."

FILENAME="labelr_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$LATEST/$FILENAME"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

curl -sL "$URL" -o "$TMPDIR/$FILENAME"
tar -xzf "$TMPDIR/$FILENAME" -C "$TMPDIR"

if [ -w "$INSTALL_DIR" ]; then
  mv "$TMPDIR/labelr" "$INSTALL_DIR/labelr"
else
  echo "Need sudo to install to $INSTALL_DIR"
  sudo mv "$TMPDIR/labelr" "$INSTALL_DIR/labelr"
fi

chmod +x "$INSTALL_DIR/labelr"

echo "labelr $LATEST installed to $INSTALL_DIR/labelr"
echo ""
echo "Get started:"
echo "  labelr setup    # first-time setup"
