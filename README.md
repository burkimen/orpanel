# OmniRoute Control Panel (orPanel)

A cross-platform desktop control panel for [OmniRoute](https://github.com/TorhunORG/OmniRoute) — the AI request router and load balancer. orPanel lives in your system tray, provides a built-in terminal, health monitoring, and one-click install/update for OmniRoute.

> Built with ❤️ for the OmniRoute community.

<p align="center">
  <img src="./screenshot.png" alt="orPanel Status" width="100%">
</p>

<p align="center">
  <img src="./screenshot_settings.png" alt="orPanel Settings" width="100%">
</p>

## Features

- **System Tray** — Always accessible from the taskbar
- **Built-in Terminal** — VS Code-style resizable bottom panel
- **Health Monitoring** — Real-time OmniRoute status, version checks, one-click install/update/repair
- **Theme System** — Light, Dark, System (auto-detect)
- **Multi-language** — Turkish, English, Spanish (extensible)
- **Cross-platform** — Windows, macOS, Linux
- **No dependencies** — Single binary, everything embedded

## Installation

### Unix (Linux/macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.sh | sh
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.ps1 | iex
```

### Update

Just run the same command again — it will download the latest version.

### Uninstall

```bash
# Linux/macOS
rm ~/.local/bin/orpanel

# Windows
Remove-Item "$env:LOCALAPPDATA\Programs\Orpanel\orPanel.exe"
```

## Usage

```bash
orpanel              # Interactive CLI menu
orpanel --web        # Open web UI in browser
orpanel --tray       # Start in system tray
orpanel --version    # Show version
orpanel --help       # Show help
```

## Quick Start

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
go build -ldflags "-X main.AppVersion=dev" -o orPanel   # Linux/macOS
go build -ldflags "-X main.AppVersion=dev" -o orPanel.exe  # Windows
```

Requires Go 1.23+.

## Architecture

```
orpanel/
├── panel.go           # HTTP server, system tray, process management
├── cli.go             # CLI menu
├── cli_windows.go     # Windows detached process
├── config.go          # Config loading/saving, i18n
├── paths.go           # Cross-platform path resolution
├── omni_health.go     # OmniRoute health checks
├── omni_install.go    # npm install/update/repair operations
├── autostart_*.go     # Platform-specific autostart
├── dialog_*.go        # Platform-specific dialogs
├── web/               # HTML templates, JS modules (embedded)
├── themes/            # CSS variables (dark/light)
├── locales/           # i18n strings (tr/en/es)
└── scripts/install/   # curl/irm install scripts
```

## License

[MIT](LICENSE) — Made for the [OmniRoute](https://github.com/TorhunORG/OmniRoute) community.
