package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pc-tracker-server/internal/auth"
	"pc-tracker-server/internal/db"
	"pc-tracker-server/internal/models"
)

// newTestServer spins up a Server backed by a throwaway on-disk SQLite DB and
// seeds a single user + device so event handlers can be exercised directly.
func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	path := t.TempDir() + "/test.db"
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if _, err := database.Exec(
		`INSERT INTO users (id, email, password_hash) VALUES (1, 'a@b.c', 'x')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	const deviceID = "dev-1"
	if _, err := database.Exec(
		`INSERT INTO devices (id, user_id, name) VALUES (?, 1, 'PC')`, deviceID); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	return &Server{DB: database}, deviceID
}

// postEvent invokes handleEvent as if the device had authenticated.
func postEvent(t *testing.T, s *Server, deviceID, event string, at time.Time) {
	t.Helper()
	body := `{"event":"` + event + `","timestamp":"` + at.UTC().Format(time.RFC3339Nano) + `"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/devices/"+deviceID+"/events",
		strings.NewReader(body))
	r = r.WithContext(context.WithValue(r.Context(), ctxDeviceID, deviceID))
	w := httptest.NewRecorder()
	s.handleEvent(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("event %q: got status %d, body %s", event, w.Code, w.Body.String())
	}
}

func beat(t *testing.T, s *Server, deviceID string, at time.Time) {
	t.Helper()
	if _, err := s.DB.Exec(
		`UPDATE devices SET last_heartbeat_at = ? WHERE id = ?`,
		at.UTC().Format(time.RFC3339Nano), deviceID); err != nil {
		t.Fatalf("beat: %v", err)
	}
}

func events(t *testing.T, s *Server, deviceID string) []string {
	t.Helper()
	rows, err := s.DB.Query(
		`SELECT event FROM device_events WHERE device_id = ? ORDER BY created_at ASC`, deviceID)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, e)
	}
	return out
}

// A boot following a previous boot (with no graceful shutdown) should backfill
// a synthetic shutdown at the last-known-alive time.
func TestBoot_BackfillsMissingShutdown(t *testing.T) {
	s, dev := newTestServer(t)
	t0 := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)

	postEvent(t, s, dev, "boot", t0)
	beat(t, s, dev, t0.Add(2*time.Minute)) // last time we heard from it
	postEvent(t, s, dev, "boot", t0.Add(1*time.Hour))

	got := events(t, s, dev)
	want := []string{"boot", "shutdown", "boot"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", got, want)
	}

	// The synthetic shutdown must sit at the last heartbeat time.
	var shutAt string
	if err := s.DB.QueryRow(
		`SELECT created_at FROM device_events WHERE event = 'shutdown'`).Scan(&shutAt); err != nil {
		t.Fatalf("read shutdown: %v", err)
	}
	ts, _ := time.Parse(time.RFC3339Nano, shutAt)
	if !ts.Equal(t0.Add(2 * time.Minute)) {
		t.Fatalf("shutdown at %v, want %v", ts, t0.Add(2*time.Minute))
	}
}

// If the agent already delivered a graceful shutdown, the next boot must NOT
// add a duplicate synthetic one.
func TestBoot_NoDuplicateAfterGracefulShutdown(t *testing.T) {
	s, dev := newTestServer(t)
	t0 := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)

	postEvent(t, s, dev, "boot", t0)
	postEvent(t, s, dev, "shutdown", t0.Add(30*time.Minute))
	postEvent(t, s, dev, "boot", t0.Add(1*time.Hour))

	got := events(t, s, dev)
	want := []string{"boot", "shutdown", "boot"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

// The very first boot after pairing has no prior state and must not backfill.
func TestBoot_FirstBootNoBackfill(t *testing.T) {
	s, dev := newTestServer(t)
	postEvent(t, s, dev, "boot", time.Now())

	got := events(t, s, dev)
	if strings.Join(got, ",") != "boot" {
		t.Fatalf("events = %v, want [boot]", got)
	}
}

func TestBoot_RetryDoesNotDuplicateHistory(t *testing.T) {
	s, dev := newTestServer(t)
	at := time.Now()
	postEvent(t, s, dev, "boot", at)
	postEvent(t, s, dev, "boot", at)
	if got := events(t, s, dev); strings.Join(got, ",") != "boot" {
		t.Fatalf("retry duplicated history: %v", got)
	}
}

func TestBoot_QuickServiceRestartDoesNotInventShutdown(t *testing.T) {
	s, dev := newTestServer(t)
	at := time.Now()
	postEvent(t, s, dev, "boot", at)
	postEvent(t, s, dev, "boot", at.Add(10*time.Second))
	if got := events(t, s, dev); strings.Join(got, ",") != "boot,boot" {
		t.Fatalf("restart invented shutdown: %v", got)
	}
}

func TestHistoryReturnsEveryLifecycleEventThroughAuthenticatedRoute(t *testing.T) {
	s, dev := newTestServer(t)
	s.Auth = auth.New("test-secret", time.Hour)
	// RFC3339Nano drops trailing fractional zeros. Sorting the timestamp text
	// would place .1Z before Z rather than after it.
	at := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	postEvent(t, s, dev, "boot", at)
	postEvent(t, s, dev, "shutdown", at.Add(100*time.Millisecond))
	postEvent(t, s, dev, "boot", at.Add(time.Minute))
	postEvent(t, s, dev, "shutdown", at.Add(2*time.Minute))
	token, err := s.Auth.IssueUserToken(1)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/devices/"+dev+"/history", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("history status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Events []models.DeviceEvent `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Events) != 4 {
		t.Fatalf("history length=%d want=4", len(body.Events))
	}
	want := []string{"shutdown", "boot", "shutdown", "boot"}
	for i, ev := range body.Events {
		if ev.Event != want[i] {
			t.Fatalf("event %d=%s want=%s", i, ev.Event, want[i])
		}
		if i > 0 && ev.CreatedAt.After(body.Events[i-1].CreatedAt) {
			t.Fatal("history is not newest-first")
		}
	}
}
