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

func TestSuspendResumeRecordsHistoryWithoutRestartingService(t *testing.T) {
	t.Setenv("ProgramData", t.TempDir())
	events := make(chan string, 10)
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
	requests := make(chan svc.ChangeRequest, 10)
	statuses := make(chan svc.Status, 10)
	done := make(chan struct{})
	go func() {
		h.Execute(nil, requests, statuses)
		close(done)
	}()
	expectEvent := func(want string) {
		t.Helper()
		select {
		case event := <-events:
			if event != want {
				t.Fatalf("event=%s, want=%s", event, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("missing %s event", want)
		}
	}
	expectEvent("boot")
	<-statuses
	if running := <-statuses; running.Accepts&svc.AcceptPowerEvent == 0 {
		t.Fatal("service does not accept Windows power events")
	}
	requests <- svc.ChangeRequest{Cmd: svc.PowerEvent, EventType: powerSuspend}
	expectEvent("shutdown")
	requests <- svc.ChangeRequest{Cmd: svc.PowerEvent, EventType: powerSuspend}
	requests <- svc.ChangeRequest{Cmd: svc.PowerEvent, EventType: powerResumeAutomatic}
	expectEvent("boot")
	requests <- svc.ChangeRequest{Cmd: svc.PowerEvent, EventType: powerResumeSuspend}
	requests <- svc.ChangeRequest{Cmd: svc.PowerEvent, EventType: powerSuspend}
	expectEvent("shutdown")
	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("suspended service did not stop")
	}
	select {
	case extra := <-events:
		t.Fatalf("duplicate lifecycle event=%s", extra)
	default:
	}
}
