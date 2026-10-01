@echo off
setlocal
cd /d "%~dp0"

if not exist "config.conf" (
    echo config.conf not found.
    echo Copy one of the config.*.conf.example templates to config.conf and fill in your keys.
    pause
    exit /b 1
)

if not exist "awg-sproxy.exe" (
    echo awg-sproxy.exe not found. Run build.bat first.
    pause
    exit /b 1
)

"%~dp0awg-sproxy.exe" %*
set "EXITCODE=%ERRORLEVEL%"
if not "%EXITCODE%"=="0" pause
exit /b %EXITCODE%
