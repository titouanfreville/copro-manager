package entities

import "time"

// DocumentAnalysisKind enumerates what the LLM classifier decided a
// document is, on a coarse-enough granularity to be useful for routing
// the user toward the right next action (create an expense, register
// a contract, or just keep it in the archive).
type DocumentAnalysisKind string

const (
	// DocumentKindExpense covers receipts, invoices, bills — anything
	// representing a one-off paid amount. Carries ExpenseExtraction.
	DocumentKindExpense DocumentAnalysisKind = "expense"
	// DocumentKindContract covers service agreements, insurance policies,
	// syndic mandates — anything with a recurring or long-term relationship.
	// Carries ContractExtraction.
	DocumentKindContract DocumentAnalysisKind = "contract"
	// DocumentKindOther is the catch-all (AG minutes, technical reports,
	// random PDFs). No structured extraction; `Reason` may carry a short
	// human-readable description.
	DocumentKindOther DocumentAnalysisKind = "other"
)

// IsKnownDocumentAnalysisKind reports whether the value is one of the
// supported kinds — used at the API boundary to reject malformed
// payloads from a stale or tampered client.
func IsKnownDocumentAnalysisKind(k DocumentAnalysisKind) bool {
	switch k {
	case DocumentKindExpense, DocumentKindContract, DocumentKindOther:
		return true
	}
	return false
}

// DocumentAnalysis is the cached LLM verdict on a document. Stored on
// the Document entity itself (single Firestore doc, no separate
// collection) so reading a doc + its analysis is one round-trip.
//
// Confidence is the model's self-assessment of overall reliability
// across both classification and extraction. The UI uses it to decide
// whether to pre-fill aggressively or just suggest.
type DocumentAnalysis struct {
	Kind       DocumentAnalysisKind `json:"kind"`
	Confidence float64              `json:"confidence"`
	AnalyzedAt time.Time            `json:"analyzed_at"`
	// Model records which Gemini variant produced this verdict —
	// informational only (debug logs, UI surface). The cache check is
	// `Analysis != nil && !force`; model upgrades don't auto-invalidate.
	// Operators trigger re-analysis via ?force=true on the route when
	// needed.
	Model string `json:"model"`
	// Expense carries the structured fields when Kind == expense. Nil
	// otherwise.
	Expense *ExpenseExtraction `json:"expense,omitempty"`
	// Contract carries the structured fields when Kind == contract. Nil
	// otherwise.
	Contract *ContractExtraction `json:"contract,omitempty"`
	// Reason is a short free-text justification — populated mainly for
	// Kind == other ("AG minutes", "technical report") so the UI can
	// give the user a hint about what the doc is. Optional.
	Reason string `json:"reason,omitempty"`
}

// ExpenseExtraction is the structured payload for Kind == expense.
// Every field is optional — Gemini fills what it can read. Dates are
// ISO-8601 (YYYY-MM-DD); the usecase doesn't parse them, just hands
// them to the UI for pre-fill.
type ExpenseExtraction struct {
	AmountEUR    float64 `json:"amount_eur,omitempty"`
	Date         string  `json:"date,omitempty"`
	Vendor       string  `json:"vendor,omitempty"`
	CategoryHint string  `json:"category_hint,omitempty"`
	Description  string  `json:"description,omitempty"`
}

// ContractExtraction is the structured payload for Kind == contract.
// Every field is optional. EndDate is ISO-8601; MonthlyAmountEUR is
// the recurring charge in EUR (annual contracts get monthly / 12).
type ContractExtraction struct {
	Provider         string  `json:"provider,omitempty"`
	ContractType     string  `json:"contract_type,omitempty"`
	StartDate        string  `json:"start_date,omitempty"`
	EndDate          string  `json:"end_date,omitempty"`
	MonthlyAmountEUR float64 `json:"monthly_amount_eur,omitempty"`
	ContractNumber   string  `json:"contract_number,omitempty"`
}
