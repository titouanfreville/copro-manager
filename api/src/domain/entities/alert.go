package entities

import (
	"fmt"
	"time"
)

// AlertKind enumerates the four alert types the app fires.
type AlertKind string

const (
	// AlertKindPendingCompletion fires when a scheduled template
	// materializes a row with amount_pending=true. Recipient: payer foyer.
	AlertKindPendingCompletion AlertKind = "pending_completion"

	// AlertKindMissingReceipt fires on an escalating cadence (D+3, D+10,
	// then every 15 days) for any non-settled, non-pending expense whose
	// attachment list is empty. Recipient: payer foyer.
	AlertKindMissingReceipt AlertKind = "missing_receipt"

	// AlertKindPeerExpenseAdded fires once per new expense, alerting the
	// foyer that didn't author it. Update events do NOT re-fire.
	AlertKindPeerExpenseAdded AlertKind = "peer_expense_added"

	// AlertKindBalanceSeasonal fires on Jul 15 + Dec 15 if the running
	// balance is non-zero. Recipient: both foyers.
	AlertKindBalanceSeasonal AlertKind = "balance_seasonal"
)

// IsKnownAlertKind reports whether the value is one of the supported kinds.
func IsKnownAlertKind(k AlertKind) bool {
	switch k {
	case AlertKindPendingCompletion, AlertKindMissingReceipt,
		AlertKindPeerExpenseAdded, AlertKindBalanceSeasonal:
		return true
	}
	return false
}

// Alert is one item in a foyer's notification feed. The same Alert is
// shared by both members of the recipient foyer (per-foyer read state, no
// per-user fan-out).
//
// `DedupeKey` is the idempotency key — the store rejects a Create when a
// row with the same (CoproID, DedupeKey) already exists, so re-running
// the daily scan or replaying a domain event is harmless.
//
// `Payload` carries kind-specific data the UI needs to render the card
// and the deep-link target. Shapes:
//
//   - pending_completion: { "expense_id": "…", "expense_name": "…", "payer_foyer_id": "…" }
//   - missing_receipt:    { "expense_id": "…", "expense_name": "…", "stage": "d3"|"d10"|"w15-N", "amount_cents": N }
//   - peer_expense_added: { "expense_id": "…", "expense_name": "…", "amount_cents": N, "author_foyer_id": "…" }
//   - balance_seasonal:   { "year": 2026, "half": "h1"|"h2", "net_cents": N, "owed_by": "…", "owed_to": "…" }
type Alert struct {
	ID               string         `json:"id"`
	CoproID          string         `json:"copro_id"`
	Kind             AlertKind      `json:"kind"`
	RecipientFoyerID string         `json:"recipient_foyer_id"`
	DedupeKey        string         `json:"dedupe_key"`
	Payload          map[string]any `json:"payload,omitempty"`
	DeepLink         string         `json:"deep_link,omitempty"`
	FiredAt          time.Time      `json:"fired_at"`
	ReadAt           *time.Time     `json:"read_at,omitempty"`
	ResolvedAt       *time.Time     `json:"resolved_at,omitempty"`
	DismissedAt      *time.Time     `json:"dismissed_at,omitempty"`
}

// DedupeKeyPendingCompletion: one alert per pending expense ever.
func DedupeKeyPendingCompletion(expenseID string) string {
	return fmt.Sprintf("%s:%s", AlertKindPendingCompletion, expenseID)
}

// DedupeKeyMissingReceipt: one alert per (expense, stage). Stages are
// d3, d10, w15-1, w15-2, … so each step in the cadence creates a fresh
// row instead of mutating an existing one (preserves history).
func DedupeKeyMissingReceipt(expenseID, stage string) string {
	return fmt.Sprintf("%s:%s:%s", AlertKindMissingReceipt, expenseID, stage)
}

// DedupeKeyMissingReceiptPrefix is used by ResolveByPrefix when an
// attachment is recorded — clears every stage at once.
func DedupeKeyMissingReceiptPrefix(expenseID string) string {
	return fmt.Sprintf("%s:%s:", AlertKindMissingReceipt, expenseID)
}

// DedupeKeyPeerExpenseAdded: one alert per new expense.
func DedupeKeyPeerExpenseAdded(expenseID string) string {
	return fmt.Sprintf("%s:%s", AlertKindPeerExpenseAdded, expenseID)
}

// DedupeKeyBalanceSeasonal: one alert per (year, half-year). Half-year
// is "h1" for Jul 15, "h2" for Dec 15.
func DedupeKeyBalanceSeasonal(year int, half string) string {
	return fmt.Sprintf("%s:%d-%s", AlertKindBalanceSeasonal, year, half)
}

// MissingReceiptStage computes the stage label for an expense aged
// `daysSinceCreated` days. Returns "" when the expense is too young
// (< 3 days) or when the cadence wouldn't fire on this exact day.
//
// Cadence:
//   - day 3:    "d3"
//   - day 10:   "d10"
//   - day 25:   "w15-1"  (10 + 15)
//   - day 40:   "w15-2"
//   - day 55:   "w15-3"  …
//
// Indefinite — every 15 days forever once the W+15 window starts (per
// the user-locked decision, no cap).
func MissingReceiptStage(daysSinceCreated int) string {
	switch daysSinceCreated {
	case 3:
		return "d3"
	case 10:
		return "d10"
	}
	if daysSinceCreated <= 10 {
		return ""
	}
	// First W+15 stage lands on day 25, then every 15 days.
	if (daysSinceCreated-10)%15 == 0 {
		n := (daysSinceCreated - 10) / 15
		return fmt.Sprintf("w15-%d", n)
	}
	return ""
}
