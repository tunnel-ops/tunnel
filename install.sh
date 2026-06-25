#!/usr/bin/env bash
# Install requests (tunnel + proxy) from GitHub Releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/tunnel-ops/tunnel/main/install.sh | bash
#
# Options (env vars):
#   INSTALL_DIR   Installation directory (default: /usr/local/bin, falls back to ~/.local/bin)
#   VERSION       Specific version tag to install (default: latest)

set -euo pipefail

REPO="tunnel-ops/tunnel"
_INSTALL_DIR_EXPLICIT="${INSTALL_DIR:-}"
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

# ─── Resolve install directory ────────────────────────────────────────────────

_PATH_HINT=""

if [ -n "$_INSTALL_DIR_EXPLICIT" ]; then
  # Caller set INSTALL_DIR explicitly — honour it, no fallback.
  if [ ! -w "$INSTALL_DIR" ]; then
    echo "error: INSTALL_DIR=${INSTALL_DIR} is not writable." >&2
    exit 1
  fi
else
  # No explicit override: try /usr/local/bin, fall back to ~/.local/bin.
  if [ ! -w "$INSTALL_DIR" ]; then
    INSTALL_DIR="${HOME}/.local/bin"
    mkdir -p "$INSTALL_DIR"
    case ":${PATH}:" in
      *":${INSTALL_DIR}:"*) ;;
      *) _PATH_HINT="$INSTALL_DIR" ;;
    esac
  fi
fi

echo "Installing requests ${VERSION} (${OS}/${ARCH}) to ${INSTALL_DIR}"

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

if [ -n "$_PATH_HINT" ]; then
  echo ""
  echo "Note: ${_PATH_HINT} is not in your PATH."
  echo "Add this to your shell profile (~/.zshrc or ~/.bashrc) and restart your shell:"
  echo ""
  echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
fi

echo ""
echo "First-time setup:"
echo "  tunnel setup       # configure your domain"
echo "  tunnel help        # all commands"
