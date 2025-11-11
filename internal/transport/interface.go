package transport

import (
	"context"

	"github.com/mohammadhprp/throttle/internal/handler"
	"github.com/mohammadhprp/throttle/internal/storage"
	"go.uber.org/zap"
)

// Server defines the interface for different transport implementations (HTTP, gRPC, etc.)
type Server interface {
	// Start starts the transport server
	Start(ctx context.Context) error

	// Stop gracefully stops the transport server
	Stop(ctx context.Context) error

	// Addr returns the address the server is listening on
	Addr() string
}

// ServerConfig contains common configuration for all transport servers
type ServerConfig struct {
	Address      string        // Address to listen on (e.g., "localhost:8080" or ":50051")
	Store        storage.Store // Shared storage backend
	Logger       *zap.Logger   // Shared logger
	ReadTimeout  int           // Read timeout in seconds
	WriteTimeout int           // Write timeout in seconds
	IdleTimeout  int           // Idle timeout in seconds
}

// ServiceHandlers contains all service handlers
type ServiceHandlers struct {
	HealthCheck *handler.HealthCheckHandler
	RateLimit   *handler.RateLimitHandler
}
