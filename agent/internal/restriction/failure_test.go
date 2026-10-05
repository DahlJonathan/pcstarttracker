package restriction

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"pc-tracker-agent/internal/api"
	"pc-tracker-agent/internal/config"
)

type unavailableScreen struct{}

func (*unavailableScreen) Ensure(_, _ string, locked bool) error {
	if locked {
		return fmt.Errorf("no console session")
	}
	return nil
}
func (*unavailableScreen) Close() error { return nil }

func TestScreenFailureIsNeverAcknowledgedAsLocked(t *testing.T) {
	hash := passwordHash(t)
	acks := make(chan api.ControlAck, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer backend.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, &config.Config{DeviceID: "pc", DeviceToken: "test-token"}, api.New(backend.URL), &unavailableScreen{}, filepath.Join(t.TempDir(), "restriction.json"))
	}()
	select {
	case ack := <-acks:
		if ack.Locked || ack.Error == "" {
			t.Fatalf("screen failure reported success: %+v", ack)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no failure acknowledgement")
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
