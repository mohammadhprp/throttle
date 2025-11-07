package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/mohammadhprp/throttle/internal/storage"
	"go.uber.org/zap"
)

type HealthCheckHanlder struct {
	store  storage.Store
	logger *zap.Logger
}

func NewHealthCheckHanlder(store storage.Store, logger *zap.Logger) *HealthCheckHanlder {
	return &HealthCheckHanlder{
		store:  store,
		logger: logger,
	}
}

// HealthCheck returns a health check handler
func (h *HealthCheckHanlder) HealthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := map[string]string{
			"status": "healthy",
			"time":   time.Now().Format(time.RFC3339),
		}

		// Check Redis connection
		if err := h.store.Ping(r.Context()); err != nil {
			status["status"] = "unhealthy"
			status["error"] = err.Error()
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}
}

// Ping verifies connectivity with the underlying store for non-HTTP health checks.
func (h *HealthCheckHanlder) Ping(ctx context.Context) error {
	return h.store.Ping(ctx)
}
