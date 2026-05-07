package entities

import "time"

// DistributionMode is how an expense's total is split across the two foyers.
type DistributionMode string

const (
	// DistributionModeEqual splits 50/50 (rounding remainder goes to payer).
	DistributionModeEqual DistributionMode = "equal"
	// DistributionModeTantiemes splits proportionally to each foyer's Parts
	// out of Copro.TotalParts (rounding remainder goes to payer).
	DistributionModeTantiemes DistributionMode = "tantiemes"
	// DistributionModeCustom takes user-provided per-foyer amounts that must
	// sum exactly to the total.
	DistributionModeCustom DistributionMode = "custom"
)

// AllDistributionModes lists every supported mode in display order.
func AllDistributionModes() []DistributionMode {
	return []DistributionMode{DistributionModeEqual, DistributionModeTantiemes, DistributionModeCustom}
}

// IsKnownDistributionMode reports whether the value is one of the modes
// the system knows how to compute. Unknown modes are rejected at validation
// time.
func IsKnownDistributionMode(m DistributionMode) bool {
	for _, k := range AllDistributionModes() {
		if k == m {
			return true
		}
	}
	return false
}

// Expense is a single shared cost. Amounts are stored in integer cents to
// avoid float rounding issues; the API surfaces them as cents and the
// presentation layer divides by 100. ShareRDCCents + Share1erCents must
// always equal AmountCents — invariant validated at write time.
//
// Name is the human-readable label of the expense (e.g. "Eau Janvier à
// Mai 2025"). It is the upsert key together with Date — useful for the CSV
// import flow that brings the user's existing spreadsheet into the app.
//
// Three independent timestamps follow the lifecycle:
//
//   - Date         — the invoice / event date (always required).
//   - PaymentDate  — when the foyer member actually paid the supplier
//     (transferred funds out). Optional; sometimes paid
//     on the same day as Date, sometimes later.
//   - SettledAt    — when the two foyers reconciled their accounts (each
//     party's share has been balanced). Only meaningful
//     when Settled is true; nil for legacy CSV imports
//     where the alignment date is unknown.
//
// Settled marks an expense whose payments have already been balanced
// outside the app. Excluded from the running-balance computation. The
// CSV import sets it on every "Paiement complet (2 parties) = TRUE" row.
type Expense struct {
	ID               string           `json:"id"`
	CoproID          string           `json:"copro_id"`
	Name             string           `json:"name"`
	AmountCents      int              `json:"amount_cents"`
	Currency         string           `json:"currency"`
	Date             time.Time        `json:"date"`
	PaymentDate      *time.Time       `json:"payment_date,omitempty"`
	PayerFoyerID     string           `json:"payer_foyer_id"`
	CategoryID       string           `json:"category_id"`
	DistributionMode DistributionMode `json:"distribution_mode"`
	ShareRDCCents    int              `json:"share_rdc_cents"`
	Share1erCents    int              `json:"share_1er_cents"`
	Settled          bool             `json:"settled"`
	SettledAt        *time.Time       `json:"settled_at,omitempty"`
	Note             string           `json:"note,omitempty"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}
