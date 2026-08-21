# OmniRoute Control Panel (orPanel)

A cross-platform desktop control panel for [OmniRoute](https://github.com/TorhunORG/OmniRoute) — the AI request router and load balancer. orPanel lives in your system tray, provides a built-in terminal, health monitoring, and one-click install/update for OmniRoute.

> Built with ❤️ for the OmniRoute community.

![orPanel Screenshot](https://raw.githubusercontent.com/burkimen/orpanel/main/screenshot.png)

## Features

- **System Tray** — Always accessible from the taskbar
- **Built-in Terminal** — VS Code-style resizable bottom panel
- **Health Monitoring** — Real-time OmniRoute status, version checks, one-click install/update/repair
- **Theme System** — Light, Dark, System (auto-detect)
- **Multi-language** — Turkish, English, Spanish (extensible)
- **Cross-platform** — Windows, macOS, Linux
- **No admin rights required** — Portable or XDG-compliant installation

## Installation

### 1. npm (recommended)

```bash
npm install -g orpanel
orpanel
```

### 2. bun

```bash
bun add -g orpanel
bunx orpanel
```

### 3. curl (Linux/macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.sh | sh
```

### 4. PowerShell (Windows)

```powershell
irm https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.ps1 | iex
```

### 5. Homebrew

```bash
brew tap burkimen/orpanel
brew install orpanel
```

### 6. Nix

```bash
nix profile install github:burkimen/orpanel
```

### 7. mise

```bash
mise use -g ubi:burkimen/orpanel
```

## Quick Start

```bash
orpanel           # Start the control panel
orpanel --help    # Show help
```

Open `http://localhost:20127` in your browser. The panel will:

1. Detect OmniRoute on your system
2. Show health status with version info
3. Offer one-click install if OmniRoute is not found
4. Stream OmniRoute logs in the built-in terminal

## OmniRoute Integration

orPanel is designed to work seamlessly with [OmniRoute](https://github.com/TorhunORG/OmniRoute):

- **Auto-detection** — Finds OmniRoute via `npm prefix -g`, NVM, system paths
- **Health check** — `GET /api/omni/health` monitors version, port, node compatibility
- **One-click operations** — Install, Update, Repair, Reinstall via web UI
- **Live terminal** — npm output streams directly to the embedded xterm
- **Version tracking** — Compares local vs npm registry latest, alerts on updates

## Development

```bash
git clone https://github.com/burkimen/orpanel.git
cd orpanel
go build -ldflags "-H=windowsgui" -o orPanel.exe   # Windows
go build -o orPanel                                  # Linux/macOS
```

Requires Go 1.22+ and `themes/`, `locales/` directories alongside the binary.

## Architecture

```
orpanel/
├── panel.go           # HTTP server, system tray, process management
├── config.go          # Config loading/saving
├── paths.go           # Cross-platform path resolution
├── omni_health.go     # OmniRoute health checks
├── omni_install.go    # npm install/update/repair operations
├── autostart_*.go     # Platform-specific autostart
├── dialog_*.go        # Platform-specific dialogs
├── web/
│   ├── templates/     # HTML templates (Go embed)
│   └── static/js/     # Frontend JavaScript modules
├── themes/            # CSS variables (dark/light)
├── locales/           # i18n strings (tr/en/es)
└── scripts/install/   # curl/irm install scripts
```

## License

[MIT](LICENSE) — Made for the [OmniRoute](https://github.com/TorhunORG/OmniRoute) community.
