package documents

import (
	"context"
	"strings"
	"time"

	"github.com/titouanfreville/copro-manager/api/src/core/rest"
	"github.com/titouanfreville/copro-manager/api/src/core/text"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const (
	descriptionMaxBytes      = 2000
	groupMaxBytes            = 64
	originalFilenameMaxBytes = 256
)

// builder turns a validated draft into a ready-to-persist Document.
// Three responsibilities:
//
//   1. applyLinkedDefaults — pull the linked expense (if set) and fill
//      missing title / category from it. Pure data flow, no validation.
//   2. build — construct the final Document for the Record path.
//   3. applyMetadata — patch metadata fields onto an existing Document
//      for the Update path.
type builder struct {
	copros   interfaces.CoprosStore
	expenses interfaces.ExpensesStore
	now      func() time.Time
}

func newBuilder(copros interfaces.CoprosStore, expenses interfaces.ExpensesStore, now func() time.Time) *builder {
	return &builder{copros: copros, expenses: expenses, now: now}
}

// applyLinkedDefaults pulls the linked expense (if set) and fills
// missing title + category_id. The validator runs AFTER this so its
// "title required" rule sees the final, post-default value.
//
// A linked expense that doesn't exist yields no defaults — the
// validator's checkLinkedExpense surfaces the not-found error.
func (b *builder) applyLinkedDefaults(ctx context.Context, d entities.DocumentDraft) (entities.DocumentDraft, error) {
	id := strings.TrimSpace(d.LinkedExpenseID)
	if id == "" {
		return d, nil
	}
	exp, err := b.expenses.FindByID(ctx, id)
	if err != nil {
		return d, err
	}
	if exp == nil {
		return d, nil
	}
	d.CategoryID = exp.CategoryID
	if strings.TrimSpace(d.Title) == "" {
		d.Title = exp.Name
	}
	return d, nil
}

// build constructs the persisted Document. Caller has already
// validated; this method only stamps server-owned fields and
// normalizes string lengths.
func (b *builder) build(ctx context.Context, docID, objectName, contentType string, d entities.DocumentDraft, actorUID string) (entities.Document, error) {
	copro, err := b.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return entities.Document{}, err
	}
	now := b.now()
	return entities.Document{
		ID:               docID,
		CoproID:          copro.ID,
		CategoryID:       strings.TrimSpace(d.CategoryID),
		Group:            normalizeGroup(d.Group),
		Title:            strings.TrimSpace(d.Title),
		Description:      text.Truncate(strings.TrimSpace(d.Description), descriptionMaxBytes),
		ObjectName:       objectName,
		ContentType:      contentType,
		SizeBytes:        d.SizeBytes,
		OriginalFilename: text.Truncate(strings.TrimSpace(d.OriginalFilename), originalFilenameMaxBytes),
		UploadedAt:       now,
		UploadedBy:       actorUID,
		LinkedExpenseID:  strings.TrimSpace(d.LinkedExpenseID),
		LinkedContractID: strings.TrimSpace(d.LinkedContractID),
	}, nil
}

// applyMetadata patches metadata fields onto an existing Document.
// Used by Update — the file blob (object_name, content_type, size,
// original_filename) is immutable.
func (b *builder) applyMetadata(existing entities.Document, m entities.DocumentMetadataDraft) entities.Document {
	existing.Title = strings.TrimSpace(m.Title)
	existing.Description = text.Truncate(strings.TrimSpace(m.Description), descriptionMaxBytes)
	existing.CategoryID = strings.TrimSpace(m.CategoryID)
	existing.Group = normalizeGroup(m.Group)
	existing.LinkedContractID = strings.TrimSpace(m.LinkedContractID)
	return existing
}

// normalizeGroup lowercases + trims so display variants merge into a
// single foldable section in the archive view ("Devis" vs "devis").
func normalizeGroup(g string) string {
	cleaned := strings.ToLower(strings.TrimSpace(g))
	if cleaned == "" {
		return ""
	}
	return text.Truncate(cleaned, groupMaxBytes)
}

// buildObjectName composes the canonical GCS key. Server authoritative
// — clients never get to choose the path.
func buildObjectName(documentID, contentType string) string {
	return "documents/" + documentID + rest.UploadExtension(contentType)
}

// isSafeID rejects ids that could escape the documents/ prefix in
// GCS. Sibling helper to the validator's isSafeReferenceID, kept
// here because the orchestration uses it on the document_id path
// before any validator call.
func isSafeID(id string) bool {
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
