package api

import (
	"database/sql"
	"net/http"
	"strconv"
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

	var duplicate int
	if err := tx.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM device_events WHERE device_id = ? AND event = ? AND created_at = ?`,
		deviceID, in.Event, ts).Scan(&duplicate); err != nil {
		writeError(w, http.StatusInternalServerError, "could not check event")
		return
	}
	if duplicate != 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// When a boot arrives, the machine must have been off since we last heard
	// from it. The agent tries to send a graceful "shutdown" on power-off, but
	// the network is frequently torn down before it can, so that event is often
	// lost. Backfill a shutdown for the previous power cycle, approximating the
	// power-off time as the last moment the machine was known to be alive.
	if in.Event == "boot" {
		var prevEvent, prevBeat, prevBoot sql.NullString
		if err := tx.QueryRowContext(r.Context(),
			`SELECT last_event, last_heartbeat_at, last_boot_at FROM devices WHERE id = ?`,
			deviceID).Scan(&prevEvent, &prevBeat, &prevBoot); err != nil && err != sql.ErrNoRows {
			writeError(w, http.StatusInternalServerError, "could not read device")
			return
		}
		if prevEvent.String != "" && prevEvent.String != "shutdown" {
			offAt := prevBeat.String
			if offAt == "" {
				offAt = prevBoot.String
			}
			if offAt != "" && heartbeatGap(offAt, ts) {
				if _, err := tx.ExecContext(r.Context(),
					`INSERT INTO device_events (device_id, event, created_at) VALUES (?, 'shutdown', ?)`,
					deviceID, offAt); err != nil {
					writeError(w, http.StatusInternalServerError, "could not record event")
					return
				}
			}
		}
	}

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
			d                                                models.Device
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

// handleDeleteDevice unpairs a device owned by the authenticated user. The
// cascade on device_events removes the event history as well. The machine keeps
// its UUID, so the agent can pair again later to re-create the device.
func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(ctxUserID).(int64)
	deviceID := r.PathValue("id")
	if deviceID == "" {
		writeError(w, http.StatusBadRequest, "missing device id")
		return
	}

	res, err := s.DB.ExecContext(r.Context(),
		`DELETE FROM devices WHERE id = ? AND user_id = ?`, deviceID, uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete device")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeviceHistory returns a device's boot/shutdown log, newest first, for
// the owning user. The default limit keeps responses small; the app groups the
// events by day to render a per-day timeline.
func (s *Server) handleDeviceHistory(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value(ctxUserID).(int64)
	deviceID := r.PathValue("id")
	if deviceID == "" {
		writeError(w, http.StatusBadRequest, "missing device id")
		return
	}

	var owned int
	err := s.DB.QueryRowContext(r.Context(),
		`SELECT 1 FROM devices WHERE id = ? AND user_id = ?`, deviceID, uid).Scan(&owned)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	limit := 500
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}

	rows, err := s.DB.QueryContext(r.Context(),
		`SELECT event, created_at FROM device_events
		 WHERE device_id = ? ORDER BY created_at DESC LIMIT ?`, deviceID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	defer rows.Close()

	events := make([]models.DeviceEvent, 0)
	for rows.Next() {
		var (
			ev      string
			created sql.NullString
		)
		if err := rows.Scan(&ev, &created); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		e := models.DeviceEvent{Event: ev}
		if t := nullTime(created); t != nil {
			e.CreatedAt = *t
		}
		events = append(events, e)
	}

	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

// --- helpers ----------------------------------------------------------------

func parseTimestamp(s string) string {
	if t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s)); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// A service restart without a heartbeat gap is not evidence of a power-off.
func heartbeatGap(a, b string) bool {
	ta, erra := time.Parse(time.RFC3339Nano, a)
	tb, errb := time.Parse(time.RFC3339Nano, b)
	if erra != nil || errb != nil {
		return false
	}
	return tb.Sub(ta) > status.HeartbeatTimeout
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
