package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"

	"github.com/mohammadhprp/throttle/internal/config"
	"github.com/mohammadhprp/throttle/internal/handler"
	"github.com/mohammadhprp/throttle/internal/storage"
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

	logger.Info("Starting distributed rate limiter server",
		zap.String("version", "1.0.0"),
		zap.String("address", cfg.ServerAddr()),
	)

	// Initialize Redis store
	store, err := storage.NewRedisStore(cfg.RedisAddr(), cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		logger.Fatal("Failed to initialize Redis store", zap.Error(err))
	}
	defer store.Close()

	logger.Info("Connected to Redis", zap.String("address", cfg.RedisAddr()))

	// Initialize router
	router := mux.NewRouter()

	healthCheckHanlder := handler.NewHealthCheckHanlder(store, logger)

	// Health check endpoint
	router.HandleFunc("/health", healthCheckHanlder.HealthCheck()).Methods("GET")

	// Configure server
	srv := &http.Server{
		Addr:         cfg.ServerAddr(),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		logger.Info("Server listening", zap.String("address", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server stopped")
}
