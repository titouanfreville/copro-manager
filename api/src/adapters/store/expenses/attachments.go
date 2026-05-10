package expenses

import (
	"context"
	"errors"
	"fmt"
	"time"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// attachmentsSubcollection is the name of the subcollection under each
// expense doc. Subcollection rather than inline array so the per-expense cap
// can be enforced atomically inside a Firestore transaction.
const attachmentsSubcollection = "attachments"

// AttachmentsStore implements interfaces.AttachmentsStore against Firestore.
type AttachmentsStore struct {
	client *fs.Client
}

// NewAttachmentsStore returns a Firestore-backed AttachmentsStore.
func NewAttachmentsStore(client *fs.Client) interfaces.AttachmentsStore {
	return &AttachmentsStore{client: client}
}

func (s *AttachmentsStore) collectionRef(expenseID string) *fs.CollectionRef {
	return s.client.Collection(collection).Doc(expenseID).Collection(attachmentsSubcollection)
}

// List returns every attachment for the given expense, ordered by
// uploaded_at asc (stable display order).
func (s *AttachmentsStore) List(ctx context.Context, expenseID string) ([]entities.Attachment, error) {
	iter := s.collectionRef(expenseID).OrderBy("uploaded_at", fs.Asc).Documents(ctx)
	defer iter.Stop()

	var out []entities.Attachment
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("attachments: list: %w", err)
		}
		var doc attachmentDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("attachments: decode: %w", err)
		}
		out = append(out, attachmentDocToEntity(doc))
	}
}

// FindByID returns the attachment with the given ID, or (nil, nil) when
// absent.
func (s *AttachmentsStore) FindByID(ctx context.Context, expenseID, attachmentID string) (*entities.Attachment, error) {
	snap, err := s.collectionRef(expenseID).Doc(attachmentID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("attachments: get: %w", err)
	}
	var doc attachmentDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("attachments: decode: %w", err)
	}
	out := attachmentDocToEntity(doc)
	return &out, nil
}

// Count returns the current number of attachments on the expense. Cheap at
// our cap (≤10).
func (s *AttachmentsStore) Count(ctx context.Context, expenseID string) (int, error) {
	iter := s.collectionRef(expenseID).Documents(ctx)
	defer iter.Stop()
	n := 0
	for {
		_, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return n, nil
		}
		if err != nil {
			return 0, fmt.Errorf("attachments: count: %w", err)
		}
		n++
	}
}

// CreateIfUnderCap atomically verifies that the cap is not reached, then
// writes the new attachment. Two concurrent callers race past the same cap
// only if both transactions land before either commits — Firestore's optimistic
// concurrency makes the loser retry.
func (s *AttachmentsStore) CreateIfUnderCap(ctx context.Context, expenseID string, att entities.Attachment, cap int) error {
	colRef := s.collectionRef(expenseID)
	docRef := colRef.Doc(att.ID)
	expenseRef := s.client.Collection(collection).Doc(expenseID)

	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
		// Verify the parent expense still exists; if it was deleted between
		// the upload-url issuance and now, refuse to write an orphan
		// attachment doc.
		if _, err := tx.Get(expenseRef); err != nil {
			if status.Code(err) == codes.NotFound {
				return fmt.Errorf("%w: expense %q", domainerrors.ErrNotFound, expenseID)
			}
			return fmt.Errorf("attachments: tx get expense: %w", err)
		}

		// Cap check inside the transaction so concurrent uploads can't both
		// pass.
		docs, err := tx.Documents(colRef).GetAll()
		if err != nil {
			return fmt.Errorf("attachments: tx list: %w", err)
		}
		if len(docs) >= cap {
			return fmt.Errorf("%w: max %d attachments per expense", domainerrors.ErrAlreadyExists, cap)
		}
		for _, d := range docs {
			if d.Ref.ID == att.ID {
				return fmt.Errorf("%w: attachment %q", domainerrors.ErrAlreadyExists, att.ID)
			}
		}

		if err := tx.Create(docRef, attachmentToDoc(att)); err != nil {
			return fmt.Errorf("attachments: tx create: %w", err)
		}
		// Touch updated_at on the parent expense so onSnapshot listeners
		// re-render the row.
		if err := tx.Update(expenseRef, []fs.Update{
			{Path: "updated_at", Value: time.Now().UTC()},
		}); err != nil {
			return fmt.Errorf("attachments: tx touch parent: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// Delete removes a single attachment doc. Idempotent.
func (s *AttachmentsStore) Delete(ctx context.Context, expenseID, attachmentID string) error {
	if _, err := s.collectionRef(expenseID).Doc(attachmentID).Delete(ctx); err != nil {
		return fmt.Errorf("attachments: delete: %w", err)
	}
	// Touch updated_at on the parent expense so onSnapshot re-renders.
	_, err := s.client.Collection(collection).Doc(expenseID).Update(ctx, []fs.Update{
		{Path: "updated_at", Value: time.Now().UTC()},
	})
	if err != nil && status.Code(err) != codes.NotFound {
		return fmt.Errorf("attachments: touch parent on delete: %w", err)
	}
	return nil
}

// DeleteAll wipes the subcollection — called by the expense-delete cascade.
// Iterates and deletes one-by-one; with a cap of 10 this is fine.
func (s *AttachmentsStore) DeleteAll(ctx context.Context, expenseID string) error {
	iter := s.collectionRef(expenseID).Documents(ctx)
	defer iter.Stop()

	var firstErr error
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("attachments: list-for-delete-all: %w", err)
			}
			break
		}
		if _, err := snap.Ref.Delete(ctx); err != nil && firstErr == nil {
			// Continue best-effort; report the first error but try to clean
			// the rest of the subcollection.
			firstErr = fmt.Errorf("attachments: delete-all %q: %w", snap.Ref.ID, err)
		}
	}
	return firstErr
}

func attachmentDocToEntity(d attachmentDoc) entities.Attachment {
	return entities.Attachment{
		ID:               d.ID,
		ObjectName:       d.ObjectName,
		ContentType:      d.ContentType,
		SizeBytes:        d.SizeBytes,
		OriginalFilename: d.OriginalFilename,
		UploadedAt:       d.UploadedAt,
		UploadedBy:       d.UploadedBy,
	}
}
