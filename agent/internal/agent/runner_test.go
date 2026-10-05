package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"pc-tracker-agent/internal/api"
	"pc-tracker-agent/internal/config"
)

func TestShutdownSurvivesNetworkFailureAndReplaysBeforeBoot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ProgramData", dir)
	offline := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer offline.Close()
	cfg := &config.Config{DeviceID: "test-device", DeviceToken: "test-token", ServerURL: offline.URL}
	NewRunner(cfg, api.New(offline.URL)).SendShutdown()

	persisted, err := config.Load(offline.URL)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.PendingShutdown == nil {
		t.Fatal("failed shutdown was not persisted")
	}
	shutdownAt := *persisted.PendingShutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	var received []string
	online := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("missing device authorization")
		}
		event := "heartbeat"
		if filepath.Base(r.URL.Path) == "events" {
			var body struct {
				Event     string    `json:"event"`
				Timestamp time.Time `json:"timestamp"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			event = body.Event
			if event == "shutdown" && !body.Timestamp.Equal(shutdownAt) {
				t.Error("replay changed original shutdown time")
			}
		}
		mu.Lock()
		received = append(received, event)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
		if event == "heartbeat" {
			cancel()
		}
	}))
	defer online.Close()
	runner := NewRunner(persisted, api.New(online.URL))
	runner.heartbeatEvery = 10 * time.Millisecond
	done := make(chan struct{})
	go func() {
		runner.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runner did not deliver heartbeat and stop")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(received) < 3 || received[0] != "shutdown" || received[1] != "boot" || received[2] != "heartbeat" {
		t.Fatalf("event order: %v", received)
	}
	loaded, err := config.Load(online.URL)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PendingShutdown != nil {
		t.Fatal("delivered shutdown was not cleared")
	}
	if _, err := os.Stat(filepath.Join(dir, "PCStatusAgent", "config.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCancelledBootDoesNotStartHeartbeats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		cancel()
	}))
	defer server.Close()
	runner := NewRunner(&config.Config{DeviceID: "test"}, api.New(server.URL))
	done := make(chan struct{})
	go func() { runner.Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled runner did not stop")
	}
}
