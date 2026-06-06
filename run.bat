@echo off
cd /d "%~dp0"
if not exist config.conf (
    echo config.conf not found. Copy config.conf.example to config.conf and fill in your keys.
    exit /b 1
)
awg-sproxy.exe %*
