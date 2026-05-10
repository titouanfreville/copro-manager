// Package aiusage persists per-month Vertex AI / Gemini call counters
// in Firestore at stats/ai_{YYYY-MM}. Atomic increments keep the
// counter consistent under concurrent access without a transaction.
//
// One counter for every Gemini-backed feature (meter OCR; document
// classifier and chat assistant slot in later as their own callers).
// Single doc per period is sufficient at the 2-foyer scope rule.
package aiusage

import (
	"context"
	"fmt"

	fs "cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	collection = "stats"
	subPath    = "ai"
)

// Store is the Firestore-backed AIUsageStore. The single document per
// period lives at stats/ai_{YYYY-MM} with a `count` field. Read-side
// returns 0 when the doc is missing (no calls yet for that month).
type Store struct {
	client *fs.Client
}

// NewStore returns a Firestore-backed AI usage counter.
func NewStore(client *fs.Client) *Store {
	return &Store{client: client}
}

func (s *Store) docRef(period string) *fs.DocumentRef {
	return s.client.Collection(collection).Doc(subPath + "_" + period)
}

// CountForPeriod returns the recorded call count for the given YYYY-MM
// bucket. Missing doc → 0.
func (s *Store) CountForPeriod(ctx context.Context, period string) (int64, error) {
	snap, err := s.docRef(period).Get(ctx)
	if err != nil {
		if e, ok := status.FromError(err); ok && e.Code() == codes.NotFound {
			return 0, nil
		}
		return 0, fmt.Errorf("ai usage: get %q: %w", period, err)
	}
	count, _ := snap.DataAt("count")
	switch v := count.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	}
	return 0, nil
}

// IncrementForPeriod atomically bumps the counter by one. Set with
// MergeAll creates the doc on first call; subsequent calls increment.
func (s *Store) IncrementForPeriod(ctx context.Context, period string) error {
	_, err := s.docRef(period).Set(ctx, map[string]any{
		"count":  fs.Increment(int64(1)),
		"period": period,
	}, fs.MergeAll)
	if err != nil {
		return fmt.Errorf("ai usage: increment %q: %w", period, err)
	}
	return nil
}
