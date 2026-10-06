@echo off
setlocal
title PC Status Agent - installation

if not exist "%~dp0install.ps1" (
    echo ERROR: install.ps1 is missing.
    echo Extract the complete installer ZIP before running this file.
    pause
    exit /b 1
)

if not exist "%~dp0pc-agent.exe" (
    echo ERROR: pc-agent.exe is missing.
    echo Keep all installer files in the same folder.
    pause
    exit /b 1
)

echo Installing PC Status Agent...
echo Approve the Windows administrator prompt if it appears.
echo If pairing is needed, scan the QR code that opens in your browser.
echo.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1"
if errorlevel 1 (
    echo.
    echo ERROR: Installation did not complete.
    echo Check the error in the installer window.
    echo Logs are in "%ProgramData%\PCStatusAgent".
    pause
    exit /b 1
)

echo.
echo Installation complete. The background service is running.
echo Existing pairing and history have been preserved.
echo To uninstall: Windows Settings ^> Apps ^> PC Status Agent ^> Uninstall.
echo.
pause
exit /b 0
