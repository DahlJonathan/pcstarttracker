package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pc-tracker-server/internal/api"
	"pc-tracker-server/internal/auth"
	"pc-tracker-server/internal/db"
)

func main() {
	log.SetOutput(os.Stdout)
	addr := envOr("PC_TRACKER_ADDR", "")
	if addr == "" {
		if p := os.Getenv("PORT"); p != "" {
			addr = ":" + p
		} else {
			addr = ":8080"
		}
	}
	dbPath := envOr("PC_TRACKER_DB", "pc_tracker.db")
	jwtSecret := envOr("PC_TRACKER_JWT_SECRET", "change-me-in-production")

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	srv := &api.Server{
		DB:   database,
		Auth: auth.New(jwtSecret, 30*24*time.Hour),
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("pc-tracker server listening on %s (db=%s)", addr, dbPath)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	log.Println("server stopped")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
