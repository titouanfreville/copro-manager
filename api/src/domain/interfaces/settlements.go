package interfaces

import (
	"context"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// SettlementsStore persists explicit balance-reducing transfers between
// the two foyers. Each settlement may audit-link a set of expenses; the
// link is informational only — the store does NOT mutate those expenses
// when a settlement is created/updated/deleted.
type SettlementsStore interface {
	List(ctx context.Context) ([]entities.Settlement, error)
	FindByID(ctx context.Context, id string) (*entities.Settlement, error)

	// FindByExpenseID returns the (single) Settlement that audit-links
	// the given expense, or (nil, nil) when none exists. Used by Create
	// and Update to enforce the one-settlement-per-expense invariant.
	FindByExpenseID(ctx context.Context, expenseID string) (*entities.Settlement, error)

	Create(ctx context.Context, s entities.Settlement) error
	Update(ctx context.Context, s entities.Settlement) error
	Delete(ctx context.Context, id string) error

	// PruneExpense removes the given expenseID from every settlement's
	// `expense_ids` array. Called by the expense-delete cascade so a
	// dangling reference doesn't outlive the deleted expense.
	PruneExpense(ctx context.Context, expenseID string) error
}
