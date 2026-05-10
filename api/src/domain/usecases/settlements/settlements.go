// Package settlements owns the explicit balance-reducing transfer
// layer. A Settlement records `Foyer A paid Foyer B €N on date D`,
// optionally audit-linking the expenses considered "covered" by that
// transfer. The link does NOT mutate Expense.Settled — balance math
// is straight subtraction of `AmountCents` (see app/src/lib/balance.ts).
//
// Validation lives in adapters/validators/settlements.go; entity
// construction lives in build.go; the seasonal-alert cascade lives
// in seasonal.go. This file is pure orchestration.
package settlements

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/core/authz"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// CreateInput is the route-layer DTO. The actor UID rides alongside
// the SettlementDraft so the validator only sees draft data.
type CreateInput struct {
	ActorUserID string
	entities.SettlementDraft
}

// UpdateInput mirrors CreateInput so adding update-only fields later
// stays a one-line change.
type UpdateInput = CreateInput

// Usecases is the settlements domain contract.
type Usecases interface {
	List(ctx context.Context, actorUserID string) ([]entities.Settlement, error)
	Create(ctx context.Context, in CreateInput) (*entities.Settlement, error)
	Update(ctx context.Context, id string, in UpdateInput) (*entities.Settlement, error)
	Delete(ctx context.Context, id, actorUserID string) error
	// PruneExpense forwards to the store. Called by the expenses-delete
	// cascade so dangling references in `expense_ids` arrays don't
	// outlive the deleted expense.
	PruneExpense(ctx context.Context, expenseID string) error
}

// AlertsHook is the narrow contract this package needs from the
// alerts usecase: clear seasonal alerts when the live balance hits
// zero post-mutation.
type AlertsHook interface {
	ResolveSeasonalAll(ctx context.Context) error
}

type usecases struct {
	logger      *zap.Logger
	settlements interfaces.SettlementsStore
	foyers      interfaces.FoyersStore
	validator   interfaces.SettlementValidator
	builder     *builder
	resolver    *seasonalResolver
}

// New builds a settlements usecase. `alerts` may be nil during
// tests — the seasonal cascade hook is guarded.
func New(
	logger *zap.Logger,
	settlements interfaces.SettlementsStore,
	expenses interfaces.ExpensesStore,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
	validator interfaces.SettlementValidator,
	alerts AlertsHook,
) Usecases {
	now := time.Now
	return &usecases{
		logger:      logger.Named("usecases.settlements"),
		settlements: settlements,
		foyers:      foyers,
		validator:   validator,
		builder:     newBuilder(copros, now),
		resolver:    newSeasonalResolver(logger, expenses, settlements, foyers, alerts),
	}
}

// List returns every settlement in the copro. Foyer-membership gated.
func (uc *usecases) List(ctx context.Context, actorUserID string) ([]entities.Settlement, error) {
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	return uc.settlements.List(ctx)
}

// Create validates the draft, builds a fresh Settlement, persists,
// then triggers the seasonal-alert cascade.
func (uc *usecases) Create(ctx context.Context, in CreateInput) (*entities.Settlement, error) {
	// NFR16: don't bind amount_cents to the parent log at INFO+.
	log := uc.logger.With(zap.String("method", "Create"))

	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}
	if err := uc.validator.Validate(ctx, in.SettlementDraft, ""); err != nil {
		return nil, err
	}
	s, err := uc.builder.build(ctx, in.SettlementDraft)
	if err != nil {
		return nil, fmt.Errorf("build settlement: %w", err)
	}
	if err := uc.settlements.Create(ctx, s); err != nil {
		log.Error("store create failed", zap.Error(err))
		return nil, fmt.Errorf("create settlement: %w", err)
	}
	uc.resolver.resolveIfZero(ctx)
	log.Info("Success", zap.String("settlement_id", s.ID))
	return &s, nil
}

// Update replaces the draft fields on the existing settlement and
// re-runs the seasonal cascade.
func (uc *usecases) Update(ctx context.Context, id string, in UpdateInput) (*entities.Settlement, error) {
	log := uc.logger.With(zap.String("method", "Update"), zap.String("settlement_id", id))

	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		return nil, err
	}
	existing, err := uc.settlements.FindByID(ctx, id)
	if err != nil {
		log.Error("lookup failed", zap.Error(err))
		return nil, fmt.Errorf("find settlement: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("%w: settlement %q", domainerrors.ErrNotFound, id)
	}
	if err := uc.validator.Validate(ctx, in.SettlementDraft, id); err != nil {
		return nil, err
	}
	updated := uc.builder.rebuild(*existing, in.SettlementDraft)
	if err := uc.settlements.Update(ctx, updated); err != nil {
		log.Error("update failed", zap.Error(err))
		return nil, fmt.Errorf("update settlement: %w", err)
	}
	uc.resolver.resolveIfZero(ctx)
	log.Info("Success")
	return &updated, nil
}

// Delete removes the row and re-runs the seasonal cascade.
func (uc *usecases) Delete(ctx context.Context, id, actorUserID string) error {
	log := uc.logger.With(zap.String("method", "Delete"), zap.String("settlement_id", id))

	if err := uc.authorize(ctx, actorUserID); err != nil {
		return err
	}
	existing, err := uc.settlements.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("find settlement: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("%w: settlement %q", domainerrors.ErrNotFound, id)
	}
	if err := uc.settlements.Delete(ctx, id); err != nil {
		log.Error("delete failed", zap.Error(err))
		return fmt.Errorf("delete settlement: %w", err)
	}
	uc.resolver.resolveIfZero(ctx)
	log.Info("Success")
	return nil
}

// PruneExpense forwards to the store. Called by the expenses-delete
// cascade when an expense disappears so any settlement audit-linking
// it drops the dangling reference.
func (uc *usecases) PruneExpense(ctx context.Context, expenseID string) error {
	return uc.settlements.PruneExpense(ctx, expenseID)
}

func (uc *usecases) authorize(ctx context.Context, actorUserID string) error {
	return authz.RequireFoyerMember(ctx, uc.foyers, actorUserID)
}
