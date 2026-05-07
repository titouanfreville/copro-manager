package middlewares

import (
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/services/firebase"
)

// Middlewares holds dependencies for custom middleware handlers.
type Middlewares struct {
	config Config
	logger *zap.Logger
	auth   firebase.AuthClient
}

// NewMiddlewares creates a new Middlewares instance.
func NewMiddlewares(config Config, logger *zap.Logger, auth firebase.AuthClient) *Middlewares {
	return &Middlewares{
		config: config,
		logger: logger.Named("Middleware"),
		auth:   auth,
	}
}
