# orpanel installer - irm | iex
# Tek komut: powershell -c "irm https://get.orpanel.dev/install.ps1 | iex"
# veya: irm https://raw.githubusercontent.com/burkimen/orpanel/main/scripts/install/install.ps1 | iex
# Standart: %LOCALAPPDATA%\Programs\Orpanel  + %LOCALAPPDATA%\Orpanel

param(
  [string]$Version = "latest",
  [string]$Repo = "burkimen/orpanel"
)

$ErrorActionPreference = "Stop"

$LocalAppData = $env:LOCALAPPDATA
if (-not $LocalAppData) { $LocalAppData = Join-Path $env:USERPROFILE "AppData\Local" }
$InstallDir = Join-Path $LocalAppData "Programs\Orpanel"
$DataDir = Join-Path $LocalAppData "Orpanel"
$BinPath = Join-Path $InstallDir "orPanel.exe"

# Detect arch
$Arch = if ([Environment]::Is64BitOperatingSystem) { "x64" } else { "x64" }
# Check ARM64
try { if ((Get-CimInstance Win32_ComputerSystem).SystemType -like "*ARM*") { $Arch = "arm64" } } catch {}

# Resolve version
if ($Version -eq "latest") {
  Write-Host "→ En son sürüm sorgulanıyor..."
  try {
    $api = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -TimeoutSec 10
    $Version = $api.tag_name.TrimStart("v")
  } catch {
    $Version = "1.0.0"
  }
}
$VersionNoV = $Version.TrimStart("v")
$Asset = "orpanel-v$VersionNoV-win32-$Arch.zip"
if ($Arch -eq "x64") { $Asset = "orpanel-v$VersionNoV-win32-x64.zip" }
$Url = "https://github.com/$Repo/releases/download/v$VersionNoV/$Asset"
$FallbackUrl = "https://raw.githubusercontent.com/$Repo/main/dist/$Asset"

Write-Host "→ Orpanel v$VersionNoV (win32/$Arch) indiriliyor..."
Write-Host "  $Url"

$TmpDir = Join-Path $env:TEMP "orpanel-install-$(Get-Random)"
New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null
try {
  # Download
  $ZipPath = Join-Path $TmpDir $Asset
  try {
    Invoke-WebRequest -Uri $Url -OutFile $ZipPath -UseBasicParsing
  } catch {
    Write-Host "  Release bulunamadı, fallback deneniyor..."
    Invoke-WebRequest -Uri $FallbackUrl -OutFile $ZipPath -UseBasicParsing
  }

  # Extract
  Expand-Archive -Path $ZipPath -DestinationPath $TmpDir\extract -Force

  # Find binary
  $BinSrc = Get-ChildItem -Recurse -Path $TmpDir\extract -Filter "orPanel.exe" | Select-Object -First 1 -ExpandProperty FullName
  if (-not $BinSrc) { $BinSrc = Get-ChildItem -Recurse -Path $TmpDir\extract -Filter "orpanel.exe" | Select-Object -First 1 -ExpandProperty FullName }
  if (-not $BinSrc) { throw "Binary bulunamadı" }

  # Install
  New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
  New-Item -ItemType Directory -Path $DataDir -Force | Out-Null
  Copy-Item -Path $BinSrc -Destination $BinPath -Force

  # Copy assets
  foreach ($d in @("themes","locales")) {
    $src = Join-Path $TmpDir\extract $d
    if (Test-Path $src) {
      Copy-Item -Recurse -Force -Path $src -Destination $InstallDir
      Copy-Item -Recurse -Force -Path $src -Destination $DataDir
    }
  }
  foreach ($f in @("app.ico","icon.png")) {
    $src = Join-Path $TmpDir\extract $f
    if (Test-Path $src) {
      Copy-Item -Force -Path $src -Destination $InstallDir
    }
  }

  # Add to user PATH
  $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $BinDir = $InstallDir
  if ($UserPath -notlike "*$BinDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$BinDir", "User")
    $env:Path += ";$BinDir"
    Write-Host "→ PATH'e eklendi: $BinDir (yeni terminalde aktif)"
  }

  # Autostart shortcut (optional)
  $StartMenu = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\Orpanel.lnk"
  # (WScript.Shell shortcut creation omitted for brevity - panel's own autostart handles registry)

  Write-Host "✓ Kuruldu: $BinPath"
  Write-Host "  Config: $DataDir\config.json"
  Write-Host "  Log:    $DataDir\panel.log"
  Write-Host ""
  Write-Host "Çalıştır: orPanel.exe   veya   orpanel (yeni terminal)"
  Write-Host "Güncelle: irm https://get.orpanel.dev/install.ps1 | iex   veya   orpanel update"
  Write-Host "Kaldır:   Remove-Item -Recurse -Force `"$InstallDir`", `"$DataDir`""
}
finally {
  Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}
