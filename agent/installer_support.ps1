function Get-AgentInstallPaths {
    $programFiles = $env:ProgramW6432
    if ([string]::IsNullOrWhiteSpace($programFiles)) {
        $programFiles = $env:ProgramFiles
    }
    if ([string]::IsNullOrWhiteSpace($programFiles) -or
        [string]::IsNullOrWhiteSpace($env:ProgramData)) {
        throw 'Windows installation directories are unavailable.'
    }
    [PSCustomObject]@{
        Data = Join-Path $env:ProgramData 'PCStatusAgent'
        Uninstaller = Join-Path $programFiles 'PCStatusAgent'
        Registry = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\PCStatusAgent'
    }
}

function Register-AgentUninstaller {
    param([Parameter(Mandatory = $true)][string]$SourceDirectory)

    $paths = Get-AgentInstallPaths
    $files = @('uninstall.ps1', 'installer_support.ps1')
    foreach ($file in $files) {
        if (-not (Test-Path -LiteralPath (Join-Path $SourceDirectory $file) -PathType Leaf)) {
            throw "The installer is incomplete: $file is missing."
        }
    }
    Get-AgentSafeDirectoryPath -Path $paths.Uninstaller | Out-Null
    New-Item -ItemType Directory -Path $paths.Uninstaller -Force -ErrorAction Stop | Out-Null
    foreach ($file in $files) {
        $source = Join-Path $SourceDirectory $file
        $destination = Join-Path $paths.Uninstaller $file
        if ([IO.Path]::GetFullPath($source) -ne [IO.Path]::GetFullPath($destination)) {
            Copy-Item -LiteralPath $source -Destination $destination -Force -ErrorAction Stop
        }
    }

    $powershell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $script = Join-Path $paths.Uninstaller 'uninstall.ps1'
    New-Item -Path $paths.Registry -Force -ErrorAction Stop | Out-Null
    $strings = @{
        DisplayName = 'PC Status Agent'
        Publisher = 'PC Status'
        InstallLocation = $paths.Data
        DisplayIcon = "$(Join-Path $paths.Data 'pc-agent.exe'),0"
        UninstallString = "`"$powershell`" -NoProfile -ExecutionPolicy Bypass -File `"$script`""
    }
    foreach ($name in $strings.Keys) {
        New-ItemProperty -Path $paths.Registry -Name $name -Value $strings[$name] `
            -PropertyType String -Force -ErrorAction Stop | Out-Null
    }
    foreach ($name in @('NoModify', 'NoRepair')) {
        New-ItemProperty -Path $paths.Registry -Name $name -Value 1 `
            -PropertyType DWord -Force -ErrorAction Stop | Out-Null
    }
}

function Get-AgentService {
    Get-Service -Name 'PCStatusAgent*' -ErrorAction Stop |
        Where-Object { $_.Name -eq 'PCStatusAgent' }
}

function Remove-AgentServiceRegistration {
    & "$env:SystemRoot\System32\sc.exe" delete PCStatusAgent
    if ($LASTEXITCODE -ne 0) {
        throw "Windows service removal failed (exit code $LASTEXITCODE)."
    }
}

function Get-AgentSafeDirectoryPath {
    param([Parameter(Mandatory = $true)][string]$Path)

    $fullPath = [IO.Path]::GetFullPath($Path).TrimEnd('\')
    if ([IO.Path]::GetFileName($fullPath) -ne 'PCStatusAgent') {
        throw "Refusing to remove an unexpected installation directory: $fullPath"
    }
    if (-not (Test-Path -LiteralPath $fullPath)) {
        return $fullPath
    }
    $directory = Get-Item -LiteralPath $fullPath -Force -ErrorAction Stop
    if (-not $directory.PSIsContainer -or
        ($directory.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
        throw "Refusing to remove a redirected installation directory: $fullPath"
    }
    $redirected = Get-ChildItem -LiteralPath $fullPath -Recurse -Force -ErrorAction Stop |
        Where-Object { $_.Attributes -band [IO.FileAttributes]::ReparsePoint }
    if ($redirected) {
        throw "The installation contains redirected files. Remove them manually before retrying: $fullPath"
    }
    return $fullPath
}

function Remove-AgentDirectory {
    param([Parameter(Mandatory = $true)][string]$Path)

    $fullPath = Get-AgentSafeDirectoryPath -Path $Path
    if (Test-Path -LiteralPath $fullPath) {
        Remove-Item -LiteralPath $fullPath -Recurse -Force -ErrorAction Stop
    }
}

function Remove-AgentInstallation {
    $paths = Get-AgentInstallPaths
    $service = Get-AgentService
    if ($null -ne $service) {
        try {
            if ($service.Status -ne 'Stopped') {
                Stop-Service -InputObject $service -ErrorAction Stop
                $service.WaitForStatus('Stopped', [TimeSpan]::FromSeconds(30))
            }
        } finally {
            $service.Dispose()
        }
        Remove-AgentServiceRegistration
        $deadline = [DateTime]::UtcNow.AddSeconds(30)
        while ($null -ne ($remaining = Get-AgentService)) {
            $remaining.Dispose()
            if ([DateTime]::UtcNow -ge $deadline) {
                throw 'Windows has not finished removing the service. Close the Services window, or restart Windows, then retry.'
            }
            Start-Sleep -Milliseconds 200
        }
    }

    Remove-AgentDirectory -Path $paths.Data
    Remove-AgentDirectory -Path $paths.Uninstaller
    if (Test-Path -LiteralPath $paths.Registry) {
        Remove-Item -LiteralPath $paths.Registry -Recurse -Force -ErrorAction Stop
    }
}
