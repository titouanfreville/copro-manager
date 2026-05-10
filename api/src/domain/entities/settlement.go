package entities

import "time"

// Settlement is a recorded transfer between the two foyers that reduces
// the running balance. It is its own ledger row, never mutating any
// Expense (PRD FR40). The optional `ExpenseIDs` audit-link the expenses
// the user considers covered by this transfer; the link is informational
// — it does NOT toggle Expense.Settled. Balance math is straight subtraction
// of `AmountCents` from the expense net regardless of linkage.
type Settlement struct {
	ID          string    `json:"id"`
	CoproID     string    `json:"copro_id"`
	FromFoyerID string    `json:"from_foyer_id"`
	ToFoyerID   string    `json:"to_foyer_id"`
	AmountCents int       `json:"amount_cents"`
	Currency    string    `json:"currency"`
	Date        time.Time `json:"date"`
	Note        string    `json:"note,omitempty"`
	// ExpenseIDs are the expenses this settlement audit-links. Empty for
	// free-form "balance clean-up" settlements. The store enforces that
	// each ID is real, in-copro, and not already linked to another
	// settlement (one-settlement-per-expense max).
	ExpenseIDs []string  `json:"expense_ids,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SettlementDraft is the user-editable subset for Create/Update.
// Server-owned fields (ID, CoproID, CreatedAt, UpdatedAt) are stamped
// at build time.
type SettlementDraft struct {
	FromFoyerID string
	ToFoyerID   string
	AmountCents int
	Currency    string
	Date        time.Time
	Note        string
	ExpenseIDs  []string
}

// SettlementMaxExpenseLinks bounds how many expenses a single
// settlement can audit-link. Each link costs Firestore reads at
// validation time — keep small so a pathological request can't burn
// quotas.
const SettlementMaxExpenseLinks = 50
