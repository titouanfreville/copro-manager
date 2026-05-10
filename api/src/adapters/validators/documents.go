package validators

import (
	"context"
	"fmt"
	"strings"

	"github.com/titouanfreville/copro-manager/api/src/adapters/validators/rules"
	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const (
	docTitleMin = 1
	docTitleMax = 200
	docDescMax  = 2000
	docGroupMax = 64
)

// Documents validates document upload + update inputs. Owns every
// store needed for the cross-resource checks (category exists, linked
// expense exists and under cap, linked contract exists) so the
// usecase only sees a single ValidateUpload / ValidateUpdate gate.
type Documents struct {
	documents  interfaces.DocumentsStore
	categories interfaces.CategoriesStore
	expenses   interfaces.ExpensesStore
	contracts  interfaces.ContractsStore
}

// NewDocuments builds the validator. Every dep is required — a wiring
// miss surfaces at boot rather than as a silent skip.
func NewDocuments(
	documents interfaces.DocumentsStore,
	categories interfaces.CategoriesStore,
	expenses interfaces.ExpensesStore,
	contracts interfaces.ContractsStore,
) interfaces.DocumentValidator {
	return &Documents{
		documents:  documents,
		categories: categories,
		expenses:   expenses,
		contracts:  contracts,
	}
}

// ValidateUpload runs the full upload-path checks. Caller is expected
// to have already applied linked-expense defaults (title/category) so
// the rules see the final draft.
func (v *Documents) ValidateUpload(ctx context.Context, d entities.DocumentDraft) error {
	if err := v.uploadPureRules(d); err != nil {
		return err
	}
	if err := v.checkCategory(ctx, d.CategoryID); err != nil {
		return err
	}
	if err := v.checkLinkedExpense(ctx, d.LinkedExpenseID); err != nil {
		return err
	}
	return v.checkLinkedContract(ctx, d.LinkedContractID)
}

// ValidateUpdate covers metadata-only edits — title, category, group,
// optional contract relink. The file blob isn't editable post-upload.
func (v *Documents) ValidateUpdate(ctx context.Context, d entities.DocumentMetadataDraft) error {
	if err := v.metadataPureRules(d); err != nil {
		return err
	}
	if err := v.checkCategory(ctx, d.CategoryID); err != nil {
		return err
	}
	return v.checkLinkedContract(ctx, d.LinkedContractID)
}

// uploadPureRules covers everything checkable without I/O.
func (v *Documents) uploadPureRules(d entities.DocumentDraft) error {
	return rules.First(
		rules.NonBlank("title", d.Title),
		rules.MinLen("title", d.Title, docTitleMin),
		rules.MaxLen("title", d.Title, docTitleMax),
		rules.MaxLen("description", d.Description, docDescMax),
		rules.MaxLen("group", d.Group, docGroupMax),
		rules.NonBlank("category_id", d.CategoryID),
		mimeAllowed("content_type", d.ContentType),
		sizeInRange("size_bytes", d.SizeBytes, entities.DocumentMaxSizeBytes),
	)
}

func (v *Documents) metadataPureRules(d entities.DocumentMetadataDraft) error {
	return rules.First(
		rules.NonBlank("title", d.Title),
		rules.MinLen("title", d.Title, docTitleMin),
		rules.MaxLen("title", d.Title, docTitleMax),
		rules.MaxLen("description", d.Description, docDescMax),
		rules.MaxLen("group", d.Group, docGroupMax),
		rules.NonBlank("category_id", d.CategoryID),
	)
}

func (v *Documents) checkCategory(ctx context.Context, categoryID string) error {
	id := strings.TrimSpace(categoryID)
	if id == "" {
		// uploadPureRules / metadataPureRules already guard NonBlank.
		return nil
	}
	cat, err := v.categories.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if cat == nil {
		return entities.ValidationError{Key: "category_id", Message: "not found"}
	}
	return nil
}

// checkLinkedExpense verifies that an expense-attach upload references
// a real expense and that the per-expense cap (10) is not yet reached.
// Empty input is a no-op (standalone document).
func (v *Documents) checkLinkedExpense(ctx context.Context, expenseID string) error {
	id := strings.TrimSpace(expenseID)
	if id == "" {
		return nil
	}
	if !isSafeReferenceID(id) {
		return entities.ValidationError{Key: "linked_expense_id", Message: "invalid id"}
	}
	exp, err := v.expenses.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if exp == nil {
		return fmt.Errorf("%w: expense %q", domainerrors.ErrNotFound, id)
	}
	count, err := v.documents.CountByLinkedExpense(ctx, id)
	if err != nil {
		return err
	}
	if count >= entities.DocumentMaxAttachmentsPerExpense {
		return entities.ValidationError{
			Key:     "attachments",
			Message: fmt.Sprintf("max %d attachments per expense", entities.DocumentMaxAttachmentsPerExpense),
		}
	}
	return nil
}

func (v *Documents) checkLinkedContract(ctx context.Context, contractID string) error {
	id := strings.TrimSpace(contractID)
	if id == "" {
		return nil
	}
	if !isSafeReferenceID(id) {
		return entities.ValidationError{Key: "linked_contract_id", Message: "invalid id"}
	}
	c, err := v.contracts.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("%w: contract %q", domainerrors.ErrNotFound, id)
	}
	return nil
}

// mimeAllowed is the bridge between the rules library (typed as Rule)
// and the rest.NormalizeUploadMime helper. Inline-defined here rather
// than in rules/ because the whitelist is HTTP-boundary metadata.
func mimeAllowed(field, contentType string) rules.Rule {
	return func() error {
		parsed, ok := rest.NormalizeUploadMime(contentType)
		if parsed == "" {
			return entities.ValidationError{Key: field, Message: "invalid"}
		}
		if !ok {
			return entities.ValidationError{Key: field, Message: "unsupported (allowed: jpeg, png, pdf)"}
		}
		return nil
	}
}

// sizeInRange enforces the upload's byte cap. Below 1 byte is a
// client mistake (empty file or a missed Content-Length); above the
// cap is a policy refusal (FR34: 10 MB).
func sizeInRange(field string, size, max int64) rules.Rule {
	return func() error {
		if size <= 0 {
			return entities.ValidationError{Key: field, Message: "must be > 0"}
		}
		if size > max {
			return entities.ValidationError{Key: field, Message: fmt.Sprintf("exceeds %d bytes (10MB)", max)}
		}
		return nil
	}
}

// isSafeReferenceID rejects values that could escape a Firestore
// collection prefix or upset the GCS object name (slashes, control
// chars, leading dots, oversize). Every linked-resource id passes
// through this — fail-safe over a broad class of injection vectors.
func isSafeReferenceID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if r == '/' || r == '.' || r < 0x20 {
			return false
		}
	}
	return true
}
