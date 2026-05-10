package interfaces

import (
	"context"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// DocumentsStore persists standalone Document entities. Per-expense
// attachments are NOT served here — they live in the expense
// subcollection (see AttachmentsStore).
type DocumentsStore interface {
	List(ctx context.Context) ([]entities.Document, error)
	FindByID(ctx context.Context, id string) (*entities.Document, error)
	Create(ctx context.Context, d entities.Document) error
	Update(ctx context.Context, d entities.Document) error
	Delete(ctx context.Context, id string) error

	// CountByCategory returns the number of documents referencing the
	// given category. Used by the categories-delete cascade-rejection
	// check (PRD FR12 — a category can't be deleted if any expense,
	// settlement, template, or document references it).
	CountByCategory(ctx context.Context, categoryID string) (int, error)

	// CountByLinkedExpense returns the number of documents linked to the
	// given expense. Used by the per-expense cap (≤10) on the unified
	// attachment flow that now writes Documents with linked_expense_id.
	CountByLinkedExpense(ctx context.Context, expenseID string) (int, error)

	// ListByLinkedExpense returns every document linked to the given
	// expense, ordered by uploaded_at asc. Powers the migration check
	// (skip-if-already-migrated) and the per-expense download path.
	ListByLinkedExpense(ctx context.Context, expenseID string) ([]entities.Document, error)
}
