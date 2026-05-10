// Package documents persists standalone Document entities in Firestore.
package documents

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const collection = "documents"

type documentDoc struct {
	ID               string    `firestore:"id"`
	CoproID          string    `firestore:"copro_id"`
	CategoryID       string    `firestore:"category_id"`
	Group            string    `firestore:"group,omitempty"`
	Title            string    `firestore:"title"`
	Description      string    `firestore:"description,omitempty"`
	ObjectName       string    `firestore:"object_name"`
	ContentType      string    `firestore:"content_type"`
	SizeBytes        int64     `firestore:"size_bytes"`
	OriginalFilename string    `firestore:"original_filename"`
	UploadedAt       time.Time `firestore:"uploaded_at"`
	UploadedBy       string    `firestore:"uploaded_by"`
	LinkedExpenseID  string    `firestore:"linked_expense_id,omitempty"`
}

type Store struct {
	client *fs.Client
}

// NewStore returns a Firestore-backed documents store.
func NewStore(client *fs.Client) interfaces.DocumentsStore {
	return &Store{client: client}
}

func (s *Store) List(ctx context.Context) ([]entities.Document, error) {
	iter := s.client.Collection(collection).Documents(ctx)
	defer iter.Stop()

	var out []entities.Document
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("documents: list: %w", err)
		}
		var doc documentDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("documents: decode: %w", err)
		}
		out = append(out, docToEntity(doc))
	}
	return out, nil
}

func (s *Store) FindByID(ctx context.Context, id string) (*entities.Document, error) {
	snap, err := s.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("documents: get by id: %w", err)
	}
	var doc documentDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("documents: decode: %w", err)
	}
	out := docToEntity(doc)
	return &out, nil
}

func (s *Store) Create(ctx context.Context, d entities.Document) error {
	if _, err := s.client.Collection(collection).Doc(d.ID).Create(ctx, entityToDoc(d)); err != nil {
		return fmt.Errorf("documents: create: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, d entities.Document) error {
	if _, err := s.client.Collection(collection).Doc(d.ID).Set(ctx, entityToDoc(d)); err != nil {
		return fmt.Errorf("documents: update: %w", err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if _, err := s.client.Collection(collection).Doc(id).Delete(ctx); err != nil {
		return fmt.Errorf("documents: delete: %w", err)
	}
	return nil
}

// CountByCategory uses an equality filter on `category_id`. Firestore
// auto-indexes single-field equality queries — no composite index needed.
func (s *Store) CountByCategory(ctx context.Context, categoryID string) (int, error) {
	iter := s.client.Collection(collection).
		Where("category_id", "==", categoryID).
		Documents(ctx)
	defer iter.Stop()

	count := 0
	for {
		_, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("documents: count by category: %w", err)
		}
		count++
	}
	return count, nil
}

// CountByLinkedExpense returns the number of documents pinned to the given
// expense. Powers the per-expense cap (max 10) on the unified attach flow.
func (s *Store) CountByLinkedExpense(ctx context.Context, expenseID string) (int, error) {
	iter := s.client.Collection(collection).
		Where("linked_expense_id", "==", expenseID).
		Documents(ctx)
	defer iter.Stop()

	count := 0
	for {
		_, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("documents: count by linked expense: %w", err)
		}
		count++
	}
	return count, nil
}

// ListByLinkedExpense returns every document linked to the given expense,
// ordered client-side by uploaded_at asc. Equality on a single field uses
// Firestore's automatic single-field index — no composite needed.
func (s *Store) ListByLinkedExpense(ctx context.Context, expenseID string) ([]entities.Document, error) {
	iter := s.client.Collection(collection).
		Where("linked_expense_id", "==", expenseID).
		Documents(ctx)
	defer iter.Stop()

	var out []entities.Document
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("documents: list by linked expense: %w", err)
		}
		var doc documentDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("documents: decode: %w", err)
		}
		out = append(out, docToEntity(doc))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UploadedAt.Before(out[j].UploadedAt)
	})
	return out, nil
}

func docToEntity(d documentDoc) entities.Document {
	return entities.Document{
		ID:               d.ID,
		CoproID:          d.CoproID,
		CategoryID:       d.CategoryID,
		Group:            d.Group,
		Title:            d.Title,
		Description:      d.Description,
		ObjectName:       d.ObjectName,
		ContentType:      d.ContentType,
		SizeBytes:        d.SizeBytes,
		OriginalFilename: d.OriginalFilename,
		UploadedAt:       d.UploadedAt,
		UploadedBy:       d.UploadedBy,
		LinkedExpenseID:  d.LinkedExpenseID,
	}
}

func entityToDoc(d entities.Document) documentDoc {
	return documentDoc{
		ID:               d.ID,
		CoproID:          d.CoproID,
		CategoryID:       d.CategoryID,
		Group:            d.Group,
		Title:            d.Title,
		Description:      d.Description,
		ObjectName:       d.ObjectName,
		ContentType:      d.ContentType,
		SizeBytes:        d.SizeBytes,
		OriginalFilename: d.OriginalFilename,
		UploadedAt:       d.UploadedAt,
		UploadedBy:       d.UploadedBy,
		LinkedExpenseID:  d.LinkedExpenseID,
	}
}
