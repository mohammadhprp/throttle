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

	logger.Info("Starting distributed rate limiter servers",
		zap.String("version", "1.0.0"),
		zap.String("http_address", cfg.HTTPServerAddr()),
		zap.String("grpc_address", cfg.GRPCServerAddr()),
	)

	// Initialize Redis store
	store, err := storage.NewRedisStore(cfg.RedisAddr(), cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		logger.Fatal("Failed to initialize Redis store", zap.Error(err))
	}
	defer store.Close()

	logger.Info("Connected to Redis", zap.String("address", cfg.RedisAddr()))

	// Create HTTP server using factory function
	httpSrv := transport.NewHTTPServer(transport.ServerConfig{
		Address:      cfg.HTTPServerAddr(),
		Store:        store,
		Logger:       logger,
		ReadTimeout:  int(cfg.HTTPServer.ReadTimeout.Seconds()),
		WriteTimeout: int(cfg.HTTPServer.WriteTimeout.Seconds()),
		IdleTimeout:  int(cfg.HTTPServer.IdleTimeout.Seconds()),
	})

	// Create gRPC server using factory function
	grpcSrv := transport.NewGRPCServer(transport.ServerConfig{
		Address: cfg.GRPCServerAddr(),
		Store:   store,
		Logger:  logger,
	})

	// Start HTTP server
	if err := httpSrv.Start(context.Background()); err != nil {
		logger.Fatal("Failed to start HTTP server", zap.Error(err))
	}

	// Start gRPC server
	if err := grpcSrv.Start(context.Background()); err != nil {
		logger.Fatal("Failed to start gRPC server", zap.Error(err))
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down servers...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop both servers
	if err := httpSrv.Stop(ctx); err != nil {
		logger.Error("HTTP server forced to shutdown", zap.Error(err))
	}

	if err := grpcSrv.Stop(ctx); err != nil {
		logger.Error("gRPC server forced to shutdown", zap.Error(err))
	}

	logger.Info("Servers stopped")
}
