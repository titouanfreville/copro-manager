package contracts

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/titouanfreville/copro-manager/api/src/core/text"
	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

const (
	noteMaxBytes = 2000
	urlMaxBytes  = 256
	textMaxBytes = 256
)

// builder turns a user-supplied draft into a ready-to-persist Contract
// entity. The transformation is two-stage:
//
//   1. normalizeDraft     — trim, truncate, default empty status
//   2. enrich             — stamp ID, copro_id, timestamps
//
// Stage 1 is pure (no I/O) and idempotent. Stage 2 reads the singleton
// Copro and the system clock — the only side-effects in this file.
type builder struct {
	copros interfaces.CoprosStore
	now    func() time.Time
}

func newBuilder(copros interfaces.CoprosStore, now func() time.Time) *builder {
	return &builder{copros: copros, now: now}
}

// build converts a draft into a freshly-created Contract entity.
// Used by the Create flow.
func (b *builder) build(ctx context.Context, d entities.ContractDraft) (entities.Contract, error) {
	copro, err := b.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return entities.Contract{}, err
	}
	now := b.now()
	c := normalize(d)
	c.ID = uuid.NewString()
	c.CoproID = copro.ID
	c.CreatedAt = now
	c.UpdatedAt = now
	return c, nil
}

// rebuild applies a draft onto an existing Contract, preserving
// identity (ID, CoproID, CreatedAt) and bumping UpdatedAt. Used by
// the Update flow.
func (b *builder) rebuild(existing entities.Contract, d entities.ContractDraft) entities.Contract {
	out := normalize(d)
	out.ID = existing.ID
	out.CoproID = existing.CoproID
	out.CreatedAt = existing.CreatedAt
	out.UpdatedAt = b.now()
	return out
}

// normalize is the pure stage: trim, truncate, default the status. No
// I/O, no clock read. Exported for use by tests if needed.
func normalize(d entities.ContractDraft) entities.Contract {
	return entities.Contract{
		Name:             strings.TrimSpace(d.Name),
		CategoryID:       strings.TrimSpace(d.CategoryID),
		Society:          normalizeSociety(d.Society),
		Contact:          normalizeContact(d.Contact),
		StartDate:        d.StartDate,
		EndDate:          d.EndDate,
		AmountCents:      d.AmountCents,
		BillingFrequency: d.BillingFrequency,
		TemplateID:       strings.TrimSpace(d.TemplateID),
		Status:           defaultStatus(d.Status),
		Note:             text.Truncate(strings.TrimSpace(d.Note), noteMaxBytes),
	}
}

func defaultStatus(s entities.ContractStatus) entities.ContractStatus {
	if s == "" {
		return entities.ContractStatusActive
	}
	return s
}

func normalizeSociety(s entities.Society) entities.Society {
	return entities.Society{
		Name:    text.Truncate(strings.TrimSpace(s.Name), textMaxBytes),
		Phone:   text.Truncate(strings.TrimSpace(s.Phone), textMaxBytes),
		Email:   text.Truncate(strings.TrimSpace(s.Email), textMaxBytes),
		Website: text.Truncate(strings.TrimSpace(s.Website), urlMaxBytes),
		Address: text.Truncate(strings.TrimSpace(s.Address), noteMaxBytes),
	}
}

func normalizeContact(c entities.Contact) entities.Contact {
	return entities.Contact{
		Name:  text.Truncate(strings.TrimSpace(c.Name), textMaxBytes),
		Role:  text.Truncate(strings.TrimSpace(c.Role), textMaxBytes),
		Phone: text.Truncate(strings.TrimSpace(c.Phone), textMaxBytes),
		Email: text.Truncate(strings.TrimSpace(c.Email), textMaxBytes),
	}
}
