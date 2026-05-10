package interfaces

import (
	"context"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// ContractValidator runs every input validation a Contract write must
// pass — both pure-data rules (lengths, ranges, enums, date order) and
// cross-resource lookups (category exists, optional template exists).
//
// The validator owns its dependencies (CategoriesStore, TemplatesStore)
// so the contracts usecase doesn't have to know which checks need a
// store and which don't. Mutations only see ValidateCreate /
// ValidateUpdate as opaque gates — fail or pass.
type ContractValidator interface {
	ValidateCreate(ctx context.Context, draft entities.ContractDraft) error
	ValidateUpdate(ctx context.Context, draft entities.ContractDraft) error
}

// DocumentValidator runs every input rule for the document upload
// flow: pure-data checks (title length, content-type whitelist, size
// bound) and cross-resource lookups (category exists, linked expense
// exists & under cap, linked contract exists). Update has its own
// path because it only edits metadata, not the file blob.
type DocumentValidator interface {
	ValidateUpload(ctx context.Context, draft entities.DocumentDraft) error
	ValidateUpdate(ctx context.Context, draft entities.DocumentMetadataDraft) error
}

// SettlementValidator runs every input rule for a settlement
// mutation: pure-data checks (amount > 0, foyer pair, date) and
// cross-resource lookups (foyers belong to the copro, linked
// expenses exist + same copro + not double-booked).
//
// `selfID` lets Update exempt the settlement-being-edited from the
// "expense already linked" conflict check; pass empty for Create.
type SettlementValidator interface {
	Validate(ctx context.Context, draft entities.SettlementDraft, selfID string) error
}

// TemplateValidator runs every input rule for an expense-template
// mutation: pure-data checks (name, amount ≥ 0, distribution mode,
// share invariants, schedule fields) plus the all-or-none coupling
// of ScheduleActive ↔ {Frequency, DayOfMonth, StartDate, EndDate}.
type TemplateValidator interface {
	Validate(ctx context.Context, draft entities.ExpenseTemplateDraft) error
}

// ExpenseValidator runs every structural input rule for an expense
// mutation: name + amount + distribution mode + payer + category +
// date, plus the water_3_meters period requirement and the
// AmountPending↔AmountCents=0 coupling. Cross-resource checks
// (foyer pair, share math) stay in the usecase because the share
// computation needs the loaded foyers anyway.
type ExpenseValidator interface {
	Validate(ctx context.Context, draft entities.ExpenseDraft) error
}
