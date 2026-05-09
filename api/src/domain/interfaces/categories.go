package interfaces

import (
	"context"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// CategoriesStore persists categories. Predefined categories are seeded
// at boot via EnsureSeeded; custom categories are managed by foyer
// members through Create/Update/Delete.
type CategoriesStore interface {
	List(ctx context.Context) ([]entities.Category, error)
	FindByID(ctx context.Context, id string) (*entities.Category, error)
	Create(ctx context.Context, c entities.Category) error
	Update(ctx context.Context, c entities.Category) error
	Delete(ctx context.Context, id string) error
	// EnsureSeeded creates the predefined categories that don't yet exist.
	// Idempotent — safe to call on every cold start.
	EnsureSeeded(ctx context.Context, seed []entities.Category) error
}
