package api

import (
	"database/sql"
	"log"
	"net/http"
	"strings"
	"time"

	"pc-tracker-server/internal/auth"
	"pc-tracker-server/internal/models"
)

// pairTTL is how long a pairing code/QR remains valid.
const pairTTL = 10 * time.Minute

type pairInitRequest struct {
	DeviceID   string `json:"device_id"`   // persistent machine UUID
	DeviceName string `json:"device_name"` // e.g. "Living Room PC"
}

// handlePairInit is called by the PC agent to start a pairing session. No auth:
// possession of the fresh code/token is the secret. Returns a QR token + code.
func (s *Server) handlePairInit(w http.ResponseWriter, r *http.Request) {
	var in pairInitRequest
	if err := readJSON(r, &in); err != nil || strings.TrimSpace(in.DeviceID) == "" {
		writeError(w, http.StatusBadRequest, "device_id required")
		return
	}
	name := strings.TrimSpace(in.DeviceName)
	if name == "" {
		name = "My PC"
	}

	token, err := auth.RandomToken(24)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token failed")
		return
	}
	code, err := auth.NumericCode(6)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "code failed")
		return
	}
	expires := time.Now().Add(pairTTL).UTC()

	_, err = s.DB.ExecContext(r.Context(),
		`INSERT INTO pairing_tokens (token, code, device_id, device_name, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		token, code, in.DeviceID, name, expires.Format(time.RFC3339Nano))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create pairing session")
		return
	}

	writeJSON(w, http.StatusCreated, models.PairInitResponse{
		PairingToken: token,
		Code:         code,
		ExpiresAt:    expires,
	})
}

type pairClaimRequest struct {
	PairingToken string `json:"pairing_token"` // from QR scan
	Code         string `json:"code"`          // or manual 6-digit entry
}

// handlePairClaim is called by the authenticated mobile user to bind the device
// to their account and provision a permanent device token (held for the agent).
func (s *Server) handlePairClaim(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(ctxUserID).(int64)

	var in pairClaimRequest
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	in.PairingToken = strings.TrimSpace(in.PairingToken)
	in.Code = strings.TrimSpace(in.Code)
	if in.PairingToken == "" && in.Code == "" {
		writeError(w, http.StatusBadRequest, "pairing_token or code required")
		return
	}

	// Locate a live, unclaimed pairing session by token or code.
	var (
		pairToken  string
		deviceID   string
		deviceName string
		expiresStr string
	)
	query := `SELECT token, device_id, device_name, expires_at
	          FROM pairing_tokens
	          WHERE claimed = 0 AND (token = ? OR code = ?)
	          ORDER BY created_at DESC LIMIT 1`
	err := s.DB.QueryRowContext(r.Context(), query, in.PairingToken, in.Code).
		Scan(&pairToken, &deviceID, &deviceName, &expiresStr)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "pairing code not found or already used")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if expires, perr := time.Parse(time.RFC3339Nano, expiresStr); perr == nil && time.Now().After(expires) {
		writeError(w, http.StatusGone, "pairing code expired")
		return
	}

	// Provision the permanent device token (returned to agent via poll).
	deviceToken, err := auth.RandomToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token failed")
		return
	}

	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tx failed")
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Upsert the device, binding it to this user with the hashed token.
	_, err = tx.ExecContext(r.Context(),
		`INSERT INTO devices (id, user_id, name, api_token_hash)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		     user_id = excluded.user_id,
		     name = excluded.name,
		     api_token_hash = excluded.api_token_hash`,
		deviceID, uid, deviceName, auth.HashToken(deviceToken))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not bind device")
		return
	}

	// Mark claimed and stash the plaintext token for the agent's one-time poll.
	_, err = tx.ExecContext(r.Context(),
		`UPDATE pairing_tokens SET claimed = 1, device_token = ? WHERE token = ?`,
		deviceToken, pairToken)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not finalize pairing")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	log.Printf("pairing claimed: device %q (%s) bound to user %d", deviceName, deviceID, uid)

	writeJSON(w, http.StatusOK, models.PairClaimResponse{
		DeviceID:   deviceID,
		DeviceName: deviceName,
	})
}

type pairStatusResponse struct {
	Claimed     bool   `json:"claimed"`
	DeviceID    string `json:"device_id,omitempty"`
	DeviceToken string `json:"device_token,omitempty"` // returned exactly once
}

// handlePairStatus is polled by the PC agent. Once the user has claimed the
// session, it returns the permanent device token a single time, then clears it.
func (s *Server) handlePairStatus(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		writeError(w, http.StatusBadRequest, "token query param required")
		return
	}

	var (
		claimed     int
		deviceID    string
		deviceToken sql.NullString
	)
	err := s.DB.QueryRowContext(r.Context(),
		`SELECT claimed, device_id, device_token FROM pairing_tokens WHERE token = ?`, token).
		Scan(&claimed, &deviceID, &deviceToken)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "unknown pairing token")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	if claimed == 0 {
		writeJSON(w, http.StatusOK, pairStatusResponse{Claimed: false})
		return
	}

	resp := pairStatusResponse{Claimed: true, DeviceID: deviceID}
	if deviceToken.Valid && deviceToken.String != "" {
		resp.DeviceToken = deviceToken.String
		// Clear the token so it is delivered only once.
		_, _ = s.DB.ExecContext(r.Context(),
			`UPDATE pairing_tokens SET device_token = NULL WHERE token = ?`, token)
		log.Printf("pairing complete: agent for device %s retrieved its token", deviceID)
	}
	writeJSON(w, http.StatusOK, resp)
}
