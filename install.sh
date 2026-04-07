#!/usr/bin/env bash
# Install requests (tunnel + proxy) from GitHub Releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/tunnel-ops/tunnel/main/install.sh | bash
#
# Options (env vars):
#   INSTALL_DIR   Installation directory (default: /usr/local/bin)
#   VERSION       Specific version tag to install (default: latest)

set -euo pipefail

REPO="tunnel-ops/tunnel"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${VERSION:-}"

# ─── Detect OS and architecture ───────────────────────────────────────────────

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64)          ARCH="amd64" ;;
  arm64|aarch64)   ARCH="arm64" ;;
  *) echo "error: unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

case "$OS" in
  darwin|linux) ;;
  *) echo "error: unsupported OS: $OS" >&2; exit 1 ;;
esac

# ─── Resolve version ──────────────────────────────────────────────────────────

if [ -z "$VERSION" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')"
fi

if [ -z "$VERSION" ]; then
  echo "error: could not determine latest release. Set VERSION=vX.Y.Z to install a specific version." >&2
  exit 1
fi

echo "Installing requests ${VERSION} (${OS}/${ARCH}) to ${INSTALL_DIR}"

# ─── Check write permissions ──────────────────────────────────────────────────

if [ ! -w "$INSTALL_DIR" ]; then
  echo "error: ${INSTALL_DIR} is not writable. Re-run with sudo or set INSTALL_DIR to a writable path." >&2
  exit 1
fi

# ─── Download and install ─────────────────────────────────────────────────────

BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

install_binary() {
  local name="$1"
  local url="${BASE_URL}/${name}_${OS}_${ARCH}"
  local dest="${INSTALL_DIR}/${name}"
  local tmp
  tmp="$(mktemp)"

  echo "  Downloading ${name}..."
  if ! curl -fsSL "$url" -o "$tmp"; then
    echo "error: failed to download ${url}" >&2
    rm -f "$tmp"
    exit 1
  fi

  chmod +x "$tmp"
  mv "$tmp" "$dest"
  echo "  Installed: ${dest}"
}

install_binary "tunnel"
install_binary "requests-proxy"

echo ""
echo "Done! Run 'tunnel help' to get started."
echo ""
echo "First-time setup:"
echo "  export DOMAIN=your-domain.com"
echo "  requests-proxy &   # start the local reverse proxy"
echo "  tunnel list        # show active services"
