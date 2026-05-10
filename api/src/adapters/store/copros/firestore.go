// Package copros persists the singleton Copro in Firestore.
package copros

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

const (
	collection = "copros"
	// FallbackSingletonID is the doc ID used as a last-resort fallback
	// when no Copro exists yet AND the seed script hasn't run. The seed
	// path always wins by creating a UUID-keyed doc first; this fallback
	// only kicks in for cold-start dev environments. Exported so the
	// consolidation endpoint can recognize legacy "singleton"-keyed docs.
	FallbackSingletonID = "singleton"
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
// defaults on first call. Mirrors the seed script's "reuse any existing
// Copro" logic so a Cloud Run instance never spawns a parallel
// `copros/singleton` row when the seed has already provisioned a
// UUID-keyed doc.
//
// Order of operations:
//  1. List the collection (limit 1) — if any Copro exists, return it.
//  2. Otherwise fall back to `Doc(FallbackSingletonID).Create(...)`.
//     Race-safe via Firestore's optimistic concurrency.
func (s *Store) GetOrCreateSingleton(ctx context.Context) (*entities.Copro, error) {
	if existing, err := s.findAny(ctx); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}

	docRef := s.client.Collection(collection).Doc(FallbackSingletonID)
	doc := coproDoc{
		ID:         FallbackSingletonID,
		Name:       "Copro",
		Address:    "",
		TotalParts: entities.DefaultTotalParts,
	}

	if _, err := docRef.Create(ctx, doc); err != nil {
		// A concurrent caller may have won the race — re-fetch through
		// findAny so we honor whichever id ended up persisted.
		if status.Code(err) == codes.AlreadyExists {
			if existing, err := s.findAny(ctx); err != nil {
				return nil, err
			} else if existing != nil {
				return existing, nil
			}
		}
		return nil, fmt.Errorf("copros: create singleton: %w", err)
	}

	return docToEntity(doc), nil
}

// findAny returns the first Copro it finds, or (nil, nil) when the
// collection is empty. Order is irrelevant — at our 2-foyer scale there
// should be exactly one row, and this method is the gate that enforces
// that invariant on read.
func (s *Store) findAny(ctx context.Context) (*entities.Copro, error) {
	iter := s.client.Collection(collection).Limit(1).Documents(ctx)
	defer iter.Stop()

	snap, err := iter.Next()
	if errors.Is(err, iterator.Done) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("copros: find any: %w", err)
	}
	var doc coproDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("copros: decode: %w", err)
	}
	// Trust the snapshot id over the stored `id` field — they should
	// match, but if a row was hand-edited we'd rather behave per the
	// canonical Firestore key.
	doc.ID = snap.Ref.ID
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
