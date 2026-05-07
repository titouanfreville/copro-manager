// Package categories exposes the read path for category metadata. The
// foyer-facing UI reads the full list straight from Firestore (auth-gated
// by the rules); the API only needs to resolve a single category by id
// during the expense-creation flow.
package categories

import (
	"context"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// Usecases is the categories domain contract. The MVP only exposes lookup
// because the foyer-facing app reads the full list straight from Firestore;
// only the expense-creation flow needs to resolve a category server-side.
type Usecases interface {
	FindByID(ctx context.Context, id string) (*entities.Category, error)
}

type usecases struct {
	logger *zap.Logger
	store  interfaces.CategoriesStore
}

// New builds a categories usecase.
func New(logger *zap.Logger, store interfaces.CategoriesStore) Usecases {
	return &usecases{
		logger: logger.Named("usecases.categories"),
		store:  store,
	}
}

func (uc *usecases) FindByID(ctx context.Context, id string) (*entities.Category, error) {
	return uc.store.FindByID(ctx, id)
}

// EnsureSeeded provisions the predefined categories. Called once at app boot
// from bin/app/app.go via fx.Invoke; idempotent.
func EnsureSeeded(ctx context.Context, store interfaces.CategoriesStore) error {
	return store.EnsureSeeded(ctx, entities.PredefinedCategories)
}
