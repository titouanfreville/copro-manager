package templates

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// builder turns a validated draft into a ready-to-persist
// ExpenseTemplate. Two modes:
//
//   - build  : Create flow — stamps ID, copro_id, timestamps,
//              schedule cursor.
//   - rebuild: Update flow — preserves identity + (when the schedule
//              continues) the running NextOccurrenceAt cursor so the
//              materializer doesn't replay past occurrences.
type builder struct {
	copros interfaces.CoprosStore
	now    func() time.Time
}

func newBuilder(copros interfaces.CoprosStore, now func() time.Time) *builder {
	return &builder{copros: copros, now: now}
}

func (b *builder) build(ctx context.Context, d entities.ExpenseTemplateDraft) (entities.ExpenseTemplate, error) {
	copro, err := b.copros.GetOrCreateSingleton(ctx)
	if err != nil {
		return entities.ExpenseTemplate{}, err
	}
	now := b.now()
	t := normalize(d)
	t.ID = uuid.NewString()
	t.CoproID = copro.ID
	t.CreatedAt = now
	t.UpdatedAt = now
	return t, nil
}

func (b *builder) rebuild(existing entities.ExpenseTemplate, d entities.ExpenseTemplateDraft) entities.ExpenseTemplate {
	out := normalize(d)
	out.ID = existing.ID
	out.CoproID = existing.CoproID
	out.CreatedAt = existing.CreatedAt
	out.UpdatedAt = b.now()

	// Preserve the running NextOccurrenceAt when the schedule
	// continues AND the user didn't move the StartDate. Edit-in-place
	// clients typically echo the current cursor as `start_date`,
	// which we treat as "no change" so the cursor isn't reset to the
	// original anchor and the materializer doesn't replay past fires.
	if existing.ScheduleActive && out.ScheduleActive && existing.NextOccurrenceAt != nil &&
		(d.StartDate.IsZero() || d.StartDate.Equal(*existing.NextOccurrenceAt)) {
		out.NextOccurrenceAt = existing.NextOccurrenceAt
	}
	return out
}

// normalize is the pure stage: trim, default currency to EUR, set
// schedule cursor from StartDate. No I/O, no clock read.
func normalize(d entities.ExpenseTemplateDraft) entities.ExpenseTemplate {
	currency := strings.ToUpper(strings.TrimSpace(d.Currency))
	if currency == "" {
		currency = "EUR"
	}
	t := entities.ExpenseTemplate{
		Name:               strings.TrimSpace(d.Name),
		AmountDefaultCents: d.AmountDefaultCents,
		Currency:           currency,
		CategoryID:         d.CategoryID,
		PayerFoyerID:       d.PayerFoyerID,
		DistributionMode:   d.DistributionMode,
		ShareRDCCents:      d.ShareRDCCents,
		Share1erCents:      d.Share1erCents,
		Note:               strings.TrimSpace(d.Note),
		ScheduleActive:     d.ScheduleActive,
	}
	if d.ScheduleActive {
		t.Frequency = d.Frequency
		t.DayOfMonth = d.DayOfMonth
		next := d.StartDate
		t.NextOccurrenceAt = &next
		if d.EndDate != nil && !d.EndDate.IsZero() {
			end := *d.EndDate
			t.EndDate = &end
		}
	}
	return t
}
