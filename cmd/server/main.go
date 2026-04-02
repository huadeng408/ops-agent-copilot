package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"ops-agent-copilot/internal/app"
)

func main() {
	cfg, err := app.LoadConfig()
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	telemetry, err := app.NewTelemetry(cfg)
	if err != nil {
		log.Fatalf("init telemetry failed: %v", err)
	}
	db, dialect, err := app.OpenDatabase(cfg)
	if err != nil {
		log.Fatalf("open database failed: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if err := app.EnsureSchema(context.Background(), db, dialect); err != nil {
		log.Fatalf("ensure schema failed: %v", err)
	}

	application := app.NewApplication(cfg, db)
	server := &http.Server{
		Addr:              cfg.Host + ":" + strconv.Itoa(cfg.Port),
		Handler:           application.Router,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("Go API listening on http://%s:%d", cfg.Host, cfg.Port)
		errCh <- server.ListenAndServe()
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stopCh:
		log.Printf("shutdown signal received: %s", sig)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("server stopped unexpectedly: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && err != http.ErrServerClosed {
		log.Printf("server shutdown error: %v", err)
	}
	if err := telemetry.Shutdown(shutdownCtx); err != nil {
		log.Printf("telemetry shutdown error: %v", err)
	}
}
