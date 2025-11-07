package transport

import (
	"context"
	"net"
	"time"

	"github.com/mohammadhprp/throttle/internal/handler"
	pb "github.com/mohammadhprp/throttle/proto"
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
	pb.RegisterHealthServer(gs.server, &HealthServiceImpl{
		healthCheck: gs.handlers.HealthCheck,
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
	pb.UnimplementedHealthServer
	healthCheck *handler.HealthCheckHanlder
}

func (hs *HealthServiceImpl) Check(ctx context.Context, _ *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	status := hs.currentStatus(ctx)
	return &pb.HealthCheckResponse{
		Status: status,
	}, nil
}

func (hs *HealthServiceImpl) Watch(_ *pb.HealthCheckRequest, stream grpc.ServerStreamingServer[pb.HealthCheckResponse]) error {
	ctx := stream.Context()
	status := hs.currentStatus(ctx)
	if err := stream.Send(&pb.HealthCheckResponse{Status: status}); err != nil {
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
			if err := stream.Send(&pb.HealthCheckResponse{Status: status}); err != nil {
				return err
			}
		}
	}
}

func (hs *HealthServiceImpl) currentStatus(ctx context.Context) pb.HealthCheckResponse_ServingStatus {
	if hs == nil || hs.healthCheck == nil {
		return pb.HealthCheckResponse_UNKNOWN
	}

	if ctx == nil {
		ctx = context.Background()
	}

	if err := hs.healthCheck.Ping(ctx); err != nil {
		return pb.HealthCheckResponse_NOT_SERVING
	}

	return pb.HealthCheckResponse_SERVING
}
