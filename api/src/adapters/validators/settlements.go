package validators

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/titouanfreville/copro-manager/api/src/adapters/validators/rules"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// allowedCurrencies is intentionally narrow: balance math is straight
// integer subtraction across the ledger, so a USD settlement against a
// EUR balance gives wrong arithmetic. Restrict until the app gains
// real multi-currency handling.
var allowedCurrencies = []string{"", "EUR"}

// Settlements validates settlement mutations. Owns every store needed
// for cross-resource checks (foyers, expenses, settlements,
// copro singleton) so the usecase only sees a single Validate gate.
type Settlements struct {
	settlements interfaces.SettlementsStore
	expenses    interfaces.ExpensesStore
	foyers      interfaces.FoyersStore
	copros      interfaces.CoprosStore
}

// NewSettlements builds the validator with all required deps.
func NewSettlements(
	settlements interfaces.SettlementsStore,
	expenses interfaces.ExpensesStore,
	foyers interfaces.FoyersStore,
	copros interfaces.CoprosStore,
) interfaces.SettlementValidator {
	return &Settlements{
		settlements: settlements,
		expenses:    expenses,
		foyers:      foyers,
		copros:      copros,
	}
}

// Validate runs the full create/update pre-condition gate. `selfID`
// is the settlement-being-edited (empty for Create); used to exempt
// pre-existing self-links from the "already linked" conflict.
func (v *Settlements) Validate(ctx context.Context, d entities.SettlementDraft, selfID string) error {
	if err := v.pureRules(d); err != nil {
		return err
	}
	if err := v.checkFoyers(ctx, d); err != nil {
		return err
	}
	return v.checkExpenseLinks(ctx, d.ExpenseIDs, selfID)
}

func (v *Settlements) pureRules(d entities.SettlementDraft) error {
	currency := strings.ToUpper(strings.TrimSpace(d.Currency))
	return rules.First(
		rules.IntAtLeast("amount_cents", d.AmountCents, 1),
		rules.NonBlank("from_foyer_id", d.FromFoyerID),
		rules.NonBlank("to_foyer_id", d.ToFoyerID),
		distinctFoyers(d.FromFoyerID, d.ToFoyerID),
		dateNonZero("date", d.Date),
		rules.OneOf("currency", currency, allowedCurrencies),
	)
}

// checkFoyers verifies both supplied IDs are actual foyer documents
// for this copro. The pure-rules guard already caught empty values
// and self-transfers; here we just confirm the IDs aren't typos.
func (v *Settlements) checkFoyers(ctx context.Context, d entities.SettlementDraft) error {
	rdc, err := v.foyers.FindByFloor(ctx, entities.FoyerFloorRDC)
	if err != nil {
		return fmt.Errorf("find rdc: %w", err)
	}
	premier, err := v.foyers.FindByFloor(ctx, entities.FoyerFloor1er)
	if err != nil {
		return fmt.Errorf("find 1er: %w", err)
	}
	if rdc == nil || premier == nil {
		return entities.ValidationError{Key: "foyer_id", Message: "both foyers must exist"}
	}
	for _, id := range []string{d.FromFoyerID, d.ToFoyerID} {
		if id != rdc.ID && id != premier.ID {
			return entities.ValidationError{Key: "foyer_id", Message: "not a foyer of this copro"}
		}
	}
	return nil
}

// checkExpenseLinks verifies each linked expense exists in the same
// copro and isn't already audit-linked to a different settlement.
// Bounded by entities.SettlementMaxExpenseLinks so a pathological
// request can't burn Firestore reads.
func (v *Settlements) checkExpenseLinks(ctx context.Context, expenseIDs []string, selfID string) error {
	if len(expenseIDs) == 0 {
		return nil
	}
	if len(expenseIDs) > entities.SettlementMaxExpenseLinks {
		return entities.ValidationError{
			Key:     "expense_ids",
			Message: fmt.Sprintf("too many linked expenses (max %d)", entities.SettlementMaxExpenseLinks),
		}
	}
	copro, err := v.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return fmt.Errorf("copro lookup: %w", err)
	}
	for _, id := range expenseIDs {
		if strings.TrimSpace(id) == "" {
			return entities.ValidationError{Key: "expense_ids", Message: "blank entry"}
		}
		if err := v.checkSingleExpenseLink(ctx, id, selfID, copro.ID); err != nil {
			return err
		}
	}
	return nil
}

func (v *Settlements) checkSingleExpenseLink(ctx context.Context, expenseID, selfID, coproID string) error {
	exp, err := v.expenses.FindByID(ctx, expenseID)
	if err != nil {
		return fmt.Errorf("expense lookup: %w", err)
	}
	if exp == nil {
		return entities.ValidationError{Key: "expense_ids", Message: fmt.Sprintf("expense %q not found", expenseID)}
	}
	if exp.CoproID != "" && exp.CoproID != coproID {
		return entities.ValidationError{
			Key:     "expense_ids",
			Message: fmt.Sprintf("expense %q does not belong to this copro", expenseID),
		}
	}
	conflict, err := v.settlements.FindByExpenseID(ctx, expenseID)
	if err != nil {
		return fmt.Errorf("settlement link lookup: %w", err)
	}
	if conflict != nil && conflict.ID != selfID {
		return entities.ValidationError{
			Key:     "expense_ids",
			Message: fmt.Sprintf("expense %q is already linked to settlement %q", expenseID, conflict.ID),
		}
	}
	return nil
}

// distinctFoyers fails when from == to (and both are set). Inline
// rule because it needs to see two fields, which the rules library's
// per-field shape doesn't model cleanly.
func distinctFoyers(from, to string) rules.Rule {
	return func() error {
		if from != "" && to != "" && from == to {
			return entities.ValidationError{Key: "to_foyer_id", Message: "must differ from from_foyer_id"}
		}
		return nil
	}
}

func dateNonZero(field string, t time.Time) rules.Rule {
	return func() error {
		if t.IsZero() {
			return entities.ValidationError{Key: field, Message: "required"}
		}
		return nil
	}
}
