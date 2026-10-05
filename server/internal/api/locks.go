package api

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"pc-tracker-server/internal/auth"
	"pc-tracker-server/internal/models"
)

type lockStatus = models.LockStatus

func (s *Server) handleSetLock(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	var in struct {
		Locked   *bool  `json:"locked"`
		Password string `json:"password,omitempty"`
	}
	if err := readJSON(r, &in); err != nil || in.Locked == nil {
		writeError(w, http.StatusBadRequest, "locked is required")
		return
	}
	// bcrypt accepts at most 72 bytes. Never trim a recovery password.
	if in.Password != "" && (len(in.Password) < 8 || len(in.Password) > 72) {
		writeError(w, http.StatusBadRequest, "recovery password must be 8 to 72 bytes")
		return
	}
	uid := r.Context().Value(ctxUserID).(int64)
	id := r.PathValue("id")
	var owned int
	err := s.DB.QueryRowContext(r.Context(), `SELECT 1 FROM devices WHERE id = ? AND user_id = ?`, id, uid).Scan(&owned)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		log.Printf("lock ownership lookup: %v", err)
		writeError(w, http.StatusInternalServerError, "lock lookup failed")
		return
	}
	hash := ""
	if in.Password != "" {
		hash, err = auth.HashPassword(in.Password)
		if err != nil {
			log.Printf("recovery password hash: %v", err)
			writeError(w, http.StatusInternalServerError, "password setup failed")
			return
		}
	}
	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lock transaction failed")
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO device_locks (device_id) VALUES (?) ON CONFLICT(device_id) DO NOTHING`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "lock setup failed")
		return
	}
	var previous string
	if err := tx.QueryRowContext(r.Context(), `SELECT password_hash FROM device_locks WHERE device_id = ?`, id).Scan(&previous); err != nil {
		writeError(w, http.StatusInternalServerError, "lock lookup failed")
		return
	}
	if hash == "" {
		hash = previous
	}
	if *in.Locked && hash == "" {
		writeError(w, http.StatusConflict, "set a recovery password before locking")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE device_locks SET revision = revision + 1, desired_locked = ?, password_hash = ?, last_error = '' WHERE device_id = ?`, *in.Locked, hash, id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not queue lock command")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save lock command")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "pending"})
}

func (s *Server) handleGetControl(w http.ResponseWriter, r *http.Request) {
	id := r.Context().Value(ctxDeviceID).(string)
	var out struct {
		Revision     int64  `json:"revision"`
		Locked       bool   `json:"locked"`
		PasswordHash string `json:"password_hash"`
	}
	err := s.DB.QueryRowContext(r.Context(), `SELECT revision, desired_locked, password_hash FROM device_locks WHERE device_id = ?`, id).
		Scan(&out.Revision, &out.Locked, &out.PasswordHash)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusOK, out)
		return
	}
	if err != nil {
		log.Printf("device control lookup: %v", err)
		writeError(w, http.StatusInternalServerError, "control lookup failed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAckControl(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision      int64  `json:"revision"`
		Locked        bool   `json:"locked"`
		OfflineUnlock bool   `json:"offline_unlock"`
		Error         string `json:"error,omitempty"`
	}
	if err := readJSON(r, &in); err != nil || in.Revision < 0 || len(in.Error) > 500 || (in.OfflineUnlock && in.Locked) {
		writeError(w, http.StatusBadRequest, "invalid control acknowledgement")
		return
	}
	id := r.Context().Value(ctxDeviceID).(string)
	res, err := s.DB.ExecContext(r.Context(), `UPDATE device_locks SET
		applied_revision = CASE WHEN ? = '' THEN ? ELSE applied_revision END,
		applied_locked = CASE WHEN ? = '' THEN ? ELSE applied_locked END,
		desired_locked = CASE WHEN ? AND ? = '' THEN 0 ELSE desired_locked END,
		last_error = ?, confirmed_at = ?
		WHERE device_id = ? AND revision = ?
		AND (? OR ? != '' OR desired_locked = ?)`,
		in.Error, in.Revision, in.Error, in.Locked, in.OfflineUnlock, in.Error,
		in.Error, time.Now().UTC().Format(time.RFC3339Nano), id, in.Revision,
		in.OfflineUnlock, in.Error, in.Locked)
	if err != nil {
		log.Printf("control acknowledgement: %v", err)
		writeError(w, http.StatusInternalServerError, "control acknowledgement failed")
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "control acknowledgement failed")
		return
	}
	if n == 0 {
		writeError(w, http.StatusConflict, "command was replaced; fetch current control")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
