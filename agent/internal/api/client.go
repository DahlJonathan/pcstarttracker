package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client talks to the pc-tracker backend.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a Client for the given backend base URL (e.g. http://host:8080).
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// PairInitResponse mirrors the backend's pairing init payload.
type PairInitResponse struct {
	PairingToken string    `json:"pairing_token"`
	Code         string    `json:"code"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// PairStatusResponse mirrors the backend's pairing poll payload.
type PairStatusResponse struct {
	Claimed     bool   `json:"claimed"`
	DeviceID    string `json:"device_id"`
	DeviceToken string `json:"device_token"`
}

// PairInit starts a pairing session for the given device identity.
func (c *Client) PairInit(ctx context.Context, deviceID, deviceName string) (*PairInitResponse, error) {
	body := map[string]string{"device_id": deviceID, "device_name": deviceName}
	var out PairInitResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/pair/init", "", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PairStatus polls whether the pairing session has been claimed.
func (c *Client) PairStatus(ctx context.Context, pairingToken string) (*PairStatusResponse, error) {
	var out PairStatusResponse
	url := "/api/v1/pair/status?token=" + pairingToken
	if err := c.do(ctx, http.MethodGet, url, "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SendEvent posts a boot or shutdown event for a paired device.
func (c *Client) SendEvent(ctx context.Context, deviceID, token, event string, ts time.Time) error {
	body := map[string]string{"event": event, "timestamp": ts.UTC().Format(time.RFC3339Nano)}
	path := fmt.Sprintf("/api/v1/devices/%s/events", deviceID)
	return c.do(ctx, http.MethodPost, path, token, body, nil)
}

// SendHeartbeat posts a keep-alive for a paired device.
func (c *Client) SendHeartbeat(ctx context.Context, deviceID, token string) error {
	path := fmt.Sprintf("/api/v1/devices/%s/heartbeat", deviceID)
	return c.do(ctx, http.MethodPost, path, token, nil, nil)
}

// do executes an HTTP request, optionally with a bearer token, JSON body, and
// JSON response decoding.
func (c *Client) do(ctx context.Context, method, path, bearer string, in, out any) error {
	var reader *bytes.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("server %d: %s", resp.StatusCode, e.Error)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
