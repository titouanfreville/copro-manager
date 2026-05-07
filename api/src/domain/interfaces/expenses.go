package interfaces

import (
	"context"
	"time"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// ExpensesStore persists shared expenses. Listing returns documents ordered
// by date desc, then created_at desc as tiebreaker.
type ExpensesStore interface {
	List(ctx context.Context) ([]entities.Expense, error)
	FindByNameAndDate(ctx context.Context, name string, date time.Time) (*entities.Expense, error)
	Create(ctx context.Context, expense entities.Expense) error
	Update(ctx context.Context, expense entities.Expense) error
}
