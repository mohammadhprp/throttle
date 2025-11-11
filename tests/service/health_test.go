package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mohammadhprp/throttle/internal/service"
	"github.com/mohammadhprp/throttle/internal/storage"
	"go.uber.org/zap"
)

// TestHealthService_GetHealthStatus_Healthy tests that GetHealthStatus returns healthy when store is healthy
func TestHealthService_GetHealthStatus_Healthy(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewHealthService(store, logger)
	status, timestamp, err := svc.GetHealthStatus(context.Background())

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if status != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", status)
	}

	if timestamp == "" {
		t.Errorf("expected non-empty timestamp")
	}

	// Verify timestamp is RFC3339 format
	if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
		t.Errorf("timestamp is not in RFC3339 format: %s", timestamp)
	}
}

// TestHealthService_GetHealthStatus_Unhealthy tests that GetHealthStatus returns unhealthy when store fails
func TestHealthService_GetHealthStatus_Unhealthy(t *testing.T) {
	mockStore := &mockStore{
		pingErr: errors.New("store is down"),
	}
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewHealthService(mockStore, logger)
	status, timestamp, err := svc.GetHealthStatus(context.Background())

	if err == nil {
		t.Errorf("expected error, got nil")
	}

	if status != "unhealthy" {
		t.Errorf("expected status 'unhealthy', got '%s'", status)
	}

	if timestamp == "" {
		t.Errorf("expected non-empty timestamp even on error")
	}
}

// TestHealthService_GetHealthStatus_ContextDeadlineExceeded tests deadline exceeded scenario
func TestHealthService_GetHealthStatus_ContextDeadlineExceeded(t *testing.T) {
	mockStore := &mockStore{
		pingErr: context.DeadlineExceeded,
	}
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewHealthService(mockStore, logger)
	status, _, err := svc.GetHealthStatus(context.Background())

	if err == nil {
		t.Errorf("expected error, got nil")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded error, got %v", err)
	}

	if status != "unhealthy" {
		t.Errorf("expected status 'unhealthy', got '%s'", status)
	}
}

// TestHealthService_GetHealthStatus_ContextCancelled tests context cancelled scenario
func TestHealthService_GetHealthStatus_ContextCancelled(t *testing.T) {
	mockStore := &mockStore{
		pingErr: context.Canceled,
	}
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewHealthService(mockStore, logger)
	status, _, err := svc.GetHealthStatus(context.Background())

	if err == nil {
		t.Errorf("expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected Canceled error, got %v", err)
	}

	if status != "unhealthy" {
		t.Errorf("expected status 'unhealthy', got '%s'", status)
	}
}

// TestHealthService_Ping_Success tests successful ping
func TestHealthService_Ping_Success(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewHealthService(store, logger)
	err := svc.Ping(context.Background())

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// TestHealthService_Ping_Failure tests failed ping
func TestHealthService_Ping_Failure(t *testing.T) {
	mockStore := &mockStore{
		pingErr: errors.New("store is down"),
	}
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewHealthService(mockStore, logger)
	err := svc.Ping(context.Background())

	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

// TestHealthService_Ping_WithContext tests ping with context
func TestHealthService_Ping_WithContext(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewHealthService(store, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := svc.Ping(ctx)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// TestHealthService_NewHealthService tests proper initialization
func TestHealthService_NewHealthService(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewHealthService(store, logger)

	if svc == nil {
		t.Errorf("expected non-nil service")
	}
}

// TestHealthService_GetHealthStatus_MultipleRequests tests multiple consecutive requests
func TestHealthService_GetHealthStatus_MultipleRequests(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewHealthService(store, logger)

	for i := 0; i < 5; i++ {
		status, timestamp, err := svc.GetHealthStatus(context.Background())

		if err != nil {
			t.Errorf("request %d: expected no error, got %v", i+1, err)
		}

		if status != "healthy" {
			t.Errorf("request %d: expected status 'healthy', got '%s'", i+1, status)
		}

		if timestamp == "" {
			t.Errorf("request %d: expected non-empty timestamp", i+1)
		}
	}
}

// TestHealthService_GetHealthStatus_ConcurrentRequests tests concurrent health checks
func TestHealthService_GetHealthStatus_ConcurrentRequests(t *testing.T) {
	store := storage.NewMemoryStore()
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	svc := service.NewHealthService(store, logger)
	done := make(chan bool)
	var successCount int

	for i := 0; i < 10; i++ {
		go func() {
			status, _, err := svc.GetHealthStatus(context.Background())
			if err == nil && status == "healthy" {
				successCount++
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if successCount != 10 {
		t.Errorf("expected 10 successful checks, got %d", successCount)
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
