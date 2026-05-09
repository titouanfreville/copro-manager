// Package templates persists ExpenseTemplate entities in Firestore.
package templates

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

const collection = "expense_templates"

type templateDoc struct {
	ID                 string                    `firestore:"id"`
	CoproID            string                    `firestore:"copro_id"`
	Name               string                    `firestore:"name"`
	AmountDefaultCents int                       `firestore:"amount_default_cents"`
	Currency           string                    `firestore:"currency"`
	CategoryID         string                    `firestore:"category_id"`
	PayerFoyerID       string                    `firestore:"payer_foyer_id"`
	DistributionMode   entities.DistributionMode `firestore:"distribution_mode"`
	ShareRDCCents      int                       `firestore:"share_rdc_cents,omitempty"`
	Share1erCents      int                       `firestore:"share_1er_cents,omitempty"`
	Note               string                    `firestore:"note,omitempty"`
	ScheduleActive     bool                      `firestore:"schedule_active"`
	Frequency          entities.Frequency        `firestore:"frequency,omitempty"`
	DayOfMonth         int                       `firestore:"day_of_month,omitempty"`
	NextOccurrenceAt   *time.Time                `firestore:"next_occurrence_at,omitempty"`
	EndDate            *time.Time                `firestore:"end_date,omitempty"`
	CreatedAt          time.Time                 `firestore:"created_at"`
	UpdatedAt          time.Time                 `firestore:"updated_at"`
}

type Store struct {
	client *fs.Client
}

// NewStore returns a Firestore-backed templates store.
func NewStore(client *fs.Client) interfaces.TemplatesStore {
	return &Store{client: client}
}

func (s *Store) List(ctx context.Context) ([]entities.ExpenseTemplate, error) {
	iter := s.client.Collection(collection).Documents(ctx)
	defer iter.Stop()

	var out []entities.ExpenseTemplate
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("templates: list: %w", err)
		}
		var doc templateDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("templates: decode: %w", err)
		}
		out = append(out, docToEntity(doc))
	}
	return out, nil
}

func (s *Store) FindByID(ctx context.Context, id string) (*entities.ExpenseTemplate, error) {
	snap, err := s.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("templates: get by id: %w", err)
	}
	var doc templateDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("templates: decode: %w", err)
	}
	out := docToEntity(doc)
	return &out, nil
}

func (s *Store) Create(ctx context.Context, t entities.ExpenseTemplate) error {
	if _, err := s.client.Collection(collection).Doc(t.ID).Create(ctx, entityToDoc(t)); err != nil {
		return fmt.Errorf("templates: create: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, t entities.ExpenseTemplate) error {
	if _, err := s.client.Collection(collection).Doc(t.ID).Set(ctx, entityToDoc(t)); err != nil {
		return fmt.Errorf("templates: update: %w", err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if _, err := s.client.Collection(collection).Doc(id).Delete(ctx); err != nil {
		return fmt.Errorf("templates: delete: %w", err)
	}
	return nil
}

// CountByCategory returns the number of templates referencing the given
// category. Single-field equality query — Firestore auto-indexes it.
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
			return 0, fmt.Errorf("templates: count by category: %w", err)
		}
		count++
	}
	return count, nil
}

// ListDue queries on schedule_active + next_occurrence_at. Firestore
// supports composite queries on these fields without an explicit index when
// the inequality is on a single field.
func (s *Store) ListDue(ctx context.Context, cutoff time.Time) ([]entities.ExpenseTemplate, error) {
	iter := s.client.Collection(collection).
		Where("schedule_active", "==", true).
		Where("next_occurrence_at", "<=", cutoff).
		Documents(ctx)
	defer iter.Stop()

	var out []entities.ExpenseTemplate
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("templates: list due: %w", err)
		}
		var doc templateDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("templates: decode due: %w", err)
		}
		out = append(out, docToEntity(doc))
	}
	return out, nil
}

func docToEntity(d templateDoc) entities.ExpenseTemplate {
	return entities.ExpenseTemplate{
		ID:                 d.ID,
		CoproID:            d.CoproID,
		Name:               d.Name,
		AmountDefaultCents: d.AmountDefaultCents,
		Currency:           d.Currency,
		CategoryID:         d.CategoryID,
		PayerFoyerID:       d.PayerFoyerID,
		DistributionMode:   d.DistributionMode,
		ShareRDCCents:      d.ShareRDCCents,
		Share1erCents:      d.Share1erCents,
		Note:               d.Note,
		ScheduleActive:     d.ScheduleActive,
		Frequency:          d.Frequency,
		DayOfMonth:         d.DayOfMonth,
		NextOccurrenceAt:   d.NextOccurrenceAt,
		EndDate:            d.EndDate,
		CreatedAt:          d.CreatedAt,
		UpdatedAt:          d.UpdatedAt,
	}
}

func entityToDoc(t entities.ExpenseTemplate) templateDoc {
	return templateDoc{
		ID:                 t.ID,
		CoproID:            t.CoproID,
		Name:               t.Name,
		AmountDefaultCents: t.AmountDefaultCents,
		Currency:           t.Currency,
		CategoryID:         t.CategoryID,
		PayerFoyerID:       t.PayerFoyerID,
		DistributionMode:   t.DistributionMode,
		ShareRDCCents:      t.ShareRDCCents,
		Share1erCents:      t.Share1erCents,
		Note:               t.Note,
		ScheduleActive:     t.ScheduleActive,
		Frequency:          t.Frequency,
		DayOfMonth:         t.DayOfMonth,
		NextOccurrenceAt:   t.NextOccurrenceAt,
		EndDate:            t.EndDate,
		CreatedAt:          t.CreatedAt,
		UpdatedAt:          t.UpdatedAt,
	}
}
