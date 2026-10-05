package status

import (
	"time"

	"pc-tracker-server/internal/models"
)

// HeartbeatTimeout is the grace window after the last heartbeat before a device
// is considered offline. Agents beat every 15s, so 45s tolerates two misses.
const HeartbeatTimeout = 45 * time.Second

// Evaluate computes whether a device is ONLINE or OFFLINE.
//
// Rules:
//   - OFFLINE if the last recorded event was a graceful "shutdown".
//   - OFFLINE if no heartbeat was ever received, or the last heartbeat is older
//     than HeartbeatTimeout (covers sudden power loss, crashes, sleep).
//   - ONLINE otherwise.
func Evaluate(lastEvent string, lastHeartbeat *time.Time, now time.Time) models.Status {
	if lastEvent == "shutdown" {
		return models.StatusOffline
	}
	if lastHeartbeat == nil {
		return models.StatusOffline
	}
	if now.Sub(*lastHeartbeat) > HeartbeatTimeout {
		return models.StatusOffline
	}
	return models.StatusOnline
}
