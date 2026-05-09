package interfaces

import (
	"context"
	"time"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// AlertsStore persists Alert docs with idempotency on (copro_id, dedupe_key).
type AlertsStore interface {
	// CreateIfNew writes the alert only when no row with the same
	// (copro_id, dedupe_key) exists. Returns the existing row when a
	// duplicate is found (along with `false`); the caller can decide
	// whether to no-op or re-resolve a stale entry.
	CreateIfNew(ctx context.Context, a entities.Alert) (existing *entities.Alert, created bool, err error)

	FindByID(ctx context.Context, id string) (*entities.Alert, error)

	// FindByDedupeKey returns the alert with the given dedupe key, or
	// (nil, nil) when none exists. Used by Resolve hooks.
	FindByDedupeKey(ctx context.Context, coproID, dedupeKey string) (*entities.Alert, error)

	// ListByFoyer returns alerts for the given foyer, sorted by fired_at
	// desc. Default excludes dismissed; the parameter overrides.
	ListByFoyer(ctx context.Context, foyerID string, includeDismissed bool) ([]entities.Alert, error)

	// Update overwrites the entire doc. Prefer MarkRead/MarkDismissed
	// for single-field state changes — Update is multi-device race prone.
	Update(ctx context.Context, a entities.Alert) error

	// MarkRead writes only `read_at`, leaving every other field intact.
	// Multi-device safe.
	MarkRead(ctx context.Context, id string, when time.Time) error

	// MarkDismissed writes `dismissed_at` (and `read_at`, since dismissed
	// implies read). Multi-device safe.
	MarkDismissed(ctx context.Context, id string, when time.Time) error

	// ResolveByPrefix marks every non-resolved alert whose dedupe_key
	// starts with the given prefix as resolved. Used by the auto-resolve
	// hooks (e.g. attachment recorded → resolve every missing_receipt
	// stage for that expense). An empty prefix is rejected (would match
	// every alert).
	ResolveByPrefix(ctx context.Context, coproID, prefix string, resolvedAt entities.Alert) error

	// ResolveByDedupeKey marks the single alert with the exact dedupe
	// key as resolved. No-op when not found or already resolved.
	ResolveByDedupeKey(ctx context.Context, coproID, dedupeKey string, resolvedAt entities.Alert) error

	// CountUnresolvedByExpense is used by the Delete cascade so we can
	// log how many alerts will be cleaned up. Optional helper.
	CountUnresolvedByExpense(ctx context.Context, coproID, expenseID string) (int, error)

	// ResolveByExpense marks every alert whose payload references the
	// given expense_id as resolved. Called from expenses.Delete.
	ResolveByExpense(ctx context.Context, coproID, expenseID string, resolvedAt entities.Alert) error
}
