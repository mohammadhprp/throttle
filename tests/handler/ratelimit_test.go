package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/mohammadhprp/throttle/internal/handler"
	"github.com/mohammadhprp/throttle/internal/service"
	"go.uber.org/zap"
)

// MockStore is a simple in-memory store for testing
type MockStore struct {
	data map[string]string
}

func NewMockStore() *MockStore {
	return &MockStore{
		data: make(map[string]string),
	}
}

func (m *MockStore) Increment(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	// Not implemented for this test
	return 0, nil
}

func (m *MockStore) IncrementBy(ctx context.Context, key string, value int64, expiration time.Duration) (int64, error) {
	// Not implemented for this test
	return 0, nil
}

func (m *MockStore) Get(ctx context.Context, key string) (string, error) {
	return m.data[key], nil
}

func (m *MockStore) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	m.data[key] = value
	return nil
}

func (m *MockStore) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *MockStore) ZAdd(ctx context.Context, key string, score float64, member string, expiration time.Duration) error {
	// Not implemented for this test
	return nil
}

func (m *MockStore) ZRemRangeByScore(ctx context.Context, key string, min, max float64) error {
	// Not implemented for this test
	return nil
}

func (m *MockStore) ZCount(ctx context.Context, key string, min, max float64) (int64, error) {
	// Not implemented for this test
	return 0, nil
}

func (m *MockStore) Expire(ctx context.Context, key string, expiration time.Duration) error {
	// Not implemented for this test
	return nil
}

func (m *MockStore) Ping(ctx context.Context) error {
	return nil
}

func (m *MockStore) Close() error {
	return nil
}

// Helper function to create logger
func createTestLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

// Test POST /ratelimit/set
func TestRateLimitSet(t *testing.T) {
	store := NewMockStore()
	logger := createTestLogger()
	defer logger.Sync()

	rateLimitService := service.NewRateLimitService(store, logger)
	h := handler.NewRateLimitHandler(rateLimitService)

	tests := []struct {
		name       string
		key        string
		config     service.RateLimitConfig
		wantStatus int
		wantError  bool
	}{
		{
			name: "Valid token bucket configuration",
			key:  "test_key",
			config: service.RateLimitConfig{
				Algorithm:      "token_bucket",
				Limit:          100,
				WindowSeconds:  10,
				RefillRate:     10,
				RefillInterval: 1,
			},
			wantStatus: http.StatusCreated,
			wantError:  false,
		},
		{
			name: "Valid fixed window configuration",
			key:  "test_key_2",
			config: service.RateLimitConfig{
				Algorithm:     "fixed_window",
				Limit:         50,
				WindowSeconds: 5,
			},
			wantStatus: http.StatusCreated,
			wantError:  false,
		},
		{
			name: "Invalid algorithm",
			key:  "test_key_3",
			config: service.RateLimitConfig{
				Algorithm:     "invalid",
				Limit:         100,
				WindowSeconds: 10,
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  true,
		},
		{
			name: "Zero limit",
			key:  "test_key_4",
			config: service.RateLimitConfig{
				Algorithm:     "fixed_window",
				Limit:         0,
				WindowSeconds: 10,
			},
			wantStatus: http.StatusInternalServerError,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := struct {
				Key    string
				Config service.RateLimitConfig
			}{
				Key:    tt.key,
				Config: tt.config,
			}

			bodyBytes, _ := json.Marshal(body)
			req := httptest.NewRequest("POST", "/ratelimit/set", bytes.NewReader(bodyBytes))
			w := httptest.NewRecorder()

			h.Set()(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantError && w.Code == http.StatusCreated {
				t.Errorf("expected error, got success")
			}
		})
	}
}

// Test POST /ratelimit/check
func TestRateLimitCheck(t *testing.T) {
	store := NewMockStore()
	logger := createTestLogger()
	defer logger.Sync()

	rateLimitService := service.NewRateLimitService(store, logger)
	h := handler.NewRateLimitHandler(rateLimitService)

	// First, set a rate limit configuration
	config := service.RateLimitConfig{
		Algorithm:     "fixed_window",
		Limit:         3,
		WindowSeconds: 60,
	}

	configKey := fmt.Sprintf("ratelimit:config:%s", "test_user")
	configJSON, _ := json.Marshal(config)
	store.Set(context.Background(), configKey, string(configJSON), 0)

	tests := []struct {
		name       string
		key        string
		wantStatus int
		wantAllow  bool
	}{
		{
			name:       "First request should be allowed",
			key:        "test_user",
			wantStatus: http.StatusOK,
			wantAllow:  true,
		},
		{
			name:       "Non-existent key should return 404",
			key:        "non_existent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := handler.CheckRequest{
				Key:       tt.key,
				Algorithm: "fixed_window",
			}

			bodyBytes, _ := json.Marshal(body)
			req := httptest.NewRequest("POST", "/ratelimit/check", bytes.NewReader(bodyBytes))
			w := httptest.NewRecorder()

			h.Check()(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var resp handler.CheckResponse
				json.NewDecoder(w.Body).Decode(&resp)

				if resp.Allowed != tt.wantAllow {
					t.Errorf("got allowed=%v, want %v", resp.Allowed, tt.wantAllow)
				}
			}
		})
	}
}

// Test GET /ratelimit/status/:key
func TestRateLimitStatus(t *testing.T) {
	store := NewMockStore()
	logger := createTestLogger()
	defer logger.Sync()

	rateLimitService := service.NewRateLimitService(store, logger)
	h := handler.NewRateLimitHandler(rateLimitService)

	// Set a rate limit configuration
	config := service.RateLimitConfig{
		Algorithm:     "fixed_window",
		Limit:         100,
		WindowSeconds: 60,
	}

	configKey := fmt.Sprintf("ratelimit:config:%s", "status_test")
	metadataKey := fmt.Sprintf("ratelimit:metadata:%s", "status_test")

	configJSON, _ := json.Marshal(config)
	store.Set(context.Background(), configKey, string(configJSON), 0)

	metadata := map[string]interface{}{
		"created_at": time.Now().Unix(),
		"updated_at": time.Now().Unix(),
	}
	metadataJSON, _ := json.Marshal(metadata)
	store.Set(context.Background(), metadataKey, string(metadataJSON), 0)

	tests := []struct {
		name       string
		key        string
		wantStatus int
		wantKey    string
	}{
		{
			name:       "Get existing status",
			key:        "status_test",
			wantStatus: http.StatusOK,
			wantKey:    "status_test",
		},
		{
			name:       "Get non-existent status",
			key:        "non_existent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := mux.NewRouter()
			router.HandleFunc("/ratelimit/status/{key}", h.Status()).Methods("GET")

			req := httptest.NewRequest("GET", fmt.Sprintf("/ratelimit/status/%s", tt.key), nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				var resp handler.StatusResponse
				json.NewDecoder(w.Body).Decode(&resp)

				if resp.Key != tt.wantKey {
					t.Errorf("got key=%s, want %s", resp.Key, tt.wantKey)
				}
			}
		})
	}
}

// Test DELETE /ratelimit/reset/:key
func TestRateLimitReset(t *testing.T) {
	store := NewMockStore()
	logger := createTestLogger()
	defer logger.Sync()

	rateLimitService := service.NewRateLimitService(store, logger)
	h := handler.NewRateLimitHandler(rateLimitService)

	// Set a rate limit configuration
	config := service.RateLimitConfig{
		Algorithm:     "fixed_window",
		Limit:         100,
		WindowSeconds: 60,
	}

	configKey := fmt.Sprintf("ratelimit:config:%s", "reset_test")
	configJSON, _ := json.Marshal(config)
	store.Set(context.Background(), configKey, string(configJSON), 0)

	tests := []struct {
		name       string
		key        string
		wantStatus int
	}{
		{
			name:       "Reset existing rate limit",
			key:        "reset_test",
			wantStatus: http.StatusOK,
		},
		{
			name:       "Reset non-existent rate limit",
			key:        "non_existent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := mux.NewRouter()
			router.HandleFunc("/ratelimit/reset/{key}", h.Reset()).Methods("DELETE")

			req := httptest.NewRequest("DELETE", fmt.Sprintf("/ratelimit/reset/%s", tt.key), nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

// Test configuration validation
func TestRateLimitConfigValidation(t *testing.T) {
	store := NewMockStore()
	logger := createTestLogger()
	defer logger.Sync()

	rateLimitService := service.NewRateLimitService(store, logger)
	h := handler.NewRateLimitHandler(rateLimitService)

	tests := []struct {
		name       string
		config     service.RateLimitConfig
		wantStatus int
	}{
		{
			name: "Valid token bucket",
			config: service.RateLimitConfig{
				Algorithm:      "token_bucket",
				Limit:          100,
				WindowSeconds:  10,
				RefillRate:     10,
				RefillInterval: 1,
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "Valid sliding window",
			config: service.RateLimitConfig{
				Algorithm:     "sliding_window",
				Limit:         50,
				WindowSeconds: 5,
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "Invalid algorithm",
			config: service.RateLimitConfig{
				Algorithm:     "invalid_algo",
				Limit:         100,
				WindowSeconds: 10,
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "Zero limit",
			config: service.RateLimitConfig{
				Algorithm:     "fixed_window",
				Limit:         0,
				WindowSeconds: 10,
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "Token bucket without refill rate",
			config: service.RateLimitConfig{
				Algorithm:      "token_bucket",
				Limit:          100,
				WindowSeconds:  10,
				RefillRate:     0,
				RefillInterval: 1,
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := struct {
				Key    string
				Config service.RateLimitConfig
			}{
				Key:    fmt.Sprintf("validation_%s", tt.name),
				Config: tt.config,
			}

			bodyBytes, _ := json.Marshal(body)
			req := httptest.NewRequest("POST", "/ratelimit/set", bytes.NewReader(bodyBytes))
			w := httptest.NewRecorder()

			h.Set()(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

// Test response headers
func TestRateLimitResponseHeaders(t *testing.T) {
	store := NewMockStore()
	logger := createTestLogger()
	defer logger.Sync()

	rateLimitService := service.NewRateLimitService(store, logger)
	h := handler.NewRateLimitHandler(rateLimitService)

	// Set a rate limit configuration
	config := service.RateLimitConfig{
		Algorithm:     "fixed_window",
		Limit:         10,
		WindowSeconds: 60,
	}

	configKey := fmt.Sprintf("ratelimit:config:%s", "header_test")
	configJSON, _ := json.Marshal(config)
	store.Set(context.Background(), configKey, string(configJSON), 0)

	body := handler.CheckRequest{
		Key:       "header_test",
		Algorithm: "fixed_window",
	}

	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/ratelimit/check", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	h.Check()(w, req)

	// Verify headers are present
	if w.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("X-RateLimit-Limit header missing")
	}

	if w.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("X-RateLimit-Remaining header missing")
	}

	if w.Header().Get("X-RateLimit-Reset") == "" {
		t.Error("X-RateLimit-Reset header missing")
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type is %s, want application/json", w.Header().Get("Content-Type"))
	}
}

// Test content type validation
func TestRateLimitInvalidJSON(t *testing.T) {
	store := NewMockStore()
	logger := createTestLogger()
	defer logger.Sync()

	rateLimitService := service.NewRateLimitService(store, logger)
	h := handler.NewRateLimitHandler(rateLimitService)

	req := httptest.NewRequest("POST", "/ratelimit/set", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()

	h.Set()(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want %d", w.Code, http.StatusBadRequest)
	}
}
