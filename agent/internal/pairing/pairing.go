package pairing

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"pc-tracker-agent/internal/api"
	"pc-tracker-agent/internal/config"
)

// Run performs interactive pairing: it starts a session with the backend,
// displays a QR code + 6-digit code in the user's browser, waits for the mobile
// app to claim the device, then persists the permanent token into cfg.
//
// It blocks until pairing completes, the session expires, or ctx is cancelled.
func Run(ctx context.Context, cfg *config.Config, client *api.Client) error {
	init, err := client.PairInit(ctx, cfg.DeviceID, cfg.DeviceName)
	if err != nil {
		return fmt.Errorf("start pairing: %w", err)
	}

	png, err := qrcode.Encode(init.PairingToken, qrcode.Medium, 320)
	if err != nil {
		return fmt.Errorf("encode qr: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	url := fmt.Sprintf("http://%s/", ln.Addr().String())

	page := renderPage(base64.StdEncoding.EncodeToString(png), init.Code, init.ExpiresAt)
	httpSrv := &http.Server{Handler: pageHandler(page)}
	go func() { _ = httpSrv.Serve(ln) }()
	defer func() { _ = httpSrv.Close() }()

	log.Printf("Pairing code: %s  (open %s to scan the QR code)", init.Code, url)
	openBrowser(url)

	// Poll until the user claims the device or the window closes.
	deadline := init.ExpiresAt
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("pairing code expired; please restart pairing")
			}
			st, err := client.PairStatus(ctx, init.PairingToken)
			if err != nil {
				log.Printf("pair poll: %v", err)
				continue
			}
			if st.Claimed && st.DeviceToken != "" {
				cfg.DeviceToken = st.DeviceToken
				if st.DeviceID != "" {
					cfg.DeviceID = st.DeviceID
				}
				if err := cfg.Save(); err != nil {
					return fmt.Errorf("save token: %w", err)
				}
				log.Printf("Device paired successfully.")
				return nil
			}
		}
	}
}

func pageHandler(page []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
}

var pageTmpl = template.Must(template.New("pair").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Pair this PC</title>
<style>
  body{font-family:system-ui,Segoe UI,Roboto,sans-serif;background:#0f1220;color:#e8eaf0;
       display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}
  .card{background:#1a1e33;border-radius:20px;padding:40px;max-width:420px;text-align:center;
        box-shadow:0 20px 60px rgba(0,0,0,.5)}
  h1{margin:0 0 8px;font-size:22px}
  p{color:#9aa0bf;margin:6px 0 24px}
  img{border-radius:16px;background:#fff;padding:12px}
  .code{font-size:40px;letter-spacing:10px;font-weight:700;margin:24px 0 6px;color:#6ee7b7}
  .hint{font-size:13px;color:#6b7194}
</style></head>
<body><div class="card">
  <h1>Pair this PC</h1>
  <p>Open the PC Status app on your phone and tap <b>Add Device</b>.</p>
  <img src="data:image/png;base64,{{.QR}}" alt="QR code" width="320" height="320">
  <div class="code">{{.Code}}</div>
  <div class="hint">Or enter this 6-digit code manually. Expires at {{.Expires}}.</div>
</div></body></html>`))

func renderPage(qrB64, code string, expires time.Time) []byte {
	var buf stringWriter
	_ = pageTmpl.Execute(&buf, map[string]string{
		"QR":      qrB64,
		"Code":    code,
		"Expires": expires.Local().Format("15:04"),
	})
	return buf.b
}

// stringWriter is a tiny io.Writer collecting bytes without importing bytes.
type stringWriter struct{ b []byte }

func (s *stringWriter) Write(p []byte) (int, error) { s.b = append(s.b, p...); return len(p), nil }

// openBrowser best-effort opens the default browser at url.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("could not open browser automatically: %v", err)
	}
}
