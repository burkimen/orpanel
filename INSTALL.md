# Orpanel — Tek Komutla Kurulum (7 Yöntem)

Aynı Go binary, aynı standart dizinler, tek komut.

> **Çalışma dizini (son kullanıcı):** Kurulum sonrası `orpanel` binary’si ve `themes/` `locales/` her zaman **aynı klasörde** kalır.
> - **Windows:** `%LOCALAPPDATA%\Programs\Orpanel\orPanel.exe` + `%LOCALAPPDATA%\Orpanel\config.json` / `panel.log` (yetkisiz, standart). Eski `%USERPROFILE%\orpanel` de desteklenir.
> - **Linux:** `~/.local/bin/orpanel` + `~/.config/orpanel/config.json` + `~/.local/share/orpanel` + `~/.local/state/orpanel/panel.log` (XDG)
> - **macOS:** `~/Library/Application Support/Orpanel/` + `~/Library/Logs/Orpanel/` (Apple Guide)
> - Portable: zip’i nereye çıkarırsan `orPanel.exe` yanına `config.json` yazılır (exeDir fallback, test_run ile doğrulandı).

---

## 1) npm — %100 uyumlu (önerilen, Node varsa)

```sh
npm install -g orpanel
# güncelle
npm update -g orpanel
# çalıştır
orpanel        # veya npx orpanel / bunx orpanel
```

`package.json:bin` `orpanel` wrapper `os+arch`’e göre `dist/orpanel-*.tar.gz`’i seçer. `orpanel --version` aynı. [orpanel on npm](https://www.npmjs.com/package/orpanel)

## 2) bun — npm ile aynı (Bun runtime)

```sh
bun add -g orpanel
bunx orpanel
```

## 3) curl — npm’siz Unix tek komut

```sh
curl -fsSL https://get.orpanel.dev/install.sh | sh
# veya pinli
curl -fsSL https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.sh | sh
# belirli sürüm
ORPANEL_VERSION=1.0.0 curl -fsSL https://get.orpanel.dev/install.sh | sh
```

`install.sh` `uname -s/m` → `linux-x64 darwin-arm64` asset’i GitHub Releases’ten indirir, `~/.local/bin` + XDG’ye kurar, `chmod +x`, PATH uyarısı verir.

## 4) irm — PowerShell tek komut (Windows, npm’siz)

```powershell
powershell -c "irm https://get.orpanel.dev/install.ps1 | iex"
# veya
irm https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.ps1 | iex
```

`%LOCALAPPDATA%\Programs\Orpanel\orPanel.exe` + `%LOCALAPPDATA%\Orpanel\config.json` + user PATH’e ekler. Yönetici gerekmez.

## 5) brew — Homebrew (macOS/Linux)

```sh
brew tap burkimen/orpanel
brew install orpanel
# veya direkt
brew install burkimen/orpanel/orpanel
# güncelle
brew upgrade orpanel
```

`Formula/orpanel.rb` `on_macos` `on_linux` `sha256` Release’den dolar.

## 6) nix — Nix Flake (NixOS, nix-darwin, home-manager)

```sh
nix profile install github:burkimen/orpanel
# veya run (kurmadan)
nix run github:burkimen/orpanel
# flake
nix flake show github:burkimen/orpanel
```

`flake.nix` `eachDefaultSystem` `fetchurl` tarball + `buildGoModule` (kaynak derleme alternatifi `orpanel-from-source`).

## 7) mise — polyglot version manager

```sh
# npm backend (Node gerekir)
mise use -g npm:orpanel
# ubi backend (npm’siz, GitHub direkt)
mise use -g ubi:burkimen/orpanel
# pinli
mise use -g ubi:burkimen/orpanel@1.0.0
```

`mise.toml` her ikisini de tanımlar.

---

## OmniRoute sağlık (web’den tek tık)

Panel `http://localhost:20127` → **Terminal** üstündeki **OmniRoute Durumu** kartı `GET /api/omni/health` ile tarar:

- Kurulu değil → `OmniRoute Kurulumunu Şimdi Başlat` → `POST /api/omni/install` → `npm install -g omniroute@latest` logları `xterm`’e akar.
- Güncelleme var (`3.8.49 → 3.9.0`) → `OmniRoute’u Güncelle` → `npm update -g`
- Bozuk/port çakışması → `Onar` (`--force`) / `Yeniden Kur` (`uninstall + install`)

Node `>=22.22.2 <23 || >=24 <27` değilse önce Node uyarısı.

## Doğrulama

```sh
orpanel --help
orpanel --version
npm view orpanel version
npm view omniroute version
```
