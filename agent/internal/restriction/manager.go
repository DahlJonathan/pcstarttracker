package restriction

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"pc-tracker-agent/internal/api"
	"pc-tracker-agent/internal/config"
)

type screen interface {
	Ensure(url, secret string, locked bool) error
	Close() error
}

type localUI struct {
	store   *store
	secret  string
	mu      sync.Mutex
	readyAt time.Time
}

func (ui *localUI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("Origin") != "" || r.Header.Get("Authorization") != "Bearer "+ui.secret {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/state":
		ui.mu.Lock()
		ui.readyAt = time.Now()
		ui.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]bool{"locked": ui.store.snapshot().Locked}); err != nil {
			log.Printf("restriction UI response: %v", err)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/unlock":
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		var in struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid password request", http.StatusBadRequest)
			return
		}
		if err := ui.store.unlock(in.Password, time.Now()); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// Run uses a separate state file: telemetry/config writes cannot overwrite a
// locally unlocked restriction, and an offline unlock is tied to one revision.
func Run(ctx context.Context, cfg *config.Config, client *api.Client) error {
	sum := sha256.Sum256([]byte(cfg.DeviceID))
	return run(ctx, cfg, client, newScreen(), filepath.Join(config.Dir(), fmt.Sprintf("restriction-%x.json", sum[:8])))
}

func run(ctx context.Context, cfg *config.Config, client *api.Client, screen screen, path string) error {
	store, err := openStore(path)
	if err != nil {
		return err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	ui := &localUI{store: store, secret: hex.EncodeToString(secret)}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("restriction local listener: %w", err)
	}
	server := &http.Server{Handler: ui, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	serverError := make(chan error, 1)
	go func() { serverError <- server.Serve(listener) }()
	defer server.Close()
	defer func() {
		if err := screen.Close(); err != nil {
			log.Printf("close restriction screen: %v", err)
		}
	}()
	url := "http://" + listener.Addr().String()
	screenTick := time.NewTicker(time.Second)
	defer screenTick.Stop()
	pollTick := time.NewTicker(5 * time.Second)
	defer pollTick.Stop()
	lastScreenError := ""
	enforce := func() {
		if err := screen.Ensure(url, ui.secret, store.snapshot().Locked); err != nil {
			if err.Error() != lastScreenError {
				log.Printf("restriction screen: %v", err)
			}
			lastScreenError = err.Error()
		} else {
			lastScreenError = ""
		}
	}
	syncControl := func() {
		requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		current := store.snapshot()
		if current.OfflineUnlock {
			err := client.AckControl(requestCtx, cfg.DeviceID, cfg.DeviceToken,
				api.ControlAck{Revision: current.Revision, OfflineUnlock: true})
			if err == nil {
				if err := store.clearOffline(current.Revision); err != nil {
					log.Printf("save offline acknowledgement: %v", err)
				}
			} else {
				log.Printf("offline unlock acknowledgement: %v", err)
			}
		}
		command, err := client.Control(requestCtx, cfg.DeviceID, cfg.DeviceToken)
		if err != nil {
			log.Printf("control poll: %v", err)
			return
		}
		if err := store.apply(*command); err != nil {
			log.Printf("apply restriction: %v", err)
			if ackErr := client.AckControl(requestCtx, cfg.DeviceID, cfg.DeviceToken, api.ControlAck{Revision: command.Revision, Error: "Could not save restriction or recovery password"}); ackErr != nil {
				log.Printf("restriction failure acknowledgement: %v", ackErr)
			}
			return
		}
		enforce()
		current = store.snapshot()
		ui.mu.Lock()
		ready := time.Since(ui.readyAt) < 4*time.Second
		ui.mu.Unlock()
		// Do not claim that a lock is applied merely because Windows created
		// the process. The displayed screen must contact the local endpoint.
		if current.Locked && (!ready || lastScreenError != "") {
			message := "Waiting for the lock screen on the signed-in console"
			if lastScreenError != "" {
				message = "Could not open lock screen; check agent.log"
			}
			if err := client.AckControl(requestCtx, cfg.DeviceID, cfg.DeviceToken, api.ControlAck{Revision: current.Revision, Error: message}); err != nil {
				log.Printf("screen status acknowledgement: %v", err)
			}
			return
		}
		if command.Revision == 0 && command.PasswordHash == "" {
			return
		}
		if err := client.AckControl(requestCtx, cfg.DeviceID, cfg.DeviceToken,
			api.ControlAck{Revision: current.Revision, Locked: current.Locked, OfflineUnlock: current.OfflineUnlock}); err != nil {
			log.Printf("control acknowledgement: %v", err)
		}
	}
	enforce()
	syncControl()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-serverError:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return fmt.Errorf("restriction local server: %w", err)
		case <-screenTick.C:
			enforce()
		case <-pollTick.C:
			syncControl()
		}
	}
}
