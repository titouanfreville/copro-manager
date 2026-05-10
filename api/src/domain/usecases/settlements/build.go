package settlements

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// builder turns a validated draft into a ready-to-persist Settlement.
//
//   - build  : Create flow — stamps ID, copro_id, timestamps.
//   - rebuild: Update flow — preserves identity, bumps UpdatedAt.
type builder struct {
	copros interfaces.CoprosStore
	now    func() time.Time
}

func newBuilder(copros interfaces.CoprosStore, now func() time.Time) *builder {
	return &builder{copros: copros, now: now}
}

func (b *builder) build(ctx context.Context, d entities.SettlementDraft) (entities.Settlement, error) {
	copro, err := b.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return entities.Settlement{}, err
	}
	now := b.now()
	s := normalize(d)
	s.ID = uuid.NewString()
	s.CoproID = copro.ID
	s.CreatedAt = now
	s.UpdatedAt = now
	return s, nil
}

func (b *builder) rebuild(existing entities.Settlement, d entities.SettlementDraft) entities.Settlement {
	out := normalize(d)
	out.ID = existing.ID
	out.CoproID = existing.CoproID
	out.CreatedAt = existing.CreatedAt
	out.UpdatedAt = b.now()
	// Preserve the existing currency on update if the draft is empty —
	// historical rows keep their original code rather than getting
	// silently re-defaulted.
	if d.Currency == "" {
		out.Currency = existing.Currency
	}
	return out
}

// normalize is the pure stage: trim, default currency to EUR, dedupe
// expense links. No I/O, no clock read.
func normalize(d entities.SettlementDraft) entities.Settlement {
	currency := strings.ToUpper(strings.TrimSpace(d.Currency))
	if currency == "" {
		currency = "EUR"
	}
	return entities.Settlement{
		FromFoyerID: d.FromFoyerID,
		ToFoyerID:   d.ToFoyerID,
		AmountCents: d.AmountCents,
		Currency:    currency,
		Date:        d.Date,
		Note:        strings.TrimSpace(d.Note),
		ExpenseIDs:  dedupeStrings(d.ExpenseIDs),
	}
}

// dedupeStrings preserves first-occurrence order. Idempotent on
// already-unique input.
func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
