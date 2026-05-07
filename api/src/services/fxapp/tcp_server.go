package fxapp

import (
	"context"

	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/servers"
)

type server struct {
	lifecycle  fx.Lifecycle
	shutdowner fx.Shutdowner
	logger     *zap.Logger
}

// NewTCPServer creates a new FX-managed TCP server lifecycle handler.
func NewTCPServer(lifecycle fx.Lifecycle, shutdowner fx.Shutdowner, logger *zap.Logger) *server {
	return &server{lifecycle: lifecycle, shutdowner: shutdowner, logger: logger}
}

// Run registers the TCP transport in the FX lifecycle.
func (s server) Run(name string, transport servers.TCP) {
	logger := s.logger.
		Named("Lifecycle").
		With(zap.String("address", transport.GetAddress()))

	s.lifecycle.Append(
		fx.Hook{
			OnStart: func(context.Context) error {
				go func() {
					if err := transport.ListenAndServe(); err != nil {
						if !ShuttingDown.Load() {
							logger.Error(name+" closed unexpectedly", zap.Error(err))
						}

						if err = s.shutdowner.Shutdown(); err != nil {
							logger.Error("Unable to shutdown properly "+name, zap.Error(err))
						}
					}
				}()

				logger.Info(name + " started")

				return nil
			},

			OnStop: func(context.Context) error {
				if err := transport.Shutdown(); err != nil {
					return err
				}

				logger.Info(name + " closed")

				return nil
			},
		},
	)
}
