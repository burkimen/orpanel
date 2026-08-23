#!/bin/bash
set -e
echo "OrPanel derleniyor..."
VERSION="0.0.0"
[ -f version.txt ] && VERSION=$(cat version.txt)
go build -ldflags "-X main.AppVersion=$VERSION" -o orPanel
echo "Derleme tamamlandi: orPanel (v$VERSION)"
