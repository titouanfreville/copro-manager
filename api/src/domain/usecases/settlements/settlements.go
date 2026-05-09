// Package settlements owns the explicit balance-reducing transfer logic.
// A Settlement records `Foyer A paid Foyer B €N on date D`, optionally
// audit-linking the expenses considered "covered" by that transfer. The
// link does NOT mutate Expense.Settled — balance math is straight
// subtraction of the settlement's AmountCents (see app/src/lib/balance.ts).
package settlements

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// CreateInput captures the user-facing fields for both Create and Update.
type CreateInput struct {
	ActorUserID string
	FromFoyerID string
	ToFoyerID   string
	AmountCents int
	Currency    string
	Date        time.Time
	Note        string
	// ExpenseIDs are the expenses the user wants to audit-link to this
	// settlement. Empty for free-form clean-up settlements. Each must be a
	// real expense in the same copro and not already linked to another
	// settlement (one-settlement-per-expense max — collision returns a
	// ValidationError surfacing the conflicting settlement ID so the UI
	// can guide the user).
	ExpenseIDs []string
}

// Usecases is the settlements domain contract.
type Usecases interface {
	List(ctx context.Context, actorUserID string) ([]entities.Settlement, error)
	Create(ctx context.Context, in CreateInput) (*entities.Settlement, error)
	Update(ctx context.Context, id string, in CreateInput) (*entities.Settlement, error)
	Delete(ctx context.Context, id, actorUserID string) error

	// PruneExpense forwards to the store. Called by the expenses-delete
	// cascade so dangling references in `expense_ids` arrays don't outlive
	// the deleted expense.
	PruneExpense(ctx context.Context, expenseID string) error
}

// AlertsHook is the narrow contract this package needs from the alerts
// usecase: settlements clear seasonal balance alerts when the live
// balance returns to zero.
type AlertsHook interface {
	ResolveSeasonalAll(ctx context.Context) error
}

type usecases struct {
	logger      *zap.Logger
	settlements interfaces.SettlementsStore
	expenses    interfaces.ExpensesStore
	foyers      interfaces.FoyersStore
	copros      interfaces.CoprosStore
	alerts      AlertsHook
	now         func() time.Time
}

// New builds a settlements usecase. `alerts` may be nil during tests —
// every hook call is guarded.
func New(
	logger *zap.Logger,
	settlements interfaces.SettlementsStore,
	expenses interfaces.ExpensesStore,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
	alerts AlertsHook,
) Usecases {
	return &usecases{
		logger:      logger.Named("usecases.settlements"),
		settlements: settlements,
		expenses:    expenses,
		foyers:      foyers,
		copros:      copros,
		alerts:      alerts,
		now:         time.Now,
	}
}

// resolveSeasonalIfZero recomputes the live balance (mirroring the cron
// formula) and, when it lands at exactly zero, clears any non-resolved
// balance_seasonal alerts. Best-effort — never blocks the settlement
// mutation that triggered it.
func (uc *usecases) resolveSeasonalIfZero(ctx context.Context) {
	if uc.alerts == nil {
		return
	}
	expenses, err := uc.expenses.List(ctx)
	if err != nil {
		uc.logger.Warn("seasonal-resolve: expense list failed", zap.Error(err))
		return
	}
	settlements, err := uc.settlements.List(ctx)
	if err != nil {
		uc.logger.Warn("seasonal-resolve: settlement list failed", zap.Error(err))
		return
	}
	rdc, premier, err := uc.loadFoyers(ctx)
	if err != nil {
		uc.logger.Warn("seasonal-resolve: load foyers failed", zap.Error(err))
		return
	}
	net := 0
	for _, e := range expenses {
		if e.Settled || e.AmountPending {
			continue
		}
		switch e.PayerFoyerID {
		case rdc.ID:
			net += e.Share1erCents
		case premier.ID:
			net -= e.ShareRDCCents
		}
	}
	for _, s := range settlements {
		if s.FromFoyerID == premier.ID && s.ToFoyerID == rdc.ID {
			net -= s.AmountCents
		} else if s.FromFoyerID == rdc.ID && s.ToFoyerID == premier.ID {
			net += s.AmountCents
		}
	}
	if net != 0 {
		return
	}
	if err := uc.alerts.ResolveSeasonalAll(ctx); err != nil {
		uc.logger.Warn("seasonal-resolve failed", zap.Error(err))
	}
}

func (uc *usecases) List(ctx context.Context, actorUserID string) ([]entities.Settlement, error) {
	if err := uc.authorize(ctx, actorUserID); err != nil {
		return nil, err
	}
	return uc.settlements.List(ctx)
}

func (uc *usecases) Create(ctx context.Context, in CreateInput) (*entities.Settlement, error) {
	// Don't bind amount_cents to the parent log — NFR16 forbids
	// expense/settlement amounts at INFO+. Identify by settlement_id
	// once we have one.
	log := uc.logger.With(zap.String("method", "Create"))

	if err := uc.authorize(ctx, in.ActorUserID); err != nil {
		log.Warn("actor unauthorized", zap.Error(err))
		return nil, err
	}
	rdc, premier, err := uc.loadFoyers(ctx)
	if err != nil {
		return nil, err
	}
	if err := uc.validate(in, rdc, premier); err != nil {
		log.Warn("validation failed", zap.Error(err))
		return nil, err
	}
	if err := uc.checkExpenseLinks(ctx, in.ExpenseIDs, ""); err != nil {
		log.Warn("expense links rejected", zap.Error(err))
		return nil, err
	}

	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return nil, fmt.Errorf("copro lookup: %w", err)
	}

	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	if currency == "" {
		currency = "EUR"
	}

	now := uc.now()
	s := entities.Settlement{
		ID:          uuid.NewString(),
		CoproID:     copro.ID,
		FromFoyerID: in.FromFoyerID,
		ToFoyerID:   in.ToFoyerID,
		AmountCents: in.AmountCents,
		Currency:    currency,
		Date:        in.Date,
		Note:        strings.TrimSpace(in.Note),
		ExpenseIDs:  dedupeStrings(in.ExpenseIDs),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := uc.settlements.Create(ctx, s); err != nil {
		log.Error("store create failed", zap.Error(err))
		return nil, fmt.Errorf("create settlement: %w", err)
	}
	uc.resolveSeasonalIfZero(ctx)
	log.Info("Success", zap.String("settlement_id", s.ID))
	return &s, nil
}

func (uc *usecases) Update(ctx context.Context, id string, in CreateInput) (*entities.Settlement, error) {
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
	rdc, premier, err := uc.loadFoyers(ctx)
	if err != nil {
		return nil, err
	}
	if err := uc.validate(in, rdc, premier); err != nil {
		return nil, err
	}
	if err := uc.checkExpenseLinks(ctx, in.ExpenseIDs, id); err != nil {
		return nil, err
	}

	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	if currency == "" {
		currency = existing.Currency
	}

	now := uc.now()
	existing.FromFoyerID = in.FromFoyerID
	existing.ToFoyerID = in.ToFoyerID
	existing.AmountCents = in.AmountCents
	existing.Currency = currency
	existing.Date = in.Date
	existing.Note = strings.TrimSpace(in.Note)
	existing.ExpenseIDs = dedupeStrings(in.ExpenseIDs)
	existing.UpdatedAt = now

	if err := uc.settlements.Update(ctx, *existing); err != nil {
		log.Error("update failed", zap.Error(err))
		return nil, fmt.Errorf("update settlement: %w", err)
	}
	uc.resolveSeasonalIfZero(ctx)
	log.Info("Success")
	return existing, nil
}

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
	uc.resolveSeasonalIfZero(ctx)
	log.Info("Success")
	return nil
}

func (uc *usecases) PruneExpense(ctx context.Context, expenseID string) error {
	return uc.settlements.PruneExpense(ctx, expenseID)
}

// validate enforces the structural invariants. Cross-store checks (linked
// expenses exist + aren't double-booked) live in checkExpenseLinks.
func (uc *usecases) validate(in CreateInput, rdc, premier *entities.Foyer) error {
	details := []entities.Detail{}
	if in.AmountCents <= 0 {
		details = append(details, entities.Detail{Key: "amount_cents", Message: "must be > 0"})
	}
	if strings.TrimSpace(in.FromFoyerID) == "" {
		details = append(details, entities.Detail{Key: "from_foyer_id", Message: "required"})
	}
	if strings.TrimSpace(in.ToFoyerID) == "" {
		details = append(details, entities.Detail{Key: "to_foyer_id", Message: "required"})
	}
	if in.FromFoyerID == in.ToFoyerID && in.FromFoyerID != "" {
		details = append(details, entities.Detail{Key: "to_foyer_id", Message: "must differ from from_foyer_id"})
	}
	for _, id := range []string{in.FromFoyerID, in.ToFoyerID} {
		if id == "" {
			continue
		}
		if id != rdc.ID && id != premier.ID {
			details = append(details, entities.Detail{Key: "foyer_id", Message: "not a foyer of this copro"})
			break
		}
	}
	if in.Date.IsZero() {
		details = append(details, entities.Detail{Key: "date", Message: "required"})
	}
	// Currency allowlist: balance math is straight integer subtraction
	// (see resolveSeasonalIfZero), so a USD settlement subtracted from a
	// EUR balance gives wrong arithmetic. Restrict to EUR until the app
	// gains real multi-currency handling.
	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	if currency != "" && currency != "EUR" {
		details = append(details, entities.Detail{Key: "currency", Message: "only EUR is supported"})
	}
	if len(details) > 0 {
		return entities.ValidationError{
			Key:     "create_settlement",
			Message: "invalid input",
			Details: details,
		}
	}
	return nil
}

// checkExpenseLinks verifies each linked expense exists in the SAME copro
// and is not already linked to another settlement. `selfID` is the settlement
// being updated (so its own pre-existing links don't trip the check); pass
// empty for Create.
func (uc *usecases) checkExpenseLinks(ctx context.Context, expenseIDs []string, selfID string) error {
	if len(expenseIDs) == 0 {
		return nil
	}
	if len(expenseIDs) > settlementMaxLinks {
		return entities.ValidationError{
			Key:     "expense_ids",
			Message: fmt.Sprintf("too many linked expenses (max %d)", settlementMaxLinks),
		}
	}
	copro, err := uc.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return fmt.Errorf("copro lookup: %w", err)
	}
	for _, id := range expenseIDs {
		if id == "" {
			return entities.ValidationError{Key: "expense_ids", Message: "blank entry"}
		}
		exp, err := uc.expenses.FindByID(ctx, id)
		if err != nil {
			return fmt.Errorf("expense lookup: %w", err)
		}
		if exp == nil {
			return entities.ValidationError{Key: "expense_ids", Message: fmt.Sprintf("expense %q not found", id)}
		}
		// Cross-tenant guard: refuse to audit-link an expense that lives
		// in a different copro (single-copro today, but the check makes
		// the contract explicit).
		if exp.CoproID != "" && exp.CoproID != copro.ID {
			return entities.ValidationError{
				Key:     "expense_ids",
				Message: fmt.Sprintf("expense %q does not belong to this copro", id),
			}
		}
		conflict, err := uc.settlements.FindByExpenseID(ctx, id)
		if err != nil {
			return fmt.Errorf("settlement link lookup: %w", err)
		}
		if conflict != nil && conflict.ID != selfID {
			return entities.ValidationError{
				Key:     "expense_ids",
				Message: fmt.Sprintf("expense %q is already linked to settlement %q", id, conflict.ID),
			}
		}
	}
	return nil
}

// settlementMaxLinks bounds the number of expenses a single settlement
// can audit-link. Each link costs one Firestore read at validation
// time (FindByID) plus one (FindByExpenseID). Keep the bound small so a
// pathological request can't burn quotas.
const settlementMaxLinks = 50

func (uc *usecases) authorize(ctx context.Context, actorUserID string) error {
	if actorUserID == "" {
		return nil
	}
	rdc, premier, err := uc.loadFoyers(ctx)
	if err != nil {
		return err
	}
	for _, mid := range rdc.MemberIDs {
		if mid == actorUserID {
			return nil
		}
	}
	for _, mid := range premier.MemberIDs {
		if mid == actorUserID {
			return nil
		}
	}
	return entities.AuthorizationError{Code: "not_foyer_member"}
}

func (uc *usecases) loadFoyers(ctx context.Context) (*entities.Foyer, *entities.Foyer, error) {
	rdc, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloorRDC)
	if err != nil {
		return nil, nil, fmt.Errorf("find rdc: %w", err)
	}
	premier, err := uc.foyers.FindByFloor(ctx, entities.FoyerFloor1er)
	if err != nil {
		return nil, nil, fmt.Errorf("find 1er: %w", err)
	}
	if rdc == nil || premier == nil {
		return nil, nil, fmt.Errorf("%w: both RDC and 1er foyers must exist", domainerrors.ErrNotFound)
	}
	return rdc, premier, nil
}

// dedupeStrings preserves first-occurrence order. Idempotent on already-
// unique input.
func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
