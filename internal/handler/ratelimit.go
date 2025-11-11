package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mohammadhprp/throttle/internal/service"
	"go.uber.org/zap"
)

// CheckRequest represents a rate limit check request
type CheckRequest struct {
	Key       string `json:"key"`
	Algorithm string `json:"algorithm"`
}

// CheckResponse represents a rate limit check response
type CheckResponse struct {
	Allowed    bool  `json:"allowed"`
	Remaining  int64 `json:"remaining"`
	ResetAt    int64 `json:"reset_at"`    // Unix timestamp
	RetryAfter int   `json:"retry_after"` // seconds
}

// StatusResponse represents the status of a rate limit
type StatusResponse struct {
	Key       string      `json:"key"`
	Algorithm string      `json:"algorithm"`
	Config    interface{} `json:"config"`
	CreatedAt int64       `json:"created_at"` // Unix timestamp
	UpdatedAt int64       `json:"updated_at"` // Unix timestamp
	NextReset int64       `json:"next_reset"` // Unix timestamp
}

// RateLimitHandler handles HTTP rate limit operations
type RateLimitHandler struct {
	service *service.RateLimitService
}

// NewRateLimitHandler creates a new rate limit handler
func NewRateLimitHandler(svc *service.RateLimitService) *RateLimitHandler {
	return &RateLimitHandler{
		service: svc,
	}
}

// Set handles POST /ratelimit/set - configure rate limits
func (h *RateLimitHandler) Set() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Key    string                  `json:"key"`
			Config service.RateLimitConfig `json:"config"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if err := h.service.SetConfig(r.Context(), req.Key, &req.Config); err != nil {
			h.service.Logger.Error("failed to set rate limit", zap.String("key", req.Key), zap.Error(err))
			h.writeError(w, http.StatusInternalServerError, "failed to save configuration")
			return
		}

		h.service.Logger.Info("rate limit configured", zap.String("key", req.Key), zap.String("algorithm", req.Config.Algorithm))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "rate limit configured",
			"key":     req.Key,
		})
	}
}

// Check handles POST /ratelimit/check - verify if request is allowed
func (h *RateLimitHandler) Check() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CheckRequest

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Key == "" {
			h.writeError(w, http.StatusBadRequest, "key is required")
			return
		}

		allowed, remaining, resetAt, retryAfter, err := h.service.CheckLimit(r.Context(), req.Key)
		if err != nil {
			h.service.Logger.Error("failed to check rate limit", zap.String("key", req.Key), zap.Error(err))
			h.writeError(w, http.StatusNotFound, "rate limit not found")
			return
		}

		resp := CheckResponse{
			Allowed:    allowed,
			Remaining:  remaining,
			ResetAt:    resetAt,
			RetryAfter: retryAfter,
		}

		// Set rate limit headers
		w.Header().Set("X-RateLimit-Limit", "TBD")
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetAt))
		if !allowed {
			w.Header().Set("X-RateLimit-Retry-After", fmt.Sprintf("%d", retryAfter))
		}
		w.Header().Set("Content-Type", "application/json")

		if !allowed {
			w.WriteHeader(http.StatusTooManyRequests)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		json.NewEncoder(w).Encode(resp)
	}
}

// Status handles GET /ratelimit/status/:key - get status of a rate limit
func (h *RateLimitHandler) Status() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		key := vars["key"]

		if key == "" {
			h.writeError(w, http.StatusBadRequest, "key is required")
			return
		}

		config, createdAt, updatedAt, nextReset, err := h.service.GetStatus(r.Context(), key)
		if err != nil {
			h.service.Logger.Error("failed to get status", zap.String("key", key), zap.Error(err))
			h.writeError(w, http.StatusNotFound, "rate limit not found")
			return
		}

		resp := StatusResponse{
			Key:       key,
			Algorithm: config.Algorithm,
			Config:    config,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			NextReset: nextReset,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}
}

// Reset handles DELETE /ratelimit/reset/:key - reset a rate limit
func (h *RateLimitHandler) Reset() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		key := vars["key"]

		if key == "" {
			h.writeError(w, http.StatusBadRequest, "key is required")
			return
		}

		if err := h.service.ResetLimit(r.Context(), key); err != nil {
			h.service.Logger.Error("failed to reset rate limit", zap.String("key", key), zap.Error(err))
			h.writeError(w, http.StatusNotFound, "rate limit not found")
			return
		}

		h.service.Logger.Info("rate limit reset", zap.String("key", key))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "rate limit reset",
			"key":     key,
		})
	}
}

// writeError writes an error response
func (h *RateLimitHandler) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
