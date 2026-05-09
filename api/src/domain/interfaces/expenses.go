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

	// CountByCategory returns the number of expenses referencing the
	// given category. Consumed by the categories-delete cascade rejection
	// (PRD FR12).
	CountByCategory(ctx context.Context, categoryID string) (int, error)

	// CountByMeterReadingPeriod returns the number of expenses whose
	// `meter_reading_period` equals the given YYYY-MM. Consumed by the
	// meters-delete cascade rejection — a reading can't be removed while
	// any water_3_meters expense still references it.
	CountByMeterReadingPeriod(ctx context.Context, period string) (int, error)
}
