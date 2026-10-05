package restriction

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"pc-tracker-agent/internal/api"
	"pc-tracker-agent/internal/config"
)

type simulatedScreen struct {
	mu          sync.Mutex
	url, secret string
	locked      bool
}

func (s *simulatedScreen) Ensure(url, secret string, locked bool) error {
	s.mu.Lock()
	s.url, s.secret, s.locked = url, secret, locked
	s.mu.Unlock()
	if locked {
		r, err := http.NewRequest(http.MethodGet, url+"/state", nil)
		if err != nil {
			return err
		}
		r.Header.Set("Authorization", "Bearer "+secret)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}
func (s *simulatedScreen) Close() error { return nil }

func TestManagerConfirmsScreenAndPersistsOfflineUnlock(t *testing.T) {
	hash := passwordHash(t)
	acks := make(chan api.ControlAck, 20)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer device-token" {
			t.Error("missing device auth")
		}
		if r.Method == http.MethodPost {
			var ack api.ControlAck
			if err := json.NewDecoder(r.Body).Decode(&ack); err != nil {
				t.Error(err)
			}
			acks <- ack
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(api.Control{Revision: 1, Locked: true, PasswordHash: hash}); err != nil {
				t.Error(err)
			}
		}
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	screen := &simulatedScreen{}
	path := filepath.Join(t.TempDir(), "restriction.json")
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, &config.Config{DeviceID: "pc", DeviceToken: "device-token"}, api.New(backend.URL), screen, path)
	}()
	select {
	case ack := <-acks:
		if !ack.Locked || ack.Revision != 1 || ack.Error != "" {
			t.Fatalf("bad acknowledgement: %+v", ack)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing acknowledgement")
	}
	// Stop the backend to prove that offline recovery does not require it.
	backend.Close()
	screen.mu.Lock()
	url, secret := screen.url, screen.secret
	screen.mu.Unlock()
	body := `{"password":"parent-only-password"}`
	r, err := http.NewRequest(http.MethodPost, url+"/unlock", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("offline recovery=%d", resp.StatusCode)
	}
	persisted, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.snapshot().Locked || !persisted.snapshot().OfflineUnlock {
		t.Fatal("offline unlock not persisted")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not stop")
	}
}
