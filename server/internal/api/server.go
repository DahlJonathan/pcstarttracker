package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"pc-tracker-server/internal/auth"
)

// Server holds shared dependencies for all HTTP handlers.
type Server struct {
	DB   *sql.DB
	Auth *auth.Manager
}

type ctxKey string

const (
	ctxUserID   ctxKey = "user_id"
	ctxDeviceID ctxKey = "device_id"
)

// Router builds the application's HTTP handler with all routes registered.
func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	// Public auth endpoints.
	mux.HandleFunc("POST /api/v1/auth/signup", s.handleSignup)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)

	// Pairing: init is called by the PC agent (no auth yet), claim by the user.
	mux.HandleFunc("POST /api/v1/pair/init", s.handlePairInit)
	mux.Handle("POST /api/v1/pair/claim", s.requireUser(http.HandlerFunc(s.handlePairClaim)))
	mux.HandleFunc("GET /api/v1/pair/status", s.handlePairStatus)

	// Device-authenticated telemetry (bearer = permanent device token).
	mux.Handle("POST /api/v1/devices/{id}/events", s.requireDevice(http.HandlerFunc(s.handleEvent)))
	mux.Handle("POST /api/v1/devices/{id}/heartbeat", s.requireDevice(http.HandlerFunc(s.handleHeartbeat)))
	mux.Handle("GET /api/v1/devices/{id}/control", s.requireDevice(http.HandlerFunc(s.handleGetControl)))
	mux.Handle("POST /api/v1/devices/{id}/control/ack", s.requireDevice(http.HandlerFunc(s.handleAckControl)))

	// User-authenticated dashboard.
	mux.Handle("GET /api/v1/devices", s.requireUser(http.HandlerFunc(s.handleListDevices)))
	mux.Handle("GET /api/v1/devices/{id}/history", s.requireUser(http.HandlerFunc(s.handleDeviceHistory)))
	mux.Handle("DELETE /api/v1/devices/{id}", s.requireUser(http.HandlerFunc(s.handleDeleteDevice)))
	mux.Handle("POST /api/v1/devices/{id}/lock", s.requireUser(http.HandlerFunc(s.handleSetLock)))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return logging(mux)
}

// --- Middleware -------------------------------------------------------------

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// requireUser validates the Authorization bearer JWT and injects the user id.
func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		uid, err := s.Auth.ParseUserToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		// The token signature can be valid while the account no longer exists
		// (e.g. the database was reset). Reject such tokens so the client is
		// forced to sign in again instead of hitting downstream FK errors.
		var exists int
		err = s.DB.QueryRowContext(r.Context(),
			`SELECT 1 FROM users WHERE id = ?`, uid).Scan(&exists)
		if err == sql.ErrNoRows {
			writeError(w, http.StatusUnauthorized, "account not found; please sign in again")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "auth lookup failed")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireDevice validates the permanent device token against the hashed value
// stored for the device id in the path.
func (s *Server) requireDevice(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deviceID := r.PathValue("id")
		token := bearer(r)
		if deviceID == "" || token == "" {
			writeError(w, http.StatusUnauthorized, "missing device credentials")
			return
		}
		var storedHash sql.NullString
		err := s.DB.QueryRowContext(r.Context(),
			`SELECT api_token_hash FROM devices WHERE id = ?`, deviceID).Scan(&storedHash)
		if err == sql.ErrNoRows || !storedHash.Valid {
			writeError(w, http.StatusUnauthorized, "unknown device")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		if auth.HashToken(token) != storedHash.String {
			writeError(w, http.StatusUnauthorized, "invalid device token")
			return
		}
		ctx := context.WithValue(r.Context(), ctxDeviceID, deviceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// --- JSON helpers -----------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
