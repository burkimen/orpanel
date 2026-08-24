#!/bin/sh
# orpanel installer - curl | sh
# Tek komut: curl -fsSL https://get.orpanel.dev/install.sh | sh
set -e

REPO="burkimen/orpanel"
BIN_NAME="orpanel"
VERSION="${ORPANEL_VERSION:-latest}"

XDG_BIN_HOME="${XDG_BIN_HOME:-$HOME/.local/bin}"
XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
XDG_DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
XDG_STATE_HOME="${XDG_STATE_HOME:-$HOME/.local/state}"

INSTALL_BIN="$XDG_BIN_HOME/$BIN_NAME"
CONFIG_DIR="$XDG_CONFIG_HOME/orpanel"
DATA_DIR="$XDG_DATA_HOME/orpanel"
STATE_DIR="$XDG_STATE_HOME/orpanel"

# Detect platform
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *) echo "Desteklenmeyen OS: $OS" >&2; exit 1 ;;
esac
case "$ARCH" in
  x86_64|amd64) ARCH="x64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Desteklenmeyen arch: $ARCH" >&2; exit 1 ;;
esac

# Resolve version
if [ "$VERSION" = "latest" ]; then
  echo "→ En son surum sorgulaniyor..."
  if command -v curl >/dev/null 2>&1; then
    TAG="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/' | head -n1)"
  elif command -v wget >/dev/null 2>&1; then
    TAG="$(wget -qO- "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/' | head -n1)"
  fi
  if [ -z "$TAG" ]; then TAG="1.0.9"; fi
  VERSION="$TAG"
fi

VERSION_NOV="${VERSION#v}"
BIN_NAME_REMOTE="orpanel-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/v${VERSION_NOV}/${BIN_NAME_REMOTE}"

echo "→ Orpanel v${VERSION_NOV} (${OS}/${ARCH}) indiriliyor..."
echo "  $URL"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# Download binary
DOWNLOADED=0
if command -v curl >/dev/null 2>&1; then
  if curl -fsSL "$URL" -o "$TMPDIR/$BIN_NAME_REMOTE"; then DOWNLOADED=1; fi
elif command -v wget >/dev/null 2>&1; then
  if wget -qO "$TMPDIR/$BIN_NAME_REMOTE" "$URL"; then DOWNLOADED=1; fi
else
  echo "curl veya wget gerekli" >&2; exit 1
fi

if [ "$DOWNLOADED" -eq 0 ]; then
  echo "Binary indirilemedi. Kaynaktan derleniyor..."
  if command -v go >/dev/null 2>&1; then
    REPO_DIR="$(mktemp -d)"
    trap 'rm -rf "$REPO_DIR" "$TMPDIR"' EXIT
    git clone --depth 1 "https://github.com/$REPO.git" "$REPO_DIR"
    cd "$REPO_DIR"
    VERSION_STR="$(cat version.txt 2>/dev/null || echo '0.0.0')"
    CGO_ENABLED=1 go build -ldflags "-X main.AppVersion=$VERSION_STR" -o "$TMPDIR/$BIN_NAME_REMOTE"
    if [ ! -f "$TMPDIR/$BIN_NAME_REMOTE" ]; then
      echo "Derleme basarisiz. Go 1.23+ gerekli." >&2; exit 1
    fi
  else
    echo "Binary indirilemedi ve Go yuklu degil." >&2
    echo "Go yukleyin: https://go.dev/dl/" >&2
    exit 1
  fi
fi

# Install
chmod +x "$TMPDIR/$BIN_NAME_REMOTE"
mkdir -p "$XDG_BIN_HOME" "$CONFIG_DIR" "$DATA_DIR" "$STATE_DIR"
cp -f "$TMPDIR/$BIN_NAME_REMOTE" "$INSTALL_BIN"

echo ""
echo "Kuruldu: $INSTALL_BIN"
echo ""
echo "Calistir: orpanel"
echo "Guncelle: curl -fsSL https://raw.githubusercontent.com/$REPO/main/scripts/install/install.sh | sh"
echo "Kaldir:   rm $INSTALL_BIN && rm -rf $CONFIG_DIR $DATA_DIR $STATE_DIR"
