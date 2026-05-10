// Package categories persists category entities in Firestore.
package categories

import (
	"context"
	"errors"
	"fmt"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const collection = "categories"

type categoryDoc struct {
	ID                      string                    `firestore:"id"`
	Name                    string                    `firestore:"name"`
	Predefined              bool                      `firestore:"predefined"`
	Hidden                  bool                      `firestore:"hidden,omitempty"`
	DefaultDistributionMode entities.DistributionMode `firestore:"default_distribution_mode,omitempty"`
	Icon                    string                    `firestore:"icon,omitempty"`
	Color                   string                    `firestore:"color,omitempty"`
}

type Store struct {
	client *fs.Client
}

// NewStore returns a Firestore-backed categories store.
func NewStore(client *fs.Client) interfaces.CategoriesStore {
	return &Store{client: client}
}

func (s *Store) List(ctx context.Context) ([]entities.Category, error) {
	iter := s.client.Collection(collection).Documents(ctx)
	defer iter.Stop()

	var out []entities.Category
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("categories: list: %w", err)
		}

		var doc categoryDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("categories: decode: %w", err)
		}
		out = append(out, docToEntity(doc))
	}
}

func (s *Store) FindByID(ctx context.Context, id string) (*entities.Category, error) {
	snap, err := s.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("categories: find by id: %w", err)
	}

	var doc categoryDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("categories: decode: %w", err)
	}
	out := docToEntity(doc)
	return &out, nil
}

func (s *Store) Create(ctx context.Context, c entities.Category) error {
	if _, err := s.client.Collection(collection).Doc(c.ID).Create(ctx, entityToDoc(c)); err != nil {
		return fmt.Errorf("categories: create: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, c entities.Category) error {
	if _, err := s.client.Collection(collection).Doc(c.ID).Set(ctx, entityToDoc(c)); err != nil {
		return fmt.Errorf("categories: update: %w", err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	if _, err := s.client.Collection(collection).Doc(id).Delete(ctx); err != nil {
		return fmt.Errorf("categories: delete: %w", err)
	}
	return nil
}

// EnsureSeeded creates the predefined categories on first boot and
// back-fills the cosmetic fields (icon, color) on existing docs that
// were seeded before those fields existed. The user-overridable bits
// (default_distribution_mode, name) are NOT touched on existing docs —
// only nil/empty cosmetic fields get the seed value, so a foyer member
// who customized "Eau" with a different emoji or color won't have it
// reverted on next deploy.
func (s *Store) EnsureSeeded(ctx context.Context, seed []entities.Category) error {
	for _, c := range seed {
		ref := s.client.Collection(collection).Doc(c.ID)
		_, err := ref.Create(ctx, entityToDoc(c))
		if err == nil {
			continue
		}
		if status.Code(err) != codes.AlreadyExists {
			return fmt.Errorf("categories: seed %q: %w", c.ID, err)
		}

		// Already exists: check if the row is missing the cosmetic
		// fields, and patch only the absent ones. This is the upgrade
		// path for deployments seeded before icon/color landed.
		snap, getErr := ref.Get(ctx)
		if getErr != nil {
			return fmt.Errorf("categories: seed re-read %q: %w", c.ID, getErr)
		}
		var current categoryDoc
		if decErr := snap.DataTo(&current); decErr != nil {
			return fmt.Errorf("categories: seed decode %q: %w", c.ID, decErr)
		}
		updates := []fs.Update{}
		if current.Icon == "" && c.Icon != "" {
			updates = append(updates, fs.Update{Path: "icon", Value: c.Icon})
		}
		if current.Color == "" && c.Color != "" {
			updates = append(updates, fs.Update{Path: "color", Value: c.Color})
		}
		if len(updates) == 0 {
			continue
		}
		if _, updErr := ref.Update(ctx, updates); updErr != nil {
			return fmt.Errorf("categories: seed patch %q: %w", c.ID, updErr)
		}
	}
	return nil
}

func docToEntity(d categoryDoc) entities.Category {
	return entities.Category{
		ID:                      d.ID,
		Name:                    d.Name,
		Predefined:              d.Predefined,
		Hidden:                  d.Hidden,
		DefaultDistributionMode: d.DefaultDistributionMode,
		Icon:                    d.Icon,
		Color:                   d.Color,
	}
}

func entityToDoc(c entities.Category) categoryDoc {
	return categoryDoc{
		ID:                      c.ID,
		Name:                    c.Name,
		Predefined:              c.Predefined,
		Hidden:                  c.Hidden,
		DefaultDistributionMode: c.DefaultDistributionMode,
		Icon:                    c.Icon,
		Color:                   c.Color,
	}
}
