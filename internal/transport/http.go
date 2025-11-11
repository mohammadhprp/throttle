package transport

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/mohammadhprp/throttle/internal/handler"
	"github.com/mohammadhprp/throttle/internal/service"
	"go.uber.org/zap"
)

// HTTPServer implements the Server interface for HTTP transport
type HTTPServer struct {
	server              *http.Server
	router              *mux.Router
	address             string
	logger              *zap.Logger
	handlers            *ServiceHandlers
	rateLimitService    *service.RateLimitService
}

// NewHTTPServer creates a new HTTP server
func NewHTTPServer(cfg ServerConfig) *HTTPServer {
	router := mux.NewRouter()

	// Create rate limit service
	rateLimitService := service.NewRateLimitService(cfg.Store, cfg.Logger)

	handlers := &ServiceHandlers{
		HealthCheck: handler.NewHealthCheckHanlder(cfg.Store, cfg.Logger),
		RateLimit:   handler.NewRateLimitHandler(rateLimitService, cfg.Logger),
	}

	hs := &HTTPServer{
		address:           cfg.Address,
		logger:            cfg.Logger,
		handlers:          handlers,
		router:            router,
		rateLimitService:  rateLimitService,
		server: &http.Server{
			Addr:         cfg.Address,
			Handler:      router,
			ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
			IdleTimeout:  time.Duration(cfg.IdleTimeout) * time.Second,
		},
	}

	hs.registerRoutes()
	return hs
}

// registerRoutes registers all HTTP routes
func (hs *HTTPServer) registerRoutes() {
	hs.router.HandleFunc("/health", hs.handlers.HealthCheck.HealthCheck()).Methods("GET")

	// Rate limit routes
	hs.router.HandleFunc("/ratelimit/set", hs.handlers.RateLimit.Set()).Methods("POST")
	hs.router.HandleFunc("/ratelimit/check", hs.handlers.RateLimit.Check()).Methods("POST")
	hs.router.HandleFunc("/ratelimit/status/{key}", hs.handlers.RateLimit.Status()).Methods("GET")
	hs.router.HandleFunc("/ratelimit/reset/{key}", hs.handlers.RateLimit.Reset()).Methods("DELETE")
}

// Start starts the HTTP server
func (hs *HTTPServer) Start(ctx context.Context) error {
	hs.logger.Info("Starting HTTP server", zap.String("address", hs.address))

	go func() {
		if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			hs.logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop gracefully stops the HTTP server
func (hs *HTTPServer) Stop(ctx context.Context) error {
	hs.logger.Info("Stopping HTTP server")
	return hs.server.Shutdown(ctx)
}

// Addr returns the address the HTTP server is listening on
func (hs *HTTPServer) Addr() string {
	return hs.address
}
