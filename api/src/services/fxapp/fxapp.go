package fxapp

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

var (
	StartedAt    time.Time
	ShuttingDown atomic.Bool
)

// Start starts the application with the given timeout.
func Start(app *fx.App, timeout time.Duration) {
	logger := zap.L().Named("Lifecycle")
	logger.Info("Starting app...")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		if graph, errGraph := fx.VisualizeError(err); errGraph == nil {
			logger.Info("Error graph", zap.String("dot", graph))
		}

		logger.Fatal("Unable to start app", zap.Error(err))
	}

	StartedAt = time.Now()
	logger.Info("App started")
}

// Shutdown gracefully shuts down the application with the given timeout.
func Shutdown(app *fx.App, timeout time.Duration) {
	ShuttingDown.Store(true)

	logger := zap.L().Named("Lifecycle")
	logger.Info("Stopping app...", zap.Duration("uptime", time.Since(StartedAt)))

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		s := <-c
		logger.Info("Termination signal received", zap.String("signal", s.String()))
		os.Exit(1)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := app.Stop(ctx); err != nil {
		logger.Fatal("Unable to cleanly stop app", zap.Error(err))
	}

	logger.Info("Shut down")
}
