package handler

import (
	"encoding/json"
	"net/http"

	"github.com/mohammadhprp/throttle/internal/service"
	"go.uber.org/zap"
)

type HealthCheckHandler struct {
	service *service.HealthService
	logger  *zap.Logger
}

func NewHealthCheckHandler(healthService *service.HealthService) *HealthCheckHandler {
	return &HealthCheckHandler{
		service: healthService,
		logger:  zap.NewNop(),
	}
}

// NewHealthCheckHandlerWithLogger creates a new health check handler with a logger
func NewHealthCheckHandlerWithLogger(healthService *service.HealthService, logger *zap.Logger) *HealthCheckHandler {
	return &HealthCheckHandler{
		service: healthService,
		logger:  logger,
	}
}

// HealthCheck returns a health check handler
func (h *HealthCheckHandler) HealthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, timestamp, err := h.service.GetHealthStatus(r.Context())

		response := map[string]string{
			"status": status,
			"time":   timestamp,
		}

		statusCode := http.StatusOK
		if err != nil {
			response["error"] = err.Error()
			statusCode = http.StatusServiceUnavailable
			h.logger.Warn("health check failed", zap.Error(err))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(response)
	}
}
