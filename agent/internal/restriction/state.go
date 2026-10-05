package restriction

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"pc-tracker-agent/internal/api"
)

type state struct {
	Revision       int64     `json:"revision"`
	Locked         bool      `json:"locked"`
	PasswordHash   string    `json:"password_hash"`
	OfflineUnlock  bool      `json:"offline_unlock"`
	FailedAttempts int       `json:"failed_attempts"`
	RetryAfter     time.Time `json:"retry_after"`
}

type store struct {
	mu    sync.Mutex
	path  string
	state state
}

func openStore(path string) (*store, error) {
	s := &store{path: path, state: state{Revision: -1}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read restriction: %w", err)
	}
	if err := json.Unmarshal(data, &s.state); err != nil {
		return nil, fmt.Errorf("parse restriction: %w", err)
	}
	if s.state.Locked && s.state.PasswordHash == "" {
		return nil, fmt.Errorf("locked state has no recovery password")
	}
	return s, nil
}

func (s *store) snapshot() state {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *store) save(next state) error {
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.state = next
	return nil
}

func (s *store) apply(command api.Control) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if command.Revision <= s.state.Revision {
		return nil
	}
	if command.PasswordHash != "" {
		cost, err := bcrypt.Cost([]byte(command.PasswordHash))
		if err != nil {
			return fmt.Errorf("invalid recovery password hash: %w", err)
		}
		if cost > 14 {
			return fmt.Errorf("recovery hash cost exceeds supported limit")
		}
	}
	if command.Locked && command.PasswordHash == "" {
		return fmt.Errorf("recovery password is required")
	}
	next := s.state
	next.Revision = command.Revision
	next.Locked = command.Locked
	next.PasswordHash = command.PasswordHash
	next.OfflineUnlock = false
	next.FailedAttempts = 0
	next.RetryAfter = time.Time{}
	return s.save(next)
}

func (s *store) unlock(password string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.state.Locked {
		return nil
	}
	if now.Before(s.state.RetryAfter) {
		return fmt.Errorf("too many attempts; wait before trying again")
	}
	next := s.state
	if len(password) > 72 || bcrypt.CompareHashAndPassword([]byte(next.PasswordHash), []byte(password)) != nil {
		next.FailedAttempts++
		if next.FailedAttempts >= 5 {
			next.RetryAfter = now.Add(time.Minute)
			next.FailedAttempts = 0
		}
		if err := s.save(next); err != nil {
			return fmt.Errorf("save attempt limit: %w", err)
		}
		return fmt.Errorf("incorrect recovery password")
	}
	next.Locked = false
	next.OfflineUnlock = true
	next.FailedAttempts = 0
	next.RetryAfter = time.Time{}
	return s.save(next)
}

func (s *store) clearOffline(revision int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Revision != revision {
		return nil
	}
	next := s.state
	next.OfflineUnlock = false
	return s.save(next)
}
