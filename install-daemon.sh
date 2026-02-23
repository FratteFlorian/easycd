#!/bin/sh
# eacdd daemon installer (Linux/amd64 only)
# Usage: curl -fsSL https://raw.githubusercontent.com/FratteFlorian/easycd/main/install-daemon.sh | sh
set -e

REPO="FratteFlorian/easycd"
INSTALL_DIR="/usr/local/bin"
SERVICE_DIR="/etc/systemd/system"

# Must be root
if [ "$(id -u)" -ne 0 ]; then
  echo "This script must be run as root (use sudo)." >&2
  exit 1
fi

# Must be Linux/amd64
if [ "$(uname -s)" != "Linux" ] || [ "$(uname -m)" != "x86_64" ]; then
  echo "eacdd only supports Linux/amd64." >&2
  exit 1
fi

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

echo "Installing eacdd ${VERSION}..."

# Binary
curl -fsSL "https://github.com/${REPO}/releases/download/${VERSION}/eacdd-linux-amd64" \
  -o "${INSTALL_DIR}/eacdd"
chmod +x "${INSTALL_DIR}/eacdd"

# Systemd service
curl -fsSL "https://github.com/${REPO}/releases/download/${VERSION}/eacdd.service" \
  -o "${SERVICE_DIR}/eacdd.service"

# Directories
mkdir -p /etc/eacd /var/log/eacd /var/lib/eacd/.global

# Write config only on first install
if [ ! -f /etc/eacd/server.yaml ]; then
  TOKEN=$(openssl rand -hex 32)
  cat > /etc/eacd/server.yaml << EOF
listen: :8765
token: ${TOKEN}
log_dir: /var/log/eacd
EOF
  echo ""
  echo "Generated token: ${TOKEN}"
  echo "Add this to your project's .eacd/config.yaml:"
  echo "  token: ${TOKEN}"
  echo "(or export EACD_TOKEN=${TOKEN})"
else
  echo "Existing /etc/eacd/server.yaml kept."
fi

# Enable and start
systemctl daemon-reload
systemctl enable --now eacdd

echo ""
echo "eacdd ${VERSION} installed and running."
echo "Config: /etc/eacd/server.yaml"
echo "Logs:   journalctl -u eacdd -f"
