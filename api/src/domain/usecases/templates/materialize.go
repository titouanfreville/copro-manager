package templates

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/titouanfreville/copro-manager/api/src/domain/entities"
	"github.com/titouanfreville/copro-manager/api/src/domain/interfaces"
)

// materializeBackfillCap bounds how many occurrences a single run
// will mint per template, so a template with a far-past StartDate
// can't run away (e.g. monthly + StartDate=2010 → 200+ inserts in
// one tick). 12 = "one year of monthlies" — enough to recover from
// a missed cron, not enough to DoS Firestore.
const materializeBackfillCap = 12

// MaterializeSummary reports how many templates fired and how many
// expense rows were minted in a single cron run. Per-template errors
// are surfaced in `Errors` so a single bad row doesn't block the
// whole batch.
type MaterializeSummary struct {
	TemplatesProcessed int                `json:"templates_processed"`
	ExpensesCreated    int                `json:"expenses_created"`
	Errors             []MaterializeError `json:"errors,omitempty"`
}

// MaterializeError captures one template's failure without aborting
// the rest.
type MaterializeError struct {
	TemplateID string `json:"template_id"`
	Message    string `json:"message"`
}

// materializer fires due occurrences for active scheduled templates.
// Reads from the template store, writes back the cursor, calls the
// expenses usecase to mint each expense, and (best-effort) fires the
// pending_completion alert when a row is born with AmountPending.
type materializer struct {
	logger    *zap.Logger
	templates interfaces.TemplatesStore
	expenses  ExpensesHook
	alerts    AlertsHook
	now       func() time.Time
}

func newMaterializer(
	logger *zap.Logger,
	templates interfaces.TemplatesStore,
	expenses ExpensesHook,
	alerts AlertsHook,
	now func() time.Time,
) *materializer {
	return &materializer{
		logger:    logger.Named("materializer"),
		templates: templates,
		expenses:  expenses,
		alerts:    alerts,
		now:       now,
	}
}

// run lists every due template (next_occurrence_at ≤ cutoff) and
// fires occurrences. Idempotent: per-iteration cursor advance + per-
// template error isolation prevent a single failure from cascading or
// being replayed from scratch on the next tick.
func (m *materializer) run(ctx context.Context, cutoff time.Time) (*MaterializeSummary, error) {
	due, err := m.templates.ListDue(ctx, cutoff)
	if err != nil {
		m.logger.Error("list due failed", zap.Error(err))
		return nil, fmt.Errorf("list due: %w", err)
	}
	summary := &MaterializeSummary{TemplatesProcessed: len(due)}
	for i := range due {
		t := due[i]
		created, err := m.fireOne(ctx, &t, cutoff)
		summary.ExpensesCreated += created
		if err != nil {
			m.logger.Error("materialize one failed", zap.String("template_id", t.ID), zap.Error(err))
			summary.Errors = append(summary.Errors, MaterializeError{TemplateID: t.ID, Message: err.Error()})
		}
	}
	return summary, nil
}

// fireOne fires up to materializeBackfillCap occurrences for a
// single template, persisting the cursor after EACH successful
// expense create so a transient failure mid-loop replays at most
// one occurrence on the next cron run.
func (m *materializer) fireOne(ctx context.Context, t *entities.ExpenseTemplate, cutoff time.Time) (int, error) {
	if t.NextOccurrenceAt == nil || !entities.IsKnownFrequency(t.Frequency) {
		return 0, nil
	}
	created := 0
	cursor := *t.NextOccurrenceAt
	for !cursor.After(cutoff) {
		if created >= materializeBackfillCap {
			break
		}
		if t.EndDate != nil && cursor.After(*t.EndDate) {
			t.ScheduleActive = false
			break
		}
		exp, err := m.expenses.Create(ctx, draftFromTemplate(t, cursor))
		if err != nil {
			return created, fmt.Errorf("create expense from template %q: %w", t.ID, err)
		}
		created++
		m.firePendingAlert(ctx, t, exp)

		next, err := entities.AdvanceDate(cursor, t.Frequency, t.DayOfMonth)
		if err != nil {
			// Persist progress so the next tick doesn't replay these
			// already-created rows.
			t.NextOccurrenceAt = &cursor
			t.UpdatedAt = m.now()
			_ = m.templates.Update(ctx, *t)
			return created, fmt.Errorf("advance date for template %q: %w", t.ID, err)
		}
		cursor = next
		t.NextOccurrenceAt = &cursor
		t.UpdatedAt = m.now()
		if err := m.templates.Update(ctx, *t); err != nil {
			return created, fmt.Errorf("advance template cursor %q: %w", t.ID, err)
		}
	}
	// Final ScheduleActive=false write when EndDate was reached.
	if !t.ScheduleActive {
		t.UpdatedAt = m.now()
		if err := m.templates.Update(ctx, *t); err != nil {
			return created, fmt.Errorf("disable expired template %q: %w", t.ID, err)
		}
	}
	return created, nil
}

// firePendingAlert fires pending_completion when the row was born
// with AmountPending (template's AmountDefault was 0). Best-effort —
// failures don't undo the materialized expense.
func (m *materializer) firePendingAlert(ctx context.Context, t *entities.ExpenseTemplate, exp *entities.Expense) {
	if m.alerts == nil || exp == nil || !exp.AmountPending {
		return
	}
	if _, err := m.alerts.FirePendingCompletion(ctx, *exp); err != nil {
		m.logger.Warn("pending_completion alert fire failed",
			zap.String("template_id", t.ID),
			zap.String("expense_id", exp.ID),
			zap.Error(err))
	}
}

// draftFromTemplate transcribes a template into the entity-level
// ExpenseDraft the ExpensesHook accepts. AmountPending is set when
// the template has no default amount — the user fills it in later.
func draftFromTemplate(t *entities.ExpenseTemplate, cursor time.Time) entities.ExpenseDraft {
	return entities.ExpenseDraft{
		Name:             t.Name,
		AmountCents:      t.AmountDefaultCents,
		Currency:         t.Currency,
		Date:             cursor,
		PayerFoyerID:     t.PayerFoyerID,
		CategoryID:       t.CategoryID,
		DistributionMode: t.DistributionMode,
		ShareRDCCents:    t.ShareRDCCents,
		Share1erCents:    t.Share1erCents,
		Note:             t.Note,
		TemplateID:       t.ID,
		AmountPending:    t.AmountDefaultCents == 0,
	}
}
