. (Join-Path $PSScriptRoot 'installer_support.ps1')
Add-Type -AssemblyName System.ServiceProcess

Describe 'Windows installed-app registration' {
    BeforeEach {
        $script:paths = [PSCustomObject]@{
            Data = 'C:\ProgramData\PCStatusAgent'
            Uninstaller = 'C:\Program Files\PCStatusAgent'
            Registry = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\PCStatusAgent'
        }
        $script:properties = @{}
        Mock Get-AgentInstallPaths { $script:paths }
        Mock Get-AgentSafeDirectoryPath { $Path }
        Mock Test-Path { $true }
        Mock New-Item {}
        Mock Copy-Item {}
        Mock New-ItemProperty { $script:properties[$Name] = $Value }
    }

    It 'registers a quoted uninstall command and copies protected scripts' {
        Register-AgentUninstaller -SourceDirectory 'C:\Downloads\PC Agent'
        $script:properties.DisplayName | Should Be 'PC Status Agent'
        $script:properties.UninstallString | Should Be (
            "`"$(Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe')`" -NoProfile -ExecutionPolicy Bypass -File `"C:\Program Files\PCStatusAgent\uninstall.ps1`""
        )
        $script:properties.NoModify | Should Be 1
        $script:properties.NoRepair | Should Be 1
        Assert-MockCalled Copy-Item -Times 2 -Exactly -Scope It
        Assert-MockCalled New-ItemProperty -Times 7 -Exactly -Scope It
    }

    It 'does not create an uninstall entry for an incomplete package' {
        Mock Test-Path { $false } -ParameterFilter { $LiteralPath -like '*uninstall.ps1' }
        { Register-AgentUninstaller -SourceDirectory 'C:\Downloads\PC Agent' } | Should Throw
        Assert-MockCalled New-ItemProperty -Times 0 -Exactly -Scope It
    }

    It 'surfaces registration failures instead of reporting success' {
        Mock New-ItemProperty { throw 'Registry access failed' }
        { Register-AgentUninstaller -SourceDirectory 'C:\Downloads\PC Agent' } | Should Throw
    }

    It 'does not write uninstall scripts through redirected directories' {
        Mock Get-AgentSafeDirectoryPath { throw 'Redirected directory' }
        { Register-AgentUninstaller -SourceDirectory 'C:\Downloads\PC Agent' } | Should Throw
        Assert-MockCalled Copy-Item -Times 0 -Exactly -Scope It
        Assert-MockCalled New-ItemProperty -Times 0 -Exactly -Scope It
    }
}

Describe 'Complete agent uninstallation' {
    BeforeEach {
        $script:operations = [System.Collections.Generic.List[string]]::new()
        $script:serviceDeleted = $false
        $script:failStop = $false
        $script:service = New-Object System.ServiceProcess.ServiceController
        $script:service | Add-Member NoteProperty Status 'Running' -Force
        $script:service | Add-Member ScriptMethod WaitForStatus {
            param($status, $timeout)
            if ($script:failStop) { throw 'Stop timed out' }
            $script:operations.Add('wait-stopped')
        } -Force
        $script:service | Add-Member ScriptMethod Dispose {
            $script:operations.Add('dispose-service')
        } -Force
        Mock Get-AgentInstallPaths {
            [PSCustomObject]@{
                Data = 'C:\ProgramData\PCStatusAgent'
                Uninstaller = 'C:\Program Files\PCStatusAgent'
                Registry = 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\PCStatusAgent'
            }
        }
        Mock Get-AgentService {
            if (-not $script:serviceDeleted) { $script:service }
        }
        Mock Stop-Service { $script:operations.Add('stop-service') }
        Mock Remove-AgentServiceRegistration {
            $script:operations.Add('delete-service')
            $script:serviceDeleted = $true
        }
        Mock Remove-AgentDirectory { $script:operations.Add("delete-directory:$Path") }
        Mock Test-Path { $true }
        Mock Remove-Item { $script:operations.Add('delete-registry') }
    }

    It 'stops and unregisters the service before deleting data and the app entry' {
        Remove-AgentInstallation
        ($script:operations -join '|') | Should Be (
            'stop-service|wait-stopped|dispose-service|delete-service|' +
            'delete-directory:C:\ProgramData\PCStatusAgent|' +
            'delete-directory:C:\Program Files\PCStatusAgent|delete-registry'
        )
    }

    It 'handles a stopped service without sending another stop' {
        $script:service.Status = 'Stopped'
        Remove-AgentInstallation
        Assert-MockCalled Stop-Service -Times 0 -Exactly -Scope It
        Assert-MockCalled Remove-AgentServiceRegistration -Times 1 -Exactly -Scope It
    }

    It 'cleans up an installation even if its service is already absent' {
        $script:serviceDeleted = $true
        Remove-AgentInstallation
        Assert-MockCalled Remove-AgentServiceRegistration -Times 0 -Exactly -Scope It
        Assert-MockCalled Remove-AgentDirectory -Times 2 -Exactly -Scope It
        Assert-MockCalled Remove-Item -Times 1 -Exactly -Scope It
    }

    It 'does not delete data if stopping the service fails' {
        $script:failStop = $true
        { Remove-AgentInstallation } | Should Throw
        Assert-MockCalled Remove-AgentServiceRegistration -Times 0 -Exactly -Scope It
        Assert-MockCalled Remove-AgentDirectory -Times 0 -Exactly -Scope It
        Assert-MockCalled Remove-Item -Times 0 -Exactly -Scope It
    }

    It 'does not delete data if service deletion fails' {
        Mock Remove-AgentServiceRegistration { throw 'Delete failed' }
        { Remove-AgentInstallation } | Should Throw
        Assert-MockCalled Remove-AgentDirectory -Times 0 -Exactly -Scope It
        Assert-MockCalled Remove-Item -Times 0 -Exactly -Scope It
    }

    It 'retains the installed-app entry if local cleanup fails' {
        Mock Remove-AgentDirectory { throw 'File is locked' }
        { Remove-AgentInstallation } | Should Throw
        Assert-MockCalled Remove-Item -Times 0 -Exactly -Scope It
    }
}

Describe 'Safe installation-directory cleanup' {
    It 'deletes local data only inside the specified app directory' {
        $directory = Join-Path $TestDrive 'PCStatusAgent'
        New-Item -ItemType Directory -Path $directory | Out-Null
        Set-Content -LiteralPath (Join-Path $directory 'config.json') -Value '{}'
        Set-Content -LiteralPath (Join-Path $directory 'agent.log') -Value 'test'
        $unrelated = Join-Path $TestDrive 'unrelated.txt'
        Set-Content -LiteralPath $unrelated -Value 'keep'
        Remove-AgentDirectory -Path $directory
        Test-Path -LiteralPath $directory | Should Be $false
        Test-Path -LiteralPath $unrelated | Should Be $true
    }

    It 'rejects broad or unrelated directories' {
        { Remove-AgentDirectory -Path $env:ProgramData } | Should Throw
        { Remove-AgentDirectory -Path $TestDrive } | Should Throw
    }

    It 'rejects redirected application directories' {
        Mock Test-Path { $true }
        Mock Get-Item {
            [PSCustomObject]@{
                PSIsContainer = $true
                Attributes = [IO.FileAttributes]::ReparsePoint
            }
        }
        Mock Remove-Item {}
        { Remove-AgentDirectory -Path 'C:\ProgramData\PCStatusAgent' } | Should Throw
        Assert-MockCalled Remove-Item -Times 0 -Exactly -Scope It
    }
}
