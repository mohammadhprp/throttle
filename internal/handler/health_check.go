package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/mohammadhprp/throttle/internal/storage"
)

// HealthCheck returns a health check handler
func HealthCheck(store storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := map[string]string{
			"status": "healthy",
			"time":   time.Now().Format(time.RFC3339),
		}

		// Check Redis connection
		if err := store.Ping(r.Context()); err != nil {
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
