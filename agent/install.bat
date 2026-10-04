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
    echo Keep install.bat and pc-agent.exe in the same folder.
    pause
    exit /b 1
)

echo ============================================================
echo  PC Status Agent - install
echo ============================================================
echo.
echo Step 1/3: Pairing this PC.
echo A browser window will open with a QR code and a 6-digit code.
echo Open the PC Status app on your phone, tap "Add Device",
echo and scan the QR (or type the code). This window continues
echo automatically once pairing is complete.
echo.
start "" /wait pc-agent.exe pair

echo Step 2/3: Installing the background service...
start "" /wait pc-agent.exe install

echo Step 3/3: Starting the service...
start "" /wait pc-agent.exe start

echo.
echo Done. The agent is installed and will start automatically
echo every time this PC boots. It runs invisibly in the background.
echo.
pause
