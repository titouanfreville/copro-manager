// Package copros persists the singleton Copro in Firestore.
package copros

import (
	"context"
	"fmt"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const (
	collection = "copros"
	// singletonID is the fixed doc ID for the only Copro doc — using a
	// constant instead of a generated UUID prevents two concurrent first
	// calls from each creating their own singleton.
	singletonID = "singleton"
)

type coproDoc struct {
	ID         string `firestore:"id"`
	Name       string `firestore:"name"`
	Address    string `firestore:"address"`
	TotalParts int    `firestore:"total_parts"`
}

// Store implements interfaces.CoprosStore against Firestore.
type Store struct {
	client *fs.Client
}

// NewStore returns a Firestore-backed copros store.
func NewStore(client *fs.Client) interfaces.CoprosStore {
	return &Store{client: client}
}

// GetOrCreateSingleton returns the only Copro doc, creating it with sane
// defaults on first call. Race-safe: two concurrent first calls both target
// `Doc(singletonID).Create(...)` — Firestore atomically lets one win and the
// other re-fetches the now-existing doc.
func (s *Store) GetOrCreateSingleton(ctx context.Context) (*entities.Copro, error) {
	docRef := s.client.Collection(collection).Doc(singletonID)

	if snap, err := docRef.Get(ctx); err == nil {
		var doc coproDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("copros: decode doc: %w", err)
		}
		return docToEntity(doc), nil
	} else if status.Code(err) != codes.NotFound {
		return nil, fmt.Errorf("copros: get singleton: %w", err)
	}

	doc := coproDoc{
		ID:         singletonID,
		Name:       "Copro",
		Address:    "",
		TotalParts: entities.DefaultTotalParts,
	}

	if _, err := docRef.Create(ctx, doc); err != nil {
		// A concurrent caller may have won the race — re-fetch.
		if status.Code(err) == codes.AlreadyExists {
			snap, getErr := docRef.Get(ctx)
			if getErr != nil {
				return nil, fmt.Errorf("copros: refetch after race: %w", getErr)
			}
			var existing coproDoc
			if err := snap.DataTo(&existing); err != nil {
				return nil, fmt.Errorf("copros: decode after race: %w", err)
			}
			return docToEntity(existing), nil
		}
		return nil, fmt.Errorf("copros: create singleton: %w", err)
	}

	return docToEntity(doc), nil
}

func docToEntity(d coproDoc) *entities.Copro {
	return &entities.Copro{
		ID:         d.ID,
		Name:       d.Name,
		Address:    d.Address,
		TotalParts: d.TotalParts,
	}
}
