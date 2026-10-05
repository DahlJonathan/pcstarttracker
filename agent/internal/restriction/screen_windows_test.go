//go:build windows

package restriction

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"pc-tracker-agent/internal/api"
)

func TestLockScreenPowerShellSyntax(t *testing.T) {
	script := fmt.Sprintf(lockScreenScript, "http://127.0.0.1:12345", "test-ui-secret")
	encoded := base64.StdEncoding.EncodeToString([]byte(script))
	parse := `$script = [System.Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + encoded + `')); $tokens = $null; $errors = $null; [System.Management.Automation.Language.Parser]::ParseInput($script, [ref]$tokens, [ref]$errors) | Out-Null; if ($errors.Count -gt 0) { $errors | ForEach-Object { $_.Message }; exit 1 }`
	output, err := exec.Command("powershell.exe", "-NoProfile", "-Command", parse).CombinedOutput()
	if err != nil {
		t.Fatalf("lock screen parser: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestInteractiveLockScreenOfflineRecovery(t *testing.T) {
	if os.Getenv("PC_STATUS_INTERACTIVE_LOCK_TEST") != "1" {
		t.Skip("requires explicit user consent to display a temporary lock screen")
	}
	s, err := openStore(filepath.Join(t.TempDir(), "restriction.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.apply(api.Control{Revision: 1, Locked: true, PasswordHash: passwordHash(t)}); err != nil {
		t.Fatal(err)
	}
	ui := &localUI{store: s, secret: "temporary-test-ui-token"}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: ui, ReadHeaderTimeout: time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Log(err)
		}
	}()
	defer server.Close()
	url := "http://" + listener.Addr().String()
	script := fmt.Sprintf(lockScreenScript, url, ui.secret)
	chars := utf16.Encode([]rune(script))
	data := make([]byte, len(chars)*2)
	for i, c := range chars {
		binary.LittleEndian.PutUint16(data[i*2:], c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-WindowStyle", "Hidden", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}()
	ready := false
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		ui.mu.Lock()
		ready = !ui.readyAt.IsZero()
		ui.mu.Unlock()
		if ready {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("screen exited before displaying: %v\n%s", err, output.String())
		case <-time.After(100 * time.Millisecond):
		}
	}
	if !ready {
		t.Fatal("screen never connected to local agent")
	}
	// No internet/backend is involved.
	for _, test := range []struct {
		password string
		want     int
	}{
		{"wrong-password", http.StatusForbidden},
	} {
		req, err := http.NewRequest(http.MethodPost, url+"/unlock",
			strings.NewReader(`{"password":"`+test.password+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+ui.secret)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != test.want {
			t.Fatalf("recovery status=%d want=%d", resp.StatusCode, test.want)
		}
	}
	automate := fmt.Sprintf(`
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Windows.Forms
$condition = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ProcessIdProperty, %d)
$windows = [System.Windows.Automation.AutomationElement]::RootElement.FindAll([System.Windows.Automation.TreeScope]::Children, $condition)
$passwordCondition = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::IsPasswordProperty, $true)
$password = $null
foreach ($candidate in $windows) {
    $password = $candidate.FindFirst([System.Windows.Automation.TreeScope]::Descendants, $passwordCondition)
    if ($null -ne $password) { $window = $candidate; break }
}
if ($null -eq $password) { throw 'Recovery password input was not found' }
$password.SetFocus()
[System.Windows.Forms.SendKeys]::SendWait('parent-only-password')
$buttonCondition = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::NameProperty, 'Unlock with recovery password')
$button = $window.FindFirst([System.Windows.Automation.TreeScope]::Descendants, $buttonCondition)
if ($null -eq $button) { throw 'Recovery button was not found' }
$invoke = $button.GetCurrentPattern([System.Windows.Automation.InvokePattern]::Pattern)
$invoke.Invoke()
`, cmd.Process.Pid)
	automation := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", automate)
	if result, err := automation.CombinedOutput(); err != nil {
		t.Fatalf("recovery password button automation: %v\n%s", err, result)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("screen recovery exit: %v\n%s", err, output.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("screen did not close after offline recovery")
	}
	if s.snapshot().Locked || !s.snapshot().OfflineUnlock {
		t.Fatal("recovery button did not persist offline unlock")
	}
}
