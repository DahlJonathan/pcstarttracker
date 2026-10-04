package models

import "time"

// User is a mobile app account.
type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// Status is the computed power state of a device.
type Status string

const (
	StatusOnline  Status = "ONLINE"
	StatusOffline Status = "OFFLINE"
)

// Device is a paired PC together with its computed status for API responses.
type Device struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Status         Status     `json:"status"`
	LastEvent      string     `json:"last_event,omitempty"`
	LastBootAt     *time.Time `json:"last_boot_at,omitempty"`
	LastShutdownAt *time.Time `json:"last_shutdown_at,omitempty"`
	LastHeartbeat  *time.Time `json:"last_heartbeat_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// PairInitResponse is returned to the PC agent when it starts a pairing session.
type PairInitResponse struct {
	PairingToken string    `json:"pairing_token"` // encoded into the QR code
	Code         string    `json:"code"`          // 6-digit fallback code
	ExpiresAt    time.Time `json:"expires_at"`
}

// PairClaimResponse is returned to the mobile app after a successful claim.
type PairClaimResponse struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
}
