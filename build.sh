#!/bin/bash
set -e
echo "Omniroute Panel derleniyor..."

# Current platform
echo "-> $(go env GOOS)/$(go env GOARCH) için derleniyor"
go build -o orPanel
echo "Derleme tamamlandi: ./orPanel"
echo "Not: app.ico Windows, icon.png Linux/mac tray icin exe yaninda kalmali"
echo "Not: themes/ ve locales/ klasorleri exe ile ayni dizinde olmali"

# Optional cross builds (requires proper CGO for systray)
# Uncomment to build all:
# echo "-> linux/amd64"
# CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o dist/orPanel-linux-amd64
# echo "-> darwin/amd64"
# CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -o dist/orPanel-darwin-amd64
# echo "-> darwin/arm64"
# CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -o dist/orPanel-darwin-arm64
# echo "-> windows/amd64"
# GOOS=windows GOARCH=amd64 go build -ldflags "-H=windowsgui" -o dist/orPanel.exe

echo "Bitti."
