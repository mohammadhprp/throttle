package transport

import (
	"context"
	"encoding/json"
	"net"
	"time"

	"github.com/mohammadhprp/throttle/internal/handler"
	pbhealth "github.com/mohammadhprp/throttle/proto/health"
	pbratelimit "github.com/mohammadhprp/throttle/proto/ratelimit"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// GRPCServer implements the Server interface for gRPC transport
type GRPCServer struct {
	server   *grpc.Server
	address  string
	logger   *zap.Logger
	handlers *ServiceHandlers
}

// NewGRPCServer creates a new gRPC server
func NewGRPCServer(cfg ServerConfig) *GRPCServer {
	gsrv := grpc.NewServer()

	handlers := &ServiceHandlers{
		HealthCheck: handler.NewHealthCheckHanlder(cfg.Store, cfg.Logger),
		RateLimit:   handler.NewRateLimitHandler(cfg.Store, cfg.Logger),
	}

	grpcSrv := &GRPCServer{
		address:  cfg.Address,
		logger:   cfg.Logger,
		handlers: handlers,
		server:   gsrv,
	}

	grpcSrv.registerServices()
	return grpcSrv
}

// registerServices registers all gRPC services
func (gs *GRPCServer) registerServices() {
	pbhealth.RegisterHealthServer(gs.server, &HealthServiceImpl{
		healthCheck: gs.handlers.HealthCheck,
	})
	pbratelimit.RegisterRateLimitServer(gs.server, &RateLimitServiceImpl{
		rateLimit: gs.handlers.RateLimit,
	})
}

// Start starts the gRPC server
func (gs *GRPCServer) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", gs.address)
	if err != nil {
		gs.logger.Error("Failed to listen on address", zap.String("address", gs.address), zap.Error(err))
		return err
	}

	gs.logger.Info("Starting gRPC server", zap.String("address", gs.address))

	go func() {
		if err := gs.server.Serve(listener); err != nil {
			gs.logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop gracefully stops the gRPC server
func (gs *GRPCServer) Stop(ctx context.Context) error {
	gs.logger.Info("Stopping gRPC server")
	stopped := make(chan struct{})
	go func() {
		gs.server.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		gs.server.Stop()
		return ctx.Err()
	}
}

// Addr returns the address the gRPC server is listening on
func (gs *GRPCServer) Addr() string {
	return gs.address
}

// HealthServiceImpl implements the Health service
type HealthServiceImpl struct {
	pbhealth.UnimplementedHealthServer
	healthCheck *handler.HealthCheckHanlder
}

func (hs *HealthServiceImpl) Check(ctx context.Context, _ *pbhealth.HealthCheckRequest) (*pbhealth.HealthCheckResponse, error) {
	status := hs.currentStatus(ctx)
	return &pbhealth.HealthCheckResponse{
		Status: status,
	}, nil
}

func (hs *HealthServiceImpl) Watch(_ *pbhealth.HealthCheckRequest, stream grpc.ServerStreamingServer[pbhealth.HealthCheckResponse]) error {
	ctx := stream.Context()
	status := hs.currentStatus(ctx)
	if err := stream.Send(&pbhealth.HealthCheckResponse{Status: status}); err != nil {
		return err
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			nextStatus := hs.currentStatus(ctx)
			if nextStatus == status {
				continue
			}

			status = nextStatus
			if err := stream.Send(&pbhealth.HealthCheckResponse{Status: status}); err != nil {
				return err
			}
		}
	}
}

func (hs *HealthServiceImpl) currentStatus(ctx context.Context) pbhealth.HealthCheckResponse_ServingStatus {
	if hs == nil || hs.healthCheck == nil {
		return pbhealth.HealthCheckResponse_UNKNOWN
	}

	if ctx == nil {
		ctx = context.Background()
	}

	if err := hs.healthCheck.Ping(ctx); err != nil {
		return pbhealth.HealthCheckResponse_NOT_SERVING
	}

	return pbhealth.HealthCheckResponse_SERVING
}

// RateLimitServiceImpl implements the RateLimit service
type RateLimitServiceImpl struct {
	pbratelimit.UnimplementedRateLimitServer
	rateLimit *handler.RateLimitHandler
}

// Set configures a new rate limit or updates an existing one
func (rs *RateLimitServiceImpl) Set(ctx context.Context, req *pbratelimit.SetRequest) (*pbratelimit.SetResponse, error) {
	// Convert proto RateLimitConfig to handler RateLimitConfig
	config := &handler.RateLimitConfig{
		Algorithm:      req.Config.Algorithm,
		Limit:          req.Config.Limit,
		WindowSeconds:  int(req.Config.WindowSeconds),
		RefillRate:     int(req.Config.RefillRate),
		RefillInterval: int(req.Config.RefillInterval),
	}

	// Validate config
	if err := rs.rateLimit.ValidateConfig(config); err != nil {
		return nil, err
	}

	// Store configuration
	configKey := rs.rateLimit.ConfigKey(req.Key)
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	metadata := map[string]interface{}{
		"config":     config,
		"created_at": now.Unix(),
		"updated_at": now.Unix(),
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	// Store both config and metadata
	if err := rs.rateLimit.Store.Set(ctx, configKey, string(configJSON), 0); err != nil {
		return nil, err
	}

	metadataKey := rs.rateLimit.MetadataKey(req.Key)
	if err := rs.rateLimit.Store.Set(ctx, metadataKey, string(metadataJSON), 0); err != nil {
		return nil, err
	}

	return &pbratelimit.SetResponse{
		Message: "rate limit configured",
		Key:     req.Key,
	}, nil
}

// Check verifies if a request is allowed under the configured rate limit
func (rs *RateLimitServiceImpl) Check(ctx context.Context, req *pbratelimit.CheckRequest) (*pbratelimit.CheckResponse, error) {
	// Get configuration
	config, err := rs.rateLimit.GetConfig(ctx, req.Key)
	if err != nil {
		return nil, err
	}

	// Get or create limiter
	limiterKey := rs.rateLimit.LimiterKey(req.Key, config.Algorithm)
	rateLimiter, err := rs.rateLimit.GetOrCreateLimiter(limiterKey, config)
	if err != nil {
		return nil, err
	}

	// Check if request is allowed
	allowed, err := rateLimiter.Allow(ctx, req.Key)
	if err != nil {
		// Fail open on error
		allowed = true
	}

	// Calculate metrics for response
	remaining := rs.rateLimit.CalculateRemaining(ctx, req.Key, config, allowed)
	resetAt := rs.rateLimit.CalculateResetAt(config)
	retryAfter := rs.rateLimit.CalculateRetryAfter(config)

	return &pbratelimit.CheckResponse{
		Allowed:    allowed,
		Remaining:  remaining,
		ResetAt:    resetAt,
		RetryAfter: int32(retryAfter),
	}, nil
}

// Status retrieves the current status of a rate limit configuration
func (rs *RateLimitServiceImpl) Status(ctx context.Context, req *pbratelimit.StatusRequest) (*pbratelimit.StatusResponse, error) {
	// Get configuration
	config, err := rs.rateLimit.GetConfig(ctx, req.Key)
	if err != nil {
		return nil, err
	}

	// Get metadata
	metadata, err := rs.rateLimit.GetMetadata(ctx, req.Key)
	if err != nil {
		metadata = map[string]interface{}{
			"created_at": time.Now().Unix(),
			"updated_at": time.Now().Unix(),
		}
	}

	createdAt := int64(0)
	updatedAt := int64(0)
	if ca, ok := metadata["created_at"].(float64); ok {
		createdAt = int64(ca)
	}
	if ua, ok := metadata["updated_at"].(float64); ok {
		updatedAt = int64(ua)
	}

	return &pbratelimit.StatusResponse{
		Key:       req.Key,
		Algorithm: config.Algorithm,
		Config: &pbratelimit.RateLimitConfig{
			Algorithm:      config.Algorithm,
			Limit:          config.Limit,
			WindowSeconds:  int32(config.WindowSeconds),
			RefillRate:     int32(config.RefillRate),
			RefillInterval: int32(config.RefillInterval),
		},
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		NextReset: rs.rateLimit.CalculateResetAt(config),
	}, nil
}

// Reset resets a rate limit configuration and clears its state
func (rs *RateLimitServiceImpl) Reset(ctx context.Context, req *pbratelimit.ResetRequest) (*pbratelimit.ResetResponse, error) {
	// Get configuration to find limiter type
	config, err := rs.rateLimit.GetConfig(ctx, req.Key)
	if err != nil {
		return nil, err
	}

	// Get or create limiter and reset it
	limiterKey := rs.rateLimit.LimiterKey(req.Key, config.Algorithm)
	rateLimiter, err := rs.rateLimit.GetOrCreateLimiter(limiterKey, config)
	if err != nil {
		return nil, err
	}

	if err := rateLimiter.Reset(ctx, req.Key); err != nil {
		return nil, err
	}

	// Delete configuration and metadata
	configKey := rs.rateLimit.ConfigKey(req.Key)
	metadataKey := rs.rateLimit.MetadataKey(req.Key)

	_ = rs.rateLimit.Store.Delete(ctx, configKey)
	_ = rs.rateLimit.Store.Delete(ctx, metadataKey)

	return &pbratelimit.ResetResponse{
		Message: "rate limit reset",
		Key:     req.Key,
	}, nil
}
