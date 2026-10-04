package api

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"pc-tracker-server/internal/models"
	"pc-tracker-server/internal/status"
)

type eventRequest struct {
	Event     string `json:"event"`     // "boot" | "shutdown"
	Timestamp string `json:"timestamp"` // RFC3339, optional (server clock used if empty)
}

// handleEvent records a boot or shutdown event for the authenticated device and
// updates its denormalized state columns.
func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	deviceID := r.Context().Value(ctxDeviceID).(string)

	var in eventRequest
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	in.Event = strings.ToLower(strings.TrimSpace(in.Event))
	if in.Event != "boot" && in.Event != "shutdown" {
		writeError(w, http.StatusBadRequest, "event must be 'boot' or 'shutdown'")
		return
	}
	ts := parseTimestamp(in.Timestamp)

	tx, err := s.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tx failed")
		return
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(r.Context(),
		`INSERT INTO device_events (device_id, event, created_at) VALUES (?, ?, ?)`,
		deviceID, in.Event, ts); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record event")
		return
	}

	// A boot both sets last_boot_at and serves as an implicit heartbeat.
	if in.Event == "boot" {
		_, err = tx.ExecContext(r.Context(),
			`UPDATE devices SET last_event = 'boot', last_boot_at = ?, last_heartbeat_at = ? WHERE id = ?`,
			ts, ts, deviceID)
	} else {
		_, err = tx.ExecContext(r.Context(),
			`UPDATE devices SET last_event = 'shutdown', last_shutdown_at = ? WHERE id = ?`,
			ts, deviceID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update device")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleHeartbeat refreshes the device's keep-alive timestamp.
func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	deviceID := r.Context().Value(ctxDeviceID).(string)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err := s.DB.ExecContext(r.Context(),
		`UPDATE devices SET last_heartbeat_at = ? WHERE id = ?`, now, deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record heartbeat")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListDevices returns the authenticated user's devices with computed status.
func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(ctxUserID).(int64)

	rows, err := s.DB.QueryContext(r.Context(),
		`SELECT id, name, last_event, last_boot_at, last_shutdown_at, last_heartbeat_at, created_at
		 FROM devices WHERE user_id = ? ORDER BY name`, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	defer rows.Close()

	now := time.Now().UTC()
	devices := make([]models.Device, 0)
	for rows.Next() {
		var (
			d                                              models.Device
			lastEvent, lastBoot, lastShut, lastBeat, created sql.NullString
		)
		if err := rows.Scan(&d.ID, &d.Name, &lastEvent, &lastBoot, &lastShut, &lastBeat, &created); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		d.LastEvent = lastEvent.String
		d.LastBootAt = nullTime(lastBoot)
		d.LastShutdownAt = nullTime(lastShut)
		d.LastHeartbeat = nullTime(lastBeat)
		if t := nullTime(created); t != nil {
			d.CreatedAt = *t
		}
		d.Status = status.Evaluate(lastEvent.String, d.LastHeartbeat, now)
		devices = append(devices, d)
	}

	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

// --- helpers ----------------------------------------------------------------

func parseTimestamp(s string) string {
	if t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s)); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func nullTime(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s.String); err == nil {
		return &t
	}
	if t, err := time.Parse(time.RFC3339, s.String); err == nil {
		return &t
	}
	return nil
}
