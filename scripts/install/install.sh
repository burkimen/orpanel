#!/bin/sh
# orpanel installer - curl | sh
# Standartlara uygun: XDG Base Directory, ~/.local/bin, yetkisiz kurulum
# Tek komut: curl -fsSL https://get.orpanel.dev/install.sh | sh
# veya: curl -fsSL https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.sh | sh
set -e

REPO="burkimen/orpanel"
BIN_NAME="orpanel"
VERSION="${ORPANEL_VERSION:-latest}"

# XDG
XDG_BIN_HOME="${XDG_BIN_HOME:-$HOME/.local/bin}"
XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
XDG_DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
XDG_STATE_HOME="${XDG_STATE_HOME:-$HOME/.local/state}"

INSTALL_BIN="$XDG_BIN_HOME/$BIN_NAME"
CONFIG_DIR="$XDG_CONFIG_HOME/orpanel"
DATA_DIR="$XDG_DATA_HOME/orpanel"
STATE_DIR="$XDG_STATE_HOME/orpanel"

# Detect OS/arch
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$OS" in
  linux) OS="linux" ;;
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
  echo "→ En son sürüm sorgulanıyor..."
  # GitHub API'den latest tag al (curl/wget fallback)
  if command -v curl >/dev/null 2>&1; then
    TAG="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '\"tag_name\"' | sed -E 's/.*\"v?([^"]+)\".*/\1/' | head -n1)"
  elif command -v wget >/dev/null 2>&1; then
    TAG="$(wget -qO- "https://api.github.com/repos/$REPO/releases/latest" | grep '\"tag_name\"' | sed -E 's/.*\"v?([^"]+)\".*/\1/' | head -n1)"
  else
    TAG=""
  fi
  if [ -z "$TAG" ]; then TAG="1.0.0"; fi
  VERSION="$TAG"
fi

VERSION_NOV="${VERSION#v}"
ASSET="orpanel-v${VERSION_NOV}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION_NOV}/${ASSET}"
# Fallback to raw dist if release yoksa
FALLBACK_URL="https://raw.githubusercontent.com/${REPO}/main/dist/${ASSET}"

echo "→ Orpanel v${VERSION_NOV} (${OS}/${ARCH}) indiriliyor..."
echo "  $URL"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# Download with curl or wget
if command -v curl >/dev/null 2>&1; then
  if ! curl -fsSL "$URL" -o "$TMPDIR/$ASSET"; then
    echo "  Release bulunamadı, fallback deneniyor..."
    curl -fsSL "$FALLBACK_URL" -o "$TMPDIR/$ASSET" || { echo "İndirme başarısız" >&2; exit 1; }
  fi
elif command -v wget >/dev/null 2>&1; then
  if ! wget -qO "$TMPDIR/$ASSET" "$URL"; then
    wget -qO "$TMPDIR/$ASSET" "$FALLBACK_URL" || { echo "İndirme başarısız" >&2; exit 1; }
  fi
else
  echo "curl veya wget gerekli" >&2; exit 1
fi

# Extract
mkdir -p "$TMPDIR/extract"
tar -xzf "$TMPDIR/$ASSET" -C "$TMPDIR/extract"

# Find binary in extract (may be orPanel / orpanel)
BIN_SRC=""
for cand in "$TMPDIR/extract/orpanel" "$TMPDIR/extract/orPanel" "$TMPDIR/extract/orpanel-${OS}-${ARCH}" "$TMPDIR/extract/orPanel-${OS}-${ARCH}"; do
  if [ -f "$cand" ]; then BIN_SRC="$cand"; break; fi
  if [ -f "$cand.tar" ]; then BIN_SRC="$cand.tar"; break; fi
done
if [ -z "$BIN_SRC" ]; then
  # fallback: first executable file
  BIN_SRC="$(find "$TMPDIR/extract" -type f -name "orpanel*" | head -n1)"
fi
if [ ! -f "$BIN_SRC" ]; then
  echo "Binary bulunamadı" >&2; ls -R "$TMPDIR/extract" >&2; exit 1
fi

# Install
mkdir -p "$XDG_BIN_HOME" "$CONFIG_DIR" "$DATA_DIR" "$STATE_DIR"
install -m 755 "$BIN_SRC" "$INSTALL_BIN" 2>/dev/null || cp -f "$BIN_SRC" "$INSTALL_BIN" && chmod +x "$INSTALL_BIN"

# Copy themes/locales if present in archive
for d in themes locales; do
  if [ -d "$TMPDIR/extract/$d" ]; then
    rm -rf "$DATA_DIR/$d"
    cp -R "$TMPDIR/extract/$d" "$DATA_DIR/"
    # also keep alongside binary for exeDir fallback
    cp -R "$TMPDIR/extract/$d" "$(dirname "$INSTALL_BIN")/" 2>/dev/null || true
  fi
done
for f in app.ico icon.png; do
  if [ -f "$TMPDIR/extract/$f" ]; then
    cp -f "$TMPDIR/extract/$f" "$(dirname "$INSTALL_BIN")/" 2>/dev/null || true
    cp -f "$TMPDIR/extract/$f" "$DATA_DIR/" 2>/dev/null || true
  fi
done

echo "✓ Kuruldu: $INSTALL_BIN"
echo "  Config: $CONFIG_DIR/config.json"
echo "  Log:    $STATE_DIR/panel.log"
echo "  Data:   $DATA_DIR"

# PATH check
case ":$PATH:" in
  *":$XDG_BIN_HOME:"*) ;;
  *) echo ""; echo "→ PATH'e ekleyin: export PATH=\"\$XDG_BIN_HOME:\$PATH\"  ( ~/.profile veya ~/.zshrc )"; echo "  Şimdi için: export PATH=\"$XDG_BIN_HOME:\$PATH\"";;
esac

echo ""
echo "Çalıştır: orpanel   (veya: $INSTALL_BIN)"
echo "Güncelle: curl -fsSL https://get.orpanel.dev/install.sh | sh   veya   orpanel update"
echo "Kaldır:   rm \"$INSTALL_BIN\" && rm -rf \"$CONFIG_DIR\" \"$DATA_DIR\""
