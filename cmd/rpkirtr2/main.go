// This app implements RPKI RTR Version 2
// It supports both version 1 and version 2 of the protocol.

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mellowdrifter/rpkirtr2/internal/config"
	"github.com/mellowdrifter/rpkirtr2/internal/logging"
	"github.com/mellowdrifter/rpkirtr2/internal/server"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Set up logger
	logger := logging.New(cfg.LogLevel)

	logger.Info("Starting daemon...")
	logger.Debug("This is a debug message")
	logger.Warn("Something might be wrong")
	logger.Error("Something definitely went wrong")

	// Create and start the server
	srv := server.New(cfg, logger)

	go func() {
		if err := srv.Start(); err != nil {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	// Graceful shutdown on interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	logger.Infof("Signal received: %s, shutting down gracefully...", sig)

	shutdownTimeout := 5 * time.Second
	if err := srv.Stop(shutdownTimeout); err != nil {
		logger.Errorf("Shutdown error: %v", err)
	} else {
		logger.Info("Daemon shut down cleanly")
	}
}
