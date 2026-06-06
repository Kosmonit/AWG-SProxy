@echo off
cd /d "%~dp0"
go build -ldflags="-s -w" -o awg-sproxy.exe .
if %ERRORLEVEL% EQU 0 (
    echo Built awg-sproxy.exe
) else (
    echo Build failed.
    exit /b 1
)
