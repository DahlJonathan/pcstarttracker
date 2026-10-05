package restriction

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"pc-tracker-agent/internal/api"
)

func passwordHash(t *testing.T) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("parent-only-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}

func TestOfflineUnlockPersistsAndIsNotReplacedBySameCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restriction.json")
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	command := api.Control{Revision: 1, Locked: true, PasswordHash: passwordHash(t)}
	if err := s.apply(command); err != nil {
		t.Fatal(err)
	}
	s, err = openStore(path)
	if err != nil || !s.snapshot().Locked {
		t.Fatalf("restart lost lock: %v", err)
	}
	if err := s.unlock("parent-only-password", time.Now()); err != nil {
		t.Fatal(err)
	}
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.apply(command); err != nil {
		t.Fatal(err)
	}
	if got := s.snapshot(); got.Locked || !got.OfflineUnlock {
		t.Fatalf("same remote revision undid offline recovery: locked=%v pending=%v", got.Locked, got.OfflineUnlock)
	}
	command.Revision++
	if err := s.apply(command); err != nil {
		t.Fatal(err)
	}
	if got := s.snapshot(); !got.Locked || got.OfflineUnlock {
		t.Fatal("new parent command did not replace offline override")
	}
}

func TestRecoveryRateLimitSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restriction.json")
	s, err := openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.apply(api.Control{Revision: 1, Locked: true, PasswordHash: passwordHash(t)}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 0; i < 5; i++ {
		if err := s.unlock("wrong", now); err == nil {
			t.Fatal("wrong password accepted")
		}
	}
	s, err = openStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.unlock("parent-only-password", now); err == nil {
		t.Fatal("restart bypassed rate limit")
	}
	if err := s.unlock("parent-only-password", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
}

func TestCannotLockWithoutRecoveryOrOverwriteValidState(t *testing.T) {
	s, err := openStore(filepath.Join(t.TempDir(), "restriction.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.apply(api.Control{Revision: 1, Locked: true}); err == nil {
		t.Fatal("locked without recovery")
	}
	if s.snapshot().Locked {
		t.Fatal("invalid state persisted")
	}
	if err := s.apply(api.Control{Revision: 1, PasswordHash: "not-a-hash"}); err == nil {
		t.Fatal("invalid hash accepted")
	}
}

func TestLocalUnlockRequiresUISecretAndRejectsBrowserOrigins(t *testing.T) {
	s, err := openStore(filepath.Join(t.TempDir(), "restriction.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.apply(api.Control{Revision: 1, Locked: true, PasswordHash: passwordHash(t)}); err != nil {
		t.Fatal(err)
	}
	ui := &localUI{store: s, secret: "test-ui-secret"}
	for _, test := range []struct {
		secret, origin string
		want           int
	}{
		{"", "", http.StatusUnauthorized},
		{"test-ui-secret", "https://example.invalid", http.StatusUnauthorized},
		{"test-ui-secret", "", http.StatusNoContent},
	} {
		r := httptest.NewRequest(http.MethodPost, "/unlock", strings.NewReader(`{"password":"parent-only-password"}`))
		r.Header.Set("Authorization", "Bearer "+test.secret)
		r.Header.Set("Origin", test.origin)
		w := httptest.NewRecorder()
		ui.ServeHTTP(w, r)
		if w.Code != test.want {
			t.Fatalf("unlock status=%d want=%d", w.Code, test.want)
		}
	}
}
