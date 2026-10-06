param([switch]$Confirmed)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
$title = 'PC Status Agent'

try {
    if (-not $Confirmed) {
        $answer = [System.Windows.Forms.MessageBox]::Show(
            "Uninstall PC Status Agent?`n`nThis stops computer monitoring and parental locking, and removes local pairing data, recovery settings and logs.`n`nCloud history is not deleted. The computer will remain in the phone app until you remove it there.",
            $title,
            [System.Windows.Forms.MessageBoxButtons]::YesNo,
            [System.Windows.Forms.MessageBoxIcon]::Question
        )
        if ($answer -ne [System.Windows.Forms.DialogResult]::Yes) {
            exit 0
        }
    }

    if ([Environment]::Is64BitOperatingSystem -and -not [Environment]::Is64BitProcess) {
        $nativePowerShell = "$env:SystemRoot\Sysnative\WindowsPowerShell\v1.0\powershell.exe"
        $process = Start-Process -FilePath $nativePowerShell `
            -ArgumentList "-NoProfile -ExecutionPolicy Bypass -File `"$PSCommandPath`" -Confirmed" `
            -Wait -PassThru
        exit $process.ExitCode
    }

    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        $process = Start-Process -FilePath 'powershell.exe' `
            -ArgumentList "-NoProfile -ExecutionPolicy Bypass -File `"$PSCommandPath`" -Confirmed" `
            -Verb RunAs -Wait -PassThru
        exit $process.ExitCode
    }

    . (Join-Path $PSScriptRoot 'installer_support.ps1')
    Remove-AgentInstallation
    [System.Windows.Forms.MessageBox]::Show(
        "PC Status Agent has been uninstalled.`n`nLocal pairing data and logs have been removed. Cloud history has not been deleted.",
        $title,
        [System.Windows.Forms.MessageBoxButtons]::OK,
        [System.Windows.Forms.MessageBoxIcon]::Information
    ) | Out-Null
} catch {
    Write-Error $_ -ErrorAction Continue
    [System.Windows.Forms.MessageBox]::Show(
        "Uninstallation did not complete.`n`n$($_.Exception.Message)`n`nYou can retry using the installer ZIP's uninstall.ps1.",
        $title,
        [System.Windows.Forms.MessageBoxButtons]::OK,
        [System.Windows.Forms.MessageBoxIcon]::Error
    ) | Out-Null
    exit 1
}
