package interfaces

import (
	"context"
	"time"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// TemplatesStore persists ExpenseTemplate docs. Active scheduled templates
// are queried by NextOccurrenceAt for the materialization cron.
type TemplatesStore interface {
	List(ctx context.Context) ([]entities.ExpenseTemplate, error)
	FindByID(ctx context.Context, id string) (*entities.ExpenseTemplate, error)
	Create(ctx context.Context, t entities.ExpenseTemplate) error
	Update(ctx context.Context, t entities.ExpenseTemplate) error
	Delete(ctx context.Context, id string) error

	// ListDue returns active scheduled templates whose NextOccurrenceAt is
	// on or before `cutoff` (typically end-of-today). Used by
	// MaterializeRecurring; the caller advances NextOccurrenceAt in a
	// follow-up Update call after each instance is created.
	ListDue(ctx context.Context, cutoff time.Time) ([]entities.ExpenseTemplate, error)
}
