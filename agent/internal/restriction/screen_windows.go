//go:build windows

package restriction

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsScreen struct {
	process windows.Handle
	session uint32
}

func newScreen() screen { return &windowsScreen{} }

func (s *windowsScreen) Close() error {
	if s.process == 0 {
		return nil
	}
	result, err := windows.WaitForSingleObject(s.process, 0)
	if err == nil && result == uint32(windows.WAIT_TIMEOUT) {
		err = windows.TerminateProcess(s.process, 0)
	}
	closeErr := windows.CloseHandle(s.process)
	s.process = 0
	if err != nil {
		return fmt.Errorf("terminate lock screen: %w", err)
	}
	return closeErr
}

func (s *windowsScreen) Ensure(url, secret string, locked bool) error {
	if !locked {
		return s.Close()
	}
	session := windows.WTSGetActiveConsoleSessionId()
	if session == 0xffffffff {
		return fmt.Errorf("no active Windows console session")
	}
	if s.process != 0 {
		result, err := windows.WaitForSingleObject(s.process, 0)
		if err != nil {
			return err
		}
		if result == uint32(windows.WAIT_TIMEOUT) && session == s.session {
			return nil
		}
		if result == uint32(windows.WAIT_TIMEOUT) {
			if err := s.Close(); err != nil {
				return err
			}
		} else {
			windows.CloseHandle(s.process)
			s.process = 0
		}
	}
	var token windows.Token
	if err := windows.WTSQueryUserToken(session, &token); err != nil {
		return fmt.Errorf("get signed-in user's token: %w", err)
	}
	defer token.Close()
	// Never show a SYSTEM-privileged GUI. Launch with the signed-in user's
	// normal token on their desktop, with no inherited service handles.
	var env *uint16
	if err := windows.CreateEnvironmentBlock(&env, token, false); err != nil {
		return err
	}
	defer windows.DestroyEnvironmentBlock(env)
	script := fmt.Sprintf(lockScreenScript, url, secret)
	chars := utf16.Encode([]rune(script))
	bytes := make([]byte, len(chars)*2)
	for i, c := range chars {
		binary.LittleEndian.PutUint16(bytes[i*2:], c)
	}
	exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	exe16, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	cmd, err := windows.UTF16PtrFromString(windows.EscapeArg(exe) + " -NoProfile -STA -WindowStyle Hidden -EncodedCommand " + base64.StdEncoding.EncodeToString(bytes))
	if err != nil {
		return err
	}
	desktop, err := windows.UTF16PtrFromString(`winsta0\default`)
	if err != nil {
		return err
	}
	si := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{})), Desktop: desktop}
	var pi windows.ProcessInformation
	if err := windows.CreateProcessAsUser(token, exe16, cmd, nil, nil, false,
		windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW, env, nil, &si, &pi); err != nil {
		return fmt.Errorf("launch lock screen: %w", err)
	}
	windows.CloseHandle(pi.Thread)
	s.process = pi.Process
	s.session = session
	return nil
}

const lockScreenScript = `
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName PresentationFramework
Add-Type -AssemblyName System.Windows.Forms
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class LockWindowPosition {
    [DllImport("user32.dll", SetLastError = true)]
    public static extern bool SetWindowPos(IntPtr hwnd, IntPtr insertAfter,
        int x, int y, int width, int height, uint flags);
}
'@
$base = '%s'
$headers = @{ Authorization = 'Bearer %s' }
$script:allowClose = $false
$script:windows = @()
$primary = [System.Windows.Forms.Screen]::PrimaryScreen
foreach ($screen in [System.Windows.Forms.Screen]::AllScreens) {
    $window = New-Object System.Windows.Window
    $window.Title = 'PC Status parental lock'
    $window.WindowStyle = 'None'
    $window.ResizeMode = 'NoResize'
    $window.Topmost = $true
    $window.Background = '#18243B'
    $window.WindowStartupLocation = 'Manual'
    $window.Left = $screen.Bounds.Left
    $window.Top = $screen.Bounds.Top
    $window.Width = $screen.Bounds.Width
    $window.Height = $screen.Bounds.Height
    $window.Tag = $screen.Bounds
    $window.Add_Loaded({
        param($sender, $e)
        $bounds = $sender.Tag
        $helper = New-Object System.Windows.Interop.WindowInteropHelper($sender)
        if (-not [LockWindowPosition]::SetWindowPos($helper.Handle, [IntPtr](-1),
            $bounds.Left, $bounds.Top, $bounds.Width, $bounds.Height, 0x40)) {
            throw 'Could not position parental lock on the monitor.'
        }
    })
    $window.Add_Closing({ param($sender, $e) if (-not $script:allowClose) { $e.Cancel = $true } })
    $panel = New-Object System.Windows.Controls.StackPanel
    $panel.VerticalAlignment = 'Center'
    $panel.HorizontalAlignment = 'Center'
    $panel.Width = 420
    $title = New-Object System.Windows.Controls.TextBlock
    $title.Text = 'Time for a break'
    $title.FontSize = 36
    $title.Foreground = 'White'
    $title.TextAlignment = 'Center'
    $title.Margin = '0,0,0,20'
    $panel.Children.Add($title) | Out-Null
    if ($screen.Primary) {
        $script:main = $window
        $help = New-Object System.Windows.Controls.TextBlock
        $help.Text = 'A parent can unlock this computer from the phone, or enter the recovery password here. This password works without internet.'
        $help.TextWrapping = 'Wrap'
        $help.Foreground = '#DEE7F7'
        $help.FontSize = 18
        $help.Margin = '0,0,0,24'
        $panel.Children.Add($help) | Out-Null
        $script:password = New-Object System.Windows.Controls.PasswordBox
        $password.FontSize = 22
        $password.Padding = '12'
        $password.MaxLength = 72
        $panel.Children.Add($password) | Out-Null
        $script:message = New-Object System.Windows.Controls.TextBlock
        $message.Foreground = '#FFD4A0'
        $message.TextWrapping = 'Wrap'
        $message.Margin = '0,12,0,12'
        $panel.Children.Add($message) | Out-Null
        $button = New-Object System.Windows.Controls.Button
        $button.Content = 'Unlock with recovery password'
        $button.Padding = '16'
        $button.FontSize = 18
        $button.Add_Click({
            try {
                $body = @{password=$script:password.Password} | ConvertTo-Json
                Invoke-RestMethod -Uri ($base + '/unlock') -Method Post -Headers $headers -ContentType 'application/json' -Body ([System.Text.Encoding]::UTF8.GetBytes($body)) -TimeoutSec 3 | Out-Null
                $script:password.Clear()
                $script:allowClose = $true
                $timer.Stop()
                foreach ($win in $script:windows) { $win.Close() }
            } catch {
                $script:password.Clear()
                $script:message.Text = 'Could not unlock. Check the password. After 5 failed attempts, wait one minute.'
            }
        })
        $panel.Children.Add($button) | Out-Null
    }
    $window.Content = $panel
    $script:windows += $window
}
$timer = New-Object System.Windows.Threading.DispatcherTimer
$timer.Interval = [TimeSpan]::FromSeconds(1)
$timer.Add_Tick({
    try {
        $state = Invoke-RestMethod -Uri ($base + '/state') -Headers $headers -TimeoutSec 2
        if (-not $state.locked) {
            $script:allowClose = $true
            $timer.Stop()
            foreach ($win in $script:windows) { $win.Close() }
            return
        }
        foreach ($win in $script:windows) {
            $win.WindowState = 'Normal'
            $win.Topmost = $true
        }
        $script:main.Activate() | Out-Null
    } catch {
        $script:message.Text = 'The local agent is unavailable. Ask a parent to restart the PC Status Agent service.'
    }
})
foreach ($win in $script:windows) { if ($win -ne $script:main) { $win.Show() } }
$timer.Start()
$script:main.ShowDialog() | Out-Null
`
