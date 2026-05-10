// Package contracts persists Contract entities in Firestore.
package contracts

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

const collection = "contracts"

type societyDoc struct {
	Name    string `firestore:"name"`
	Phone   string `firestore:"phone,omitempty"`
	Email   string `firestore:"email,omitempty"`
	Website string `firestore:"website,omitempty"`
	Address string `firestore:"address,omitempty"`
}

type contactDoc struct {
	Name  string `firestore:"name,omitempty"`
	Role  string `firestore:"role,omitempty"`
	Phone string `firestore:"phone,omitempty"`
	Email string `firestore:"email,omitempty"`
}

type contractDoc struct {
	ID         string `firestore:"id"`
	CoproID    string `firestore:"copro_id"`
	Name       string `firestore:"name"`
	CategoryID string `firestore:"category_id"`

	Society societyDoc `firestore:"society"`
	Contact contactDoc `firestore:"contact,omitempty"`

	StartDate time.Time `firestore:"start_date,omitempty"`
	EndDate   time.Time `firestore:"end_date,omitempty"`

	AmountCents      int                       `firestore:"amount_cents,omitempty"`
	BillingFrequency entities.BillingFrequency `firestore:"billing_frequency,omitempty"`

	TemplateID string                  `firestore:"template_id,omitempty"`
	Status     entities.ContractStatus `firestore:"status"`
	Note       string                  `firestore:"note,omitempty"`

	CreatedAt time.Time `firestore:"created_at"`
	UpdatedAt time.Time `firestore:"updated_at"`
}

type Store struct {
	client *fs.Client
}

// NewStore returns a Firestore-backed contracts store.
func NewStore(client *fs.Client) interfaces.ContractsStore {
	return &Store{client: client}
}

func (s *Store) List(ctx context.Context) ([]entities.Contract, error) {
	iter := s.client.Collection(collection).Documents(ctx)
	defer iter.Stop()

	var out []entities.Contract
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("contracts: list: %w", err)
		}
		var doc contractDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("contracts: decode: %w", err)
		}
		out = append(out, docToEntity(doc))
	}
}

func (s *Store) FindByID(ctx context.Context, id string) (*entities.Contract, error) {
	snap, err := s.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("contracts: get by id: %w", err)
	}
	var doc contractDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("contracts: decode: %w", err)
	}
	out := docToEntity(doc)
	return &out, nil
}

func (s *Store) Create(ctx context.Context, c entities.Contract) error {
	if _, err := s.client.Collection(collection).Doc(c.ID).Create(ctx, entityToDoc(c)); err != nil {
		return fmt.Errorf("contracts: create: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, c entities.Contract) error {
	if _, err := s.client.Collection(collection).Doc(c.ID).Set(ctx, entityToDoc(c)); err != nil {
		return fmt.Errorf("contracts: update: %w", err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if _, err := s.client.Collection(collection).Doc(id).Delete(ctx); err != nil {
		return fmt.Errorf("contracts: delete: %w", err)
	}
	return nil
}

// CountByCategory powers the categories-delete cascade rejection so a
// category referenced by any contract can't be removed.
func (s *Store) CountByCategory(ctx context.Context, categoryID string) (int, error) {
	iter := s.client.Collection(collection).
		Where("category_id", "==", categoryID).
		Documents(ctx)
	defer iter.Stop()

	count := 0
	for {
		_, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return count, nil
		}
		if err != nil {
			return 0, fmt.Errorf("contracts: count by category: %w", err)
		}
		count++
	}
}

func docToEntity(d contractDoc) entities.Contract {
	return entities.Contract{
		ID:         d.ID,
		CoproID:    d.CoproID,
		Name:       d.Name,
		CategoryID: d.CategoryID,
		Society: entities.Society{
			Name:    d.Society.Name,
			Phone:   d.Society.Phone,
			Email:   d.Society.Email,
			Website: d.Society.Website,
			Address: d.Society.Address,
		},
		Contact: entities.Contact{
			Name:  d.Contact.Name,
			Role:  d.Contact.Role,
			Phone: d.Contact.Phone,
			Email: d.Contact.Email,
		},
		StartDate:        d.StartDate,
		EndDate:          d.EndDate,
		AmountCents:      d.AmountCents,
		BillingFrequency: d.BillingFrequency,
		TemplateID:       d.TemplateID,
		Status:           d.Status,
		Note:             d.Note,
		CreatedAt:        d.CreatedAt,
		UpdatedAt:        d.UpdatedAt,
	}
}

func entityToDoc(c entities.Contract) contractDoc {
	return contractDoc{
		ID:         c.ID,
		CoproID:    c.CoproID,
		Name:       c.Name,
		CategoryID: c.CategoryID,
		Society: societyDoc{
			Name:    c.Society.Name,
			Phone:   c.Society.Phone,
			Email:   c.Society.Email,
			Website: c.Society.Website,
			Address: c.Society.Address,
		},
		Contact: contactDoc{
			Name:  c.Contact.Name,
			Role:  c.Contact.Role,
			Phone: c.Contact.Phone,
			Email: c.Contact.Email,
		},
		StartDate:        c.StartDate,
		EndDate:          c.EndDate,
		AmountCents:      c.AmountCents,
		BillingFrequency: c.BillingFrequency,
		TemplateID:       c.TemplateID,
		Status:           c.Status,
		Note:             c.Note,
		CreatedAt:        c.CreatedAt,
		UpdatedAt:        c.UpdatedAt,
	}
}
