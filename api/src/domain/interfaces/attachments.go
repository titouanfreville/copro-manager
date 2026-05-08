package interfaces

import (
	"context"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
)

// AttachmentsStore persists Attachment metadata in the subcollection
// `expenses/{expenseID}/attachments/{attachmentID}`. Each attachment is its
// own Firestore doc so the per-expense cap can be enforced atomically (via
// a transaction reading Count) and a single attachment write doesn't have to
// rewrite the whole list.
type AttachmentsStore interface {
	// List returns every attachment for the given expense, ordered by
	// uploaded_at ascending.
	List(ctx context.Context, expenseID string) ([]entities.Attachment, error)
	// FindByID returns (nil, nil) when the attachment is absent.
	FindByID(ctx context.Context, expenseID, attachmentID string) (*entities.Attachment, error)
	// Count returns the current number of attachments on the expense.
	Count(ctx context.Context, expenseID string) (int, error)
	// CreateIfUnderCap atomically verifies that adding one more attachment
	// would not exceed `cap`, then writes the new attachment. Returns
	// ErrAlreadyExists when the cap is reached (so concurrent uploaders are
	// rejected without a separate dedup step) or when an attachment with
	// the same ID already exists.
	CreateIfUnderCap(ctx context.Context, expenseID string, att entities.Attachment, cap int) error
	// Delete removes a single attachment. Idempotent — missing attachments
	// are no-ops.
	Delete(ctx context.Context, expenseID, attachmentID string) error
	// DeleteAll wipes the subcollection — used by the expense-delete cascade.
	DeleteAll(ctx context.Context, expenseID string) error
}
