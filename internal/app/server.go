package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const shutdownTimeout = 15 * time.Second

// RunServer starts the HTTP server and blocks until the process
// receives SIGINT/SIGTERM. On signal, it stops accepting new
// connections and drains in-flight requests (up to shutdownTimeout)
// before returning — this is what makes zero-downtime deploys possible
// later (Stage 8, item #43). Named RunServer, not Run, so it reads
// clearly at the main.go call site (app.RunServer(...)) alongside
// app.NewRouter(...) and app.New(...).
func RunServer(container *Container, handler http.Handler) error {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", container.Config.Port),
		Handler: handler,
	}

	// Run the server in a goroutine so this function can also listen
	// for the OS signal that should trigger shutdown.
	serverErr := make(chan error, 1)
	go func() {
		container.Logger.Info("starting server", "port", container.Config.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("server: listen error: %w", err)

	case sig := <-quit:
		container.Logger.Info("shutdown signal received", "signal", sig.String())

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("server: graceful shutdown failed: %w", err)
		}

		container.Logger.Info("server shut down cleanly")
		return nil
	}
}
