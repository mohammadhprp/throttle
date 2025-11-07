package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mohammadhprp/throttle/internal/handler"
	"github.com/mohammadhprp/throttle/internal/storage"
	"go.uber.org/zap"
)

// newTestLogger creates a test logger with minimal output
func newTestLogger(t *testing.T) *zap.Logger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return logger
}

// HealthCheckResponse represents the JSON response from health check endpoint
type HealthCheckResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
	Error  string `json:"error,omitempty"`
}

// TestHealthCheckHandler_Healthy tests that handler returns 200 when store is healthy
func TestHealthCheckHandler_Healthy(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	defer logger.Sync()

	h := handler.NewHealthCheckHanlder(store, logger)
	healthCheckFunc := h.HealthCheck()

	// Create a test request
	req := httptest.NewRequest("GET", "http://example.com/health", nil)
	w := httptest.NewRecorder()

	// Call the handler
	healthCheckFunc(w, req)

	// Check status code
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Parse response
	var resp HealthCheckResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check status
	if resp.Status != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", resp.Status)
	}

	// Check that time is not empty
	if resp.Time == "" {
		t.Errorf("expected non-empty time field")
	}

	// Check that error is empty
	if resp.Error != "" {
		t.Errorf("expected empty error field, got '%s'", resp.Error)
	}
}

// TestHealthCheckHandler_UnhealthyStoreDown tests that handler returns 503 when store is down
func TestHealthCheckHandler_UnhealthyStoreDown(t *testing.T) {
	// Create a mock store that fails on Ping
	mockStore := &mockStore{
		pingErr: context.DeadlineExceeded,
	}
	logger := newTestLogger(t)
	defer logger.Sync()

	h := handler.NewHealthCheckHanlder(mockStore, logger)
	healthCheckFunc := h.HealthCheck()

	// Create a test request
	req := httptest.NewRequest("GET", "http://example.com/health", nil)
	w := httptest.NewRecorder()

	// Call the handler
	healthCheckFunc(w, req)

	// Check status code
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	// Parse response
	var resp HealthCheckResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check status
	if resp.Status != "unhealthy" {
		t.Errorf("expected status 'unhealthy', got '%s'", resp.Status)
	}

	// Check error field
	if resp.Error == "" {
		t.Errorf("expected non-empty error field")
	}

	// Check that time is still set
	if resp.Time == "" {
		t.Errorf("expected non-empty time field")
	}
}

// TestHealthCheckHandler_ContentTypeJSON tests that response has correct content type
func TestHealthCheckHandler_ContentTypeJSON(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	defer logger.Sync()

	h := handler.NewHealthCheckHanlder(store, logger)
	healthCheckFunc := h.HealthCheck()

	// Create a test request
	req := httptest.NewRequest("GET", "http://example.com/health", nil)
	w := httptest.NewRecorder()

	// Call the handler
	healthCheckFunc(w, req)

	// Check Content-Type header
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentType)
	}
}

// TestHealthCheckHandler_ValidJSON tests that response is valid JSON
func TestHealthCheckHandler_ValidJSON(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	defer logger.Sync()

	h := handler.NewHealthCheckHanlder(store, logger)
	healthCheckFunc := h.HealthCheck()

	// Create a test request
	req := httptest.NewRequest("GET", "http://example.com/health", nil)
	w := httptest.NewRecorder()

	// Call the handler
	healthCheckFunc(w, req)

	// Try to unmarshal response
	var resp HealthCheckResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}
}

// TestHealthCheckHandler_RequiredFields tests that response has all required fields
func TestHealthCheckHandler_RequiredFields(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	defer logger.Sync()

	h := handler.NewHealthCheckHanlder(store, logger)
	healthCheckFunc := h.HealthCheck()

	// Create a test request
	req := httptest.NewRequest("GET", "http://example.com/health", nil)
	w := httptest.NewRecorder()

	// Call the handler
	healthCheckFunc(w, req)

	// Parse response
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check required fields
	if _, ok := resp["status"]; !ok {
		t.Errorf("missing 'status' field in response")
	}

	if _, ok := resp["time"]; !ok {
		t.Errorf("missing 'time' field in response")
	}
}

// TestHealthCheckHandler_TimeFormat tests that time is in RFC3339 format
func TestHealthCheckHandler_TimeFormat(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	defer logger.Sync()

	h := handler.NewHealthCheckHanlder(store, logger)
	healthCheckFunc := h.HealthCheck()

	// Create a test request
	req := httptest.NewRequest("GET", "http://example.com/health", nil)
	w := httptest.NewRecorder()

	// Call the handler
	healthCheckFunc(w, req)

	// Parse response
	var resp HealthCheckResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify time can be parsed as RFC3339
	if _, err := time.Parse(time.RFC3339, resp.Time); err != nil {
		t.Errorf("time field is not in RFC3339 format: %s", resp.Time)
	}
}

// TestHealthCheckHandler_GET tests that handler works with GET requests
func TestHealthCheckHandler_GET(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	defer logger.Sync()

	h := handler.NewHealthCheckHanlder(store, logger)
	healthCheckFunc := h.HealthCheck()

	// Create a GET request
	req := httptest.NewRequest("GET", "http://example.com/health", nil)
	w := httptest.NewRecorder()

	// Call the handler
	healthCheckFunc(w, req)

	// Should succeed
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d for GET request, got %d", http.StatusOK, w.Code)
	}
}

// TestHealthCheckHandler_POST tests that handler works with POST requests
func TestHealthCheckHandler_POST(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	defer logger.Sync()

	h := handler.NewHealthCheckHanlder(store, logger)
	healthCheckFunc := h.HealthCheck()

	// Create a POST request
	req := httptest.NewRequest("POST", "http://example.com/health", nil)
	w := httptest.NewRecorder()

	// Call the handler
	healthCheckFunc(w, req)

	// Should succeed (handler doesn't check method)
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d for POST request, got %d", http.StatusOK, w.Code)
	}
}

// TestHealthCheckHandler_MultipleRequests tests multiple consecutive requests
func TestHealthCheckHandler_MultipleRequests(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	defer logger.Sync()

	h := handler.NewHealthCheckHanlder(store, logger)
	healthCheckFunc := h.HealthCheck()

	// Make multiple requests
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "http://example.com/health", nil)
		w := httptest.NewRecorder()

		healthCheckFunc(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}

		var resp HealthCheckResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Errorf("request %d: failed to decode response: %v", i+1, err)
		}

		if resp.Status != "healthy" {
			t.Errorf("request %d: expected status 'healthy', got '%s'", i+1, resp.Status)
		}
	}
}

// TestHealthCheckHandler_ContextTimeout tests that handler respects context timeout in store.Ping()
func TestHealthCheckHandler_ContextTimeout(t *testing.T) {
	// Create a mock store that returns context deadline exceeded error
	mockStore := &mockStore{
		pingErr: context.DeadlineExceeded,
	}
	logger := newTestLogger(t)
	defer logger.Sync()

	h := handler.NewHealthCheckHanlder(mockStore, logger)
	healthCheckFunc := h.HealthCheck()

	// Create a request with a short timeout (store will return deadline exceeded)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("GET", "http://example.com/health", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	// Call the handler
	healthCheckFunc(w, req)

	// Should fail due to context timeout from store
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	var resp HealthCheckResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "unhealthy" {
		t.Errorf("expected status 'unhealthy', got '%s'", resp.Status)
	}
}

// TestHealthCheckHandler_StoreError tests that various store errors are handled
func TestHealthCheckHandler_StoreError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		expectError bool
	}{
		{
			name:        "context deadline exceeded",
			err:         context.DeadlineExceeded,
			expectError: true,
		},
		{
			name:        "context cancelled",
			err:         context.Canceled,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &mockStore{
				pingErr: tt.err,
			}
			logger := newTestLogger(t)
			defer logger.Sync()

			h := handler.NewHealthCheckHanlder(mockStore, logger)
			healthCheckFunc := h.HealthCheck()

			req := httptest.NewRequest("GET", "http://example.com/health", nil)
			w := httptest.NewRecorder()

			healthCheckFunc(w, req)

			if tt.expectError {
				if w.Code != http.StatusServiceUnavailable {
					t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
				}

				var resp HealthCheckResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if resp.Status != "unhealthy" {
					t.Errorf("expected status 'unhealthy', got '%s'", resp.Status)
				}

				if resp.Error == "" {
					t.Errorf("expected non-empty error field")
				}
			}
		})
	}
}

// TestHealthCheckHandler_ResponseStructure tests complete response structure
func TestHealthCheckHandler_ResponseStructure(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	defer logger.Sync()

	h := handler.NewHealthCheckHanlder(store, logger)
	healthCheckFunc := h.HealthCheck()

	req := httptest.NewRequest("GET", "http://example.com/health", nil)
	w := httptest.NewRecorder()

	healthCheckFunc(w, req)

	// Parse response body
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify structure
	if len(resp) < 2 {
		t.Errorf("expected at least 2 fields in response, got %d", len(resp))
	}

	// Check expected fields
	if status, ok := resp["status"]; !ok || status != "healthy" {
		t.Errorf("missing or invalid status field")
	}

	if _, ok := resp["time"]; !ok {
		t.Errorf("missing time field")
	}
}

// TestHealthCheckHandler_ConcurrentRequests tests handler under concurrent load
func TestHealthCheckHandler_ConcurrentRequests(t *testing.T) {
	store := storage.NewMemoryStore()
	logger := newTestLogger(t)
	defer logger.Sync()

	h := handler.NewHealthCheckHanlder(store, logger)
	healthCheckFunc := h.HealthCheck()

	done := make(chan struct{})
	var successCount int

	// Send 10 concurrent requests
	for i := 0; i < 10; i++ {
		go func() {
			req := httptest.NewRequest("GET", "http://example.com/health", nil)
			w := httptest.NewRecorder()
			healthCheckFunc(w, req)

			if http.StatusOK == w.Code {
				successCount++
			}
			done <- struct{}{}
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// All requests should succeed
	if successCount != 10 {
		t.Errorf("expected 10 successful requests, got %d", successCount)
	}
}

// mockStore is a mock implementation of storage.Store for testing
type mockStore struct {
	pingErr   error
	pingDelay int // milliseconds
}

func (m *mockStore) Ping(ctx context.Context) error {
	if m.pingDelay > 0 {
		time.Sleep(time.Duration(m.pingDelay) * time.Millisecond)
	}
	if m.pingErr != nil {
		return m.pingErr
	}
	return nil
}

func (m *mockStore) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	return nil
}

func (m *mockStore) Get(ctx context.Context, key string) (string, error) {
	return "", nil
}

func (m *mockStore) Delete(ctx context.Context, key string) error {
	return nil
}

func (m *mockStore) Increment(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	return 0, nil
}

func (m *mockStore) IncrementBy(ctx context.Context, key string, value int64, expiration time.Duration) (int64, error) {
	return 0, nil
}

func (m *mockStore) ZAdd(ctx context.Context, key string, score float64, member string, expiration time.Duration) error {
	return nil
}

func (m *mockStore) ZRemRangeByScore(ctx context.Context, key string, min, max float64) error {
	return nil
}

func (m *mockStore) ZCount(ctx context.Context, key string, min, max float64) (int64, error) {
	return 0, nil
}

func (m *mockStore) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return nil
}

func (m *mockStore) Close() error {
	return nil
}
