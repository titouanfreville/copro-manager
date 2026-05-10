// Package foyers persists foyer entities in Firestore.
//
// The Firestore-shape struct lives here (not in the domain) because storage
// tags are an adapter concern — see AGENTS.md ("Layering — entities are
// storage-agnostic").
package foyers

import (
	"context"
	"errors"
	"fmt"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	domainerrors "github.com/titouanfreville/copro-manager/api/src/domain/errors"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const collection = "foyers"

// foyerDoc is the on-disk shape of a foyer. MemberIDs is the slice of
// User.IDs (= Firebase UIDs) attached to this foyer.
type foyerDoc struct {
	ID        string              `firestore:"id"`
	CoproID   string              `firestore:"copro_id"`
	Floor     entities.FoyerFloor `firestore:"floor"`
	Name      string              `firestore:"name"`
	MemberIDs []string            `firestore:"member_ids"`
	Parts     int                 `firestore:"parts"`
}

// Store implements interfaces.FoyersStore against Firestore.
type Store struct {
	client *fs.Client
}

// NewStore returns a Firestore-backed foyers store.
func NewStore(client *fs.Client) interfaces.FoyersStore {
	return &Store{client: client}
}

// FindByID returns the foyer with the given doc ID or (nil, nil) when absent.
func (s *Store) FindByID(ctx context.Context, id string) (*entities.Foyer, error) {
	snap, err := s.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("foyers: get by id: %w", err)
	}

	var doc foyerDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("foyers: decode doc: %w", err)
	}
	return docToEntity(doc), nil
}

// AddMember appends a User.ID to the foyer's member_ids array using
// Firestore's ArrayUnion — atomic and idempotent on the storage side.
func (s *Store) AddMember(ctx context.Context, foyerID, userID string) error {
	_, err := s.client.Collection(collection).Doc(foyerID).Update(ctx, []fs.Update{
		{Path: "member_ids", Value: fs.ArrayUnion(userID)},
	})
	if err != nil {
		return fmt.Errorf("foyers: add member: %w", err)
	}
	return nil
}

// UpdateParts overwrites the foyer's Parts field.
func (s *Store) UpdateParts(ctx context.Context, foyerID string, parts int) error {
	_, err := s.client.Collection(collection).Doc(foyerID).Update(ctx, []fs.Update{
		{Path: "parts", Value: parts},
	})
	if err != nil {
		return fmt.Errorf("foyers: update parts: %w", err)
	}
	return nil
}

// FindByFloor returns the foyer for a given floor or (nil, nil) when no doc
// matches.
func (s *Store) FindByFloor(ctx context.Context, floor entities.FoyerFloor) (*entities.Foyer, error) {
	iter := s.client.Collection(collection).
		Where("floor", "==", string(floor)).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()

	snap, err := iter.Next()
	if errors.Is(err, iterator.Done) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("foyers: query by floor: %w", err)
	}

	var doc foyerDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("foyers: decode doc: %w", err)
	}

	return docToEntity(doc), nil
}

// List returns every foyer in the collection. Order is unspecified — caller
// must sort if presentation order matters.
func (s *Store) List(ctx context.Context) ([]entities.Foyer, error) {
	iter := s.client.Collection(collection).Documents(ctx)
	defer iter.Stop()

	var out []entities.Foyer
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("foyers: list: %w", err)
		}

		var doc foyerDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("foyers: decode doc: %w", err)
		}
		out = append(out, *docToEntity(doc))
	}
}

// Create inserts a new foyer document. Returns domainerrors.ErrAlreadyExists
// when a doc with the same ID already exists — the foyers usecase relies on
// this so two concurrent creates for the same floor don't both succeed.
func (s *Store) Create(ctx context.Context, foyer entities.Foyer) error {
	doc := entityToDoc(foyer)

	if _, err := s.client.Collection(collection).Doc(doc.ID).Create(ctx, doc); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return fmt.Errorf("%w: foyer %q", domainerrors.ErrAlreadyExists, foyer.ID)
		}
		return fmt.Errorf("foyers: create doc: %w", err)
	}

	return nil
}

func docToEntity(d foyerDoc) *entities.Foyer {
	return &entities.Foyer{
		ID:        d.ID,
		CoproID:   d.CoproID,
		Floor:     d.Floor,
		Name:      d.Name,
		Parts:     d.Parts,
		MemberIDs: d.MemberIDs,
	}
}

func entityToDoc(f entities.Foyer) foyerDoc {
	return foyerDoc{
		ID:        f.ID,
		CoproID:   f.CoproID,
		Floor:     f.Floor,
		Name:      f.Name,
		Parts:     f.Parts,
		MemberIDs: f.MemberIDs,
	}
}
