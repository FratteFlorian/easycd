#!/bin/sh
# eacd installer — installs eacd (client) + eacdd (daemon binary for CT uploads)
# Usage: curl -fsSL https://raw.githubusercontent.com/FratteFlorian/easycd/main/install.sh | sh
set -e

REPO="FratteFlorian/easycd"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect arch
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)          ARCH="amd64" ;;
  aarch64|arm64)   ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Resolve version
if [ -n "$EACD_VERSION" ]; then
  VERSION="$EACD_VERSION"
else
  echo "Fetching latest release..."
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed 's/.*"tag_name": *"\(.*\)".*/\1/')
fi

if [ -z "$VERSION" ]; then
  echo "Could not determine latest version." >&2
  exit 1
fi

BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

install_bin() {
  src="$1"
  dst="$2"
  curl -fsSL "$src" -o /tmp/_eacd_tmp
  chmod +x /tmp/_eacd_tmp
  if [ -w "$INSTALL_DIR" ]; then
    mv /tmp/_eacd_tmp "$dst"
  else
    sudo mv /tmp/_eacd_tmp "$dst"
  fi
}

echo "Installing eacd ${VERSION} (${OS}/${ARCH}) → ${INSTALL_DIR}/eacd"
install_bin "${BASE_URL}/eacd-${OS}-${ARCH}" "${INSTALL_DIR}/eacd"

# eacdd is always linux/amd64 — it runs on the CT, not on this machine.
# eacd init needs it locally to upload it during LXC provisioning.
echo "Installing eacdd ${VERSION} (linux/amd64) → ${INSTALL_DIR}/eacdd"
install_bin "${BASE_URL}/eacdd-linux-amd64" "${INSTALL_DIR}/eacdd"

echo ""
echo "eacd ${VERSION} installed."
echo "Run 'eacd init' in your project directory to get started."
