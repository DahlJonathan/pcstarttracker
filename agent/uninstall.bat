@echo off
setlocal
cd /d "%~dp0"

rem Re-launch elevated if not already running as administrator.
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo Requesting administrator privileges...
    powershell -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b
)

if not exist "pc-agent.exe" (
    echo ERROR: pc-agent.exe not found next to this script.
    echo Keep uninstall.bat and pc-agent.exe in the same folder.
    pause
    exit /b 1
)

echo ============================================================
echo  PC Status Agent - uninstall
echo ============================================================
echo.
echo Stopping the service...
start "" /wait pc-agent.exe stop

echo Removing the service...
start "" /wait pc-agent.exe uninstall

echo.
echo Done. The agent service has been stopped and removed.
echo.
pause
