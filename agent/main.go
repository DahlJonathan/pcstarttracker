package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"

	"pc-tracker-agent/internal/agent"
	"pc-tracker-agent/internal/api"
	"pc-tracker-agent/internal/config"
	"pc-tracker-agent/internal/pairing"
	"pc-tracker-agent/internal/winsvc"
)

// defaultServerURL is used on first run; override with PC_TRACKER_SERVER.
const defaultServerURL = "https://pcstarttracker-production.up.railway.app"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[pc-agent] ")

	isSvc, err := winsvc.IsWindowsService()
	if err != nil {
		log.Fatalf("detect service: %v", err)
	}
	if isSvc {
		if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
			log.Fatalf("create log directory: %v", err)
		}
		f, err := os.OpenFile(filepath.Join(config.Dir(), "agent.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			log.Fatalf("open service log: %v", err)
		}
		defer f.Close()
		log.SetOutput(f)
		if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
			log.Fatalf("capture service crashes: %v", err)
		}
		log.Printf("service process starting")
	}

	serverURL := defaultServerURL
	if v := os.Getenv("PC_TRACKER_SERVER"); v != "" {
		serverURL = v
	}

	cfg, err := config.Load(serverURL)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	// Allow the env var to override a previously saved server URL.
	if os.Getenv("PC_TRACKER_SERVER") != "" {
		cfg.ServerURL = serverURL
	}
	client := api.New(cfg.ServerURL)

	// If the SCM started us, always run as a service.
	if isSvc {
		if !cfg.Paired() {
			log.Fatal("service is not paired; run install.bat to pair this computer")
		}
		if err := winsvc.RunService(cfg, client); err != nil {
			log.Fatalf("service: %v", err)
		}
		return
	}

	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "pair":
		runPairing(cfg, client)
	case "install":
		mustAdmin(installService())
	case "uninstall":
		mustAdmin(winsvc.Uninstall())
		fmt.Println("Service uninstalled.")
	case "reset":
		mustAdmin(config.Remove())
		fmt.Println("Pairing data removed. Run 'pc-agent pair' to pair this PC again.")
	case "start":
		mustAdmin(winsvc.Start())
		fmt.Println("Service started.")
	case "stop":
		mustAdmin(winsvc.Stop())
		fmt.Println("Service stopped.")
	case "run":
		runInteractive(cfg, client)
	default:
		usage()
	}
}

// runPairing performs interactive onboarding.
func runPairing(cfg *config.Config, client *api.Client) {
	if cfg.Paired() {
		fmt.Println("This PC is already paired.")
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := pairing.Run(ctx, cfg, client); err != nil {
		log.Fatalf("pairing failed: %v", err)
	}
	fmt.Println("Pairing complete. Install the background service with:  pc-agent install")
}

// runInteractive runs the agent in the foreground (dev / manual mode). If the
// device is not yet paired it first walks the user through pairing.
func runInteractive(cfg *config.Config, client *api.Client) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !cfg.Paired() {
		log.Printf("device not paired yet; starting pairing...")
		if err := pairing.Run(ctx, cfg, client); err != nil {
			log.Fatalf("pairing failed: %v", err)
		}
	}

	runner := agent.NewRunner(cfg, client)
	done := make(chan struct{})
	go func() {
		runner.Run(ctx)
		close(done)
	}()

	<-ctx.Done()
	<-done
	log.Printf("stopping; sending shutdown event...")
	runner.SendShutdown()
}

func installService() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := winsvc.Install(exe); err != nil {
		return err
	}
	fmt.Println("Service installed. Start it now with:  pc-agent start")
	return nil
}

func mustAdmin(err error) {
	if err != nil {
		log.Fatalf("%v\n(hint: service commands require an elevated/Administrator prompt)", err)
	}
}

func usage() {
	fmt.Print(`PC Status Agent

Usage:
  pc-agent pair        Pair this PC with your account (shows QR + code)
  pc-agent run         Run in the foreground (pairs first if needed)
  pc-agent install     Install the background Windows service (admin)
  pc-agent start       Start the installed service (admin)
  pc-agent stop        Stop the installed service (admin)
  pc-agent uninstall   Remove the Windows service (admin)
  pc-agent reset       Remove local pairing data (shows QR again on next pair)

Environment:
  PC_TRACKER_SERVER    Backend base URL (default http://localhost:8080)
`)
}
