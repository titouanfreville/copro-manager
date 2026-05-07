package interfaces

import (
	"context"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// CategoriesStore persists categories. The MVP only needs read + bootstrap;
// custom-category CRUD is a later story.
type CategoriesStore interface {
	List(ctx context.Context) ([]entities.Category, error)
	FindByID(ctx context.Context, id string) (*entities.Category, error)
	// EnsureSeeded creates the predefined categories that don't yet exist.
	// Idempotent — safe to call on every cold start.
	EnsureSeeded(ctx context.Context, seed []entities.Category) error
}
