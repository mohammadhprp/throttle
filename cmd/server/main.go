package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mohammadhprp/throttle/internal/config"
	"github.com/mohammadhprp/throttle/internal/storage"
	"github.com/mohammadhprp/throttle/internal/transport"
	"go.uber.org/zap"
)

func main() {
	config.LoadDotEnv()

	cfg := config.Load()

	// Initialize logger
	logger, err := config.InitLogger(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting distributed rate limiter HTTP server",
		zap.String("version", "1.0.0"),
		zap.String("address", cfg.HTTPServerAddr()),
	)

	// Initialize Redis store
	store, err := storage.NewRedisStore(cfg.RedisAddr(), cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		logger.Fatal("Failed to initialize Redis store", zap.Error(err))
	}
	defer store.Close()

	logger.Info("Connected to Redis", zap.String("address", cfg.RedisAddr()))

	// Create HTTP server using factory function
	srv := transport.NewHTTPServer(transport.ServerConfig{
		Address:      cfg.HTTPServerAddr(),
		Store:        store,
		Logger:       logger,
		ReadTimeout:  int(cfg.HTTPServer.ReadTimeout.Seconds()),
		WriteTimeout: int(cfg.HTTPServer.WriteTimeout.Seconds()),
		IdleTimeout:  int(cfg.HTTPServer.IdleTimeout.Seconds()),
	})

	// Start server
	if err := srv.Start(context.Background()); err != nil {
		logger.Fatal("Failed to start HTTP server", zap.Error(err))
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server stopped")
}
