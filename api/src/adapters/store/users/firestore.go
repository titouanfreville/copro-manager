// Package users persists the User entity in Firestore.
package users

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

const collection = "users"

// userDoc is the on-disk shape. ID is also the Firestore document ID (so we
// don't denormalize it — tags drive serialization either way for symmetry).
type userDoc struct {
	ID          string `firestore:"id"`
	Email       string `firestore:"email"`
	DisplayName string `firestore:"display_name"`
}

// Store implements interfaces.UsersStore against Firestore.
type Store struct {
	client *fs.Client
}

// NewStore returns a Firestore-backed users store.
func NewStore(client *fs.Client) interfaces.UsersStore {
	return &Store{client: client}
}

// FindByID fetches the User keyed by the Firebase UID. Returns (nil, nil)
// when the doc is absent — the caller decides whether that's a missing
// auth user or a legitimately not-yet-provisioned one.
func (s *Store) FindByID(ctx context.Context, id string) (*entities.User, error) {
	snap, err := s.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("users: get by id: %w", err)
	}

	var doc userDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("users: decode doc: %w", err)
	}
	return docToEntity(doc), nil
}

// Create inserts a new User. The doc ID is User.ID (the Firebase UID).
func (s *Store) Create(ctx context.Context, user entities.User) error {
	doc := entityToDoc(user)
	if _, err := s.client.Collection(collection).Doc(user.ID).Create(ctx, doc); err != nil {
		return fmt.Errorf("users: create doc: %w", err)
	}
	return nil
}

// List returns every user. Order is unspecified.
func (s *Store) List(ctx context.Context) ([]entities.User, error) {
	iter := s.client.Collection(collection).Documents(ctx)
	defer iter.Stop()

	var out []entities.User
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("users: list: %w", err)
		}

		var doc userDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("users: decode doc: %w", err)
		}
		out = append(out, *docToEntity(doc))
	}
}

// ListByIDs fetches a set of users by their IDs. Missing IDs are silently
// dropped from the result — callers decide what "missing" means.
func (s *Store) ListByIDs(ctx context.Context, ids []string) ([]entities.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	refs := make([]*fs.DocumentRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, s.client.Collection(collection).Doc(id))
	}

	snaps, err := s.client.GetAll(ctx, refs)
	if err != nil {
		return nil, fmt.Errorf("users: get all: %w", err)
	}

	out := make([]entities.User, 0, len(snaps))
	for _, snap := range snaps {
		if !snap.Exists() {
			continue
		}
		var doc userDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("users: decode doc: %w", err)
		}
		out = append(out, *docToEntity(doc))
	}
	return out, nil
}

func docToEntity(d userDoc) *entities.User {
	return &entities.User{
		ID:          d.ID,
		Email:       d.Email,
		DisplayName: d.DisplayName,
	}
}

func entityToDoc(u entities.User) userDoc {
	return userDoc{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
	}
}
