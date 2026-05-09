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
	// DistributionModeWater3Meters splits a water bill using the three
	// detail submeter deltas of the chosen MeterReadingPeriod against its
	// immediate prior period:
	//   share_rdc = (Δrdc + Δcommon/2) / total × amount
	//   share_1er = amount − share_rdc
	// The expense carries `MeterReadingPeriod`; the usecase pulls both the
	// current and the prior reading at compute time.
	DistributionModeWater3Meters DistributionMode = "water_3_meters"
)

// AllDistributionModes lists every supported mode in display order.
func AllDistributionModes() []DistributionMode {
	return []DistributionMode{
		DistributionModeEqual,
		DistributionModeTantiemes,
		DistributionModeCustom,
		DistributionModeWater3Meters,
	}
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
	// TemplateID is the ID of the template that minted this row, when
	// applicable — empty for hand-typed expenses. Used to surface a
	// "modèle" badge in the ledger and to keep generated rows traceable.
	TemplateID string `json:"template_id,omitempty"`
	// AmountPending marks a row whose amount has not yet been filled in
	// (typically a scheduled cron-created row for an utility bill that
	// hasn't arrived). Pending rows are excluded from the running balance,
	// allow AmountCents == 0, and surface a "Montant à compléter" CTA.
	// Cleared automatically when the user submits an Update with a valid
	// (>0) amount.
	AmountPending bool `json:"amount_pending,omitempty"`
	// MeterReadingPeriod (YYYY-MM) is set on `water_3_meters` expenses to
	// pin the bill to a specific reading period; computeShares uses this
	// to load the current + prior MeterReading and run the formula. Empty
	// for every other distribution mode.
	MeterReadingPeriod string    `json:"meter_reading_period,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	// Attachments live in the subcollection `expenses/{id}/attachments/{aid}`.
	// They are loaded on demand via the AttachmentsStore (not embedded on the
	// expense doc) so the cap stays atomic and the ledger row stays compact.
	// This field exists only on the wire to surface attachments to the
	// foyer-facing onSnapshot — populated by the adapter, never persisted.
	Attachments []Attachment `json:"attachments,omitempty" firestore:"-"`
}
