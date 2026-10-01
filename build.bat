@echo off
setlocal
cd /d "%~dp0"

where go >nul 2>&1
if errorlevel 1 (
    echo Go is not installed or not in PATH.
    echo Install Go 1.25+ from https://go.dev/dl/
    pause
    exit /b 1
)

go build -ldflags="-s -w" -o awg-sproxy.exe .
if errorlevel 1 (
    echo Build failed.
    pause
    exit /b 1
)

echo Built awg-sproxy.exe
