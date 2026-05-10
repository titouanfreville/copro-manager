package routes

import (
	fs "cloud.google.com/go/firestore"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/usecases"
)

// Endpoints holds all route handler dependencies. The Firestore client
// is injected here only for the handful of admin-level data-fix
// endpoints (copro consolidation, future re-link tooling) that span
// multiple collections without going through a usecase. Domain
// endpoints route through `usecases` exclusively.
type Endpoints struct {
	logger    *zap.Logger
	usecases  *usecases.Usecases
	firestore *fs.Client
}

// NewEndpoints creates a new Endpoints instance.
func NewEndpoints(logger *zap.Logger, uc *usecases.Usecases, fsClient *fs.Client) *Endpoints {
	return &Endpoints{
		logger:    logger.Named("HTTP"),
		usecases:  uc,
		firestore: fsClient,
	}
}
