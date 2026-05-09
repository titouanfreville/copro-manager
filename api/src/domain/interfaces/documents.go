package interfaces

import (
	"context"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// DocumentsStore persists standalone Document entities. Per-expense
// attachments are NOT served here — they live in the expense
// subcollection (see AttachmentsStore).
type DocumentsStore interface {
	List(ctx context.Context) ([]entities.Document, error)
	FindByID(ctx context.Context, id string) (*entities.Document, error)
	Create(ctx context.Context, d entities.Document) error
	Update(ctx context.Context, d entities.Document) error
	Delete(ctx context.Context, id string) error

	// CountByCategory returns the number of documents referencing the
	// given category. Used by the categories-delete cascade-rejection
	// check (PRD FR12 — a category can't be deleted if any expense,
	// settlement, template, or document references it).
	CountByCategory(ctx context.Context, categoryID string) (int, error)
}
