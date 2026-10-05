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
echo and scan the QR (or type the code). This installer continues
echo automatically once pairing is complete.
echo.
powershell -NoProfile -Command "$ErrorActionPreference = 'Stop'; $dir = Join-Path $env:ProgramData 'PCStatusAgent'; New-Item -ItemType Directory -Path $dir -Force | Out-Null; $p = Start-Process -FilePath (Join-Path $PWD 'pc-agent.exe') -ArgumentList 'pair' -WindowStyle Hidden -RedirectStandardOutput (Join-Path $dir 'pair.log') -RedirectStandardError (Join-Path $dir 'pair-error.log') -Wait -PassThru; exit $p.ExitCode"
if %errorlevel% neq 0 (
    echo.
    echo Pairing was not completed. Nothing was installed.
    echo Check "%ProgramData%\PCStatusAgent\pair-error.log".
    echo Run install.bat again to try a new code.
    echo.
    pause
    exit /b 1
)

echo Step 2/3: Installing the background service...
powershell -NoProfile -Command "$ErrorActionPreference = 'Stop'; try { $s = Get-Service -Name PCStatusAgent -ErrorAction SilentlyContinue; if ($null -ne $s) { Stop-Service -InputObject $s; $s.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(20)) }; $dir = Join-Path $env:ProgramData 'PCStatusAgent'; New-Item -ItemType Directory -Path $dir -Force | Out-Null; $source = Join-Path $PWD 'pc-agent.exe'; $dest = Join-Path $dir 'pc-agent.exe'; if ($source -ne $dest) { Copy-Item -LiteralPath $source -Destination $dest -Force }; & $dest install; exit $LASTEXITCODE } catch { Write-Error $_; exit 1 }"
if errorlevel 1 goto failed

echo Step 3/3: Starting the service...
pc-agent.exe start
if errorlevel 1 goto failed

echo.
echo Done. The agent is installed and will start automatically
echo every time this PC boots. It runs invisibly in the background.
echo.
pause
exit /b 0

:failed
echo.
echo ERROR: The background service was not installed or started correctly.
echo Check the error above and "%ProgramData%\PCStatusAgent\agent.log".
echo Pairing data has been kept. Fix the error and run install.bat again.
pause
exit /b 1
