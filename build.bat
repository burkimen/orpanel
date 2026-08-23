@echo off
echo Omniroute Panel derleniyor...
set VERSION=0.0.0
if exist version.txt set /p VERSION=<version.txt
go build -ldflags "-X main.AppVersion=%VERSION%" -o orPanel.exe
if %errorlevel% neq 0 (
  echo Derleme hatasi!
  exit /b %errorlevel%
)
echo Derleme tamamlandi: orPanel.exe (v%VERSION%)
