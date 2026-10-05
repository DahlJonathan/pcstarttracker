//go:build windows

package winsvc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"

	"pc-tracker-agent/internal/agent"
	"pc-tracker-agent/internal/api"
	"pc-tracker-agent/internal/config"
)

func TestPreShutdownDeliversFinalEventAndStopsRunner(t *testing.T) {
	t.Setenv("ProgramData", t.TempDir())
	events := make(chan string, 5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Event string `json:"event"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusAccepted)
		events <- body.Event
	}))
	defer server.Close()
	cfg := &config.Config{DeviceID: "test-device", DeviceToken: "test-token", ServerURL: server.URL}
	h := &handler{cfg: cfg, runner: agent.NewRunner(cfg, api.New(server.URL))}
	requests := make(chan svc.ChangeRequest, 1)
	statuses := make(chan svc.Status, 5)
	done := make(chan struct{})
	go func() {
		h.Execute(nil, requests, statuses)
		close(done)
	}()
	select {
	case event := <-events:
		if event != "boot" {
			t.Fatalf("first event=%s", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("service did not send boot")
	}
	requests <- svc.ChangeRequest{Cmd: svc.PreShutdown}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("service did not stop")
	}
	select {
	case event := <-events:
		if event != "shutdown" {
			t.Fatalf("final event=%s", event)
		}
	default:
		t.Fatal("missing shutdown event")
	}
	if cfg.PendingShutdown != nil {
		t.Fatal("delivered shutdown remains queued")
	}
}
