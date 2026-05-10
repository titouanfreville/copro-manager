package validators

import (
	"context"
	"fmt"
	"strings"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// Templates validates expense-template inputs. No store deps today —
// all checks are structural. (Cross-resource checks for category /
// payer existence could land here later if we want to fail fast at
// template-create time rather than at materializer-fire time.)
type Templates struct{}

// NewTemplates returns the validator. No deps so the constructor is
// argument-less, but kept as a factory for future dependency growth.
func NewTemplates() interfaces.TemplateValidator {
	return &Templates{}
}

// Validate runs the full check chain. Aggregates every violation
// into the `Details` slice on entities.ValidationError so the
// frontend can highlight every bad field at once (templates have a
// dense form; surfacing one error at a time would frustrate the user).
func (v *Templates) Validate(_ context.Context, d entities.ExpenseTemplateDraft) error {
	details := []entities.Detail{}
	details = append(details, structural(d)...)
	details = append(details, customMode(d)...)
	if d.ScheduleActive {
		details = append(details, schedule(d)...)
	}
	if len(details) == 0 {
		return nil
	}
	return entities.ValidationError{
		Key:     "create_template",
		Message: "invalid input",
		Details: details,
	}
}

// structural covers the always-required fields.
func structural(d entities.ExpenseTemplateDraft) []entities.Detail {
	out := []entities.Detail{}
	if strings.TrimSpace(d.Name) == "" {
		out = append(out, entities.Detail{Key: "name", Message: "required"})
	}
	if d.AmountDefaultCents < 0 {
		out = append(out, entities.Detail{Key: "amount_default_cents", Message: "must be >= 0"})
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
	return out
}

// customMode enforces the share invariants for DistributionModeCustom:
// non-negative shares, sum equals amount when amount > 0.
func customMode(d entities.ExpenseTemplateDraft) []entities.Detail {
	if d.DistributionMode != entities.DistributionModeCustom {
		return nil
	}
	out := []entities.Detail{}
	if d.ShareRDCCents < 0 || d.Share1erCents < 0 {
		out = append(out, entities.Detail{Key: "shares", Message: "must be >= 0"})
	}
	if d.AmountDefaultCents > 0 && d.ShareRDCCents+d.Share1erCents != d.AmountDefaultCents {
		out = append(out, entities.Detail{
			Key: "shares",
			Message: fmt.Sprintf(
				"share_rdc + share_1er (%d) ≠ amount_default (%d)",
				d.ShareRDCCents+d.Share1erCents, d.AmountDefaultCents,
			),
		})
	}
	return out
}

// schedule enforces the all-or-none coupling of ScheduleActive with
// Frequency / DayOfMonth / StartDate / EndDate.
func schedule(d entities.ExpenseTemplateDraft) []entities.Detail {
	out := []entities.Detail{}
	if !entities.IsKnownFrequency(d.Frequency) {
		out = append(out, entities.Detail{Key: "frequency", Message: "unknown — required when schedule active"})
	}
	if d.DayOfMonth < 1 || d.DayOfMonth > 31 {
		out = append(out, entities.Detail{Key: "day_of_month", Message: "must be 1–31"})
	}
	if d.StartDate.IsZero() {
		out = append(out, entities.Detail{Key: "start_date", Message: "required when schedule active"})
	}
	// First fire is anchored at StartDate; if the user supplied a
	// separate DayOfMonth it must match — otherwise the first fire
	// and subsequent fires would land on different days. Tolerate
	// the case where DayOfMonth doesn't fit StartDate's month
	// (e.g. 31 in Feb): AdvanceDate clamps to the last valid day.
	if !d.StartDate.IsZero() && d.DayOfMonth >= 1 && d.DayOfMonth <= 31 &&
		d.StartDate.Day() != d.DayOfMonth &&
		d.DayOfMonth <= entities.LastDayOfMonth(d.StartDate.Year(), d.StartDate.Month()) {
		out = append(out, entities.Detail{
			Key: "day_of_month",
			Message: fmt.Sprintf(
				"day_of_month (%d) must match start_date.Day() (%d)",
				d.DayOfMonth, d.StartDate.Day(),
			),
		})
	}
	if d.EndDate != nil && !d.EndDate.IsZero() && d.EndDate.Before(d.StartDate) {
		out = append(out, entities.Detail{Key: "end_date", Message: "must be on or after start_date"})
	}
	return out
}

