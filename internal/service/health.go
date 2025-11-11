package service

import (
	"context"
	"time"

	"github.com/mohammadhprp/throttle/internal/storage"
	"go.uber.org/zap"
)

// HealthService provides health check functionality
type HealthService struct {
	store  storage.Store
	logger *zap.Logger
}

// NewHealthService creates a new health service
func NewHealthService(store storage.Store, logger *zap.Logger) *HealthService {
	return &HealthService{
		store:  store,
		logger: logger,
	}
}

// GetHealthStatus returns the current health status
func (s *HealthService) GetHealthStatus(ctx context.Context) (status string, timestamp string, err error) {
	timestamp = time.Now().Format(time.RFC3339)

	if err := s.store.Ping(ctx); err != nil {
		return "unhealthy", timestamp, err
	}

	return "healthy", timestamp, nil
}

// Ping verifies connectivity with the underlying store
func (s *HealthService) Ping(ctx context.Context) error {
	return s.store.Ping(ctx)
}
