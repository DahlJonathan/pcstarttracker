$ErrorActionPreference = 'Stop'
if ([Environment]::Is64BitOperatingSystem -and -not [Environment]::Is64BitProcess) {
    $nativePowerShell = "$env:SystemRoot\Sysnative\WindowsPowerShell\v1.0\powershell.exe"
    $p = Start-Process -FilePath $nativePowerShell `
        -ArgumentList "-NoProfile -ExecutionPolicy Bypass -File `"$PSCommandPath`"" `
        -Wait -PassThru
    exit $p.ExitCode
}
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    $p = Start-Process -FilePath 'powershell.exe' -ArgumentList "-NoProfile -ExecutionPolicy Bypass -File `"$PSCommandPath`"" -Verb RunAs -Wait -PassThru
    exit $p.ExitCode
}

try {
    foreach ($file in @('installer_support.ps1', 'uninstall.ps1')) {
        if (-not (Test-Path -LiteralPath (Join-Path $PSScriptRoot $file) -PathType Leaf)) {
            throw "The installer is incomplete: $file is missing. Extract the complete ZIP."
        }
    }
    . (Join-Path $PSScriptRoot 'installer_support.ps1')
    $source = Join-Path $PSScriptRoot 'pc-agent.exe'
    if (-not (Test-Path -LiteralPath $source)) {
        throw 'Keep install.ps1 and pc-agent.exe in the same folder.'
    }
    $dir = Join-Path $env:ProgramData 'PCStatusAgent'
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $service = Get-Service -Name PCStatusAgent -ErrorAction SilentlyContinue
    if ($null -ne $service -and $service.Status -ne 'Stopped') {
        Stop-Service -InputObject $service
        $service.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
    }
    Write-Host 'Pairing this computer if needed. Scan the browser QR code in the phone app.'
    $pair = Start-Process -FilePath $source -ArgumentList 'pair' -WindowStyle Hidden -RedirectStandardOutput (Join-Path $dir 'pair.log') -RedirectStandardError (Join-Path $dir 'pair-error.log') -Wait -PassThru
    if ($pair.ExitCode -ne 0) {
        throw "Pairing failed. Check $dir\pair-error.log."
    }
    $dest = Join-Path $dir 'pc-agent.exe'
    if ([IO.Path]::GetFullPath($source) -ne [IO.Path]::GetFullPath($dest)) {
        Copy-Item -LiteralPath $source -Destination $dest -Force
    }
    & $dest install
    if ($LASTEXITCODE -ne 0) { throw 'Windows service installation failed.' }
    & $dest start
    if ($LASTEXITCODE -ne 0) { throw "Windows service startup failed. Check $dir\agent.log." }
    Register-AgentUninstaller -SourceDirectory $PSScriptRoot
    Write-Host 'Installed. Pairing and history have been preserved.'
    Write-Host 'To uninstall: Windows Settings > Apps > PC Status Agent > Uninstall.'
    Write-Host 'Set a recovery password in the phone app before using parental lock.'
} catch {
    Write-Error $_ -ErrorAction Continue
    exit 1
}
