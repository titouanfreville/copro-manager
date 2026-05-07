package home

import (
	"context"

	"go.uber.org/zap"
)

// Usecases defines the interface for the home domain.
// All domain dependencies must have an interface for unit test mocking.
type Usecases interface {
	Hello(ctx context.Context) string
}

type usecases struct {
	logger *zap.Logger
}

// New creates a new home usecases instance.
func New(logger *zap.Logger) Usecases {
	return &usecases{logger: logger.Named("usecases.home")}
}

func (uc *usecases) Hello(ctx context.Context) string {
	log := uc.logger.With(zap.String("method", "Hello"))
	log.Info("Success")

	return "Copro manager API"
}
