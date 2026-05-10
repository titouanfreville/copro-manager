package validators

import (
	"context"
	"strings"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// Expenses validates expense Create/Update inputs. Structural-only —
// the usecase still owns the foyer-pair load + share math because
// those reads are needed downstream regardless.
type Expenses struct{}

func NewExpenses() interfaces.ExpenseValidator {
	return &Expenses{}
}

// Validate aggregates every violation into a single
// entities.ValidationError so the form can highlight all bad fields
// at once.
func (v *Expenses) Validate(_ context.Context, d entities.ExpenseDraft) error {
	details := []entities.Detail{}
	details = append(details, basics(d)...)
	details = append(details, amountAndPending(d)...)
	details = append(details, waterPeriod(d)...)
	if len(details) == 0 {
		return nil
	}
	return entities.ValidationError{
		Key:     "create_expense",
		Message: "invalid input",
		Details: details,
	}
}

// basics: required strings + known distribution mode + non-zero date.
func basics(d entities.ExpenseDraft) []entities.Detail {
	out := []entities.Detail{}
	if strings.TrimSpace(d.Name) == "" {
		out = append(out, entities.Detail{Key: "name", Message: "required"})
	}
	if !entities.IsKnownDistributionMode(d.DistributionMode) {
		out = append(out, entities.Detail{Key: "distribution_mode", Message: "unknown mode"})
	}
	if strings.TrimSpace(d.PayerFoyerID) == "" {
		out = append(out, entities.Detail{Key: "payer_foyer_id", Message: "required"})
	}
	if strings.TrimSpace(d.CategoryID) == "" {
		out = append(out, entities.Detail{Key: "category_id", Message: "required"})
	}
	if d.Date.IsZero() {
		out = append(out, entities.Detail{Key: "date", Message: "required"})
	}
	return out
}

// amountAndPending enforces the AmountPending ↔ AmountCents=0
// coupling: pending rows are minted with amount = 0 (the materializer
// pattern); every other row needs a positive amount.
func amountAndPending(d entities.ExpenseDraft) []entities.Detail {
	if d.AmountPending {
		if d.AmountCents != 0 {
			return []entities.Detail{{Key: "amount_cents", Message: "must be 0 when amount_pending is true"}}
		}
		return nil
	}
	if d.AmountCents <= 0 {
		return []entities.Detail{{Key: "amount_cents", Message: "must be > 0"}}
	}
	return nil
}

// waterPeriod requires `meter_reading_period` for the water_3_meters
// mode unless the row is pending (amount comes later) or the caller
// trusts explicit shares (CSV import, handled at orchestration).
func waterPeriod(d entities.ExpenseDraft) []entities.Detail {
	if d.DistributionMode != entities.DistributionModeWater3Meters {
		return nil
	}
	if d.AmountPending {
		return nil
	}
	if !entities.IsValidMeterPeriod(d.MeterReadingPeriod) {
		return []entities.Detail{{Key: "meter_reading_period", Message: "required for water_3_meters mode (YYYY-MM)"}}
	}
	return nil
}
