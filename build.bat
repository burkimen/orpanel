@echo off
echo Omniroute Panel Windows icin derleniyor...
go build -ldflags "-H=windowsgui" -o orPanel.exe
if %errorlevel% neq 0 (
  echo Derleme hatasi!
  exit /b %errorlevel%
)
echo Derleme tamamlandi: orPanel.exe
echo Not: app.ico, icon.png, themes\ ve locales\ exe ile ayni dizinde olmali
