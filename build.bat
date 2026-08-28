@echo off
cd /d "%~dp0"

if not exist dist mkdir dist
go build -trimpath -ldflags="-s -w" -o dist\jetkvm-desktop.exe .\cmd\jetkvm-desktop

echo Built: dist\jetkvm-desktop.exe
