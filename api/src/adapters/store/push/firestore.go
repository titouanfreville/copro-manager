// Package push persists Web Push subscriptions in Firestore.
package push

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

const collection = "push_subscriptions"

type pushDoc struct {
	ID        string    `firestore:"id"`
	FoyerID   string    `firestore:"foyer_id"`
	Endpoint  string    `firestore:"endpoint"`
	P256dh    string    `firestore:"p256dh"`
	Auth      string    `firestore:"auth"`
	UserAgent string    `firestore:"user_agent,omitempty"`
	CreatedAt time.Time `firestore:"created_at"`
}

type Store struct {
	client *fs.Client
}

func NewStore(client *fs.Client) interfaces.PushSubscriptionsStore {
	return &Store{client: client}
}

// endpointID is the Firestore doc ID — a deterministic SHA256 of the
// endpoint URL, so the same browser re-subscribing overwrites its prior
// row and we never accumulate duplicates.
func endpointID(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])
}

func (s *Store) Upsert(ctx context.Context, sub entities.PushSubscription) error {
	id := endpointID(sub.Endpoint)
	doc := pushDoc{
		ID:        id,
		FoyerID:   sub.FoyerID,
		Endpoint:  sub.Endpoint,
		P256dh:    sub.P256dh,
		Auth:      sub.Auth,
		UserAgent: sub.UserAgent,
		CreatedAt: sub.CreatedAt,
	}
	if _, err := s.client.Collection(collection).Doc(id).Set(ctx, doc); err != nil {
		return fmt.Errorf("push: upsert: %w", err)
	}
	return nil
}

// FindByEndpoint returns the row stored for the given endpoint URL, or
// (nil, nil) when absent. Used by the usecase to verify ownership before
// Upsert/Delete (no cross-foyer takeover, no foreign-endpoint griefing).
func (s *Store) FindByEndpoint(ctx context.Context, endpoint string) (*entities.PushSubscription, error) {
	id := endpointID(endpoint)
	snap, err := s.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("push: find by endpoint: %w", err)
	}
	var doc pushDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("push: decode: %w", err)
	}
	return &entities.PushSubscription{
		ID:        doc.ID,
		FoyerID:   doc.FoyerID,
		Endpoint:  doc.Endpoint,
		P256dh:    doc.P256dh,
		Auth:      doc.Auth,
		UserAgent: doc.UserAgent,
		CreatedAt: doc.CreatedAt,
	}, nil
}

func (s *Store) DeleteByEndpoint(ctx context.Context, endpoint string) error {
	id := endpointID(endpoint)
	if _, err := s.client.Collection(collection).Doc(id).Delete(ctx); err != nil {
		return fmt.Errorf("push: delete: %w", err)
	}
	return nil
}

func (s *Store) ListByFoyer(ctx context.Context, foyerID string) ([]entities.PushSubscription, error) {
	iter := s.client.Collection(collection).
		Where("foyer_id", "==", foyerID).
		Documents(ctx)
	defer iter.Stop()

	var out []entities.PushSubscription
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("push: list: %w", err)
		}
		var doc pushDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, err
		}
		out = append(out, entities.PushSubscription{
			ID:        doc.ID,
			FoyerID:   doc.FoyerID,
			Endpoint:  doc.Endpoint,
			P256dh:    doc.P256dh,
			Auth:      doc.Auth,
			UserAgent: doc.UserAgent,
			CreatedAt: doc.CreatedAt,
		})
	}
	return out, nil
}
