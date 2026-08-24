# orpanel installer - irm | iex
# Tek komut: powershell -c "irm https://get.orpanel.dev/install.ps1 | iex"

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
$Arch = "x64"
try { if ((Get-CimInstance Win32_ComputerSystem).SystemType -like "*ARM*") { $Arch = "arm64" } } catch {}

# Resolve version
if ($Version -eq "latest") {
  Write-Host "-> En son surum sorgulaniyor..."
  try {
    $api = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -TimeoutSec 10
    $Version = $api.tag_name.TrimStart("v")
  } catch {
    $Version = "1.0.0"
  }
}
$VersionNoV = $Version.TrimStart("v")
$ExeName = "orpanel-win32-x64.exe"
if ($Arch -eq "arm64") { $ExeName = "orpanel-win32-arm64.exe" }
$Url = "https://github.com/$Repo/releases/download/v$VersionNoV/$ExeName"

Write-Host "-> Orpanel v$VersionNoV (win32/$Arch) indiriliyor..."
Write-Host "  $Url"

$TmpDir = Join-Path $env:TEMP "orpanel-install-$(Get-Random)"
New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null
try {
  $ExePath = Join-Path $TmpDir $ExeName
  Invoke-WebRequest -Uri $Url -OutFile $ExePath -UseBasicParsing

  # Install
  New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
  New-Item -ItemType Directory -Path $DataDir -Force | Out-Null
  Copy-Item -Path $ExePath -Destination $BinPath -Force

  # Add to user PATH
  $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
    $env:Path += ";$InstallDir"
    Write-Host "-> PATH'e eklendi: $InstallDir"
  }

  Write-Host ""
  Write-Host "Kuruldu: $BinPath"
  Write-Host ""
  Write-Host "Calistir: orPanel.exe   veya   orpanel (yeni terminal)"
  Write-Host "Guncelle: irm https://raw.githubusercontent.com/$Repo/main/scripts/install/install.ps1 | iex"
  Write-Host "Kaldir:   Remove-Item -Recurse -Force `"$InstallDir`", `"$DataDir`""
}
finally {
  Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}
