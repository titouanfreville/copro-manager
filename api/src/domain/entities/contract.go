package entities

import "time"

// ContractStatus identifies where a contract sits in its lifecycle.
// `active` is the default for newly created records; the user can flip
// to `cancelled` when a contract is terminated mid-term, and the
// scanner / display layer auto-derive `expired` from the end_date.
type ContractStatus string

const (
	ContractStatusActive    ContractStatus = "active"
	ContractStatusExpired   ContractStatus = "expired"
	ContractStatusCancelled ContractStatus = "cancelled"
)

// IsKnownContractStatus reports whether the value is one of the
// supported statuses. Used by Update validation to refuse free-text
// status payloads.
func IsKnownContractStatus(s ContractStatus) bool {
	switch s {
	case ContractStatusActive, ContractStatusExpired, ContractStatusCancelled:
		return true
	}
	return false
}

// BillingFrequency captures how often a contract bills. Empty is allowed
// (one-shot or unknown). Mirrors Frequency on ExpenseTemplate but stays
// a separate type so the contract domain doesn't pull in the templates
// package.
type BillingFrequency string

const (
	BillingFrequencyMonthly   BillingFrequency = "monthly"
	BillingFrequencyQuarterly BillingFrequency = "quarterly"
	BillingFrequencyYearly    BillingFrequency = "yearly"
)

// IsKnownBillingFrequency reports whether the value is one of the
// supported cadences.
func IsKnownBillingFrequency(f BillingFrequency) bool {
	switch f {
	case BillingFrequencyMonthly, BillingFrequencyQuarterly, BillingFrequencyYearly:
		return true
	}
	return false
}

// ContractExpiringSoonDays is the threshold (in days) at which a
// contract's end_date triggers the renewal banner + the
// contract_expiring alert. Hardcoded for v1 — making it per-contract
// is post-MVP polish.
const ContractExpiringSoonDays = 30

// Society is the company providing the service (insurer, energy
// provider, syndic, …). Inline on the contract — no separate
// `societies` collection at 2-foyer scale, the duplication of
// Maaf / EDF / Foncia across a handful of contracts is cheap.
type Society struct {
	Name    string `json:"name"`
	Phone   string `json:"phone,omitempty"`
	Email   string `json:"email,omitempty"`
	Website string `json:"website,omitempty"`
	Address string `json:"address,omitempty"`
}

// Contact is the human counterpart at the society — the agent the user
// calls when something goes wrong. Inline for the same reason.
type Contact struct {
	Name  string `json:"name,omitempty"`
	Role  string `json:"role,omitempty"`
	Phone string `json:"phone,omitempty"`
	Email string `json:"email,omitempty"`
}

// Contract groups a service agreement with its provider, contact,
// billing cadence, and linked documents. Rooted at the copro level
// (both foyers see every contract) since these agreements bind the
// building, not an individual household.
//
// Documents and the optional recurring ExpenseTemplate FK back to
// the contract via their own fields (Document.LinkedContractID,
// ExpenseTemplate.ContractID) so a contract row never has to mutate
// when its dependents change.
type Contract struct {
	ID         string `json:"id"`
	CoproID    string `json:"copro_id"`
	Name       string `json:"name"`
	CategoryID string `json:"category_id"`

	Society Society `json:"society"`
	Contact Contact `json:"contact,omitempty"`

	// StartDate and EndDate are calendar dates (no time-of-day) stored
	// as ISO-8601. EndDate may be zero for open-ended contracts (CDI-
	// style: tacite reconduction without a fixed term).
	StartDate time.Time `json:"start_date,omitempty"`
	EndDate   time.Time `json:"end_date,omitempty"`

	// AmountCents + BillingFrequency are display-only metadata — the
	// authoritative billing flow remains the ExpenseTemplate
	// materializer when TemplateID is set.
	AmountCents      int              `json:"amount_cents,omitempty"`
	BillingFrequency BillingFrequency `json:"billing_frequency,omitempty"`

	// TemplateID optionally pins a recurring ExpenseTemplate that
	// generates the monthly/annual expense rows for this contract.
	// Empty when the user enters expenses manually.
	TemplateID string `json:"template_id,omitempty"`

	Status ContractStatus `json:"status"`
	Note   string         `json:"note,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ContractDraft is the user-editable subset of a Contract — what the
// validator sees and what the builder normalizes. It deliberately
// omits server-owned fields (ID, CoproID, CreatedAt, UpdatedAt).
//
// Lives at the entity layer so any package (validator, builder, route
// handler) can consume the type without depending on the contracts
// usecase package.
type ContractDraft struct {
	Name             string
	CategoryID       string
	Society          Society
	Contact          Contact
	StartDate        time.Time
	EndDate          time.Time
	AmountCents      int
	BillingFrequency BillingFrequency
	TemplateID       string
	Status           ContractStatus
	Note             string
}

// IsExpiringSoon returns true when the contract's end_date is set and
// falls within `ContractExpiringSoonDays` from `ref`. The comparison
// is date-only in `ref`'s location: end_date is parsed as UTC midnight
// at write time but represents a calendar date, so we project both
// sides onto the same TZ to avoid off-by-one drift around midnight
// Paris vs UTC.
func (c Contract) IsExpiringSoon(ref time.Time) bool {
	if c.EndDate.IsZero() {
		return false
	}
	if c.Status != ContractStatusActive {
		return false
	}
	days := DaysUntil(ref, c.EndDate)
	return days >= 0 && days <= ContractExpiringSoonDays
}

// IsExpired returns true when the contract's end_date is set and has
// passed (date-only comparison in `ref`'s location).
func (c Contract) IsExpired(ref time.Time) bool {
	if c.EndDate.IsZero() {
		return false
	}
	return DaysUntil(ref, c.EndDate) < 0
}

// DaysUntil returns the integer number of calendar days from `ref` to
// `target`. Both sides are normalized to date-only in `ref`'s location
// before subtracting, so the result is unaffected by DST or by the
// time-of-day at which the scanner runs. Same-day → 0; tomorrow → 1;
// yesterday → -1.
func DaysUntil(ref, target time.Time) int {
	loc := ref.Location()
	r := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, loc)
	t := time.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, loc)
	return int(t.Sub(r) / (24 * time.Hour))
}
