package transport

import (
	"context"
	"net"
	"time"

	"github.com/mohammadhprp/throttle/internal/service"
	pbhealth "github.com/mohammadhprp/throttle/proto/health"
	pbratelimit "github.com/mohammadhprp/throttle/proto/ratelimit"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// GRPCServer implements the Server interface for gRPC transport
type GRPCServer struct {
	server           *grpc.Server
	address          string
	logger           *zap.Logger
	handlers         *ServiceHandlers
	healthService    *service.HealthService
	rateLimitService *service.RateLimitService
}

// NewGRPCServer creates a new gRPC server
func NewGRPCServer(cfg ServerConfig) *GRPCServer {
	gsrv := grpc.NewServer()
	healthService, rateLimitService := createServices(cfg)
	handlers := createServiceHandlers(cfg, healthService, rateLimitService)

	grpcSrv := &GRPCServer{
		address:          cfg.Address,
		logger:           cfg.Logger,
		handlers:         handlers,
		server:           gsrv,
		healthService:    healthService,
		rateLimitService: rateLimitService,
	}

	grpcSrv.registerServices()
	return grpcSrv
}

// registerServices registers all gRPC services
func (gs *GRPCServer) registerServices() {
	pbhealth.RegisterHealthServer(gs.server, &HealthServiceImpl{
		healthService: gs.healthService,
	})
	pbratelimit.RegisterRateLimitServer(gs.server, &RateLimitServiceImpl{
		rateLimitService: gs.rateLimitService,
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
	healthService *service.HealthService
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
	if err := hs.healthService.Ping(ctx); err != nil {
		return pbhealth.HealthCheckResponse_NOT_SERVING
	}

	return pbhealth.HealthCheckResponse_SERVING
}

// protoConfigToService converts proto RateLimitConfig to service RateLimitConfig
func protoConfigToService(pbConfig *pbratelimit.RateLimitConfig) *service.RateLimitConfig {
	return &service.RateLimitConfig{
		Algorithm:      pbConfig.Algorithm,
		Limit:          pbConfig.Limit,
		WindowSeconds:  int(pbConfig.WindowSeconds),
		RefillRate:     int(pbConfig.RefillRate),
		RefillInterval: int(pbConfig.RefillInterval),
	}
}

// serviceConfigToProto converts service RateLimitConfig to proto RateLimitConfig
func serviceConfigToProto(cfg *service.RateLimitConfig) *pbratelimit.RateLimitConfig {
	return &pbratelimit.RateLimitConfig{
		Algorithm:      cfg.Algorithm,
		Limit:          cfg.Limit,
		WindowSeconds:  int32(cfg.WindowSeconds),
		RefillRate:     int32(cfg.RefillRate),
		RefillInterval: int32(cfg.RefillInterval),
	}
}

// RateLimitServiceImpl implements the RateLimit service
type RateLimitServiceImpl struct {
	pbratelimit.UnimplementedRateLimitServer
	rateLimitService *service.RateLimitService
}

// Set configures a new rate limit or updates an existing one
func (rs *RateLimitServiceImpl) Set(ctx context.Context, req *pbratelimit.SetRequest) (*pbratelimit.SetResponse, error) {
	config := protoConfigToService(req.Config)

	if err := rs.rateLimitService.SetConfig(ctx, req.Key, config); err != nil {
		return nil, err
	}

	return &pbratelimit.SetResponse{
		Message: "rate limit configured",
		Key:     req.Key,
	}, nil
}

// Check verifies if a request is allowed under the configured rate limit
func (rs *RateLimitServiceImpl) Check(ctx context.Context, req *pbratelimit.CheckRequest) (*pbratelimit.CheckResponse, error) {
	limitResp, err := rs.rateLimitService.CheckLimit(ctx, req.Key)
	if err != nil {
		return nil, err
	}

	return &pbratelimit.CheckResponse{
		Allowed:    limitResp.Allowed,
		Remaining:  limitResp.Remaining,
		ResetAt:    limitResp.ResetAt,
		RetryAfter: int32(limitResp.RetryAfter),
	}, nil
}

// Status retrieves the current status of a rate limit configuration
func (rs *RateLimitServiceImpl) Status(ctx context.Context, req *pbratelimit.StatusRequest) (*pbratelimit.StatusResponse, error) {
	statusResp, err := rs.rateLimitService.GetStatus(ctx, req.Key)
	if err != nil {
		return nil, err
	}

	return &pbratelimit.StatusResponse{
		Key:       req.Key,
		Algorithm: statusResp.Config.Algorithm,
		Config:    serviceConfigToProto(statusResp.Config),
		CreatedAt: statusResp.CreatedAt,
		UpdatedAt: statusResp.UpdatedAt,
		NextReset: statusResp.NextReset,
	}, nil
}

// Reset resets a rate limit configuration and clears its state
func (rs *RateLimitServiceImpl) Reset(ctx context.Context, req *pbratelimit.ResetRequest) (*pbratelimit.ResetResponse, error) {
	if err := rs.rateLimitService.ResetLimit(ctx, req.Key); err != nil {
		return nil, err
	}

	return &pbratelimit.ResetResponse{
		Message: "rate limit reset",
		Key:     req.Key,
	}, nil
}
