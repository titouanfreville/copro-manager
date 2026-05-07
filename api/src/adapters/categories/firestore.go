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

// EnsureSeeded uses Create-if-missing for each seed entry — uses .Create which
// errors on existing docs, so we swallow AlreadyExists.
func (s *Store) EnsureSeeded(ctx context.Context, seed []entities.Category) error {
	for _, c := range seed {
		_, err := s.client.Collection(collection).Doc(c.ID).Create(ctx, entityToDoc(c))
		if err == nil {
			continue
		}
		if status.Code(err) == codes.AlreadyExists {
			continue
		}
		return fmt.Errorf("categories: seed %q: %w", c.ID, err)
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
	}
}

func entityToDoc(c entities.Category) categoryDoc {
	return categoryDoc{
		ID:                      c.ID,
		Name:                    c.Name,
		Predefined:              c.Predefined,
		Hidden:                  c.Hidden,
		DefaultDistributionMode: c.DefaultDistributionMode,
	}
}
