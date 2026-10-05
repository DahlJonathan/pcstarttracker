package agent

import (
	"context"
	"log"
	"time"

	"pc-tracker-agent/internal/api"
	"pc-tracker-agent/internal/config"
)

const (
	heartbeatInterval = 15 * time.Second
	maxBackoff        = 60 * time.Second
	initialBackoff    = 2 * time.Second
)

// Runner owns the lifecycle telemetry for a paired device.
type Runner struct {
	cfg            *config.Config
	client         *api.Client
	heartbeatEvery time.Duration
}

func NewRunner(cfg *config.Config, client *api.Client) *Runner {
	return &Runner{cfg: cfg, client: client, heartbeatEvery: heartbeatInterval}
}

// Run sends a boot event (retrying until the network is ready) and then emits a
// heartbeat every 15s. It blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	bootAt := time.Now()
	if r.cfg.PendingShutdown != nil {
		if !r.sendEventWithBackoff(ctx, "shutdown", *r.cfg.PendingShutdown) {
			return
		}
		r.cfg.PendingShutdown = nil
		if err := r.cfg.Save(); err != nil {
			log.Printf("clear pending shutdown: %v", err)
		}
	}
	if !r.sendEventWithBackoff(ctx, "boot", bootAt) {
		return
	}

	ticker := time.NewTicker(r.heartbeatEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ctxHB, cancel := context.WithTimeout(ctx, 15*time.Second)
			if err := r.client.SendHeartbeat(ctxHB, r.cfg.DeviceID, r.cfg.DeviceToken); err != nil {
				log.Printf("heartbeat failed: %v", err)
			} else {
				log.Printf("heartbeat delivered")
			}
			cancel()
		}
	}
}

// Keep the original event time across retries, including shutdown replay.
func (r *Runner) sendEventWithBackoff(ctx context.Context, event string, at time.Time) bool {
	backoff := initialBackoff
	for {
		ctxEv, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := r.client.SendEvent(ctxEv, r.cfg.DeviceID, r.cfg.DeviceToken, event, at)
		cancel()
		if err == nil {
			log.Printf("%s event delivered", event)
			return true
		}
		log.Printf("%s event failed (%v); retrying in %s", event, err, backoff)

		select {
		case <-ctx.Done():
			return false
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
	at := time.Now()
	r.cfg.PendingShutdown = &at
	if err := r.cfg.Save(); err != nil {
		log.Printf("persist pending shutdown: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := r.client.SendEvent(ctx, r.cfg.DeviceID, r.cfg.DeviceToken, "shutdown", at); err != nil {
		log.Printf("shutdown event failed: %v", err)
		return
	}
	log.Printf("shutdown event delivered")
	r.cfg.PendingShutdown = nil
	if err := r.cfg.Save(); err != nil {
		log.Printf("clear pending shutdown: %v", err)
	}
}
