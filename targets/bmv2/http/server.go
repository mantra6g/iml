package http

import (
	"bmv2-driver/handlers"
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-logr/logr"
)

const (
	ServerTeardownTimeout = 5 * time.Second
)

type Server struct {
	Log        logr.Logger
	httpServer *http.Server
}

func NewServer(listenAddr string, driver *handlers.Driver, log logr.Logger) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", driver.HealthHandler)
	mux.HandleFunc("/api/tables", driver.TablesHandler)
	mux.HandleFunc("/api/counters", driver.ReadCountersHandler)
	mux.HandleFunc("/api/p4/program", driver.DeployProgramHandler)
	mux.HandleFunc("/api/p4/verify", driver.VerifyProgramHandler)
	mux.HandleFunc("/api/metrics", driver.MetricsHandler)
	mux.HandleFunc("/api/registers", driver.ReadRegistersHandler)

	log.Info("Starting HTTP server", "listenAddr", listenAddr)

	httpServer := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	return &Server{
		Log:        log,
		httpServer: httpServer,
	}
}

func (s *Server) Start(ctx context.Context) error {
	idleConnsClosed := make(chan struct{})
	go func() {
		// Wait until the context is canceled
		<-ctx.Done()

		// We received an interrupt signal, shut down.
		ctx, cancel := context.WithTimeout(context.Background(), ServerTeardownTimeout)
		if err := s.httpServer.Shutdown(ctx); err != nil {
			// Error from closing listeners, or context timeout:
			s.Log.Error(err, "HTTP server Shutdown failed")
		}
		cancel()
		close(idleConnsClosed)
	}()

	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.Log.Error(err, "Encountered error while running HTTP server")
		return err
	}
	<-idleConnsClosed
	return nil
}
