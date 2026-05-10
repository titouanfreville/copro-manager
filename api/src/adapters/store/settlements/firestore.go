// Package settlements persists Settlement entities in Firestore.
package settlements

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
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const collection = "settlements"

type settlementDoc struct {
	ID          string    `firestore:"id"`
	CoproID     string    `firestore:"copro_id"`
	FromFoyerID string    `firestore:"from_foyer_id"`
	ToFoyerID   string    `firestore:"to_foyer_id"`
	AmountCents int       `firestore:"amount_cents"`
	Currency    string    `firestore:"currency"`
	Date        time.Time `firestore:"date"`
	Note        string    `firestore:"note,omitempty"`
	ExpenseIDs  []string  `firestore:"expense_ids,omitempty"`
	CreatedAt   time.Time `firestore:"created_at"`
	UpdatedAt   time.Time `firestore:"updated_at"`
}

type Store struct {
	client *fs.Client
}

func NewStore(client *fs.Client) interfaces.SettlementsStore {
	return &Store{client: client}
}

func (s *Store) List(ctx context.Context) ([]entities.Settlement, error) {
	iter := s.client.Collection(collection).Documents(ctx)
	defer iter.Stop()

	var out []entities.Settlement
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("settlements: list: %w", err)
		}
		var doc settlementDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("settlements: decode: %w", err)
		}
		out = append(out, docToEntity(doc))
	}
	return out, nil
}

func (s *Store) FindByID(ctx context.Context, id string) (*entities.Settlement, error) {
	snap, err := s.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("settlements: get by id: %w", err)
	}
	var doc settlementDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("settlements: decode: %w", err)
	}
	out := docToEntity(doc)
	return &out, nil
}

// FindByExpenseID scans for the (single) settlement whose expense_ids
// array contains the given ID. Firestore's `array-contains` operator is
// the right primitive here — single-shot query, no fan-out.
func (s *Store) FindByExpenseID(ctx context.Context, expenseID string) (*entities.Settlement, error) {
	iter := s.client.Collection(collection).
		Where("expense_ids", "array-contains", expenseID).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()

	snap, err := iter.Next()
	if errors.Is(err, iterator.Done) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("settlements: find by expense id: %w", err)
	}
	var doc settlementDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("settlements: decode: %w", err)
	}
	out := docToEntity(doc)
	return &out, nil
}

func (s *Store) Create(ctx context.Context, st entities.Settlement) error {
	if _, err := s.client.Collection(collection).Doc(st.ID).Create(ctx, entityToDoc(st)); err != nil {
		return fmt.Errorf("settlements: create: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, st entities.Settlement) error {
	if _, err := s.client.Collection(collection).Doc(st.ID).Set(ctx, entityToDoc(st)); err != nil {
		return fmt.Errorf("settlements: update: %w", err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if _, err := s.client.Collection(collection).Doc(id).Delete(ctx); err != nil {
		return fmt.Errorf("settlements: delete: %w", err)
	}
	return nil
}

// PruneExpense walks every settlement that links the given expenseID and
// rewrites its `expense_ids` array minus that ID. Best-effort under
// concurrent writes — the next call would re-prune anyway.
func (s *Store) PruneExpense(ctx context.Context, expenseID string) error {
	iter := s.client.Collection(collection).
		Where("expense_ids", "array-contains", expenseID).
		Documents(ctx)
	defer iter.Stop()

	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return fmt.Errorf("settlements: prune scan: %w", err)
		}
		var doc settlementDoc
		if err := snap.DataTo(&doc); err != nil {
			return fmt.Errorf("settlements: prune decode: %w", err)
		}
		filtered := make([]string, 0, len(doc.ExpenseIDs))
		for _, id := range doc.ExpenseIDs {
			if id != expenseID {
				filtered = append(filtered, id)
			}
		}
		if _, err := snap.Ref.Update(ctx, []fs.Update{
			{Path: "expense_ids", Value: filtered},
			{Path: "updated_at", Value: time.Now().UTC()},
		}); err != nil {
			return fmt.Errorf("settlements: prune update: %w", err)
		}
	}
	return nil
}

func docToEntity(d settlementDoc) entities.Settlement {
	return entities.Settlement{
		ID:          d.ID,
		CoproID:     d.CoproID,
		FromFoyerID: d.FromFoyerID,
		ToFoyerID:   d.ToFoyerID,
		AmountCents: d.AmountCents,
		Currency:    d.Currency,
		Date:        d.Date,
		Note:        d.Note,
		ExpenseIDs:  d.ExpenseIDs,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}

func entityToDoc(s entities.Settlement) settlementDoc {
	return settlementDoc{
		ID:          s.ID,
		CoproID:     s.CoproID,
		FromFoyerID: s.FromFoyerID,
		ToFoyerID:   s.ToFoyerID,
		AmountCents: s.AmountCents,
		Currency:    s.Currency,
		Date:        s.Date,
		Note:        s.Note,
		ExpenseIDs:  s.ExpenseIDs,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
}
