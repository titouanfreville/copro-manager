package interfaces

import (
	"context"
	"time"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// ExpensesStore persists shared expenses. Listing returns documents ordered
// by date desc, then created_at desc as tiebreaker.
//
// Attachments are NOT stored on the expense doc — they live in the
// subcollection exposed via AttachmentsStore. The Expense entity carries
// `Attachments` only as a wire-format convenience for callers that want the
// pair atomically; the store never persists that field.
type ExpensesStore interface {
	List(ctx context.Context) ([]entities.Expense, error)
	FindByID(ctx context.Context, id string) (*entities.Expense, error)
	FindByNameAndDate(ctx context.Context, name string, date time.Time) (*entities.Expense, error)
	Create(ctx context.Context, expense entities.Expense) error
	Update(ctx context.Context, expense entities.Expense) error
	Delete(ctx context.Context, id string) error
}
