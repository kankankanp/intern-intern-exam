package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"url-collector/internal/config"
	"url-collector/internal/observability"
	"url-collector/internal/repository"
	"url-collector/internal/service"
)

func main() {
	logger := observability.NewLogger("worker")

	cfg := config.Load()

	// Connect to database
	db, err := repository.NewDB(cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	logger.Info("connected to database")

	// Create worker
	worker := service.NewWorker(db, cfg, logger)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Handle shutdown signals
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
		<-quit
		logger.Info("received shutdown signal")
		cancel()
	}()

	// Run worker
	if err := worker.Run(ctx); err != nil {
		logger.Error("worker error", "error", err)
		os.Exit(1)
	}

	logger.Info("worker stopped")
}
