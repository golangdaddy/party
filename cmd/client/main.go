package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"minecraft-server-manager/internal/config"
	"minecraft-server-manager/internal/github"
	"minecraft-server-manager/internal/server"

	"github.com/sirupsen/logrus"
)

func main() {
	// Initialize logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Log which branch is being used
	logger.Infof("Using branch '%s' for configuration", cfg.GitHub.Branch)

	// Create GitHub client for public repository
	githubClient := github.NewClient(cfg.GitHub.RepoOwner, cfg.GitHub.RepoName)

	// Create server manager
	serverManager := server.NewManager(cfg, logger)

	// Create HTTP server for health checks and status
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		status := serverManager.GetStatus()
		json.NewEncoder(w).Encode(status)
	})

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler: mux,
	}

	// Start HTTP server
	go func() {
		logger.Infof("Starting HTTP server on port %d", cfg.HTTP.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("HTTP server error: %v", err)
		}
	}()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Received shutdown signal, stopping servers...")
		cancel()

		// Shutdown HTTP server
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		httpServer.Shutdown(shutdownCtx)
	}()

	// Start the main polling loop
	serverManager.Start(ctx, githubClient)
}
