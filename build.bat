@echo off
echo Omniroute Panel Windows icin derleniyor...
go build -ldflags "-H=windowsgui" -o orPanel.exe
echo Derleme tamamlandi: orPanel.exe
pause