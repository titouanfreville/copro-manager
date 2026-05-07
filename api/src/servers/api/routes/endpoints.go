package routes

import (
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/usecases"
)

// Endpoints holds all route handler dependencies.
type Endpoints struct {
	logger   *zap.Logger
	usecases *usecases.Usecases
}

// NewEndpoints creates a new Endpoints instance.
func NewEndpoints(logger *zap.Logger, uc *usecases.Usecases) *Endpoints {
	return &Endpoints{
		logger:   logger.Named("HTTP"),
		usecases: uc,
	}
}
