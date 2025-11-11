package handler

import (
	"encoding/json"
	"net/http"

	"github.com/mohammadhprp/throttle/internal/service"
)

type HealthCheckHandler struct {
	service *service.HealthService
}

func NewHealthCheckHandler(healthService *service.HealthService) *HealthCheckHandler {
	return &HealthCheckHandler{
		service: healthService,
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
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(response)
	}
}
