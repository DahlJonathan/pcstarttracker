package agent

import (
	"context"
	"log"
	"time"

	"pc-tracker-agent/internal/api"
	"pc-tracker-agent/internal/config"
)

const (
	heartbeatInterval = 60 * time.Second
	maxBackoff        = 60 * time.Second
	initialBackoff    = 2 * time.Second
)

// Runner owns the lifecycle telemetry for a paired device.
type Runner struct {
	cfg    *config.Config
	client *api.Client
}

func NewRunner(cfg *config.Config, client *api.Client) *Runner {
	return &Runner{cfg: cfg, client: client}
}

// Run sends a boot event (retrying until the network is ready) and then emits a
// heartbeat every 60s. It blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	r.sendBootWithBackoff(ctx)

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ctxHB, cancel := context.WithTimeout(ctx, 15*time.Second)
			if err := r.client.SendHeartbeat(ctxHB, r.cfg.DeviceID, r.cfg.DeviceToken); err != nil {
				log.Printf("heartbeat failed: %v", err)
			}
			cancel()
		}
	}
}

// sendBootWithBackoff retries the boot event with exponential backoff, tolerating
// Wi-Fi/Ethernet that is slow to come up right after the OS starts.
func (r *Runner) sendBootWithBackoff(ctx context.Context) {
	backoff := initialBackoff
	for {
		ctxEv, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := r.client.SendEvent(ctxEv, r.cfg.DeviceID, r.cfg.DeviceToken, "boot", time.Now())
		cancel()
		if err == nil {
			log.Printf("boot event delivered")
			return
		}
		log.Printf("boot event failed (%v); retrying in %s", err, backoff)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// SendShutdown delivers a final graceful shutdown event. It uses a fresh,
// short-lived context because the parent is typically already cancelled.
func (r *Runner) SendShutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := r.client.SendEvent(ctx, r.cfg.DeviceID, r.cfg.DeviceToken, "shutdown", time.Now()); err != nil {
		log.Printf("shutdown event failed: %v", err)
		return
	}
	log.Printf("shutdown event delivered")
}
