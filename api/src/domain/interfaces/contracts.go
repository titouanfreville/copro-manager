package interfaces

import (
	"context"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// ContractsStore persists Contract entities — long-lived service
// agreements rooted at the copro level (insurance, syndic, energy,
// maintenance, …). Two contracts with the same provider duplicate
// the inline society fields; this is acceptable at 2-foyer scale.
type ContractsStore interface {
	List(ctx context.Context) ([]entities.Contract, error)
	FindByID(ctx context.Context, id string) (*entities.Contract, error)
	Create(ctx context.Context, c entities.Contract) error
	Update(ctx context.Context, c entities.Contract) error
	Delete(ctx context.Context, id string) error

	// CountByCategory feeds the categories-delete cascade-rejection
	// check (PRD FR12) so a category in active use can't be removed.
	CountByCategory(ctx context.Context, categoryID string) (int, error)
}
