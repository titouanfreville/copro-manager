// Package expenses persists shared-expense entities in Firestore.
package expenses

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const collection = "expenses"

type expenseDoc struct {
	ID               string                    `firestore:"id"`
	CoproID          string                    `firestore:"copro_id"`
	Name             string                    `firestore:"name"`
	AmountCents      int                       `firestore:"amount_cents"`
	Currency         string                    `firestore:"currency"`
	Date             time.Time                 `firestore:"date"`
	PaymentDate      *time.Time                `firestore:"payment_date,omitempty"`
	PayerFoyerID     string                    `firestore:"payer_foyer_id"`
	CategoryID       string                    `firestore:"category_id"`
	DistributionMode entities.DistributionMode `firestore:"distribution_mode"`
	ShareRDCCents    int                       `firestore:"share_rdc_cents"`
	Share1erCents    int                       `firestore:"share_1er_cents"`
	Settled          bool                      `firestore:"settled"`
	SettledAt        *time.Time                `firestore:"settled_at,omitempty"`
	Note             string                    `firestore:"note,omitempty"`
	CreatedAt        time.Time                 `firestore:"created_at"`
	UpdatedAt        time.Time                 `firestore:"updated_at"`
}

type Store struct {
	client *fs.Client
}

// NewStore returns a Firestore-backed expenses store.
func NewStore(client *fs.Client) interfaces.ExpensesStore {
	return &Store{client: client}
}

// List loads every expense and sorts in-memory by date desc then created_at
// desc — the dataset is small enough that we don't need server-side ordering
// (and avoids creating composite indexes for now).
func (s *Store) List(ctx context.Context) ([]entities.Expense, error) {
	iter := s.client.Collection(collection).Documents(ctx)
	defer iter.Stop()

	var out []entities.Expense
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("expenses: list: %w", err)
		}

		var doc expenseDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("expenses: decode: %w", err)
		}
		out = append(out, docToEntity(doc))
	}

	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Date.Equal(out[j].Date) {
			return out[i].Date.After(out[j].Date)
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) Create(ctx context.Context, exp entities.Expense) error {
	if _, err := s.client.Collection(collection).Doc(exp.ID).Create(ctx, entityToDoc(exp)); err != nil {
		return fmt.Errorf("expenses: create: %w", err)
	}
	return nil
}

// Update overwrites an existing expense doc by ID. Caller is responsible for
// having a valid ID (read-modify-write); use FindByNameAndDate to resolve.
func (s *Store) Update(ctx context.Context, exp entities.Expense) error {
	if _, err := s.client.Collection(collection).Doc(exp.ID).Set(ctx, entityToDoc(exp)); err != nil {
		return fmt.Errorf("expenses: update: %w", err)
	}
	return nil
}

// FindByNameAndDate is the upsert lookup used by the CSV import. Returns
// (nil, nil) when no match exists.
func (s *Store) FindByNameAndDate(ctx context.Context, name string, date time.Time) (*entities.Expense, error) {
	iter := s.client.Collection(collection).
		Where("name", "==", name).
		Where("date", "==", date).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()

	snap, err := iter.Next()
	if errors.Is(err, iterator.Done) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("expenses: find by name+date: %w", err)
	}
	var doc expenseDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("expenses: decode: %w", err)
	}
	out := docToEntity(doc)
	return &out, nil
}

func docToEntity(d expenseDoc) entities.Expense {
	return entities.Expense{
		ID:               d.ID,
		CoproID:          d.CoproID,
		Name:             d.Name,
		AmountCents:      d.AmountCents,
		Currency:         d.Currency,
		Date:             d.Date,
		PaymentDate:      d.PaymentDate,
		PayerFoyerID:     d.PayerFoyerID,
		CategoryID:       d.CategoryID,
		DistributionMode: d.DistributionMode,
		ShareRDCCents:    d.ShareRDCCents,
		Share1erCents:    d.Share1erCents,
		Settled:          d.Settled,
		SettledAt:        d.SettledAt,
		Note:             d.Note,
		CreatedAt:        d.CreatedAt,
		UpdatedAt:        d.UpdatedAt,
	}
}

func entityToDoc(e entities.Expense) expenseDoc {
	return expenseDoc{
		ID:               e.ID,
		CoproID:          e.CoproID,
		Name:             e.Name,
		AmountCents:      e.AmountCents,
		Currency:         e.Currency,
		Date:             e.Date,
		PaymentDate:      e.PaymentDate,
		PayerFoyerID:     e.PayerFoyerID,
		CategoryID:       e.CategoryID,
		DistributionMode: e.DistributionMode,
		ShareRDCCents:    e.ShareRDCCents,
		Share1erCents:    e.Share1erCents,
		Settled:          e.Settled,
		SettledAt:        e.SettledAt,
		Note:             e.Note,
		CreatedAt:        e.CreatedAt,
		UpdatedAt:        e.UpdatedAt,
	}
}
