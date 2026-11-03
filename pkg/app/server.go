package app

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// StartServer starts the HTTP server in a background goroutine
func StartServer(srv *http.Server, log *zap.Logger, port string) {
	go func() {
		log.Info("server listening", zap.String("port", port))

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("failed to start server", zap.Error(err))
		}
	}()
}

// WaitForShutdown blocks until an interrupt signal is received and then performs graceful shutdown
func WaitForShutdown(srv *http.Server, log *zap.Logger, timeout time.Duration) {
	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")

	// Performs graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("server forced to shutdown", zap.Error(err))
	}

	log.Info("server stopped gracefully")
}
