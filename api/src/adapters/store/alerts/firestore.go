// Package alerts persists Alert entities in Firestore.
package alerts

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

const collection = "alerts"

type alertDoc struct {
	ID               string             `firestore:"id"`
	CoproID          string             `firestore:"copro_id"`
	Kind             entities.AlertKind `firestore:"kind"`
	RecipientFoyerID string             `firestore:"recipient_foyer_id"`
	DedupeKey        string             `firestore:"dedupe_key"`
	Payload          map[string]any     `firestore:"payload,omitempty"`
	DeepLink         string             `firestore:"deep_link,omitempty"`
	FiredAt          time.Time          `firestore:"fired_at"`
	ReadAt           *time.Time         `firestore:"read_at,omitempty"`
	ResolvedAt       *time.Time         `firestore:"resolved_at,omitempty"`
	DismissedAt      *time.Time         `firestore:"dismissed_at,omitempty"`
}

type Store struct {
	client *fs.Client
}

// NewStore returns a Firestore-backed alerts store.
func NewStore(client *fs.Client) interfaces.AlertsStore {
	return &Store{client: client}
}

// CreateIfNew atomically writes the alert if no row with the same
// (copro_id, dedupe_key) exists. Wrapped in a Firestore transaction so
// two concurrent Fire calls with the same dedupe key cannot both pass
// the lookup and both write — the loser sees the existing row.
func (s *Store) CreateIfNew(ctx context.Context, a entities.Alert) (*entities.Alert, bool, error) {
	col := s.client.Collection(collection)

	var (
		result  entities.Alert
		created bool
	)
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *fs.Transaction) error {
		// Re-read inside the transaction so a parallel writer's row is
		// visible if it landed between our two RPCs.
		query := col.
			Where("copro_id", "==", a.CoproID).
			Where("dedupe_key", "==", a.DedupeKey).
			Limit(1)
		docs, err := tx.Documents(query).GetAll()
		if err != nil {
			return fmt.Errorf("alerts: tx find: %w", err)
		}
		if len(docs) > 0 {
			var doc alertDoc
			if err := docs[0].DataTo(&doc); err != nil {
				return fmt.Errorf("alerts: tx decode: %w", err)
			}
			result = docToEntity(doc)
			created = false
			return nil
		}
		if err := tx.Create(col.Doc(a.ID), entityToDoc(a)); err != nil {
			return fmt.Errorf("alerts: tx create: %w", err)
		}
		result = a
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &result, created, nil
}

func (s *Store) FindByID(ctx context.Context, id string) (*entities.Alert, error) {
	snap, err := s.client.Collection(collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("alerts: get by id: %w", err)
	}
	var doc alertDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("alerts: decode: %w", err)
	}
	out := docToEntity(doc)
	return &out, nil
}

func (s *Store) FindByDedupeKey(ctx context.Context, coproID, dedupeKey string) (*entities.Alert, error) {
	iter := s.client.Collection(collection).
		Where("copro_id", "==", coproID).
		Where("dedupe_key", "==", dedupeKey).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()

	snap, err := iter.Next()
	if errors.Is(err, iterator.Done) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("alerts: find by dedupe: %w", err)
	}
	var doc alertDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, fmt.Errorf("alerts: decode: %w", err)
	}
	out := docToEntity(doc)
	return &out, nil
}

// listByFoyerLimit caps the number of alerts returned by a single
// ListByFoyer call. The frontend feed shows newest-first; once the
// foyer accumulates years of resolved alerts, returning all of them on
// every snapshot would balloon read costs without any user benefit.
const listByFoyerLimit = 200

func (s *Store) ListByFoyer(ctx context.Context, foyerID string, includeDismissed bool) ([]entities.Alert, error) {
	iter := s.client.Collection(collection).
		Where("recipient_foyer_id", "==", foyerID).
		OrderBy("fired_at", fs.Desc).
		Limit(listByFoyerLimit).
		Documents(ctx)
	defer iter.Stop()

	var out []entities.Alert
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("alerts: list by foyer: %w", err)
		}
		var doc alertDoc
		if err := snap.DataTo(&doc); err != nil {
			return nil, fmt.Errorf("alerts: decode: %w", err)
		}
		if !includeDismissed && doc.DismissedAt != nil {
			continue
		}
		out = append(out, docToEntity(doc))
	}
	return out, nil
}

// Update overwrites the whole doc; kept for callers that own the full
// in-memory entity. Multi-device race-prone — prefer the field-level
// helpers below for single-field state changes.
func (s *Store) Update(ctx context.Context, a entities.Alert) error {
	if _, err := s.client.Collection(collection).Doc(a.ID).Set(ctx, entityToDoc(a)); err != nil {
		return fmt.Errorf("alerts: update: %w", err)
	}
	return nil
}

// MarkRead writes only `read_at`, leaving every other field intact. Two
// concurrent devices marking different state (one reads, one dismisses)
// won't clobber each other's writes the way a full Set would.
func (s *Store) MarkRead(ctx context.Context, id string, when time.Time) error {
	_, err := s.client.Collection(collection).Doc(id).Update(ctx, []fs.Update{
		{Path: "read_at", Value: when},
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return fmt.Errorf("alerts: mark read: %w", err)
	}
	return nil
}

// MarkDismissed writes both `dismissed_at` and `read_at` (dismissed
// implies read for UX purposes). Field-level update for the same lost-
// update protection as MarkRead.
func (s *Store) MarkDismissed(ctx context.Context, id string, when time.Time) error {
	_, err := s.client.Collection(collection).Doc(id).Update(ctx, []fs.Update{
		{Path: "dismissed_at", Value: when},
		{Path: "read_at", Value: when},
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil
		}
		return fmt.Errorf("alerts: mark dismissed: %w", err)
	}
	return nil
}

// ResolveByPrefix scans alerts on (copro_id, recipient_foyer_id) and
// flips each match's resolved_at. Firestore lacks a native "starts with"
// operator on a string field, so we list and filter in-memory — fine at
// 2-foyer scale (the alerts collection stays small).
//
// Refuses an empty prefix as a defensive guard: an empty prefix would
// match every alert and silently nuke the whole feed.
func (s *Store) ResolveByPrefix(ctx context.Context, coproID, prefix string, resolvedAt entities.Alert) error {
	if prefix == "" {
		return fmt.Errorf("alerts: ResolveByPrefix refuses empty prefix")
	}
	iter := s.client.Collection(collection).
		Where("copro_id", "==", coproID).
		Documents(ctx)
	defer iter.Stop()

	now := resolvedAt.ResolvedAt
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return fmt.Errorf("alerts: resolve scan: %w", err)
		}
		var doc alertDoc
		if err := snap.DataTo(&doc); err != nil {
			return fmt.Errorf("alerts: resolve decode: %w", err)
		}
		if doc.ResolvedAt != nil {
			continue
		}
		if !startsWith(doc.DedupeKey, prefix) {
			continue
		}
		if _, err := snap.Ref.Update(ctx, []fs.Update{{Path: "resolved_at", Value: now}}); err != nil {
			return fmt.Errorf("alerts: resolve update: %w", err)
		}
	}
	return nil
}

func (s *Store) ResolveByDedupeKey(ctx context.Context, coproID, dedupeKey string, resolvedAt entities.Alert) error {
	existing, err := s.FindByDedupeKey(ctx, coproID, dedupeKey)
	if err != nil {
		return err
	}
	if existing == nil || existing.ResolvedAt != nil {
		return nil
	}
	now := resolvedAt.ResolvedAt
	if _, err := s.client.Collection(collection).Doc(existing.ID).Update(ctx, []fs.Update{
		{Path: "resolved_at", Value: now},
	}); err != nil {
		return fmt.Errorf("alerts: resolve dedupe: %w", err)
	}
	return nil
}

// CountUnresolvedByExpense walks the copro's alerts and counts those
// whose payload references the given expense_id and aren't resolved.
func (s *Store) CountUnresolvedByExpense(ctx context.Context, coproID, expenseID string) (int, error) {
	iter := s.client.Collection(collection).
		Where("copro_id", "==", coproID).
		Documents(ctx)
	defer iter.Stop()

	count := 0
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("alerts: count by expense: %w", err)
		}
		var doc alertDoc
		if err := snap.DataTo(&doc); err != nil {
			return 0, err
		}
		if doc.ResolvedAt != nil {
			continue
		}
		if eid, _ := doc.Payload["expense_id"].(string); eid == expenseID {
			count++
		}
	}
	return count, nil
}

// ResolveByExpense marks alerts referencing the expense as resolved.
func (s *Store) ResolveByExpense(ctx context.Context, coproID, expenseID string, resolvedAt entities.Alert) error {
	iter := s.client.Collection(collection).
		Where("copro_id", "==", coproID).
		Documents(ctx)
	defer iter.Stop()

	now := resolvedAt.ResolvedAt
	for {
		snap, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return fmt.Errorf("alerts: resolve by expense scan: %w", err)
		}
		var doc alertDoc
		if err := snap.DataTo(&doc); err != nil {
			return err
		}
		if doc.ResolvedAt != nil {
			continue
		}
		if eid, _ := doc.Payload["expense_id"].(string); eid != expenseID {
			continue
		}
		if _, err := snap.Ref.Update(ctx, []fs.Update{{Path: "resolved_at", Value: now}}); err != nil {
			return fmt.Errorf("alerts: resolve by expense update: %w", err)
		}
	}
	return nil
}

func startsWith(s, prefix string) bool {
	if len(prefix) == 0 {
		return true
	}
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

func docToEntity(d alertDoc) entities.Alert {
	return entities.Alert{
		ID:               d.ID,
		CoproID:          d.CoproID,
		Kind:             d.Kind,
		RecipientFoyerID: d.RecipientFoyerID,
		DedupeKey:        d.DedupeKey,
		Payload:          d.Payload,
		DeepLink:         d.DeepLink,
		FiredAt:          d.FiredAt,
		ReadAt:           d.ReadAt,
		ResolvedAt:       d.ResolvedAt,
		DismissedAt:      d.DismissedAt,
	}
}

func entityToDoc(a entities.Alert) alertDoc {
	return alertDoc{
		ID:               a.ID,
		CoproID:          a.CoproID,
		Kind:             a.Kind,
		RecipientFoyerID: a.RecipientFoyerID,
		DedupeKey:        a.DedupeKey,
		Payload:          a.Payload,
		DeepLink:         a.DeepLink,
		FiredAt:          a.FiredAt,
		ReadAt:           a.ReadAt,
		ResolvedAt:       a.ResolvedAt,
		DismissedAt:      a.DismissedAt,
	}
}
